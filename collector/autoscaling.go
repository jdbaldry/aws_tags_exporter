package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	autoscalingMaxRecords int64 = 100
)

var autoscalingCollector = TagsCollector{
	name:          prometheus.BuildFQName(namespace, "autoscaling", "tags"),
	help:          "AWS autoscaling tags converted to Prometheus labels.",
	defaultLabels: []string{"autoscaling_group_name", "region"},
	lister:        &autoscalingLister{},
}

type autoscalingLister struct {
	region  string
	session *autoscaling.AutoScaling
}

func (al *autoscalingLister) Initialise(region string) (err error) {
	al.region = region
	sess, err := session.NewSession(&aws.Config{Region: &al.region})
	if err != nil {
		return
	}
	al.session = autoscaling.New(sess)
	return
}

func (al *autoscalingLister) List() ([]tags, error) {

	out, err := al.session.DescribeTags(&autoscaling.DescribeTagsInput{MaxRecords: &autoscalingMaxRecords})
	RequestTotalMetric.With(prometheus.Labels{"service": "autoscaling", "region": al.region}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "autoscaling", "region": al.region}).Inc()
		return []tags{}, err
	}

	// convert to temporary map
	tagMap := make(map[string]tags, 0)
	for _, tagDesc := range out.Tags {
		ts, ok := tagMap[*tagDesc.ResourceId]
		if !ok {
			ts = tags{make([]string, 0), make([]string, 0)}
		}

		ts.keys = append(ts.keys, *tagDesc.Key)
		ts.values = append(ts.values, *tagDesc.Value)
		tagMap[*tagDesc.ResourceId] = ts
	}

	// build []tags
	tagsList := make([]tags, 0, len(tagMap))
	for k, v := range tagMap {
		v.keys = append(v.keys, autoscalingCollector.defaultLabels...)
		v.values = append(v.values, k, al.region)

		tagsList = append(tagsList, v)
	}
	return tagsList, nil
}
