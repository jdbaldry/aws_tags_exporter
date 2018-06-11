package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descELBTagsName          = prometheus.BuildFQName(namespace, "elb", "tags")
	descELBTagsHelp          = "AWS ELB tags converted to Prometheus labels."
	descELBTagsDefaultLabels = []string{"load_balancer_name", "region"}

	descELBTags = prometheus.NewDesc(
		descELBTagsName,
		descELBTagsHelp,
		descELBTagsDefaultLabels, nil,
	)
)

// elbCollector collects the tags of ELBs in the region.
type elbCollector struct {
	elbNames []string
	elbTags  map[string]string
	region   string
	session  *elb.ELB
}

func RegisterELBCollector(registry prometheus.Registerer, region *string) error {
	elbSession := elb.New(session.New(&aws.Config{
		Region: aws.String(*region)},
	))

	if elbNames, err := getELBNames(elbSession); err != nil {
		return err
	} else {
		if elbTags, err := getELBTagKeys(elbSession, elbNames); err != nil {
			return err
		} else {
			registry.MustRegister(&elbCollector{elbNames: elbNames, region: *region,
				elbTags: elbTags, session: elbSession})
		}
	}
	return nil
}

// Describe implements the prometheus.Collector interface.
func (ec *elbCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descELBTags
}

// Collect implements the prometheus.Collector interface.
func (ec *elbCollector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting ELB tags")

	elbs := ec.elbNames

	for _, elb := range elbs {
		ec.collectELBTags(ch, elb)
	}

	glog.V(4).Infof("Collected tags of %d ELBs", len(elbs))
}

func elbTagsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descELBTagsName,
		descELBTagsHelp,
		append(descELBTagsDefaultLabels, labelKeys...),
		nil,
	)
}

func (ec *elbCollector) collectELBTags(ch chan<- prometheus.Metric, elb string) error {

	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{elb, ec.region}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys, labelValues, err := getELBTags(elb, ec.session, ec.elbTags)
	if err != nil {
		return err
	}
	addGauge(elbTagsDesc(labelKeys), 1, labelValues...)

	return nil
}

func getELBNames(elbSession *elb.ELB) (loadBalancerNames []string, err error) {
	glog.V(4).Infof("Finding ELBs")
	input := &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{},
	}
	result, err := elbSession.DescribeLoadBalancers(input)
	if err != nil {
		return nil, err
	}

	for _, lbname := range result.LoadBalancerDescriptions {
		loadBalancerNames = append(loadBalancerNames, *lbname.LoadBalancerName)
	}
	glog.V(4).Infof("Found %d ELBs", len(loadBalancerNames))
	return
}

func getELBTagKeys(elbSession *elb.ELB, elbNames []string) (elbTags map[string]string, err error) {
	glog.V(4).Infof("Finding unique ELB tag keys")
	elbTags = make(map[string]string)
	for _, l := range elbNames {
		tagsInput := &elb.DescribeTagsInput{
			LoadBalancerNames: []*string{
				aws.String(l),
			},
		}
		tagsResult, err := elbSession.DescribeTags(tagsInput)
		if err != nil {
			return nil, err
		}

		for _, description := range tagsResult.TagDescriptions {
			for _, tag := range description.Tags {
				elbTags[*tag.Key] = ""
			}
		}
	}
	glog.V(4).Infof("Found %d unique ELB tag keys", len(elbTags))
	return
}

func getELBTags(lbname string, elbSession *elb.ELB, elbTags map[string]string) (tagKey, tagValue []string, err error) {
	tagsInput := &elb.DescribeTagsInput{
		LoadBalancerNames: []*string{
			aws.String(lbname),
		},
	}
	tagsResult, err := elbSession.DescribeTags(tagsInput)
	if err != nil {
		return nil, nil, err
	}
	for k := range elbTags {
		curValue := ""
		for _, tag := range tagsResult.TagDescriptions {
			for _, AllTags := range tag.Tags {
				if *AllTags.Key == k {
					curValue = *AllTags.Value
					break
				}
			}
		}

		tagKey = append(tagKey, sanitizeLabelName(k))
		tagValue = append(tagValue, curValue)
	}
	return
}
