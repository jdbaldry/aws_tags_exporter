package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descEC2TagsName          = prometheus.BuildFQName(namespace, "ec2", "tags")
	descEC2TagsHelp          = "AWS EC2 tags converted to Prometheus labels."
	descEC2TagsDefaultLabels = []string{"resource_id", "resource_type"}

	descEC2Tags = prometheus.NewDesc(
		descEC2TagsName,
		descEC2TagsHelp,
		descEC2TagsDefaultLabels, nil,
	)
)

type ec2Resources struct {
	idMap map[string]*ec2Dim
}

type ec2Collector struct {
	store  ec2Store
	region string
}

type ec2Dim struct {
	resType   string
	tagKeys   []string
	tagValues []string
}

type ec2Store interface {
	List() (ec2Resources, error)
}

type ec2Lister func() (ec2Resources, error)

func (l ec2Lister) List() (ec2Resources, error) {
	return l()
}

func RegisterEC2Collector(registry prometheus.Registerer, region string) {

	ec2Session := ec2.New(session.New(&aws.Config{Region: aws.String(region)}))

	lister := ec2Lister(func() (er ec2Resources, err error) {
		result, err := ec2Session.DescribeTags(nil)

		if err != nil {
			glog.Errorf("Error collecting: ec2\n", err)
		} else {
			tags := result.Tags
			er.idMap = make(map[string]*ec2Dim)
			for _, tag := range tags {
				_, exist := er.idMap[*tag.ResourceId]
				if exist {
					er.idMap[*tag.ResourceId].tagKeys = append(er.idMap[*tag.ResourceId].tagKeys, *tag.Key)
					er.idMap[*tag.ResourceId].tagValues = append(er.idMap[*tag.ResourceId].tagValues, *tag.Value)
				} else {
					er.idMap[*tag.ResourceId] = &ec2Dim{*tag.ResourceType, []string{*tag.Key}, []string{*tag.Value}}
				}
			}
		}

		return

	})
	registry.MustRegister(&ec2Collector{store: lister, region: region})
}

func ec2TagsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descEC2TagsName,
		descEC2TagsHelp,
		append(descEC2TagsDefaultLabels, labelKeys...),
		nil,
	)
}

// Describe implements the prometheus.Collector interface.
func (ec *ec2Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descEC2Tags
}

// Collect implements the prometheus.Collector interface.
func (ec *ec2Collector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: ec2")
	ec2Resources, err := ec.store.List()
	if err != nil {
		glog.Errorf("Error collecting: ec2\n", err)
	}

	for i, instance := range ec2Resources.idMap {
		ec.collectEC2(ch, i, *instance)
	}
}

func awsEC2TagToPrometheusLabels(keys, values []string) (labelKeys, labelValues []string) {
	for _, key := range keys {
		labelKeys = append(labelKeys, sanitizeLabelName(key))
	}
	labelValues = values
	return
}

func (ec *ec2Collector) collectEC2(ch chan<- prometheus.Metric, rID string, ed ec2Dim) {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{rID, ed.resType}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys, labelValues := awsEC2TagToPrometheusLabels(ed.tagKeys, ed.tagValues)
	addGauge(ec2TagsDesc(labelKeys), 1, labelValues...)
}
