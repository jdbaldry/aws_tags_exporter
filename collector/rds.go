package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
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

// RegisterRDSCollector registers a collector of RDS tags.
// It also creates the lister function that performs tag collection
func RegisterRDSCollector(registry prometheus.Registerer, region string) {
	glog.V(4).Infof("Registering collector: rds")

	rdsSession := rds.New(session.New(&aws.Config{
		Region: aws.String(region)},
	))

	lister := rdsLister(func() (rdsList, error) {

		dbInput := &rds.DescribeDBInstancesInput{}
		dbOut, err := rdsSession.DescribeDBInstances(dbInput)

		RequestTotalMetric.With(prometheus.Labels{"service": "rds", "region": region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "rds", "region": region}).Inc()
			return rdsList{}, err
		}

		reqs := make([]*request.Request, 0, len(dbOut.DBInstances))
		outs := make([]*rds.ListTagsForResourceOutput, 0, len(dbOut.DBInstances))
		for _, db := range dbOut.DBInstances {
			ltInput := &rds.ListTagsForResourceInput{
				ResourceName: aws.String(*db.DBInstanceArn),
			}
			req, out := rdsSession.ListTagsForResourceRequest(ltInput)
			reqs = append(reqs, req)
			outs = append(outs, out)
		}

		errs := makeConcurrentRequests(reqs, "rds")
		rdsTags := make([][]*rds.Tag, 0, len(dbOut.DBInstances))
		for i := range outs {
			RequestTotalMetric.With(prometheus.Labels{"service": "rds", "region": region}).Inc()
			if errs[i] != nil {
				RequestErrorTotalMetric.With(prometheus.Labels{"service": "rds", "region": region}).Inc()
				continue
			}

			rdsTags = append(rdsTags, outs[i].TagList)
		}

		return rdsList{instances: dbOut.DBInstances, tags: rdsTags}, nil
	})

	registry.MustRegister(&rdsCollector{store: lister, region: region})
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
		rc.collectRDS(ch, *instance, rdsList.tags[i])
	}
}

func awsTagToPrometheusLabels(tags []*rds.Tag) (labelKeys, labelValues []string) {
	for _, tag := range tags {
		labelKeys = append(labelKeys, sanitizeLabelName(*tag.Key))
		labelValues = append(labelValues, *tag.Value)
	}
	return
}

func (rc *rdsCollector) collectRDS(ch chan<- prometheus.Metric, i rds.DBInstance, t []*rds.Tag) {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{*i.DBName, *i.DBInstanceIdentifier, *i.AvailabilityZone}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys, labelValues := awsTagToPrometheusLabels(t)
	addGauge(rdsTagsDesc(labelKeys), 1, labelValues...)
}
