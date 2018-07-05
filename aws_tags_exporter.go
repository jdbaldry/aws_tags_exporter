package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/golang/glog"
	acollector "github.com/grapeshot/aws_tags_exporter/collector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// promLogger implements promhttp.Logger
type promLogger struct{}

func (pl promLogger) Println(v ...interface{}) {
	glog.Error(v)
}

type collectorSet map[string]struct{}

func (cs *collectorSet) String() string {
	cSlice := make([]string, 0, len(*cs))
	for c := range *cs {
		cSlice = append(cSlice, c)
	}

	return strings.Join(cSlice, ",")
}

func (cs *collectorSet) Set(value string) error {
	cSlice := strings.Split(value, ",")
	for _, c := range cSlice {
		(*cs)[c] = struct{}{}
	}

	return nil
}

type registryCollection struct {
	Registry   *prometheus.Registry
	Collectors collectorSet
	Region     *string
}

func telemetryServer(registry prometheus.Gatherer, host string, port int) {
	// Address to listen on for web interface and telemetry
	listenAddress := net.JoinHostPort(host, strconv.Itoa(port))
	glog.Infof("Starting telemetry server: %s", listenAddress)

	mux := http.NewServeMux()

	// Add metricsPath
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{ErrorLog: promLogger{}}))

	// Add index
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>AWS Tags Exporter Server</title></head>
             <body>
             <h1>AWS Tags Exporter Metrics</h1>
			 <ul>
             <li><a href='` + "/metrics" + `'>metrics</a></li>
			 </ul>
             </body>
             </html>`))
	})
	glog.Fatal(http.ListenAndServe(listenAddress, mux))
}

func metricsServer(registry prometheus.Gatherer, host string, port int) {
	// Address to listen on for web interface and telemetry
	listenAddress := net.JoinHostPort(host, strconv.Itoa(port))
	glog.Infof("Starting metrics server: %s", listenAddress)

	mux := http.NewServeMux()

	// Add metricsPath
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{ErrorLog: promLogger{}}))

	// Add index
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>AWS Tags Server</title></head>
             <body>
             <h1>AWS Tags Metrics</h1>
			 <ul>
             <li><a href='` + "/metrics" + `'>metrics</a></li>
			 </ul>
             </body>
             </html>`))
	})
	glog.Fatal(http.ListenAndServe(listenAddress, mux))
}

// registerCollectors creates and starts informers and initializes and
// registers metrics for collection.
func registerCollectors(r registryCollection) []string {

	if len(r.Collectors) == 0 {
		glog.Exit("There are no active collectors")
	}

	activeCollectors := []string{}
	for c := range r.Collectors {
		if f, ok := acollector.AvailableCollectors[c]; ok {
			f(r.Registry, *r.Region)
			activeCollectors = append(activeCollectors, c)
		} else {
			glog.Warningf("No requested collector: %s", c)
		}
	}

	return activeCollectors
}

func getCollectorsAfterExclude(ex collectorSet) collectorSet {
	available := make(collectorSet)

	for col := range acollector.AvailableCollectors {
		available[col] = struct{}{}
	}

	for c := range ex {
		delete(available, c)
	}

	return available
}

func main() {
	// Parse the args (expecting -aws.region)
	TelemetryPort := flag.Int("web.telemetry-port", 60021, "Port number to listen on for telemetry")
	Port := flag.Int("web.port", 60020, "Port number to listen on for metrics")
	Host := flag.String("web.host", "0.0.0.0", "Port number to listen on, default is 0.0.0.0")
	Region := flag.String("aws.region", "", "AWS region to query")

	Includes := make(collectorSet)
	flag.Var(&Includes, "include", "Comma-seperated list of collectors to include")
	Excludes := make(collectorSet)
	flag.Var(&Excludes, "exclude", "Comma-separated list to exclude from all available collectors")

	List := flag.Bool("list", false, "List all available collectors")

	flag.Parse()

	if *List {
		cs := getCollectorsAfterExclude(collectorSet{})
		fmt.Println("Available Collectors: ")
		fmt.Println(cs.String())
		return
	}

	if *Region == "" {
		glog.Exit("Please supply a region")
	}

	if len(Includes) != 0 && len(Excludes) != 0 {
		glog.Exit("Only specify either included or excluded collectors")
	}

	var cols collectorSet

	if len(Includes) != 0 {
		cols = Includes
	} else {
		cols = getCollectorsAfterExclude(Excludes)
	}

	collectorRegistry := registryCollection{
		Registry:   prometheus.NewRegistry(),
		Collectors: cols,
		Region:     Region,
	}

	awsTagsMetricsRegistry := prometheus.NewRegistry()
	awsTagsMetricsRegistry.MustRegister(acollector.RequestTotalMetric)
	awsTagsMetricsRegistry.MustRegister(acollector.RequestErrorTotalMetric)
	awsTagsMetricsRegistry.MustRegister(prometheus.NewProcessCollector(os.Getpid(), ""))
	awsTagsMetricsRegistry.MustRegister(prometheus.NewGoCollector())

	activeCollectors := registerCollectors(collectorRegistry)
	glog.Infof("Active collectors: %s", strings.Join(activeCollectors, ","))
	go telemetryServer(awsTagsMetricsRegistry, *Host, *TelemetryPort)
	metricsServer(collectorRegistry.Registry, *Host, *Port)

}
