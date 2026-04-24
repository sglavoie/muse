package tui

import (
	"testing"
)

func TestShouldCountPlay(t *testing.T) {
	tests := []struct {
		name    string
		elapsed float64
		total   float64
		want    bool
	}{
		// Boundary: zero/negative values.
		{"zero elapsed", 0, 120, false},
		{"zero total", 30, 0, false},
		{"negative elapsed", -1, 120, false},
		{"negative total", 10, -5, false},
		{"both zero", 0, 0, false},

		// 30-second threshold.
		{"exactly 30s elapsed", 30, 120, true},
		{"29s elapsed long track", 29, 120, false},
		{"31s elapsed", 31, 120, true},

		// 50% threshold for short tracks.
		{"50% of 10s track", 5, 10, true},
		{"49% of 10s track", 4.9, 10, false},
		{"50% of 40s track", 20, 40, true},
		{"short track below both thresholds", 2, 10, false},

		// Edge: very short track where 50% < 30s.
		{"100% of 5s track", 5, 5, true},
		{"1s of 1s track", 1, 1, true},

		// Edge: very long track.
		{"30s of 10min track", 30, 600, true},
		{"29s of 10min track", 29, 600, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCountPlay(tt.elapsed, tt.total)
			if got != tt.want {
				t.Errorf("shouldCountPlay(%v, %v) = %v, want %v", tt.elapsed, tt.total, got, tt.want)
			}
		})
	}
}
