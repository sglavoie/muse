package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderModeCluster(t *testing.T) {
	base := StatusBar{artist: "Artist", title: "Title", playbackState: PlaybackPlaying}
	out := renderStatusBarForTest(base, 0)
	if !strings.Contains(out, "RSND") {
		t.Errorf("expected RSND cluster when modes off: got %q", out)
	}
	for _, verbose := range []string{"Repeat", "Shuffle", "Auto-DJ", "Stop Next"} {
		if strings.Contains(out, verbose) {
			t.Errorf("verbose label %q should no longer appear: got %q", verbose, out)
		}
	}
	if out = renderStatusBarForTest(func() StatusBar { sb := base; sb.repeatMode = RepeatOne; return sb }(), 0); !strings.Contains(out, "1SND") {
		t.Errorf("expected 1SND cluster with RepeatOne: got %q", out)
	}
	if out = renderStatusBarForTest(func() StatusBar { sb := base; sb.shuffle = true; return sb }(), 0); !strings.Contains(out, "RSND") {
		t.Errorf("expected RSND cluster with shuffle on: got %q", out)
	}
}

func TestRenderModeClusterWidthStable(t *testing.T) {
	measure := func(sb StatusBar) int {
		out := renderStatusBarForTest(sb, 0)
		i := strings.Index(out, "  ")
		if i < 0 {
			t.Fatalf("expected '  ' lead-in for cluster: got %q", out)
		}
		return ansi.StringWidth(out[i+2 : i+6])
	}
	base := StatusBar{artist: "A", title: "T", playbackState: PlaybackPlaying}
	cases := []StatusBar{
		base,
		func() StatusBar { sb := base; sb.repeatMode = RepeatAll; return sb }(),
		func() StatusBar { sb := base; sb.repeatMode = RepeatOne; return sb }(),
		func() StatusBar { sb := base; sb.shuffle = true; return sb }(),
		func() StatusBar { sb := base; sb.stopAfterCurrent = true; return sb }(),
		func() StatusBar { sb := base; sb.radioMode = true; return sb }(),
		func() StatusBar {
			sb := base
			sb.repeatMode, sb.shuffle, sb.stopAfterCurrent, sb.radioMode = RepeatAll, true, true, true
			return sb
		}(),
	}
	for i, sb := range cases {
		if width := measure(sb); width != 4 {
			t.Errorf("case %d: cluster width=%d, want 4", i, width)
		}
	}
}

func TestRenderTransientDoesNotOverwritePersistent(t *testing.T) {
	sb := StatusBar{artist: "Persistent Artist", title: "Persistent Title", playbackState: PlaybackPlaying, notifications: []transientNotification{{msg: "Scanning… 100 tracks"}}}
	out := renderStatusBarForTest(sb, 0)
	for _, want := range []string{"Persistent Artist", "Persistent Title", "▶", "Scanning… 100 tracks"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output: got %q", want, out)
		}
	}
}

func TestRenderTransientVisibleWithWidthConstraint(t *testing.T) {
	sb := StatusBar{artist: "My Artist", title: "A Very Long Title Indeed", playbackState: PlaybackPlaying, notifications: []transientNotification{{msg: "Some notification"}}}
	out := renderStatusBarForTest(sb, 45)
	for _, want := range []string{"My Artist", "▶", "Some notification", "…"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output: got %q", want, out)
		}
	}
}
