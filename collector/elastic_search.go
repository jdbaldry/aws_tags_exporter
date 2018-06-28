package collector

import (
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	esservice "github.com/aws/aws-sdk-go/service/elasticsearchservice"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descESSTagsName          = prometheus.BuildFQName(namespace, "elasticsearch", "tags")
	descESSTagsHelp          = "AWS Elastic Search tags converted to prometheus labels"
	descESSTagsDefaultLabels = []string{"domain", "region"}

	descESSTags = prometheus.NewDesc(
		descESSTagsName,
		descESSTagsHelp,
		descESSTagsDefaultLabels, nil,
	)

	essDomains  = []string{}
	domainNames = []string{}
)

type esTagDescription struct {
	DomainName string
	Tags       []*esservice.Tag
}
type essCollector struct {
	store  essStore
	region string
}

type essList struct {
	domains []*esservice.ElasticsearchDomainStatus
	tags    []*esTagDescription
}

type essStore interface {
	List() (essList, error)
}

type essLister func() (essList, error)

func (l essLister) List() (essList, error) {
	return l()
}

func RegisterESSCollector(registry prometheus.Registerer, region string) error {
	// Define the variables that will be used to store

	glog.V(4).Infof("Registering collector: ess")

	essSession := esservice.New(session.New(&aws.Config{
		Region: aws.String(region)},
	))

	lister := essLister(func() (el essList, err error) {
		start := time.Now()

		var theDomains []*esservice.ElasticsearchDomainStatus
		var allTags []*esTagDescription
		//
		eldInput := &esservice.ListDomainNamesInput{}

		esses, err := essSession.ListDomainNames(eldInput)
		RequestTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
		}

		for _, domain := range esses.DomainNames {
			domainNames = append(domainNames, *domain.DomainName)
		}

		var wg sync.WaitGroup
		for _, domain := range domainNames {
			glog.V(4).Infof("Collecting elastic search info for %s", domain)

			eddInput := &esservice.DescribeElasticsearchDomainInput{
				DomainName: &domain,
			}
			wg.Add(1)

			go func(input *esservice.DescribeElasticsearchDomainInput, domain string) error {
				var DomainInfo *esTagDescription
				result, err := essSession.DescribeElasticsearchDomain(eddInput)
				defer wg.Done()
				RequestTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
				if err != nil {
					RequestErrorTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
				}
				essDomains = append(essDomains, *result.DomainStatus.ARN)
				estInput := &esservice.ListTagsInput{
					ARN: result.DomainStatus.ARN,
				}
				glog.V(4).Infof("Getting list tags for %s", domain)
				tagResult, err := essSession.ListTags(estInput)
				RequestTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
				if err != nil {
					RequestErrorTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
				}
				glog.V(4).Infof("Got the tags")

				DomainInfo = &esTagDescription{DomainName: domain, Tags: tagResult.TagList}
				allTags = append(allTags, DomainInfo)
				theDomains = append(theDomains, result.DomainStatus)

				return nil
			}(eddInput, domain)

			wg.Wait()
			glog.V(4).Infof("Past the wait")
		}
		el = essList{domains: theDomains, tags: allTags}
		elapsed := time.Since(start)
		glog.V(4).Info("Collecting Elastic Search Service took %s", elapsed)
		return
	})
	fmt.Println(lister)
	registry.MustRegister(&essCollector{store: lister, region: region})
	return nil
}

func (ec *essCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descESSTags
}

// Collect implements the prometheus.Collector interface.
func (ec *essCollector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: ess")
	essList, err := ec.store.List()
	if err != nil {
		glog.Errorf("Error collecting: esses\n", err)
	}

	for i, ess := range essList.domains {
		ec.collectESS(ch, *ess, *essList.tags[i])
	}
}

// essTagsDesc takes an array of strings that are AWS tag keys and returns a pointer to a Prometheus
// description from the base set of 'default' labels with the tag keys as additional labels
func essTagsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descESSTagsName,
		descESSTagsHelp,
		append(descESSTagsDefaultLabels, labelKeys...),
		nil,
	)
}

// awsEsTagDescriptionToPrometheusLabels takes a pointer to a TagDescription and returns
// a list of label values and sanitized label keys
func awsEsTagDescriptionToPrometheusLabels(tagDescription esTagDescription) (labelKeys, labelValues []string) {
	for _, tag := range tagDescription.Tags {
		labelKeys = append(labelKeys, sanitizeLabelName(*tag.Key))
		labelValues = append(labelValues, *tag.Value)
	}

	return
}

// collectESS takes a pointer to both a ELasticsearchDomainStatus and TagDescription and builds the lists of
// label keys and label values used subsequently as labels to the tags gauge
func (ec *essCollector) collectESS(ch chan<- prometheus.Metric, e esservice.ElasticsearchDomainStatus, t esTagDescription) error {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{*e.DomainName, ec.region}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys, labelValues := awsEsTagDescriptionToPrometheusLabels(t)
	addGauge(essTagsDesc(labelKeys), 1, labelValues...)
	return nil
}
