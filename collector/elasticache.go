package collector

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/golang/glog"
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

	cacheClusterResourceType = "cluster"
)

type elasticacheCollector struct {
	store     elasticacheStore
	region    string
	accountid string
}

type elasticacheList [][]*elasticache.Tag

type elasticacheStore interface {
	List(string) (elasticacheList, error)
}

type elasticacheLister func(accountid string) (elasticacheList, error)

func (l elasticacheLister) List(accountid string) (elasticacheList, error) {
	return l(accountid)
}

type elasticacheTags struct {
	defaultTags map[string]*string
	taglist     *elasticache.TagListMessage
}

func generateARN(base arn.ARN, resourceType, resourceName string) *string {
	base.Resource = fmt.Sprintf("%s:%s", resourceType, resourceName)
	return aws.String(base.String())
}

func RegisterElasticacheCollector(registry prometheus.Registerer, region string) {
	glog.V(4).Infof("Registering collector: elasticache")

	elasticacheSession := elasticache.New(session.New(&aws.Config{Region: aws.String(region)}))

	lister := elasticacheLister(func(accountid string) (elasticacheList, error) {

		base := arn.ARN{Partition: "aws", Service: "elasticache", Region: region, AccountID: accountid}

		clusters, err := elasticacheSession.DescribeCacheClusters(&elasticache.DescribeCacheClustersInput{})
		RequestTotalMetric.With(prometheus.Labels{"service": "elasticache", "region": region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "elasticache", "region": region}).Inc()
			return elasticacheList{}, err
		}

		clusterarns := make([]*string, 0, len(clusters.CacheClusters))
		for _, cluster := range clusters.CacheClusters {

			clusterarns = append(clusterarns, generateARN(base, cacheClusterResourceType, *cluster.CacheClusterId))
		}

		numReqs := len(clusterarns)
		reqs := make([]*request.Request, 0, numReqs)
		outs := make([]*elasticacheTags, 0, numReqs)

		for i := range clusterarns {
			req, out := elasticacheSession.ListTagsForResourceRequest(&elasticache.ListTagsForResourceInput{ResourceName: clusterarns[i]})
			reqs = append(reqs, req)
			defaults := map[string]*string{
				descElasticacheTagsDefaultLabels[0]: clusters.CacheClusters[i].CacheClusterId,
				descElasticacheTagsDefaultLabels[1]: &cacheClusterResourceType,
				descElasticacheTagsDefaultLabels[2]: &region,
			}
			outs = append(outs, &elasticacheTags{defaultTags: defaults, taglist: out})
		}

		errs := makeConcurrentRequests(reqs, "elasticache")

		els := make(elasticacheList, numReqs)

		for i := range errs {
			RequestTotalMetric.With(prometheus.Labels{"service": "elasticache", "region": "global"}).Inc()
			if errs[i] != nil {
				RequestErrorTotalMetric.With(prometheus.Labels{"service": "elasticache", "region": "global"}).Inc()
				glog.Warning(errs[i])
				continue
			}

			els[i] = outs[i].taglist.TagList
			if len(outs[i].defaultTags) != len(descElasticacheTagsDefaultLabels) {
				glog.Warningf("The default tags are not all initialised")
			}

			for key, value := range outs[i].defaultTags {
				els[i] = append(els[i], &elasticache.Tag{Key: aws.String(key), Value: value})
			}
		}

		return els, nil
	})

	accountid, err := getAccountID()
	if err != nil {
		glog.Error(err)
	}

	registry.MustRegister(&elasticacheCollector{store: lister, region: region, accountid: accountid})
}

func (ec *elasticacheCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descElasticacheTags
}

func (ec *elasticacheCollector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: elasticache")
	els, err := ec.store.List(ec.accountid)
	if err != nil {
		glog.Errorf("Error collecting: elasticaches\n", err)
	}

	for _, tags := range els {
		ec.collectElasticache(ch, tags)
	}
}

func elasticacheTagsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descElasticacheTagsName,
		descElasticacheTagsHelp,
		append(descElasticacheTagsDefaultLabels, labelKeys...),
		nil,
	)
}

func (ec *elasticacheCollector) collectElasticache(ch chan<- prometheus.Metric, tags []*elasticache.Tag) {
	labelKeys := make([]string, 0, len(tags))
	labelValues := make([]string, 0, len(tags))

	for _, t := range tags {
		labelKeys = append(labelKeys, sanitizeLabelName(*t.Key))
		labelValues = append(labelValues, *t.Value)
	}

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			descElasticacheTagsName,
			descElasticacheTagsHelp,
			labelKeys,
			nil,
		),
		prometheus.GaugeValue,
		1,
		labelValues...,
	)
}
