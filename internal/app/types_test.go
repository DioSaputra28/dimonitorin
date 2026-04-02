package app

import "testing"

func TestThresholdEvaluate(t *testing.T) {
	threshold := Threshold{Warning: 80, Critical: 90}
	tests := []struct {
		value float64
		want  Status
	}{
		{20, StatusHealthy},
		{80, StatusWarning},
		{91, StatusCritical},
	}
	for _, tt := range tests {
		if got := threshold.Evaluate(tt.value); got != tt.want {
			t.Fatalf("value %v: got %s want %s", tt.value, got, tt.want)
		}
	}
}
