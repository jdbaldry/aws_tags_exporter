package collector

import (
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	esservice "github.com/aws/aws-sdk-go/service/elasticsearchservice"
)

var (
	descESSTagsName          = prometheus.BuildFQName(namespace, "elasticsearch", "tags")
	descESSTagsHelp          = "AWS Elastic Search tags converted to prometheus labels"
	descESSTagsDefaultLabels = []string{"domain", "domainID", "ARN"}

	descESSTags = prometheus.NewDesc(
		descESSTagsName,
		descESSTagsHelp,
		descESSTagsDefaultLabels,
		nil,
	)
)

type esDomainTags struct {
	DomainName        string
	DomainARN         string
	DomainTags        map[string]string
	DomainDescription esservice.ElasticsearchDomainStatus
}

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
	tags    [][]*esservice.Tag
}

type essStore interface {
	List() (essList, error)
}

type essLister func() (essList, error)

func (l essLister) List() (essList, error) {
	return l()
}

func RegisterESSCollector(registry prometheus.Registerer, region string) {
	glog.V(4).Infof("Registering collector: elasticsearchService")
	essSession := esservice.New(session.New(&aws.Config{
		Region: aws.String(region)},
	))

	lister := essLister(func() (essList, error) {
		eldInput := &esservice.ListDomainNamesInput{}
		eldOutput, err := essSession.ListDomainNames(eldInput)
		RequestTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
			return essList{}, err
		}
		DomainNames := []*string{}
		for _, domain := range eldOutput.DomainNames {
			DomainNames = append(DomainNames, domain.DomainName)
		}

		dedInput := &esservice.DescribeElasticsearchDomainsInput{
			DomainNames: DomainNames,
		}
		req, out := essSession.DescribeElasticsearchDomainsRequest(dedInput)
		dedReqs := []*request.Request{req}
		dedOuts := []*esservice.DescribeElasticsearchDomainsOutput{out}

		dedErrs := makeConcurrentRequests(dedReqs, "elasticsearchService")
		if len(dedErrs) != 0 {
			for _, errored := range dedErrs {
				if errored != nil {
					RequestErrorTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
					glog.Error(errored)
				}
			}
		}
		letReqs := make([]*request.Request, 0, len(dedOuts))
		letOuts := make([]*esservice.ListTagsOutput, 0, len(dedOuts))
		domainStatuses := []*esservice.ElasticsearchDomainStatus{}
		for _, domain := range dedOuts {
			for i := range domain.DomainStatusList {
				RequestTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
				if err != nil {
					RequestErrorTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
					return essList{}, err
				}
				letInput := &esservice.ListTagsInput{
					ARN: domain.DomainStatusList[i].ARN,
				}
				domainStatuses = append(domainStatuses, domain.DomainStatusList[i])
				req, out := essSession.ListTagsRequest(letInput)
				letReqs = append(letReqs, req)
				letOuts = append(letOuts, out)
			}
		}
		letErrs := makeConcurrentRequests(letReqs, "elasticsearchService")
		for _, errored := range letErrs {
			if errored != nil {
				glog.Error(errored)
			}
		}
		essTags := make([][]*esservice.Tag, 0, len(dedOuts))
		for i := range letOuts {
			RequestTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
			if err != nil {
				RequestErrorTotalMetric.With(prometheus.Labels{"service": "esservice", "region": region}).Inc()
				return essList{}, err
			}
			essTags = append(essTags, letOuts[i].TagList)
		}
		return essList{domains: domainStatuses, tags: essTags}, nil
	})
	registry.MustRegister(&essCollector{store: lister, region: region})

}

func essTagsDesc(labelKeys []string) *prometheus.Desc {
	glog.V(4).Infof("LabelKeys: %s", labelKeys)
	//if len(labelKeys) > 0 {
	return prometheus.NewDesc(
		descESSTagsName,
		descESSTagsHelp,
		append(descESSTagsDefaultLabels, labelKeys...),
		nil,
	)
}

// Describe implements the prometheus.Collector interface.
func (rc *essCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descESSTags
}

// Collect implements the prometheus.Collector interface.
func (rc *essCollector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: elasticsearchService")
	essList, err := rc.store.List()
	if err != nil {
		glog.Errorf("Error collecting: elasticsearchService\n", err)
	}

	for i, domain := range essList.domains {
		rc.collectESS(ch, *domain, essList.tags[i])
	}
}

func awsESSTagToPrometheusLabels(tags []*esservice.Tag) (labelKeys, labelValues []string) {
	for _, tag := range tags {
		labelKeys = append(labelKeys, sanitizeLabelName(*tag.Key))
		labelValues = append(labelValues, *tag.Value)
	}
	return
}

func (rc *essCollector) collectESS(ch chan<- prometheus.Metric, i esservice.ElasticsearchDomainStatus, t []*esservice.Tag) {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{*i.DomainName, *i.DomainId, *i.ARN}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys, labelValues := awsESSTagToPrometheusLabels(t)
	addGauge(essTagsDesc(labelKeys), 1, labelValues...)
}
