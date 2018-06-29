package collector

import (
	"regexp"
	"sync"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace       = "aws"
	defaultEnabled  = true
	defaultDisabled = false
)

var (
	// RequestTotalMetric counts the total requests made to AWS by all collectors
	RequestTotalMetric = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_tags_request_total",
			Help: "Total requests made by the aws_tags_exporter for a service",
		},
		[]string{"service", "region"},
	)
	// RequestErrorTotalMetric counts the total errors encountered by all collectors
	// when making requests to AWS
	RequestErrorTotalMetric = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_tags_request_error_total",
			Help: "Total errors encountered when collecting a service",
		},
		[]string{"service", "region"},
	)
	invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)
)

// AvailableCollectors is a map of all implemented collectors with the associated
// registration function
var AvailableCollectors = map[string]func(registry prometheus.Registerer, region string){
	"efs":   RegisterEFSCollector,
	"elb":   RegisterELBCollector,
	"rds":   RegisterRDSCollector,
	"ec2":   RegisterEC2Collector,
	"elbv2": RegisterELBV2Collector,
}

func makeConcurrentRequests(reqs []*request.Request, service string) []error {
	var wg sync.WaitGroup
	var errs = make([]error, len(reqs))
	glog.V(4).Infof("Collecting %s", service)
	wg.Add(len(reqs))
	for i := range reqs {
		go func(i int, req *request.Request) {
			defer wg.Done()
			errs[i] = req.Send()
		}(i, reqs[i])
	}
	wg.Wait()
	return errs
}

func sanitizeLabelName(s string) string {
	return invalidLabelCharRE.ReplaceAllString(s, "_")
}
