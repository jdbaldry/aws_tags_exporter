package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// The fully qualified metric name
	descELBV2TagsName = prometheus.BuildFQName(namespace, "elbv2", "tags")
	// Helper message describing the metrics
	descELBV2TagsHelp = "AWS ELBv2 tags converted to Prometheus labels."
	// Labels expected to exist on all Elastic Load Balancers. These are independent of the
	// tags created by users and are instead a product of the Load Balancer description
	descELBV2TagsDefaultLabels = []string{"load_balancer_name", "region"}

	// Used by the Describe implementation of the Prometheus Collector interface
	descELBV2Tags = prometheus.NewDesc(
		descELBV2TagsName,
		descELBV2TagsHelp,
		descELBV2TagsDefaultLabels, nil,
	)

	// The number of LoadBalancerNames that can be used in a single DescribeTags request.
	describeELBV2TagsBatch = 20
)

// elbv2Collector ...
type elbv2Collector struct {
	store  elbv2Store
	region string
}

// elbv2List is a collection of LoadBalancers and TagDescriptions stored
// in arrays in the same order such that elbv2List.tags[i] should be for the elbv2List.elbv2s[i].LoadBalancerName
type elbv2List struct {
	// Array of all pointers to LoadBalancers with various metadata fields that may
	// become useful labels
	elbv2s []*elbv2.LoadBalancer
	// Array of pointers to TagDescriptions which include LoadBalancerName and []*Tag where
	// Tag has the Key and Value fields
	tags []*elbv2.TagDescription
}

// elbv2Store ...
type elbv2Store interface {
	List() (elbv2List, error)
}

// elbv2Lister returns an elbv2List
type elbv2Lister func() (elbv2List, error)

// elbv2Lister method that implements the elbv2Store interface
func (l elbv2Lister) List() (elbv2List, error) {
	return l()
}

// RegisterELBV2Collector receives a prometheus Registry and AWS region and creates
// an elbv2Lister that can return the elbv2List struct that arrays of pointers to
// LoadBalancer and TagDescription
func RegisterELBV2Collector(registry prometheus.Registerer, region string) {
	glog.V(4).Infof("Registering collector: elbv2")

	elbv2Session := elbv2.New(session.New(&aws.Config{
		Region: aws.String(region)},
	))

	lister := elbv2Lister(func() (elbv2List, error) {

		dlbInput := &elbv2.DescribeLoadBalancersInput{}
		elbv2s, err := elbv2Session.DescribeLoadBalancers(dlbInput)

		RequestTotalMetric.With(prometheus.Labels{"service": "elbv2", "region": region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "elbv2", "region": region}).Inc()
			return elbv2List{}, err
		}

		elbv2Arns := []*string{}
		for _, description := range elbv2s.LoadBalancers {
			elbv2Arns = append(elbv2Arns, description.LoadBalancerArn)
		}

		numReqs := len(elbv2s.LoadBalancers)/describeELBV2TagsBatch + 1
		reqs := make([]*request.Request, 0, numReqs)
		outs := make([]*elbv2.DescribeTagsOutput, 0, numReqs)
		for i := 0; i < len(elbv2s.LoadBalancers); i += describeELBV2TagsBatch {
			j := i + describeELBV2TagsBatch
			if j > len(elbv2s.LoadBalancers) {
				j = len(elbv2s.LoadBalancers)
			}
			dtInput := &elbv2.DescribeTagsInput{
				ResourceArns: elbv2Arns[i:j],
			}

			req, out := elbv2Session.DescribeTagsRequest(dtInput)
			reqs = append(reqs, req)
			outs = append(outs, out)
		}

		errs := makeConcurrentRequests(reqs, "elbv2")
		elbv2Tags := make([]*elbv2.TagDescription, 0, len(elbv2s.LoadBalancers))
		for i := range outs {
			RequestTotalMetric.With(prometheus.Labels{"service": "elbv2", "region": region}).Inc()
			if errs[i] != nil {
				RequestErrorTotalMetric.With(prometheus.Labels{"service": "elbv2", "region": region}).Inc()
				continue
			}

			elbv2Tags = append(elbv2Tags, outs[i].TagDescriptions...)
		}

		return elbv2List{elbv2s: elbv2s.LoadBalancers, tags: elbv2Tags}, nil
	})

	registry.MustRegister(&elbv2Collector{store: lister, region: region})
}

// Describe implements the prometheus.Collector interface.
func (ec *elbv2Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descELBV2Tags
}

// Collect implements the prometheus.Collector interface.
func (ec *elbv2Collector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: elbv2")
	elbv2List, err := ec.store.List()
	if err != nil {
		glog.Errorf("Error collecting: elbv2s\n", err)
	}

	for i, elbv2 := range elbv2List.elbv2s {
		ec.collectELBV2(ch, *elbv2, *elbv2List.tags[i])
	}
}

// elbv2TagsDesc takes an array of strings that are AWS tag keys and returns a pointer to a Prometheus
// description from the base set of 'default' labels with the tag keys as additional labels
func elbv2TagsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descELBV2TagsName,
		descELBV2TagsHelp,
		append(descELBV2TagsDefaultLabels, labelKeys...),
		nil,
	)
}

// awsTagDescriptionToPrometheusLabels takes a pointer to a TagDescription and returns
// a list of label values and sanitized label keys
func awsTagDescriptionToPrometheusLabelselbv2(tagDescription elbv2.TagDescription) (labelKeys, labelValues []string) {
	for _, tag := range tagDescription.Tags {
		labelKeys = append(labelKeys, sanitizeLabelName(*tag.Key))
		labelValues = append(labelValues, *tag.Value)
	}

	return
}

// collectelbv2 takes a pointer to both a LoadBalancerDescription and TagDescription and builds the lists of
// label keys and label values used subsequently as labels to the tags gauge
func (ec *elbv2Collector) collectELBV2(ch chan<- prometheus.Metric, e elbv2.LoadBalancer, t elbv2.TagDescription) {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{*e.LoadBalancerName, ec.region}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys, labelValues := awsTagDescriptionToPrometheusLabelselbv2(t)
	addGauge(elbv2TagsDesc(labelKeys), 1, labelValues...)
}
