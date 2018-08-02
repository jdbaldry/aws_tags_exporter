package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ec2MaxRecords int64 = 1000
)

var ec2Collector = TagsCollector{
	name:          prometheus.BuildFQName(namespace, "ec2", "tags"),
	help:          "AWS EC2 tags converted to Prometheus labels.",
	defaultLabels: []string{"resource_id", "resource_type", "region"},
	lister:        &ec2Lister{},
}

type ec2Lister struct {
	region  string
	session *ec2.EC2
}

func (ec *ec2Lister) Initialise(region string) (err error) {
	ec.region = region
	sess, err := session.NewSession(&aws.Config{Region: &ec.region})
	if err != nil {
		return
	}
	ec.session = ec2.New(sess)
	return
}

func (ec *ec2Lister) List() ([]tags, error) {
	res, err := ec.session.DescribeTags(&ec2.DescribeTagsInput{MaxResults: &ec2MaxRecords})

	RequestTotalMetric.With(prometheus.Labels{"service": "ec2", "region": ec.region}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "ec2", "region": ec.region}).Inc()
		return []tags{}, err
	}

	tagMap := make(map[string]tags, 0)
	typeMap := make(map[string]*string, 0)
	for _, tagDesc := range res.Tags {
		ts, ok := tagMap[*tagDesc.ResourceId]
		if !ok {
			ts = tags{make([]string, 0), make([]string, 0)}
		}

		ts.keys = append(ts.keys, *tagDesc.Key)
		ts.values = append(ts.values, *tagDesc.Value)
		tagMap[*tagDesc.ResourceId] = ts
		typeMap[*tagDesc.ResourceId] = tagDesc.ResourceType
	}

	tagsList := make([]tags, 0, len(tagMap))
	for k, v := range tagMap {
		v.keys = append(v.keys, ec2Collector.defaultLabels...)
		v.values = append(v.values, k, *typeMap[k], ec.region)

		tagsList = append(tagsList, v)
	}
	return tagsList, nil
}
