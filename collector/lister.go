package collector

import "github.com/prometheus/client_golang/prometheus"

type Lister struct{}

func NewLister() *Lister {
	return &Lister{}
}

// Lister implements the Collector interface.
func (l Lister) Describe(ch chan<- *prometheus.Desc) {}
