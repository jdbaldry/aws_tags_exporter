package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	elbMaxRecords        int64 = 400
	describeELBTagsBatch       = 20
)

var elbCollector = TagsCollector{
	name:          prometheus.BuildFQName(namespace, "elb", "tags"),
	help:          "AWS ELB tags converted to Prometheus labels.",
	defaultLabels: []string{"load_balancer_name", "region"},
	lister:        &elbLister{},
}

type elbLister struct {
	region  string
	session *elb.ELB
}

func (el *elbLister) Initialise(region string) (err error) {
	el.region = region
	sess, err := session.NewSession(&aws.Config{Region: &el.region})
	if err != nil {
		return
	}
	el.session = elb.New(sess)
	return
}

func (el *elbLister) List() ([]tags, error) {
	elbs, err := el.session.DescribeLoadBalancers(&elb.DescribeLoadBalancersInput{PageSize: &elbMaxRecords})

	RequestTotalMetric.With(prometheus.Labels{"service": "elb", "region": el.region}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "elb", "region": el.region}).Inc()
		return []tags{}, err
	}

	elbNames := make([]*string, 0, len(elbs.LoadBalancerDescriptions))
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

		req, out := el.session.DescribeTagsRequest(&elb.DescribeTagsInput{
			LoadBalancerNames: elbNames[i:j],
		})
		reqs = append(reqs, req)
		outs = append(outs, out)
	}

	errs := makeConcurrentRequests(reqs, "elb")

	tagsList := make([]tags, 0, len(elbs.LoadBalancerDescriptions))
	for i := range errs {
		RequestTotalMetric.With(prometheus.Labels{"service": "elb", "region": el.region}).Inc()
		if errs[i] != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "elb", "region": el.region}).Inc()
			continue
		}

		for _, tagDesc := range outs[i].TagDescriptions {
			ts := tags{
				make([]string, 0, len(tagDesc.Tags)+len(elbCollector.defaultLabels)),
				make([]string, 0, len(tagDesc.Tags)+len(elbCollector.defaultLabels)),
			}

			ts.keys = append(ts.keys, elbCollector.defaultLabels...)
			ts.values = append(ts.values, *tagDesc.LoadBalancerName, el.region)

			tagsList = append(tagsList, ts)
		}
	}

	return tagsList, nil
}
