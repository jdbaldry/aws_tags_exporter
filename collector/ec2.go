package collector

import (
	"time"

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

	ec2Resources = make(map[string]*ec2_dim)
)

type ec2Collector struct {
	store  ec2Store
	region string
}

type ec2_dim struct {
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

func RegisterEC2Collector(registry prometheus.Registerer, region string) error {

	start := time.Now()

	sess := session.New(&aws.Config{Region: aws.String(region)})

	ec2Session := ec2.New(sess)

	lister := ec2Lister(func() (er ec2Resources, err error) {
		result, err := ec2Session.DescribeTags(nil)

		if err != nil {
			glog.Errorf("Error collecting: ec2\n", err)
			//fmt.Println("error")
		} else {
			tags := result.Tags
			for _, tag := range tags {
				_, exist := ec2Resources[*tag.ResourceId]
				if exist {
					ec2Resources[*tag.ResourceId].tagKeys = append(ec2Resources[*tag.ResourceId].tagKeys, *tag.Key)
					ec2Resources[*tag.ResourceId].tagValues = append(ec2Resources[*tag.ResourceId].tagValues, *tag.Value)
				} else {
					ec2Resources[*tag.ResourceId] = &ec2_dim{*tag.ResourceType, []string{*tag.Key}, []string{*tag.Value}}
				}
			}
			//fmt.Println(ec2Resources)
		}
		elapsed := time.Since(start)
		glog.V(4).Infof("Collecting EC2 Resources took %s", elapsed)
		return

	})
	registry.MustRegister(&ec2Collector{store: lister, region: region})

	return nil
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

	for i, instance := range ec2Resources.instances {
		ec.collectEC2(ch, *instance, ec2Resources.tags[i])
	}
}

func awsTagToPrometheusLabels(keys, values []string) (labelKeys, labelValues []string) {
	for _, key := range keys {
		labelKeys = append(labelKeys, sanitizeLabelName(key))
	}
	labelValues = values
	return
}

func (ec *ec2Collector) collectEC2(ch chan<- prometheus.Metric, ed ec2_dim) {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{ed.keys, ed.values}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys, labelValues := awsTagToPrometheusLabels(ed.keys, ed.values)
	addGauge(ec2TagsDesc(labelKeys), 1, labelValues...)
}
