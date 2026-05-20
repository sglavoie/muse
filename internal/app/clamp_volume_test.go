package app_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/app"
)

// TestClampVolume verifies that clampVolume normalises persisted session
// volume values into the [0, 100] range so that a corrupted session row
// cannot poison the player with an out-of-range value.
func TestClampVolume(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero stays zero (muted)", 0, 0},
		{"in-range volume preserved", 75, 75},
		{"max stays max", 100, 100},
		{"negative clamped to zero (not treated as sentinel)", -1, 0},
		{"large negative clamped to zero", -250, 0},
		{"above max clamped to max", 101, 100},
		{"way above max clamped to max", 9999, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := app.ClampVolume(tc.in); got != tc.want {
				t.Errorf("ClampVolume(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
