package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/efs"
	"github.com/prometheus/client_golang/prometheus"
)

var efsCollector = TagsCollector{
	name:          prometheus.BuildFQName(namespace, "efs", "tags"),
	help:          "AWS EFS tags converted to Prometheus labels.",
	defaultLabels: []string{"file_system_name", "region"},
	lister:        &efsLister{},
}

type efsLister struct {
	region  string
	session *efs.EFS
}

func (ef *efsLister) Initialise(region string) (err error) {
	ef.region = region
	sess, err := session.NewSession(&aws.Config{Region: &ef.region})
	if err != nil {
		return
	}
	ef.session = efs.New(sess)
	return
}

func (ef *efsLister) List() ([]tags, error) {

	dfsInput := &efs.DescribeFileSystemsInput{}
	fsOut, err := ef.session.DescribeFileSystems(dfsInput)
	RequestTotalMetric.With(prometheus.Labels{"service": "autoscaling", "region": ef.region}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "autoscaling", "region": ef.region}).Inc()
		return []tags{}, err
	}

	reqs := make([]*request.Request, 0, len(fsOut.FileSystems))
	outs := make([]*efs.DescribeTagsOutput, 0, len(fsOut.FileSystems))
	for _, fs := range fsOut.FileSystems {
		in := &efs.DescribeTagsInput{FileSystemId: fs.FileSystemId}
		req, out := ef.session.DescribeTagsRequest(in)
		reqs = append(reqs, req)
		outs = append(outs, out)
	}

	errs := makeConcurrentRequests(reqs, "efs")
	tagsList := make([]tags, 0, len(fsOut.FileSystems))
	for i := range outs {
		RequestTotalMetric.With(prometheus.Labels{"service": "efs", "region": ef.region}).Inc()
		if errs[i] != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "efs", "region": ef.region}).Inc()
			continue
		}

		ts := tags{
			make([]string, 0, len(outs[i].Tags)+len(efsCollector.defaultLabels)),
			make([]string, 0, len(outs[i].Tags)+len(efsCollector.defaultLabels)),
		}

		ts.keys = append(ts.keys, efsCollector.defaultLabels...)
		ts.values = append(ts.values, *fsOut.FileSystems[i].Name, ef.region)

		for _, t := range outs[i].Tags {
			ts.keys = append(ts.keys, *t.Key)
			ts.values = append(ts.values, *t.Value)
		}

		tagsList = append(tagsList, ts)
	}

	return tagsList, nil
}
