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
		{"simple string", "simple", "simple", false},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result, err := sanitizeLabelName(tc.input)
			if tc.shouldErr && err == nil {
				t.Errorf("expected returned error to be not nil but it wasn't")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.desired != result {
				t.Errorf("incorrect result; expected %s, got  %s", tc.desired, result)
			}
		})
	}
}
