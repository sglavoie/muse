package statusbar

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
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
		if got := text.FormatPositionSecs(tc.secs); got != tc.want {
			t.Errorf("text.FormatPositionSecs(%v) = %q, want %q", tc.secs, got, tc.want)
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
		if got := text.Truncate(tc.input, tc.max); got != tc.expected {
			t.Errorf("text.Truncate(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.expected)
		}
	}
}

func TestNotifyUsesConfiguredDuration(t *testing.T) {
	sb := StatusBar{TransientDur: 15 * time.Second}
	before := time.Now()
	sb.Notify("test message")

	top := sb.TopNotification()
	if top.Msg != "test message" {
		t.Errorf("expected msg %q, got %q", "test message", top.Msg)
	}
	if diff := top.ExpiresAt.Sub(before); diff < 14*time.Second || diff > 16*time.Second {
		t.Errorf("expected expiry ~15s from now, got %v", diff)
	}
}

func TestNotifyFallsBackToDefault(t *testing.T) {
	sb := StatusBar{}
	before := time.Now()
	sb.Notify("fallback test")

	if diff := sb.TopNotification().ExpiresAt.Sub(before); diff < 4*time.Second || diff > 6*time.Second {
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

func TestRenderProgressBarEighthCellPrecision(t *testing.T) {
	st := styles.Styles{}
	cases := []struct {
		name           string
		elapsed, total float64
		width          int
		want           string
	}{
		{"zero progress", 0, 32, 4, "░░░░"},
		{"one eighth cell", 1, 32, 4, "▏░░░"},
		{"half cell", 4, 32, 4, "▌░░░"},
		{"seven eighths cell", 7, 32, 4, "▉░░░"},
		{"one full cell", 8, 32, 4, "█░░░"},
		{"half bar", 16, 32, 4, "██░░"},
		{"full bar", 32, 32, 4, "████"},
		{"over total clamps full", 40, 32, 4, "████"},
		{"negative elapsed clamps empty", -1, 32, 4, "░░░░"},
		{"zero total renders empty", 1, 0, 4, "░░░░"},
		{"nan elapsed renders empty", math.NaN(), 32, 4, "░░░░"},
		{"infinite elapsed renders empty", math.Inf(1), 32, 4, "░░░░"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderProgressBar(tc.elapsed, tc.total, tc.width, st); got != tc.want {
				t.Errorf("renderProgressBar(%v,%v,%d) = %q, want %q", tc.elapsed, tc.total, tc.width, got, tc.want)
			}
		})
	}
}

func TestRenderProgressBarThemedPartialCellUsesEmptyTrackBackground(t *testing.T) {
	st := styles.NewStyles(true)
	emptyBg := st.ProgressEmpty.GetBackground()
	if emptyBg == nil {
		t.Fatal("test requires ProgressEmpty background to use as progress track background")
	}

	got := renderProgressBar(1, 32, 4, st)
	wantPrefix := st.ProgressFilled.Background(emptyBg).Render("▏")
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("partial progress cell should use empty-track background: got %q, want prefix %q", got, wantPrefix)
	}
	if w := ansi.StringWidth(got); w != 4 {
		t.Errorf("themed partial progress bar display width = %d, want 4", w)
	}
}

func TestRenderProgressBarThemedFullCellsUseSolidFilledBackground(t *testing.T) {
	st := styles.NewStyles(true)
	fillBg := st.ProgressFilled.GetForeground()
	if fillBg == nil {
		t.Fatal("test requires ProgressFilled foreground to use as progress track background")
	}

	got := renderProgressBar(8, 32, 4, st)
	wantPrefix := st.ProgressFilled.Background(fillBg).Render(" ")
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("full progress cell should use solid filled background: got %q, want prefix %q", got, wantPrefix)
	}
	if w := ansi.StringWidth(got); w != 4 {
		t.Errorf("themed full progress bar display width = %d, want 4", w)
	}
}
