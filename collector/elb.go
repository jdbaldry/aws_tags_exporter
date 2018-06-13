package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descELBTagsName          = prometheus.BuildFQName(namespace, "elb", "tags")
	descELBTagsHelp          = "AWS ELB tags converted to Prometheus labels."
	descELBTagsDefaultLabels = []string{"load_balancer_name", "region"}

	descELBTags = prometheus.NewDesc(
		descELBTagsName,
		descELBTagsHelp,
		descELBTagsDefaultLabels, nil,
	)

	describeELBTagsBatch = 20
)

type elbCollector struct {
	store  elbStore
	region string
}

type elbList struct {
	// Array of all LoadBalancerDescriptions with various metadata fields that may
	// become useful labels
	elbs []*elb.LoadBalancerDescription
	// Array of TagDesciptions which include LoadBalancerName and []*Tag where
	// Tag has the Key and Value fields
	tags []*elb.TagDescription
}

type elbStore interface {
	List() (elbList, error)
}

type elbLister func() (elbList, error)

func (l elbLister) List() (elbList, error) {
	return l()
}

func RegisterELBCollector(registry prometheus.Registerer, region *string) error {
	glog.V(4).Infof("Registering collector: elb")

	elbSession := elb.New(session.New(&aws.Config{
		Region: aws.String(*region)},
	))

	lister := elbLister(func() (el elbList, err error) {
		var elbTags []*elb.TagDescription

		dlbInput := &elb.DescribeLoadBalancersInput{}
		elbs, err := elbSession.DescribeLoadBalancers(dlbInput)
		RequestTotalMetric.With(prometheus.Labels{"service": "elb", "region": *region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "elb", "region": *region}).Inc()
		}

		loadBalancerNames := []*string{}
		for _, description := range elbs.LoadBalancerDescriptions {
			loadBalancerNames = append(loadBalancerNames, description.LoadBalancerName)
		}

		for i:=0; i < len(elbs.LoadBalancerDescriptions); i += describeELBTagsBatch {
			j := i + describeELBTagsBatch
			if j > len(elbs.LoadBalancerDescriptions) {
				j = len(elbs.LoadBalancerDescriptions)
			}
			dtInput := &elb.DescribeTagsInput{
				LoadBalancerNames: loadBalancerNames[i:j],
			}
			result, err := elbSession.DescribeTags(dtInput)
			RequestTotalMetric.With(prometheus.Labels{"service": "elb", "region": *region}).Inc()
			if err != nil {
				RequestErrorTotalMetric.With(prometheus.Labels{"service": "elb", "region": *region}).Inc()
				return el, err
			}
			elbTags = append(elbTags, result.TagDescriptions...)
		}

		el = elbList{elbs: elbs.LoadBalancerDescriptions, tags: elbTags}
		return
	})

	registry.MustRegister(&elbCollector{store: lister, region: *region})
  return nil
}

// Describe implements the prometheus.Collector interface.
func (ec *elbCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descELBTags
}

// Collect implements the prometheus.Collector interface.
func (ec *elbCollector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: elb")
	elbList, err := ec.store.List()
	if err != nil {
		glog.Errorf("Error collecting: elbs\n", err)
	}

	for i, elb := range elbList.elbs {
		ec.collectELB(ch, elb, elbList.tags[i])
	}
}

func elbTagsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descELBTagsName,
		descELBTagsHelp,
		append(descELBTagsDefaultLabels, labelKeys...),
		nil,
	)
}

func awsTagDescriptionToPrometheusLabels(tagDescription *elb.TagDescription) (labelKeys, labelValues []string) {
	for _, tag := range tagDescription.Tags {
		labelKeys = append(labelKeys, sanitizeLabelName(*tag.Key))
		labelValues = append(labelValues, *tag.Value)
	}

  return
}

func (ec *elbCollector) collectELB(ch chan<- prometheus.Metric, e *elb.LoadBalancerDescription, t *elb.TagDescription) error {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{*e.LoadBalancerName, ec.region}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys, labelValues := awsTagDescriptionToPrometheusLabels(t)
	addGauge(elbTagsDesc(labelKeys), 1, labelValues...)
	return nil
}
