package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descDynamoDBTagsName          = prometheus.BuildFQName(namespace, "dynamodb", "tags")
	descDynamoDBTagsHelp          = "AWS DynamoDB tags converted to Prometheus labels."
	descDynamoDBTagsDefaultLabels = []string{"name", "identifier", "availability_zone"}

	descDynamoDBTags = prometheus.NewDesc(
		descDynamoDBTagsName,
		descDynamoDBTagsHelp,
		descDynamoDBTagsDefaultLabels, nil,
	)
)

type dynamodbCollector struct {
	store  dynamodbStore
	region string
}

// dynamodbList contains ordered arrays of database instances and tag lists
type dynamodbItem struct {
	Table *dynamodb.TableDescription
	tags  []*dynamodb.Tag
}

type dynamodbStore interface {
	List() ([]dynamodbItem, error)
}

type dynamodbLister func() ([]dynamodbItem, error)

func (l dynamodbLister) List() ([]dynamodbItem, error) {
	return l()
}

// RegisterDynamoDBCollector registers a collector of DynamoDB tags.
// It also creates the lister function that performs tag collection
func RegisterDynamoDBCollector(registry prometheus.Registerer, region string) {
	glog.V(4).Infof("Registering collector: dynamodb")

	dynamodbSession := dynamodb.New(session.New(&aws.Config{
		Region: aws.String(region)},
	))

	lister := dynamodbLister(func() ([]dynamodbItem, error) {

		dbInput := &dynamodb.ListTablesInput{}
		dbOut, err := dynamodbSession.ListTables(dbInput)

		RequestTotalMetric.With(prometheus.Labels{"service": "dynamodb", "region": region}).Inc()
		if err != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "dynamodb", "region": region}).Inc()
			return []dynamodbItem{}, err
		}

		reqs := make([]*request.Request, 0, len(dbOut.TableNames))
		outs := make([]*dynamodb.ListTagsOfResourceOutput, 0, len(dbOut.TableNames))
		tableOuts := make([]*dynamodb.TableDescription, 0, len(dbOut.TableNames))
		tableDesc := &dynamodb.DescribeTableOutput{}
		for _, name := range dbOut.TableNames {
			in := &dynamodb.DescribeTableInput{TableName: name}
			tableDesc, _ = dynamodbSession.DescribeTable(in)
			ltInput := &dynamodb.ListTagsOfResourceInput{
				ResourceArn: tableDesc.Table.TableArn,
			}
			req, out := dynamodbSession.ListTagsOfResourceRequest(ltInput)
			reqs = append(reqs, req)
			outs = append(outs, out)
			tableOuts = append(tableOuts, tableDesc.Table)
		}

		errs := makeConcurrentRequests(reqs, "dynamodb")
		dynamodbTags := make([]dynamodbItem, 0, len(dbOut.TableNames))
		for i := range outs {
			RequestTotalMetric.With(prometheus.Labels{"service": "dynamodb", "region": region}).Inc()
			if errs[i] != nil {
				RequestErrorTotalMetric.With(prometheus.Labels{"service": "dynamodb", "region": region}).Inc()
				continue
			}

			dynamodbTags = append(dynamodbTags, dynamodbItem{Table: tableOuts[i], tags: outs[i].Tags})
		}

		return dynamodbTags, nil
	})

	registry.MustRegister(&dynamodbCollector{store: lister, region: region})
}

func dynamodbTagsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descDynamoDBTagsName,
		descDynamoDBTagsHelp,
		append(descDynamoDBTagsDefaultLabels, labelKeys...),
		nil,
	)
}

// Describe implements the prometheus.Collector interface.
func (rc *dynamodbCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descDynamoDBTags
}

// Collect implements the prometheus.Collector interface.
func (dc *dynamodbCollector) Collect(ch chan<- prometheus.Metric) {
	glog.V(4).Infof("Collecting: dynamodb")
	dynamodbList, err := dc.store.List()
	if err != nil {
		glog.Errorf("Error collecting: dynamodb\n", err)
	}

	for _, table := range dynamodbList {
		dc.collectDynamoDB(ch, table.Table, table.tags)
	}
}

func (dc *dynamodbCollector) collectDynamoDB(ch chan<- prometheus.Metric, d *dynamodb.TableDescription, ts []*dynamodb.Tag) {
	addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
		lv = append([]string{*d.TableName, *d.TableId, dc.region}, lv...)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
	}

	labelKeys := make([]string, 0, len(ts))
	labelValues := make([]string, 0, len(ts))
	for _, t := range ts {
		labelKeys = append(labelKeys, sanitizeLabelName(*t.Key))
		labelValues = append(labelValues, *t.Value)
	}
	addGauge(dynamodbTagsDesc(labelKeys), 1, labelValues...)
}
