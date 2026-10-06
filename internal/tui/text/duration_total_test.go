package text

import (
	"math"
	"testing"
)

func TestDurationTotal(t *testing.T) {
	for _, tc := range []struct {
		values []float64
		want   string
	}{
		{[]float64{60.5, 60.5}, "2:01"},
		{[]float64{3600, 1}, "1:00:01"},
		{[]float64{60, 0, -1, math.NaN(), math.Inf(1)}, "1:00+"},
		{[]float64{0}, "0:00+"},
	} {
		var total DurationTotal
		for _, value := range tc.values {
			total.Add(value)
		}
		if total.String() != tc.want {
			t.Errorf("got %s, want %s", total.String(), tc.want)
		}
	}
}
