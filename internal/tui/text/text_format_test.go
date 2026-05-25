package text

import (
	"math"
	"testing"
)

func TestFormatDurationSecs(t *testing.T) {
	tests := []struct {
		name string
		secs float64
		want string
	}{
		{"zero", 0, "--:--"},
		{"negative", -5.0, "--:--"},
		{"NaN", math.NaN(), "--:--"},
		{"positive infinity", math.Inf(1), "--:--"},
		{"negative infinity", math.Inf(-1), "--:--"},
		{"seconds only", 45.0, "0:45"},
		{"minutes and seconds", 125.0, "2:05"},
		{"hours", 3661.0, "1:01:01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDurationSecs(tt.secs)
			if got != tt.want {
				t.Errorf("FormatDurationSecs(%v) = %q, want %q", tt.secs, got, tt.want)
			}
		})
	}
}

func TestFormatDurationMs(t *testing.T) {
	tests := []struct {
		name string
		ms   int64
		want string
	}{
		{"zero", 0, "--:--"},
		{"negative", -1000, "--:--"},
		{"typical value", 125000, "2:05"},
		{"hours", 3661000, "1:01:01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDurationMs(tt.ms)
			if got != tt.want {
				t.Errorf("FormatDurationMs(%d) = %q, want %q", tt.ms, got, tt.want)
			}
		})
	}
}

func TestFormatTrackPos(t *testing.T) {
	tests := []struct {
		name     string
		trackNum int
		discNum  int
		fallback int
		want     string
	}{
		{"unknown zero uses fallback", 0, 1, 7, "7"},
		{"invalid negative uses fallback", -1, 1, 7, "7"},
		{"invalid negative with disc uses fallback", -1, 2, 7, "7"},
		{"single disc uses bare track", 3, 1, 7, "3"},
		{"multi disc prefixes disc", 3, 2, 7, "2.03"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTrackPos(tt.trackNum, tt.discNum, tt.fallback)
			if got != tt.want {
				t.Errorf("FormatTrackPos(%d, %d, %d) = %q, want %q",
					tt.trackNum, tt.discNum, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestFormatTrackPosSimple(t *testing.T) {
	tests := []struct {
		name     string
		trackNum int
		fallback int
		want     string
	}{
		{"unknown zero uses fallback", 0, 7, "7"},
		{"invalid negative uses fallback", -1, 7, "7"},
		{"positive uses track number", 3, 7, "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTrackPosSimple(tt.trackNum, tt.fallback)
			if got != tt.want {
				t.Errorf("FormatTrackPosSimple(%d, %d) = %q, want %q",
					tt.trackNum, tt.fallback, got, tt.want)
			}
		})
	}
}
