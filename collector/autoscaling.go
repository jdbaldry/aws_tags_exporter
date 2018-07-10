package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descAutoscalingTagsName          = prometheus.BuildFQName(namespace, "autoscaling", "tags")
	descAutoscalingTagsHelp          = "AWS autoscaling tags converted to Prometheus labels."
	descAutoscalingTagsDefaultLabels = []string{"autoscaling_group_name", "region"}
	descAutoscalingTags              = prometheus.NewDesc(
		descAutoscalingTagsName,
		descAutoscalingTagsHelp,
		descAutoscalingTagsDefaultLabels, nil,
	)
	autoscalingMaxRecords int64 = 100
)

type autoscalingCollector struct {
	store  autoscalingStore
	region string
}

type autoscalingList map[string][]*autoscaling.TagDescription

type autoscalingStore interface {
	List() (autoscalingList, error)
}

type autoscalingLister func() (autoscalingList, error)

func (l autoscalingLister) List() (autoscalingList, error) {
	return l()
}

func RegisterAutoscalingCollector(registry prometheus.Registerer, region string) {
	glog.V(4).Infof("Registering collector: autoscaling")

	autoscalingSession := autoscaling.New(session.New(&aws.Config{
		Region: aws.String(region),
	}))

	lister := autoscalingLister(func() (autoscalingList, error) {

		out, err := autoscalingSession.DescribeTags(&autoscaling.DescribeTagsInput{MaxRecords: &autoscalingMaxRecords})
		RequestTotalMetric.With(prometheus.Labels{"service": "autoscaling", "region": region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "autoscaling", "region": region}).Inc()
			return autoscalingList{}, err
		}

		al := make(autoscalingList)
		for _, tg := range out.Tags {
			al[*tg.ResourceId] = append(al[*tg.ResourceId], tg)
		}

		return al, nil
	})

	registry.MustRegister(&autoscalingCollector{store: lister, region: region})
}

func (ef *autoscalingCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descAutoscalingTags
}

func (ef *autoscalingCollector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: autoscaling")
	al, err := ef.store.List()
	if err != nil {
		glog.Errorf("Error collecting autoscaling: %s", err)
	}

	for key, tags := range al {
		ef.collectAutoscaling(ch, key, tags)
	}
}

func (ef *autoscalingCollector) collectAutoscaling(ch chan<- prometheus.Metric, key string, tags []*autoscaling.TagDescription) {

	labelKeys := make([]string, 0, len(tags)+len(descAutoscalingTagsDefaultLabels))
	labelKeys = append(labelKeys, descAutoscalingTagsDefaultLabels...)

	labelValues := make([]string, 0, len(tags)+len(descAutoscalingTagsDefaultLabels))
	labelValues = append(labelValues, key, ef.region)

	for _, t := range tags {
		labelKeys = append(labelKeys, sanitizeLabelName(*t.Key))
		labelValues = append(labelValues, *t.Value)
	}

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			descAutoscalingTagsName,
			descAutoscalingTagsHelp,
			labelKeys,
			nil,
		),
		prometheus.GaugeValue,
		1,
		labelValues...,
	)
}
