package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// Notifications use the same readable names as the playback bar.
func TestModeToggleNotificationsUseReadableNames(t *testing.T) {
	t.Run("repeat cycles through all three states", func(t *testing.T) {
		m := New(context.Background())
		want := []string{"Repeat: all", "Repeat: one", "Repeat: off"}
		for i, w := range want {
			next, _, handled := m.handlePlaybackKeyAction(keys.ActionRepeatCycle)
			if !handled {
				t.Fatalf("press %d: actionRepeatCycle not handled", i+1)
			}
			m = next.(Model)
			if got := m.statusBar.TopNotification().Msg; got != w {
				t.Errorf("press %d: notification = %q, want %q", i+1, got, w)
			}
		}
	})

	t.Run("shuffle", func(t *testing.T) {
		m := New(context.Background(), WithPlayer(&stubRepeatShuffler{}), WithSessionWriter(&stubSessionWriter{}))
		for i, w := range []string{"Shuffle: on", "Shuffle: off"} {
			next, _, handled := m.handlePlaybackKeyAction(keys.ActionShuffleToggle)
			if !handled {
				t.Fatalf("press %d: actionShuffleToggle not handled", i+1)
			}
			m = next.(Model)
			if got := m.statusBar.TopNotification().Msg; got != w {
				t.Errorf("press %d: notification = %q, want %q", i+1, got, w)
			}
		}
	})

	t.Run("stop after current", func(t *testing.T) {
		m := New(context.Background(), WithPlayer(&stubStopAfterPlayer{}))
		m.playback.state = playback.StatePlaying
		for i, w := range []string{"Stop after track: on", "Stop after track: off"} {
			next, _, handled := m.handlePlaybackKeyAction(keys.ActionStopAfterCurrent)
			if !handled {
				t.Fatalf("press %d: actionStopAfterCurrent not handled", i+1)
			}
			m = next.(Model)
			if got := m.statusBar.TopNotification().Msg; got != w {
				t.Errorf("press %d: notification = %q, want %q", i+1, got, w)
			}
		}
	})

	t.Run("radio", func(t *testing.T) {
		m := New(context.Background())
		for i, w := range []string{"Auto-DJ: on", "Auto-DJ: off"} {
			next, _, handled := m.handlePlaybackKeyAction(keys.ActionRadioToggle)
			if !handled {
				t.Fatalf("press %d: actionRadioToggle not handled", i+1)
			}
			m = next.(Model)
			if got := m.statusBar.TopNotification().Msg; got != w {
				t.Errorf("press %d: notification = %q, want %q", i+1, got, w)
			}
		}
	})
}

// Shared definitions keep notifications aligned with the playback bar.
func TestModeToggleMsgMatchesModeNames(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = playback.RepeatOne
	m.playback.shuffle = true
	m.playback.stopAfterCurrent = true
	m.radio.enabled = true

	indicators := playback.ModeIndicators(m.playback.repeatMode, m.playback.shuffle, m.playback.stopAfterCurrent, m.radio.enabled)
	for i, ind := range indicators {
		want := ind.Name + ": " + ind.State
		if got := m.modeToggleMsg(i); got != want {
			t.Errorf("modeToggleMsg(%d) = %q, want %q", i, got, want)
		}
	}
}
