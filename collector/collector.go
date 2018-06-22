package collector

import (
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace       = "aws"
	defaultEnabled  = true
	defaultDisabled = false
)

var (
	RequestTotalMetric = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_tags_request_total",
			Help: "Total requests made by the aws_tags_exporter for a service",
		},
		[]string{"service", "region"},
	)
	RequestErrorTotalMetric = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_tags_request_error_total",
			Help: "Total errors encountered when collecting a service",
		},
		[]string{"service", "region"},
	)
	invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)
)

var AvailableCollectors = map[string]func(registry prometheus.Registerer, Region string) error{
	"elb": RegisterELBCollector,
	"rds": RegisterRDSCollector,
	//	"elasticsearchservice": RegisterESCollector,
}

type Collector interface {
	// Get new metrics and expose them via prometheus registry.
	Update(ch chan<- prometheus.Metric) error
}

func sanitizeLabelName(s string) string {
	return invalidLabelCharRE.ReplaceAllString(s, "_")
}
