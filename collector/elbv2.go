package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	describeELBV2TagsBatch = 20
)

var elbv2Collector = TagsCollector{
	name:          prometheus.BuildFQName(namespace, "elbv2", "tags"),
	help:          "AWS ELBv2 tags converted to Prometheus labels.",
	defaultLabels: []string{"load_balancer_name", "region"},
	lister:        &elbv2Lister{},
}

type elbv2Lister struct {
	region  string
	session *elbv2.ELBV2
}

func (el *elbv2Lister) Initialise(region string) (err error) {
	el.region = region
	sess, err := session.NewSession(&aws.Config{Region: &el.region})
	if err != nil {
		return
	}
	el.session = elbv2.New(sess)
	return
}

func (el *elbv2Lister) List() ([]tags, error) {
	elbs, err := el.session.DescribeLoadBalancers(&elbv2.DescribeLoadBalancersInput{})

	RequestTotalMetric.With(prometheus.Labels{"service": "elbv2", "region": el.region}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "elbv2", "region": el.region}).Inc()
		return []tags{}, err
	}

	elbv2Arns := make([]*string, 0, len(elbs.LoadBalancers))
	for _, description := range elbs.LoadBalancers {
		elbv2Arns = append(elbv2Arns, description.LoadBalancerArn)
	}

	numReqs := len(elbs.LoadBalancers)/describeELBV2TagsBatch + 1
	reqs := make([]*request.Request, 0, numReqs)
	outs := make([]*elbv2.DescribeTagsOutput, 0, numReqs)
	for i := 0; i < len(elbs.LoadBalancers); i += describeELBV2TagsBatch {
		j := i + describeELBV2TagsBatch
		if j > len(elbs.LoadBalancers) {
			j = len(elbs.LoadBalancers)
		}

		req, out := el.session.DescribeTagsRequest(&elbv2.DescribeTagsInput{
			ResourceArns: elbv2Arns[i:j],
		})
		reqs = append(reqs, req)
		outs = append(outs, out)
	}

	errs := makeConcurrentRequests(reqs, "elbv2")

	tagsList := make([]tags, 0, len(elbs.LoadBalancers))
	for i := range errs {
		RequestTotalMetric.With(prometheus.Labels{"service": "elbv2", "region": el.region}).Inc()
		if errs[i] != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "elbv2", "region": el.region}).Inc()
			continue
		}

		for j, tagDesc := range outs[i].TagDescriptions {
			ts := tags{
				make([]string, 0, len(tagDesc.Tags)+len(elbv2Collector.defaultLabels)),
				make([]string, 0, len(tagDesc.Tags)+len(elbv2Collector.defaultLabels)),
			}

			ts.keys = append(ts.keys, elbv2Collector.defaultLabels...)
			ts.values = append(
				ts.values,
				*elbs.LoadBalancers[i*describeELBV2TagsBatch+j].LoadBalancerName, // calculates index in original list
				el.region,
			)

			tagsList = append(tagsList, ts)
		}
	}

	return tagsList, nil
}
