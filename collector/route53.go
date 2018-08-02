package collector

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	route53TagsBatch        = 10
	hostedzoneResourceType  = "hostedzone"
	healthcheckResourceType = "healthcheck"
)

var route53Collector = TagsCollector{
	name:          prometheus.BuildFQName(namespace, "route53", "tags"),
	help:          "AWS Route53 tags converted to Prometheus labels.",
	defaultLabels: []string{"identifier", "resource_type"},
	lister:        &route53Lister{},
}

type route53Lister struct {
	region  string
	session *route53.Route53
}

func (ro *route53Lister) Initialise(_ string) (err error) {
	ro.region = "global"
	sess, err := session.NewSession(&aws.Config{})
	if err != nil {
		return
	}
	ro.session = route53.New(sess)
	return
}

func (ro *route53Lister) parseHostedZoneID(id string) string {
	return strings.TrimPrefix(id, fmt.Sprintf("/%s/", hostedzoneResourceType))
}

func (ro *route53Lister) groupIntoSlices(ids []*string) [][]*string {

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

func (ro *route53Lister) List() ([]tags, error) {

	hostedzones, err := ro.session.ListHostedZones(&route53.ListHostedZonesInput{})
	RequestTotalMetric.With(prometheus.Labels{"service": "route53", "region": ro.region}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "route53", "region": ro.region}).Inc()
		return []tags{}, err
	}

	hostedzonenames := make(map[string]string)
	zoneIDs := make([]*string, 0, len(hostedzones.HostedZones))
	for _, zone := range hostedzones.HostedZones {
		actualID := ro.parseHostedZoneID(*zone.Id)
		hostedzonenames[actualID] = *zone.Name
		zoneIDs = append(zoneIDs, &actualID)
	}

	healthchecks, err := ro.session.ListHealthChecks(&route53.ListHealthChecksInput{})
	RequestTotalMetric.With(prometheus.Labels{"service": "route53", "region": ro.region}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "route53", "region": ro.region}).Inc()
		return []tags{}, err
	}

	healthcheckIDs := make([]*string, 0, len(healthchecks.HealthChecks))
	for _, healthcheck := range healthchecks.HealthChecks {
		healthcheckIDs = append(healthcheckIDs, healthcheck.Id)
	}

	numReqs := (len(zoneIDs)+len(healthcheckIDs))/10 + 2
	reqs := make([]*request.Request, 0, numReqs)
	outs := make([]*route53.ListTagsForResourcesOutput, 0, numReqs)

	for _, group := range ro.groupIntoSlices(zoneIDs) {
		req, out := ro.session.ListTagsForResourcesRequest(&route53.ListTagsForResourcesInput{ResourceIds: group, ResourceType: &hostedzoneResourceType})
		reqs = append(reqs, req)
		outs = append(outs, out)
	}

	for _, group := range ro.groupIntoSlices(healthcheckIDs) {
		req, out := ro.session.ListTagsForResourcesRequest(&route53.ListTagsForResourcesInput{ResourceIds: group, ResourceType: &healthcheckResourceType})
		reqs = append(reqs, req)
		outs = append(outs, out)
	}

	errs := makeConcurrentRequests(reqs, "route53")

	tagsList := make([]tags, 0, numReqs)

	for i := range errs {
		RequestTotalMetric.With(prometheus.Labels{"service": "route53", "region": ro.region}).Inc()
		if errs[i] != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "route53", "region": ro.region}).Inc()
			continue
		}

		for _, rts := range outs[i].ResourceTagSets {
			ts := tags{
				make([]string, 0, len(rts.Tags)+len(route53Collector.defaultLabels)),
				make([]string, 0, len(rts.Tags)+len(route53Collector.defaultLabels)),
			}

			ts.keys = append(ts.keys, route53Collector.defaultLabels...)
			ts.values = append(ts.values, *rts.ResourceId, *rts.ResourceType)

			for _, t := range rts.Tags {
				ts.keys = append(ts.keys, *t.Key)
				ts.values = append(ts.values, *t.Value)
			}

			tagsList = append(tagsList, ts)
		}
	}

	return tagsList, nil
}
