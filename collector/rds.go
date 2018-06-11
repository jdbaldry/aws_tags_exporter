package collector

import (
  "github.com/aws/aws-sdk-go/aws"
  "github.com/aws/aws-sdk-go/aws/session"
  "github.com/aws/aws-sdk-go/service/rds"
  "github.com/golang/glog"
  "github.com/prometheus/client_golang/prometheus"
)

var (
  descRDSTagsName          = prometheus.BuildFQName(namespace, "rds", "tags")
  descRDSTagsHelp          = "AWS ELB tags converted to Prometheus labels."
  descRDSTagsDefaultLabels = []string{"name", "identifier", "availability_zone"}

  descRDSTags = prometheus.NewDesc(
    descRDSTagsName,
    descRDSTagsHelp,
    descRDSTagsDefaultLabels, nil,
  )
)

type rdsCollector struct {
  store rdsStore
}

type rdsStore interface {
  List() (rdsInstances []*rds.DBInstance, rdsTags [][]*rds.Tag, err error)
}

type rdsLister func() ([]*rds.DBInstance, [][]*rds.Tag, error)

func (l rdsLister) List() ([]*rds.DBInstance, [][]*rds.Tag, error) {
	return l()
}

func RegisterRDSCollector(registry prometheus.Registerer, region *string) error {
  rdsSession := rds.New(session.New(&aws.Config{
		Region: aws.String(*region)},
	))

	lister := rdsLister(func() (rdsInstances []*rds.DBInstance, rdsTags [][]*rds.Tag, err error) {
    dbInput := &rds.DescribeDBInstancesInput{}
		result, err := rdsSession.DescribeDBInstances(dbInput)
    for _, dbi := range result.DBInstances {
      ltInput := &rds.ListTagsForResourceInput{
        ResourceName: aws.String(*dbi.DBInstanceArn),
      }
      result, err := rdsSession.ListTagsForResource(ltInput)
      if err != nil {
        return nil, nil, err
      }
      rdsTags = append(rdsTags, result.TagList)
    }
    return result.DBInstances, rdsTags, nil
	})

	registry.MustRegister(&rdsCollector{store: lister})

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
  glog.V(4).Infof("Collecting RDS instance tags")
  rdsInstances, rdsTags, err := rc.store.List()
  if err != nil {
    glog.Errorf("Error listing RDS instances: ", err)
  }

  for i, instance := range rdsInstances {
    rc.collectRDS(ch, instance, rdsTags[i])
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
