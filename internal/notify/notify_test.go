package notify

import (
	"testing"
)

func TestMessage(t *testing.T) {
	tests := []struct {
		name  string
		track Track
		want  string
	}{
		{"album", Track{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}, "Dawn"},
		{"paused album", Track{Title: "Sunrise", Album: "Dawn", Paused: true}, "Paused — Dawn"},
		{"no album", Track{Title: "Sunrise", Artist: "Aurora"}, "Now playing"},
		{"degraded wins", Track{Title: "Sunrise", Album: "Dawn", Degraded: "Not in library"}, "Not in library"},
		{"paused degraded", Track{Title: "Sunrise", Degraded: "Not in library", Paused: true}, "Paused — Not in library"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.track.Message(); got != tt.want {
				t.Errorf("Message() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummaryLine(t *testing.T) {
	n := Track{Title: "Sunrise", Artist: "Aurora", Album: "Dawn", Paused: true}

	want := "Sunrise — Aurora (Paused — Dawn)"
	if got := n.SummaryLine(); got != want {
		t.Fatalf("SummaryLine() = %q, want %q", got, want)
	}

	idle := Track{Title: "muse", Degraded: "Nothing playing"}
	if got := idle.SummaryLine(); got != "muse (Nothing playing)" {
		t.Fatalf("idle SummaryLine() = %q, want %q", got, "muse (Nothing playing)")
	}
}
