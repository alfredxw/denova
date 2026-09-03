package agent

import "testing"

func TestCapacityAwareTokenReserveUsesExistingThresholdHeadroom(t *testing.T) {
	tests := []struct {
		name                 string
		base, output, window int
		threshold            float64
		want                 int
	}{
		{name: "output fits existing headroom", base: 1000, output: 1200, window: 10_000, threshold: .85, want: 1000},
		{name: "reserve only excess over headroom", base: 1000, output: 4000, window: 10_000, threshold: .85, want: 2500},
		{name: "keep larger baseline", base: 3000, output: 4000, window: 10_000, threshold: .85, want: 3000},
		{name: "missing window keeps baseline", base: 1000, output: 4000, window: 0, threshold: .85, want: 1000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CapacityAwareTokenReserve(test.base, test.output, test.window, test.threshold); got != test.want {
				t.Fatalf("reserve = %d, want %d", got, test.want)
			}
		})
	}
}
