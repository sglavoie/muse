package statusbar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestRenderModeCluster(t *testing.T) {
	base := StatusBar{Artist: "Artist", Title: "Title", PlaybackState: playback.StatePlaying}
	out := renderStatusBarForTest(base, 0)
	if !strings.Contains(out, "RSND") {
		t.Errorf("expected RSND cluster when modes off: got %q", out)
	}
	for _, verbose := range []string{"Repeat", "Shuffle", "Auto-DJ", "Stop Next"} {
		if strings.Contains(out, verbose) {
			t.Errorf("verbose label %q should no longer appear: got %q", verbose, out)
		}
	}
	if out = renderStatusBarForTest(func() StatusBar { sb := base; sb.RepeatMode = playback.RepeatOne; return sb }(), 0); !strings.Contains(out, "1SND") {
		t.Errorf("expected 1SND cluster with RepeatOne: got %q", out)
	}
	if out = renderStatusBarForTest(func() StatusBar { sb := base; sb.Shuffle = true; return sb }(), 0); !strings.Contains(out, "RSND") {
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
	base := StatusBar{Artist: "A", Title: "T", PlaybackState: playback.StatePlaying}
	cases := []StatusBar{
		base,
		func() StatusBar { sb := base; sb.RepeatMode = playback.RepeatAll; return sb }(),
		func() StatusBar { sb := base; sb.RepeatMode = playback.RepeatOne; return sb }(),
		func() StatusBar { sb := base; sb.Shuffle = true; return sb }(),
		func() StatusBar { sb := base; sb.StopAfterCurrent = true; return sb }(),
		func() StatusBar { sb := base; sb.RadioMode = true; return sb }(),
		func() StatusBar {
			sb := base
			sb.RepeatMode, sb.Shuffle, sb.StopAfterCurrent, sb.RadioMode = playback.RepeatAll, true, true, true
			return sb
		}(),
	}
	for i, sb := range cases {
		if width := measure(sb); width != 4 {
			t.Errorf("case %d: cluster width=%d, want 4", i, width)
		}
	}
}

func TestModeIndicatorsDriveRenderedCluster(t *testing.T) {
	st := styles.NewStyles(true)
	cases := []struct {
		repeat                    playback.RepeatMode
		shuffle, stopAfter, radio bool
		wantGlyphs                string
		wantStates                [4]string
	}{
		{playback.RepeatOff, false, false, false, "RSND", [4]string{"off", "off", "off", "off"}},
		{playback.RepeatAll, false, false, false, "RSND", [4]string{"all", "off", "off", "off"}},
		{playback.RepeatOne, false, false, false, "1SND", [4]string{"one", "off", "off", "off"}},
		{playback.RepeatOff, true, false, false, "RSND", [4]string{"off", "on", "off", "off"}},
		{playback.RepeatOff, false, true, false, "RSND", [4]string{"off", "off", "on", "off"}},
		{playback.RepeatOff, false, false, true, "RSND", [4]string{"off", "off", "off", "on"}},
		{playback.RepeatAll, true, true, true, "RSND", [4]string{"all", "on", "on", "on"}},
		{playback.RepeatOne, true, true, true, "1SND", [4]string{"one", "on", "on", "on"}},
	}
	for _, tc := range cases {
		indicators := playback.ModeIndicators(tc.repeat, tc.shuffle, tc.stopAfter, tc.radio)
		if len(indicators) != 4 {
			t.Fatalf("repeat=%v: got %d indicators, want 4", tc.repeat, len(indicators))
		}

		var glyphs strings.Builder
		for _, ind := range indicators {
			glyphs.WriteString(ind.Glyph)
		}
		if glyphs.String() != tc.wantGlyphs {
			t.Errorf("%+v: glyphs=%q, want %q", tc, glyphs.String(), tc.wantGlyphs)
		}

		sb := StatusBar{PlaybackState: playback.StatePlaying, RepeatMode: tc.repeat, Shuffle: tc.shuffle, StopAfterCurrent: tc.stopAfter, RadioMode: tc.radio}
		cluster, width := sb.renderModeCluster(newStatusBarRenderContext(0, st, sb.PlaybackState))
		if got := ansi.Strip(cluster); got != glyphs.String() {
			t.Errorf("%+v: rendered cluster %q, want indicator glyphs %q", tc, got, glyphs.String())
		}
		if width != 4 {
			t.Errorf("%+v: cluster width=%d, want 4", tc, width)
		}

		wantActive := [4]bool{tc.repeat != playback.RepeatOff, tc.shuffle, tc.stopAfter, tc.radio}
		for i, ind := range indicators {
			if ind.Active != wantActive[i] {
				t.Errorf("%+v: indicator %d (%s) active=%v, want %v", tc, i, ind.Name, ind.Active, wantActive[i])
			}
			if ind.State != tc.wantStates[i] {
				t.Errorf("%+v: indicator %d (%s) state=%q, want %q", tc, i, ind.Name, ind.State, tc.wantStates[i])
			}
		}
	}
}

// TestModeIndicatorsDescribeToggleKeys pins the metadata the legend overlay and
// the toggle notifications read, so it cannot drift from the keybindings.
func TestModeIndicatorsDescribeToggleKeys(t *testing.T) {
	want := []struct {
		name, key string
		action    keys.Action
	}{
		{"Repeat", "r", keys.ActionRepeatCycle},
		{"Shuffle", "z", keys.ActionShuffleToggle},
		{"Stop after current", "ctrl+n", keys.ActionStopAfterCurrent},
		{"Radio (Auto-DJ)", "A", keys.ActionRadioToggle},
	}
	indicators := playback.ModeIndicators(playback.RepeatOff, false, false, false)
	if len(indicators) != len(want) {
		t.Fatalf("got %d indicators, want %d", len(indicators), len(want))
	}
	registry := keys.NewRegistry()
	for i, ind := range indicators {
		if ind.Name != want[i].name || ind.Key != want[i].key {
			t.Errorf("indicator %d: name=%q key=%q, want name=%q key=%q", i, ind.Name, ind.Key, want[i].name, want[i].key)
		}
		if got := registry.Lookup(keys.ContextGlobal, ind.Key); got != want[i].action {
			t.Errorf("indicator %d (%s): key %q binds to action %v, want %v", i, ind.Name, ind.Key, got, want[i].action)
		}
	}
}

func TestRenderTransientDoesNotOverwritePersistent(t *testing.T) {
	sb := StatusBar{Artist: "Persistent Artist", Title: "Persistent Title", PlaybackState: playback.StatePlaying, Notifications: []TransientNotification{{Msg: "Scanning… 100 tracks"}}}
	out := renderStatusBarForTest(sb, 0)
	for _, want := range []string{"Persistent Artist", "Persistent Title", "▶", "Scanning… 100 tracks"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output: got %q", want, out)
		}
	}
}

func TestRenderTransientVisibleWithWidthConstraint(t *testing.T) {
	sb := StatusBar{Artist: "My Artist", Title: "A Very Long Title Indeed", PlaybackState: playback.StatePlaying, Notifications: []TransientNotification{{Msg: "Some notification"}}}
	out := renderStatusBarForTest(sb, 45)
	for _, want := range []string{"My Artist", "▶", "Some notification", "…"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output: got %q", want, out)
		}
	}
}
