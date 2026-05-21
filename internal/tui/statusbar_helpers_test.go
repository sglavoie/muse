package tui

import (
	"math"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestFormatTime(t *testing.T) {
	tests := []struct {
		secs float64
		want string
	}{
		{0, "0:00"},
		{59, "0:59"},
		{60, "1:00"},
		{61.9, "1:01"},
		{3600, "1:00:00"},
		{3723.5, "1:02:03"},
		{math.NaN(), "0:00"},
		{math.Inf(1), "0:00"},
		{math.Inf(-1), "0:00"},
	}

	for _, tc := range tests {
		if got := formatTime(tc.secs); got != tc.want {
			t.Errorf("formatTime(%v) = %q, want %q", tc.secs, got, tc.want)
		}
	}
}

func TestTruncateTitle(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		{"日本語タイトル", 14, "日本語タイトル"},
		{"日本語タイトル", 7, "日本語…"},
		{"日本語タイトル", 4, "日…"},
	}

	for _, tc := range tests {
		if got := truncateTitle(tc.input, tc.max); got != tc.expected {
			t.Errorf("truncateTitle(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.expected)
		}
	}
}

func TestNotifyUsesConfiguredDuration(t *testing.T) {
	sb := StatusBar{transientDur: 15 * time.Second}
	before := time.Now()
	sb.notify("test message")

	top := sb.topNotification()
	if top.msg != "test message" {
		t.Errorf("expected msg %q, got %q", "test message", top.msg)
	}
	if diff := top.expiresAt.Sub(before); diff < 14*time.Second || diff > 16*time.Second {
		t.Errorf("expected expiry ~15s from now, got %v", diff)
	}
}

func TestNotifyFallsBackToDefault(t *testing.T) {
	sb := StatusBar{}
	before := time.Now()
	sb.notify("fallback test")

	if diff := sb.topNotification().expiresAt.Sub(before); diff < 4*time.Second || diff > 6*time.Second {
		t.Errorf("expected expiry ~5s (fallback) from now, got %v", diff)
	}
}

func TestRenderProgressBarNonPositiveWidth(t *testing.T) {
	st := styles.Styles{}
	for _, w := range []int{0, -1, -100} {
		if got := renderProgressBar(10, 100, w, st); got != "" {
			t.Errorf("renderProgressBar(width=%d) = %q, want empty string", w, got)
		}
	}
}

func TestRenderProgressBarValidWidths(t *testing.T) {
	st := styles.Styles{}
	cases := []struct {
		name           string
		elapsed, total float64
		width          int
	}{
		{"zero progress", 0, 100, 10},
		{"half progress", 50, 100, 10},
		{"full progress", 100, 100, 10},
		{"over 100%", 200, 100, 10},
		{"negative elapsed", -5, 100, 10},
		{"zero total", 50, 0, 10},
		{"width 1", 50, 100, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderProgressBar(tc.elapsed, tc.total, tc.width, st)
			if w := ansi.StringWidth(got); w != tc.width {
				t.Errorf("renderProgressBar(%v,%v,%d) display width = %d, want %d", tc.elapsed, tc.total, tc.width, w, tc.width)
			}
		})
	}
}
