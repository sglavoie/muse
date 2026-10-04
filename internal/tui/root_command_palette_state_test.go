package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestPaletteReportsCurrentSettingsOnReopen(t *testing.T) {
	m := New(context.Background())
	m.radio.enabled = true
	m.playback.repeatMode = playback.RepeatOne
	m.playback.shuffle = true
	m.playback.stopAfterCurrent = true
	m.showFooterHints = true
	m.notifyOnTrackChange = true
	m.followNowPlaying = true
	m.cfg.NormalizeVolume = true
	m.cfg.EQEnabled = true
	m.cfg.StereoWideEnabled = true
	// Auto theme must report the resolved appearance, not an empty config value.
	m.cfg.Theme = ""
	m.terminalHasDark = false
	m = m.applyTheme()
	want := map[string]string{
		"radio.toggle":                "On",
		"playback.repeat.cycle":       "one",
		"playback.shuffle.toggle":     "On",
		"playback.stop_after_current": "On",
		"ui.toggle.theme":             "Light",
		"ui.toggle.footer_hints":      "On",
		"ui.toggle.track_notify":      "On",
		"queue.toggle.follow":         "On",
		"audio.toggle.normalize":      "On",
		"audio.toggle.eq":             "On",
		"audio.toggle.stereo":         "On",
	}
	for _, cmd := range m.BuildPaletteCommands() {
		if state, ok := want[cmd.ID]; ok {
			if cmd.CurrentState != state {
				t.Errorf("%s state = %q, want %q", cmd.ID, cmd.CurrentState, state)
			}
			delete(want, cmd.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing commands: %v", want)
	}
	m, _ = m.handleToggleQueueFollowMsg()
	m, _ = m.handleToggleThemeMsg()
	for _, cmd := range m.BuildPaletteCommands() {
		if cmd.ID == "queue.toggle.follow" && (cmd.CurrentState != "Off" || cmd.Name != "Toggle queue follow") {
			t.Fatalf("reopened queue follow command = %+v", cmd)
		}
		if cmd.ID == "ui.toggle.theme" && cmd.CurrentState != "Dark" {
			t.Fatalf("reopened theme state = %q", cmd.CurrentState)
		}
	}
}

func TestPalettePlaybackDetailsRefreshWithConfiguration(t *testing.T) {
	m := New(context.Background())
	for _, tc := range []struct {
		short, long float64
		volume      int
		want        map[string]string
	}{
		{5, 30, 5, map[string]string{
			"playback.seek.back":         "Backward 5 seconds",
			"playback.seek.forward":      "Forward 5 seconds",
			"playback.seek.back.long":    "Backward 30 seconds",
			"playback.seek.forward.long": "Forward 30 seconds",
			"playback.volume.up":         "Raise volume by 5 percentage points",
			"playback.volume.down":       "Lower volume by 5 percentage points",
		}},
		{1, 5, 1, map[string]string{
			"playback.seek.forward": "Forward 1 second",
			"playback.volume.up":    "Raise volume by 1 percentage point",
		}},
		{2.5, 45.5, 10, map[string]string{
			"playback.seek.back":         "Backward 2.5 seconds",
			"playback.seek.forward":      "Forward 2.5 seconds",
			"playback.seek.back.long":    "Backward 45.5 seconds",
			"playback.seek.forward.long": "Forward 45.5 seconds",
			"playback.volume.up":         "Raise volume by 10 percentage points",
			"playback.volume.down":       "Lower volume by 10 percentage points",
		}},
	} {
		m.cfg.SeekOffsetShort, m.cfg.SeekOffsetLong = tc.short, tc.long
		m.cfg.VolumeIncrement = tc.volume
		for _, cmd := range m.BuildPaletteCommands() {
			if want, ok := tc.want[cmd.ID]; ok {
				if cmd.Detail != want {
					t.Errorf("%s detail = %q, want %q", cmd.ID, cmd.Detail, want)
				}
				if cmd.ID == "playback.seek.forward" && (cmd.IsEnabled() || cmd.DisabledReason != "Load a track first") {
					t.Fatal("seek availability changed for empty player")
				}
				delete(tc.want, cmd.ID)
			}
		}
		if len(tc.want) != 0 {
			t.Fatalf("missing commands: %v", tc.want)
		}
	}
}
