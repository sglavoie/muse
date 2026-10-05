package library

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
)

func TestSelectionDuration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		durations []float64
		want      string
	}{
		{"known", []float64{183, 245}, "7:08"},
		{"fractional", []float64{60.6, 60.6}, "2:01"},
		{"hours", []float64{3600, 61}, "1:01:01"},
		{"unknown", []float64{183, 0}, "3:03+"},
		{"all unknown", []float64{0, -1}, "0:00+"},
		{"invalid", []float64{math.NaN(), math.Inf(1)}, "0:00+"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newRenderView([]musiclib.Track{
				{Hash: "a", Title: "A", Duration: tc.durations[0]},
				{Hash: "b", Title: "B", Duration: tc.durations[1]},
				{Hash: "c", Title: "C", Duration: 999},
			})
			v.SelectedHashes = map[string]struct{}{"a": {}, "b": {}}
			// Filtering to an unselected row must not change the selection total.
			v = activateFilterQuery(v, "C")
			line := ansi.Strip(v.renderStateStrip(3, 1))
			if !strings.Contains(line, "2 selected · "+tc.want) {
				t.Fatalf("selection total = %q, want %s", line, tc.want)
			}
		})
	}
}

func TestSelectionDurationSourceAndStaleHashes(t *testing.T) {
	v := newRenderView(nil)
	v.OverrideTracks = []musiclib.Track{{Hash: "a", Duration: 60}, {Hash: "a", Duration: 60}}
	v.SelectedHashes = map[string]struct{}{"a": {}}
	if got := v.selectedDuration(); got != "1:00" {
		t.Fatalf("source-backed selection = %q", got)
	}
	v.SelectedHashes["missing"] = struct{}{}
	if got := v.selectedDuration(); got != "1:00+" {
		t.Fatalf("unresolved selection = %q", got)
	}
	v.SelectedHashes = nil
	if got := ansi.Strip(v.renderStateStrip(2, 2)); strings.Contains(got, "selected") {
		t.Fatalf("cleared selection retained chip: %q", got)
	}
}
