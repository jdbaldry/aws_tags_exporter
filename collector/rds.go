package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	rdsMaxRecords int64 = 100
)

var rdsCollector = TagsCollector{
	name:          prometheus.BuildFQName(namespace, "rds", "tags"),
	help:          "AWS RDS tags converted to Prometheus labels.",
	defaultLabels: []string{"name", "identifier", "availability_zone"},
	lister:        &rdsLister{},
}

type rdsLister struct {
	region  string
	session *rds.RDS
}

func (rd *rdsLister) Initialise(region string) (err error) {
	rd.region = region
	sess, err := session.NewSession(&aws.Config{Region: &rd.region})
	if err != nil {
		return
	}
	rd.session = rds.New(sess)
	return
}

func (rd *rdsLister) List() ([]tags, error) {
	dbs, err := rd.session.DescribeDBInstances(&rds.DescribeDBInstancesInput{MaxRecords: &rdsMaxRecords})
	RequestTotalMetric.With(prometheus.Labels{"service": "rds", "region": rd.region}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "rds", "region": rd.region}).Inc()
		return []tags{}, err
	}

	reqs := make([]*request.Request, 0, len(dbs.DBInstances))
	outs := make([]*rds.ListTagsForResourceOutput, 0, len(dbs.DBInstances))
	for _, db := range dbs.DBInstances {
		req, out := rd.session.ListTagsForResourceRequest(&rds.ListTagsForResourceInput{
			ResourceName: aws.String(*db.DBInstanceArn),
		})
		reqs = append(reqs, req)
		outs = append(outs, out)
	}

	errs := makeConcurrentRequests(reqs, "rds")

	tagsList := make([]tags, 0, len(dbs.DBInstances))
	for i := range errs {
		RequestTotalMetric.With(prometheus.Labels{"service": "rds", "region": rd.region}).Inc()
		if errs[i] != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "rds", "region": rd.region}).Inc()
			continue
		}

		ts := tags{
			make([]string, 0, len(outs[i].TagList)+len(rdsCollector.defaultLabels)),
			make([]string, 0, len(outs[i].TagList)+len(rdsCollector.defaultLabels)),
		}

		ts.keys = append(ts.keys, rdsCollector.defaultLabels...)
		ts.values = append(
			ts.values,
			*dbs.DBInstances[i].DBName,
			*dbs.DBInstances[i].DBInstanceIdentifier,
			*dbs.DBInstances[i].AvailabilityZone,
		)

		for _, t := range outs[i].TagList {
			ts.keys = append(ts.keys, *t.Key)
			ts.values = append(ts.values, *t.Value)
		}

		tagsList = append(tagsList, ts)
	}

	return tagsList, nil
}
