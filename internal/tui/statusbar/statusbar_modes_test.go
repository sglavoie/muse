package statusbar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestPlaybackModesReadableWithoutColor(t *testing.T) {
	cases := []struct {
		name              string
		repeat            playback.RepeatMode
		shuffle, stop, dj bool
		wide, compact     string
	}{
		{"off", playback.RepeatOff, false, false, false, "Sequential", "Sequential"},
		{"repeat all", playback.RepeatAll, false, false, false, "Repeat all", "Repeat all"},
		{"repeat one", playback.RepeatOne, false, false, false, "Repeat one", "Repeat one"},
		{"shuffle", playback.RepeatOff, true, false, false, "Shuffle", "Shuffle"},
		{"stop", playback.RepeatOff, false, true, false, "Stop after track", "Stop after track"},
		{"dj", playback.RepeatOff, false, false, true, "Auto-DJ", "Auto-DJ"},
		{"all", playback.RepeatAll, true, true, true, "Repeat all · Shuffle · Stop after track · Auto-DJ", "Rpt:all Shuf Stop DJ"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sb := StatusBar{Title: "Song", PlaybackState: playback.StatePlaying, RepeatMode: tc.repeat, Shuffle: tc.shuffle, StopAfterCurrent: tc.stop, RadioMode: tc.dj}
			for _, width := range []int{60, 80, 160} {
				want := tc.compact
				if width == 160 {
					want = tc.wide
				}
				out := ansi.Strip(sb.Render(width, styles.NewStyles(true)))
				if !strings.Contains(out, want) || !strings.Contains(out, "Song") {
					t.Fatalf("width %d: missing song or %q: %s", width, want, out)
				}
				for _, line := range strings.Split(out, "\n") {
					if ansi.StringWidth(line) > width {
						t.Fatalf("overflow: %s", line)
					}
				}
			}
		})
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
		{"Auto-DJ", "A", keys.ActionRadioToggle},
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
	out := renderStatusBarForTest(sb, 60)
	for _, want := range []string{"A Very", "▶", "Some notification", "…"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output: got %q", want, out)
		}
	}
}
