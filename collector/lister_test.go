package collector

import "testing"

func TestLister(t *testing.T) {
	tt := []struct {
		name string
	}{
		{"lister should return no descriptor"},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			lister := NewLister()
			lister.Describe()
		})
	}
}
