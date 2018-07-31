package collector

import (
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "aws"
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

type tags struct {
	keys   []string
	values []string
}

// sanitizeKeys is a helper function to convert label keys which have characters not accepted by prometheus to "_"
func (ls *tags) sanitizeKeys() {
	for i := range ls.keys {
		ls.keys[i] = invalidLabelCharRE.ReplaceAllString(ls.keys[i], "_")
	}
}

// sendToPrometheus creates a new metric and sends it to the specified channel
func (ls *tags) sendToPrometheus(ch chan<- prometheus.Metric, name, help string) {
	ls.sanitizeKeys()
	desc := prometheus.NewDesc(
		name,
		help,
		ls.keys,
		nil,
	)

	ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, 1, ls.values...)
}

type tagsLister interface {
	// Initialise initialises with any state required to collect tags
	// It is run exactly once, when a TagsCollector is registered.
	Initialise(region string) error
	// List is called on an initialised tagsLister to get the tags
	// It is run every time the tags are collected.
	List() ([]tags, error)
}

// TagsCollector is a struct which represents a prometheus Collector
// It is initialised once per resource type.
type TagsCollector struct {
	name          string           // name of collector
	help          string           // help message of collector
	defaultLabels []string         // defaultLabels are the required labels that a collector must return
	defaultDesc   *prometheus.Desc // defaultDesc is the prometheus description (initialised on the first Describe call)
	lister        tagsLister       // lister is used to get the tags for a particular resource
}

// Describe is required to implement the prometheus.Collector interface.
// It also initialises defaultDesc when it is first called.
func (tc *TagsCollector) Describe(ch chan<- *prometheus.Desc) {
	if tc.defaultDesc == nil {
		tc.defaultDesc = prometheus.NewDesc(tc.name, tc.help, tc.defaultLabels, nil)
	}
	ch <- tc.defaultDesc
}

// Collect is required to implement the prometheus.Collector interface.
func (tc *TagsCollector) Collect(ch chan<- prometheus.Metric) {
	tagsList, err := tc.lister.List()
	if err != nil {
		return
	}

	for _, tags := range tagsList {
		tags.sendToPrometheus(ch, tc.name, tc.help)
	}
}

// Register registers the collector in the specified prometheus.Registry to collect tags in the specified region.
// region can be set to anything if the resource is region agnostic (e.g. Route53).
func (tc *TagsCollector) Register(registry *prometheus.Registry, region string) (err error) {
	err = tc.lister.Initialise(region)
	registry.MustRegister(tc)
	return
}

// AvailableCollector maps a string key to each collector (that has been implemented).
// This is used by the main package to Register the required collectors.
var AvailableCollectors = map[string]TagsCollector{
	"autoscaling": autoscalingCollector,
	"ec2":         ec2Collector,
	"efs":         efsCollector,
	"elasticache": elasticacheCollector,
}
