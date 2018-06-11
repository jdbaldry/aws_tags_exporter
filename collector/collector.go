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
	invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)
)

var AvailableCollectors = map[string]func(registry prometheus.Registerer, Region *string) error{
	"elb": RegisterELBCollector,
	"rds": RegisterRDSCollector,
	"elasticsearchservice": RegisterESCollector,
}

type Collector interface {
	// Get new metrics and expose them via prometheus registry.
	Update(ch chan<- prometheus.Metric) error
}

func sanitizeLabelName(s string) string {
	return invalidLabelCharRE.ReplaceAllString(s, "_")
}
