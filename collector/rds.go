package collector

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descRDSTagsName          = prometheus.BuildFQName(namespace, "rds", "tags")
	descRDSTagsHelp          = "AWS RDS tags converted to Prometheus labels."
	descRDSTagsDefaultLabels = []string{"name", "identifier", "availability_zone"}

	descRDSTags = prometheus.NewDesc(
		descRDSTagsName,
		descRDSTagsHelp,
		descRDSTagsDefaultLabels, nil,
	)
)

type rdsCollector struct {
	store  rdsStore
	region string
}

// rdsList contains ordered arrays of database instances and tag lists
type rdsList struct {
	instances []*rds.DBInstance
	tags      [][]*rds.Tag
}

type rdsStore interface {
	List() (rdsList, error)
}

type rdsLister func() (rdsList, error)

func (l rdsLister) List() (rdsList, error) {
	return l()
}

func RegisterRDSCollector(registry prometheus.Registerer, region string) error {
	rdsSession := rds.New(session.New(&aws.Config{
		Region: aws.String(region)},
	))

	lister := rdsLister(func() (rl rdsList, err error) {
		start := time.Now()
		var rdsTags [][]*rds.Tag
		dbInput := &rds.DescribeDBInstancesInput{}
		result, err := rdsSession.DescribeDBInstances(dbInput)
		RequestTotalMetric.With(prometheus.Labels{"service": "rds", "region": region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "rds", "region": region}).Inc()
			return rl, err
		}
		for _, dbi := range result.DBInstances {
			ltInput := &rds.ListTagsForResourceInput{
				ResourceName: aws.String(*dbi.DBInstanceArn),
			}
			result, err := rdsSession.ListTagsForResource(ltInput)
			RequestTotalMetric.With(prometheus.Labels{"service": "rds", "region": region}).Inc()
			if err != nil {
				RequestErrorTotalMetric.With(prometheus.Labels{"service": "rds", "region": region}).Inc()
				return rl, err
			}
			rdsTags = append(rdsTags, result.TagList)
		}
		rl = rdsList{instances: result.DBInstances, tags: rdsTags}
		elapsed := time.Since(start)
		glog.V(4).Infof("Collecting RDS took %s", elapsed)
		return
	})

	registry.MustRegister(&rdsCollector{store: lister, region: region})

	return nil
}

func rdsTagsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descRDSTagsName,
		descRDSTagsHelp,
		append(descRDSTagsDefaultLabels, labelKeys...),
		nil,
	)
}

// Describe implements the prometheus.Collector interface.
func (rc *rdsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descRDSTags
}

// Collect implements the prometheus.Collector interface.
func (rc *rdsCollector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: rds")
	rdsList, err := rc.store.List()
	if err != nil {
		glog.Errorf("Error collecting: rds\n", err)
	}

	for i, instance := range rdsList.instances {
		rc.collectRDS(ch, instance, rdsList.tags[i])
	}
}

func awsTagToPrometheusLabels(tags []*rds.Tag) (labelKeys, labelValues []string) {
	for _, tag := range tags {
		labelKeys = append(labelKeys, sanitizeLabelName(*tag.Key))
		labelValues = append(labelValues, *tag.Value)
	}
	return
}

func (rc *rdsCollector) collectRDS(ch chan<- prometheus.Metric, i *rds.DBInstance, t []*rds.Tag) {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{*i.DBName, *i.DBInstanceIdentifier, *i.AvailabilityZone}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys, labelValues := awsTagToPrometheusLabels(t)
	addGauge(rdsTagsDesc(labelKeys), 1, labelValues...)
}
