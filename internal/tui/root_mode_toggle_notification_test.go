package tui

import (
	"context"
	"testing"
)

// TestModeToggleNotificationsNameTheGlyph verifies that each mode toggle tells
// the user which status-bar letter it just changed. The glyph is read from
// modeIndicators, so these assertions also guard against the notification text
// drifting away from the cluster — including the repeat glyph, which is "1"
// only for repeat-one and "R" for both all and off.
func TestModeToggleNotificationsNameTheGlyph(t *testing.T) {
	t.Run("repeat cycles through all three states", func(t *testing.T) {
		m := New(context.Background())
		want := []string{"Repeat: all (R)", "Repeat: one (1)", "Repeat: off (R)"}
		for i, w := range want {
			next, _, handled := m.handlePlaybackKeyAction(actionRepeatCycle)
			if !handled {
				t.Fatalf("press %d: actionRepeatCycle not handled", i+1)
			}
			m = next.(Model)
			if got := m.statusBar.topNotification().msg; got != w {
				t.Errorf("press %d: notification = %q, want %q", i+1, got, w)
			}
		}
	})

	t.Run("shuffle", func(t *testing.T) {
		m := New(context.Background(), WithPlayer(&stubRepeatShuffler{}), WithSessionWriter(&stubSessionWriter{}))
		for i, w := range []string{"Shuffle: on (S)", "Shuffle: off (S)"} {
			next, _, handled := m.handlePlaybackKeyAction(actionShuffleToggle)
			if !handled {
				t.Fatalf("press %d: actionShuffleToggle not handled", i+1)
			}
			m = next.(Model)
			if got := m.statusBar.topNotification().msg; got != w {
				t.Errorf("press %d: notification = %q, want %q", i+1, got, w)
			}
		}
	})

	t.Run("stop after current", func(t *testing.T) {
		m := New(context.Background(), WithPlayer(&stubStopAfterPlayer{}))
		m.playback.state = PlaybackPlaying
		for i, w := range []string{"Stop after current: on (N)", "Stop after current: off (N)"} {
			next, _, handled := m.handlePlaybackKeyAction(actionStopAfterCurrent)
			if !handled {
				t.Fatalf("press %d: actionStopAfterCurrent not handled", i+1)
			}
			m = next.(Model)
			if got := m.statusBar.topNotification().msg; got != w {
				t.Errorf("press %d: notification = %q, want %q", i+1, got, w)
			}
		}
	})

	t.Run("radio", func(t *testing.T) {
		m := New(context.Background())
		for i, w := range []string{"Radio mode: on (D)", "Radio mode: off (D)"} {
			next, _, handled := m.handlePlaybackKeyAction(actionRadioToggle)
			if !handled {
				t.Fatalf("press %d: actionRadioToggle not handled", i+1)
			}
			m = next.(Model)
			if got := m.statusBar.topNotification().msg; got != w {
				t.Errorf("press %d: notification = %q, want %q", i+1, got, w)
			}
		}
	})
}

// TestModeToggleMsgMatchesClusterGlyph pins the notification text to the very
// glyph renderModeCluster would draw for the same state, so a future change to
// a glyph cannot silently leave the notifications teaching the old letter.
func TestModeToggleMsgMatchesClusterGlyph(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = RepeatOne
	m.playback.shuffle = true
	m.playback.stopAfterCurrent = true
	m.radio.enabled = true

	indicators := modeIndicators(m.playback.repeatMode, m.playback.shuffle, m.playback.stopAfterCurrent, m.radio.enabled)
	labels := map[int]string{
		modeIndexRepeat:    "Repeat",
		modeIndexShuffle:   "Shuffle",
		modeIndexStopAfter: "Stop after current",
		modeIndexRadio:     "Radio mode",
	}
	for i, ind := range indicators {
		want := labels[i] + ": " + ind.state + " (" + ind.glyph + ")"
		if got := m.modeToggleMsg(labels[i], i); got != want {
			t.Errorf("modeToggleMsg(%q, %d) = %q, want %q", labels[i], i, got, want)
		}
	}
}
