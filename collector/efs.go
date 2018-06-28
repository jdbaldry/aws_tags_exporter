package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/efs"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descEFSTagsName          = prometheus.BuildFQName(namespace, "efs", "tags")
	descEFSTagsHelp          = "AWS EFS tags converted to Prometheus labels."
	descEFSTagsDefaultLabels = []string{"file_system_name", "region"}
	descEFSTags              = prometheus.NewDesc(
		descEFSTagsName,
		descEFSTagsHelp,
		descEFSTagsDefaultLabels, nil,
	)

	describeEFSTagsBatch = 20 // Worth making a global batch size for all batched APIs?
)

type efsCollector struct {
	store  efsStore
	region string
}

type efsItem struct {
	efs  *efs.FileSystemDescription
	tags []*efs.Tag
}

type efsStore interface {
	List() ([]efsItem, error)
}

type efsLister func() ([]efsItem, error)

func (l efsLister) List() ([]efsItem, error) {
	return l()
}

func RegisterEFSCollector(registry prometheus.Registerer, region string) error {
	glog.V(4).Infof("Registering collector: efs")

	efsSession := efs.New(session.New(&aws.Config{
		Region: aws.String(region),
	}))

	lister := efsLister(func() ([]efsItem, error) {

		dfsInput := &efs.DescribeFileSystemsInput{}
		fsOut, err := efsSession.DescribeFileSystems(dfsInput)

		RequestTotalMetric.With(prometheus.Labels{"service": "efs", "region": region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "efs", "region": region}).Inc()
			return []efsItem{}, err
		}

		reqs := make([]*request.Request, 0, len(fsOut.FileSystems))
		outs := make([]*efs.DescribeTagsOutput, 0, len(fsOut.FileSystems))
		for _, fs := range fsOut.FileSystems {
			in := &efs.DescribeTagsInput{FileSystemId: fs.FileSystemId}
			req, out := efsSession.DescribeTagsRequest(in)
			reqs = append(reqs, req)
			outs = append(outs, out)
		}

		errs := makeConcurrentRequests(reqs, "efs")

		// Metrics set by concurrent requests function
		efsTags := make([]efsItem, 0, len(fsOut.FileSystems))
		for i := range outs {
			if errs[i] != nil {
				continue
			}

			efsTags = append(efsTags, efsItem{efs: fsOut.FileSystems[i], tags: outs[i].Tags})

		}

		return efsTags, nil
	})

	registry.MustRegister(&efsCollector{store: lister, region: region})
	return nil
}

func (ef *efsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descEFSTags
}

func (ef *efsCollector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: efs")
	efsList, err := ef.store.List()
	if err != nil {
		glog.Errorf("Error collecting efs: %s", err)
	}

	for _, efs := range efsList {
		ef.collectEFS(ch, efs.efs, efs.tags)
	}
}

func (ef *efsCollector) collectEFS(ch chan<- prometheus.Metric, e *efs.FileSystemDescription, ts []*efs.Tag) {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{*e.Name, ef.region}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys := make([]string, 0, len(ts))
	labelValues := make([]string, 0, len(ts))
	for _, t := range ts {
		labelKeys = append(labelKeys, sanitizeLabelName(*t.Key))
		labelValues = append(labelValues, *t.Value)
	}

	addGauge(
		prometheus.NewDesc(descEFSTagsName, descEFSTagsHelp, append(descEFSTagsDefaultLabels, labelKeys...), nil),
		1,
		labelValues...,
	)
}
