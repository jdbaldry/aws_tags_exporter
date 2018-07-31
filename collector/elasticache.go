package collector

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descElasticacheTagsName          = prometheus.BuildFQName(namespace, "elasticache", "tags")
	descElasticacheTagsHelp          = "AWS Elasticache tags converted to Prometheus labels."
	descElasticacheTagsDefaultLabels = []string{"name", "resource_type", "region"}

	descElasticacheTags = prometheus.NewDesc(
		descElasticacheTagsName,
		descElasticacheTagsHelp,
		descElasticacheTagsDefaultLabels, nil,
	)
)

var elasticacheCollector = TagsCollector{
	name:          prometheus.BuildFQName(namespace, "elasticache", "tags"),
	help:          "AWS Elasticache tags converted to Prometheus labels.",
	defaultLabels: []string{"name", "resource_type", "region"},
	lister:        &elasticacheLister{},
}

type elasticacheLister struct {
	region    string
	accountID string
	session   *elasticache.ElastiCache
}

func (el *elasticacheLister) Initialise(region string) (err error) {
	el.region = region
	sess, err := session.NewSession(&aws.Config{Region: &el.region})
	if err != nil {
		return
	}
	el.session = elasticache.New(sess)
	el.accountID, err = getAccountID()
	return
}

func (el *elasticacheLister) generateARN(resourceName *string) *string {
	return aws.String(arn.ARN{
		Partition: "aws",
		Service:   "elasticache",
		Region:    el.region,
		AccountID: el.accountID,
		Resource:  fmt.Sprintf("%s:%s", "cluster", *resourceName),
	}.String())
}

func (el *elasticacheLister) List() ([]tags, error) {
	clusters, err := el.session.DescribeCacheClusters(&elasticache.DescribeCacheClustersInput{})
	RequestTotalMetric.With(prometheus.Labels{"service": "elasticache", "region": el.region}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "elasticache", "region": el.region}).Inc()
		return []tags{}, err
	}

	reqs := make([]*request.Request, 0, len(clusters.CacheClusters))
	outs := make([]*elasticache.TagListMessage, 0, len(clusters.CacheClusters))

	for _, c := range clusters.CacheClusters {
		req, out := el.session.ListTagsForResourceRequest(&elasticache.ListTagsForResourceInput{
			ResourceName: el.generateARN(c.CacheClusterId),
		})

		reqs = append(reqs, req)
		outs = append(outs, out)
	}

	errs := makeConcurrentRequests(reqs, "elasticache")

	tagsList := make([]tags, 0, len(clusters.CacheClusters))
	for i := range errs {
		RequestTotalMetric.With(prometheus.Labels{"service": "elasticache", "region": el.region}).Inc()
		if errs[i] != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "elasticache", "region": el.region}).Inc()
			continue
		}

		ts := tags{
			make([]string, 0, len(outs[i].TagList)+len(elasticacheCollector.defaultLabels)),
			make([]string, 0, len(outs[i].TagList)+len(elasticacheCollector.defaultLabels)),
		}

		ts.keys = append(ts.keys, elasticacheCollector.defaultLabels...)
		ts.values = append(ts.values, *clusters.CacheClusters[i].CacheClusterId, "cluster", el.region)

		tagsList = append(tagsList, ts)
	}

	return tagsList, nil
}
