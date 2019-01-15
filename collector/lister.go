package collector

import "github.com/prometheus/client_golang/prometheus"

type Lister struct{}

// Lister implements the Collector interface.
type ListerCollector struct{}

func NewLister() *Lister {
	return &Lister{}
}

func (l ListerCollector) Describe(ch chan<- *prometheus.Desc) {
	// ch <- prometheus.NewDesc("test", "blah", []string{"test"}, nil)
	close(ch)
}
