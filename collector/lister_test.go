package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestListerCollector(t *testing.T) {
	tt := []struct {
		name string
	}{
		{"lister should not send descriptor"},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan *prometheus.Desc)
			lc := NewLister()
			lc.Describe(ch)
			descriptor := <-ch
			if descriptor != nil {
				t.Errorf("ListerCollector should not send any descriptors but we received %v", descriptor)
			}
		})
	}
}
