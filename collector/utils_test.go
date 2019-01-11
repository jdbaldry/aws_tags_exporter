package collector

import "testing"

func TestSanitizeLabelName(t *testing.T) {
	tt := []struct {
		name      string
		input     string
		desired   string
		shouldErr bool
	}{
		{"empty string", "", "", true},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sanitizeLabelName(tc.input)
			if tc.shouldErr && err == nil {
				t.Errorf("expected returned error to be not nil but it wasn't.")
			}
		})
	}
}
