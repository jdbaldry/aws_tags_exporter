package collector

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	descDynamoDBTagsName          = prometheus.BuildFQName(namespace, "dynamodb", "tags")
	descDynamoDBTagsHelp          = "AWS DynamoDB tags converted to Prometheus labels."
	descDynamoDBTagsDefaultLabels = []string{"name", "identifier", "region"}

	descDynamoDBTags = prometheus.NewDesc(
		descDynamoDBTagsName,
		descDynamoDBTagsHelp,
		descDynamoDBTagsDefaultLabels, nil,
	)
	dynamodbMaxRecords int64 = 100
)

var dynamodbCollector = TagsCollector{
	name:          prometheus.BuildFQName(namespace, "dynamodb", "tags"),
	help:          "AWS DynamoDB tags converted to Prometheus labels.",
	defaultLabels: []string{"name", "identifier", "region"},
	lister:        &dynamodbLister{},
}

type dynamodbLister struct {
	region  string
	session *dynamodb.DynamoDB
}

func (db *dynamodbLister) Initialise(region string) (err error) {
	db.region = region
	sess, err := session.NewSession(&aws.Config{Region: &db.region})
	if err != nil {
		return
	}
	db.session = dynamodb.New(sess)
	return
}

func (db *dynamodbLister) List() ([]tags, error) {
	listDBInput := &dynamodb.ListTablesInput{Limit: &dynamodbMaxRecords}
	tableList, err := db.session.ListTables(listDBInput)
	RequestTotalMetric.With(prometheus.Labels{"service": "dynamodb", "region": db.region}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "dynamodb", "region": db.region}).Inc()
		return []tags{}, err
	}

	descReqs := make([]*request.Request, 0, len(tableList.TableNames))
	descOuts := make([]*dynamodb.DescribeTableOutput, 0, len(tableList.TableNames))
	for i := range tableList.TableNames {
		descReqsIn := &dynamodb.DescribeTableInput{TableName: tableList.TableNames[i]}
		req, out := db.session.DescribeTableRequest(descReqsIn)
		descReqs = append(descReqs, req)
		descOuts = append(descOuts, out)
	}

	errs := makeConcurrentRequests(descReqs, "dynamodb")
	tagsReqs := make([]*request.Request, 0, len(tableList.TableNames))
	tagsOuts := make([]*dynamodb.ListTagsOfResourceOutput, 0, len(tableList.TableNames))
	for i := range descOuts {
		RequestTotalMetric.With(prometheus.Labels{"service": "dynamodb", "region": db.region}).Inc()
		if errs[i] != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "dynamodb", "region": db.region}).Inc()
			// Required for indexing logic
			tagsReqs = append(tagsReqs, &request.Request{})
			tagsOuts = append(tagsOuts, &dynamodb.ListTagsOfResourceOutput{})
			continue
		}

		tagsIn := &dynamodb.ListTagsOfResourceInput{ResourceArn: descOuts[i].Table.TableArn}
		req, out := db.session.ListTagsOfResourceRequest(tagsIn)
		tagsReqs = append(tagsReqs, req)
		tagsOuts = append(tagsOuts, out)
	}

	errs = makeConcurrentRequests(tagsReqs, "dynamodb")

	tagsList := make([]tags, 0, len(tableList.TableNames))
	for i := range tagsOuts {
		RequestTotalMetric.With(prometheus.Labels{"service": "dynamodb", "region": db.region}).Inc()
		if errs[i] != nil {
			RequestErrorTotalMetric.With(prometheus.Labels{"service": "dynamodb", "region": db.region}).Inc()
			// Don't need to maintain size anymore
			continue
		}

		ts := tags{
			make([]string, 0, len(tagsOuts[i].Tags)+len(dynamodbCollector.defaultLabels)),
			make([]string, 0, len(tagsOuts[i].Tags)+len(dynamodbCollector.defaultLabels)),
		}

		ts.keys = append(ts.keys, dynamodbCollector.defaultLabels...)
		ts.values = append(ts.values, *descOuts[i].Table.TableName, *descOuts[i].Table.TableId, db.region)

		for _, t := range tagsOuts[i].Tags {
			ts.keys = append(ts.keys, *t.Key)
			ts.values = append(ts.values, *t.Value)
		}

		tagsList = append(tagsList, ts)
	}

	return tagsList, nil
}
