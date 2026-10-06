package text

import "math"

// DurationTotal sums known durations and marks incomplete totals with +.
type DurationTotal struct {
	seconds float64
	unknown bool
}

func (d *DurationTotal) Add(seconds float64) {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		d.unknown = true
		return
	}
	d.seconds += seconds
}

func (d DurationTotal) String() string {
	label := FormatPositionSecs(d.seconds)
	if d.unknown {
		label += "+"
	}
	return label
}
