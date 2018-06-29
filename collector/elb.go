package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// The fully qualified metric name
	descELBTagsName = prometheus.BuildFQName(namespace, "elb", "tags")
	// Helper message describing the metrics
	descELBTagsHelp = "AWS ELB tags converted to Prometheus labels."
	// Labels expected to exist on all Elastic Load Balancers. These are independent of the
	// tags created by users and are instead a product of the Load Balancer description
	descELBTagsDefaultLabels = []string{"load_balancer_name", "region"}

	// Used by the Describe implementation of the Prometheus Collector interface
	descELBTags = prometheus.NewDesc(
		descELBTagsName,
		descELBTagsHelp,
		descELBTagsDefaultLabels, nil,
	)

	// The number of LoadBalancerNames that can be used in a single DescribeTags request.
	describeELBTagsBatch = 20
)

// elbCollector ...
type elbCollector struct {
	store  elbStore
	region string
}

// elbList is a collection of LoadBalancerDescriptions and TagDescriptions stored
// in arrays in the same order such that elbList.tags[i] should be for the elbList.elbs[i].LoadBalancerName
type elbList struct {
	// Array of all pointers to LoadBalancerDescriptions with various metadata fields that may
	// become useful labels
	elbs []*elb.LoadBalancerDescription
	// Array of pointers to TagDescriptions which include LoadBalancerName and []*Tag where
	// Tag has the Key and Value fields
	tags []*elb.TagDescription
}

// elbStore ...
type elbStore interface {
	List() (elbList, error)
}

// elbLister returns an elbList
type elbLister func() (elbList, error)

// elbLister method that implements the elbStore interface
func (l elbLister) List() (elbList, error) {
	return l()
}

// RegisterELBCollector receives a prometheus Registry and AWS region and creates
// an elbLister that can return the elbList struct that arrays of pointers to
// LoadBalancerDescription and TagDescription
func RegisterELBCollector(registry prometheus.Registerer, region string) {
	glog.V(4).Infof("Registering collector: elb")

	elbSession := elb.New(session.New(&aws.Config{
		Region: aws.String(region)},
	))

	lister := elbLister(func() (elbList, error) {

		dlbInput := &elb.DescribeLoadBalancersInput{}
		elbs, err := elbSession.DescribeLoadBalancers(dlbInput)

		RequestTotalMetric.With(prometheus.Labels{"service": "elb", "region": region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "elb", "region": region}).Inc()
			return elbList{}, err
		}

		elbNames := []*string{}
		for _, description := range elbs.LoadBalancerDescriptions {
			elbNames = append(elbNames, description.LoadBalancerName)
		}

		numReqs := len(elbs.LoadBalancerDescriptions)/describeELBTagsBatch + 1
		reqs := make([]*request.Request, 0, numReqs)
		outs := make([]*elb.DescribeTagsOutput, 0, numReqs)
		for i := 0; i < len(elbs.LoadBalancerDescriptions); i += describeELBTagsBatch {
			j := i + describeELBTagsBatch
			if j > len(elbs.LoadBalancerDescriptions) {
				j = len(elbs.LoadBalancerDescriptions)
			}
			dtInput := &elb.DescribeTagsInput{
				LoadBalancerNames: elbNames[i:j],
			}

			req, out := elbSession.DescribeTagsRequest(dtInput)
			reqs = append(reqs, req)
			outs = append(outs, out)
		}

		errs := makeConcurrentRequests(reqs, "elb")
		elbTags := make([]*elb.TagDescription, 0, len(elbs.LoadBalancerDescriptions))
		for i := range outs {
			RequestTotalMetric.With(prometheus.Labels{"service": "elb", "region": region}).Inc()
			if errs[i] != nil {
				RequestErrorTotalMetric.With(prometheus.Labels{"service": "elb", "region": region}).Inc()
				continue
			}

			elbTags = append(elbTags, outs[i].TagDescriptions...)
		}

		return elbList{elbs: elbs.LoadBalancerDescriptions, tags: elbTags}, nil
	})

	registry.MustRegister(&elbCollector{store: lister, region: region})
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
		ec.collectELB(ch, *elb, *elbList.tags[i])
	}
}

// elbTagsDesc takes an array of strings that are AWS tag keys and returns a pointer to a Prometheus
// description from the base set of 'default' labels with the tag keys as additional labels
func elbTagsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descELBTagsName,
		descELBTagsHelp,
		append(descELBTagsDefaultLabels, labelKeys...),
		nil,
	)
}

// awsTagDescriptionToPrometheusLabels takes a pointer to a TagDescription and returns
// a list of label values and sanitized label keys
func awsTagDescriptionToPrometheusLabels(tagDescription elb.TagDescription) (labelKeys, labelValues []string) {
	for _, tag := range tagDescription.Tags {
		labelKeys = append(labelKeys, sanitizeLabelName(*tag.Key))
		labelValues = append(labelValues, *tag.Value)
	}

	return
}

// collectELB takes a pointer to both a LoadBalancerDescription and TagDescription and builds the lists of
// label keys and label values used subsequently as labels to the tags gauge
func (ec *elbCollector) collectELB(ch chan<- prometheus.Metric, e elb.LoadBalancerDescription, t elb.TagDescription) error {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{*e.LoadBalancerName, ec.region}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys, labelValues := awsTagDescriptionToPrometheusLabels(t)
	addGauge(elbTagsDesc(labelKeys), 1, labelValues...)
	return nil
}
