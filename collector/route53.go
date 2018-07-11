package collector

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descRoute53TagsName          = prometheus.BuildFQName(namespace, "route53", "tags")
	descRoute53TagsHelp          = "AWS Route53 tags converted to Prometheus labels."
	descRoute53TagsDefaultLabels = []string{"identifier", "resource_type"}

	descRoute53Tags = prometheus.NewDesc(
		descRoute53TagsName,
		descRoute53TagsHelp,
		descRoute53TagsDefaultLabels, nil,
	)

	route53TagsBatch        = 10
	hostedzoneResourceType  = "hostedzone"
	healthcheckResourceType = "healthcheck"
)

type route53Collector struct {
	store route53Store
}

type route53List struct {
	hostedzones  map[string][]*route53.Tag
	healthchecks map[string][]*route53.Tag
}

type route53Store interface {
	List() (route53List, error)
}

type route53Lister func() (route53List, error)

func (l route53Lister) List() (route53List, error) {
	return l()
}

func groupIntoSlices(ids []*string) [][]*string {

	groups := make([][]*string, 0, len(ids)/route53TagsBatch+1)
	for i := 0; i < len(ids); i += route53TagsBatch {

		j := i + route53TagsBatch
		if j > len(ids) {
			j = len(ids)
		}

		groups = append(groups, ids[i:j])
	}

	return groups
}

func parseHostedZoneID(id string) string {
	return strings.TrimPrefix(id, fmt.Sprintf("/%s/", hostedzoneResourceType))
}

func RegisterRoute53Collector(registry prometheus.Registerer, _ string) {
	glog.V(4).Infof("Registering collector: route53")

	route53Session := route53.New(session.New())

	lister := route53Lister(func() (route53List, error) {

		hostedzones, err := route53Session.ListHostedZones(&route53.ListHostedZonesInput{})
		RequestTotalMetric.With(prometheus.Labels{"service": "route53", "region": "global"}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "route53", "region": "global"}).Inc()
			return route53List{}, err
		}

		hostedzonenames := make(map[string]string)
		zoneIDs := make([]*string, 0, len(hostedzones.HostedZones))
		for _, zone := range hostedzones.HostedZones {
			actualID := parseHostedZoneID(*zone.Id)
			hostedzonenames[actualID] = *zone.Name
			zoneIDs = append(zoneIDs, &actualID)
		}

		healthchecks, err := route53Session.ListHealthChecks(&route53.ListHealthChecksInput{})
		RequestTotalMetric.With(prometheus.Labels{"service": "route53", "region": "global"}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "route53", "region": "global"}).Inc()
			return route53List{}, err
		}

		healthcheckIDs := make([]*string, 0, len(healthchecks.HealthChecks))
		for _, healthcheck := range healthchecks.HealthChecks {
			healthcheckIDs = append(healthcheckIDs, healthcheck.Id)
		}

		numReqs := (len(zoneIDs)+len(healthcheckIDs))/10 + 2
		reqs := make([]*request.Request, 0, numReqs)
		outs := make([]*route53.ListTagsForResourcesOutput, 0, numReqs)

		for _, group := range groupIntoSlices(zoneIDs) {
			req, out := route53Session.ListTagsForResourcesRequest(&route53.ListTagsForResourcesInput{ResourceIds: group, ResourceType: &hostedzoneResourceType})
			reqs = append(reqs, req)
			outs = append(outs, out)
		}

		for _, group := range groupIntoSlices(healthcheckIDs) {
			req, out := route53Session.ListTagsForResourcesRequest(&route53.ListTagsForResourcesInput{ResourceIds: group, ResourceType: &healthcheckResourceType})
			reqs = append(reqs, req)
			outs = append(outs, out)
		}

		errs := makeConcurrentRequests(reqs, "route53")

		r53l := route53List{make(map[string][]*route53.Tag), make(map[string][]*route53.Tag)}
		for i := range errs {
			RequestTotalMetric.With(prometheus.Labels{"service": "route53", "region": "global"}).Inc()
			if errs[i] != nil {
				RequestErrorTotalMetric.With(prometheus.Labels{"service": "route53", "region": "global"}).Inc()
				glog.Warning(errs[i])
				continue
			}

			for _, tagsOut := range outs[i].ResourceTagSets {
				if *tagsOut.ResourceType == hostedzoneResourceType {
					name := hostedzonenames[*tagsOut.ResourceId]
					r53l.hostedzones[*tagsOut.ResourceId] = append(
						tagsOut.Tags,
						&route53.Tag{
							Key:   aws.String("Name"),
							Value: &name,
						},
					)
				} else if *tagsOut.ResourceType == healthcheckResourceType {
					r53l.healthchecks[*tagsOut.ResourceId] = tagsOut.Tags
				} else {
					glog.Warningf("Unknown route53 resource_type received %s", *tagsOut.ResourceType)
				}
			}
		}

		return r53l, nil
	})

	registry.MustRegister(&route53Collector{store: lister})
}

func (rc *route53Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descRoute53Tags
}

func (rc *route53Collector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: route53")
	rl, err := rc.store.List()
	if err != nil {
		glog.Errorf("Error collecting: route53s\n", err)
	}

	for key, tags := range rl.hostedzones {
		rc.collectRoute53(ch, key, "hostedzone", tags)
	}

	for key, tags := range rl.healthchecks {
		rc.collectRoute53(ch, key, "healthcheck", tags)
	}
}

func route53TagsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descRoute53TagsName,
		descRoute53TagsHelp,
		append(descRoute53TagsDefaultLabels, labelKeys...),
		nil,
	)
}

func (rc *route53Collector) collectRoute53(ch chan<- prometheus.Metric, key, resourceType string, tags []*route53.Tag) {
	labelKeys := make([]string, 0, len(tags)+len(descRoute53TagsDefaultLabels))
	labelKeys = append(labelKeys, descRoute53TagsDefaultLabels...)

	labelValues := make([]string, 0, len(tags)+len(descRoute53TagsDefaultLabels))
	labelValues = append(labelValues, key, resourceType)

	for _, t := range tags {
		labelKeys = append(labelKeys, sanitizeLabelName(*t.Key))
		labelValues = append(labelValues, *t.Value)
	}

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			descRoute53TagsName,
			descRoute53TagsHelp,
			labelKeys,
			nil,
		),
		prometheus.GaugeValue,
		1,
		labelValues...,
	)
}
