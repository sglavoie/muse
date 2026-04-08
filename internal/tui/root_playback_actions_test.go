package tui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestSpaceTogglesPlayPauseFromAnyView verifies that pressing space triggers
// a play or pause call on the player regardless of which view is focused.
func TestSpaceTogglesPlayPauseFromAnyView(t *testing.T) {
	views := []struct {
		name   string
		viewID viewID
	}{
		{"Library", viewLibrary},
		{"Playlists", viewPlaylists},
		{"Queue", viewQueue},
	}

	spaceMsg := tea.KeyPressMsg{Code: tea.KeySpace}

	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			p := &stubPlayer{}
			m := New(context.Background(), WithPlayer(p))
			m.activeView = tc.viewID
			m.queue.entries = []QueueEntry{{ID: 1, Display: "Track 1"}}

			// First press: not playing -> Play() called.
			next, _ := m.Update(spaceMsg)
			nm := next.(Model)
			if p.playCalls != 1 || p.pauseCalls != 0 {
				t.Fatalf("first space (%s): expected 1 Play call, got play=%d pause=%d", tc.name, p.playCalls, p.pauseCalls)
			}

			// Second press: playing -> Pause() called.
			_, _ = nm.Update(spaceMsg)
			if p.playCalls != 1 || p.pauseCalls != 1 {
				t.Fatalf("second space (%s): expected 1 Pause call, got play=%d pause=%d", tc.name, p.playCalls, p.pauseCalls)
			}
		})
	}
}

// TestModelPlaybackStateTransitions verifies that the playbackState field on
// root Model transitions correctly through playing, paused, and idle states.
func TestModelPlaybackStateTransitions(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{Path: "/a.flac", Display: "Artist – Song", Artist: "Artist", Title: "Song"},
	}

	if m.playback.state != PlaybackIdle {
		t.Fatalf("initial: expected PlaybackIdle, got %v", m.playback.state)
	}

	// PlayQueueIndexMsg -> playing.
	next, _ := m.Update(PlayQueueIndexMsg{Index: 0})
	nm := next.(Model)
	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("after PlayQueueIndexMsg: expected PlaybackPlaying, got %v", nm.playback.state)
	}

	// Space while playing -> paused.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm = next.(Model)
	if nm.playback.state != PlaybackPaused {
		t.Fatalf("space (playing->paused): expected PlaybackPaused, got %v", nm.playback.state)
	}

	// Space while paused -> playing.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm = next.(Model)
	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("space (paused->playing): expected PlaybackPlaying, got %v", nm.playback.state)
	}

	// PlaybackEndedMsg -> idle.
	next, _ = nm.Update(PlaybackEndedMsg{})
	nm = next.(Model)
	if nm.playback.state != PlaybackIdle {
		t.Fatalf("after PlaybackEndedMsg: expected PlaybackIdle, got %v", nm.playback.state)
	}
}

// TestModelStopKeyTransitionsToStopped verifies that the s key transitions
// the playback state to stopped when the player implements commandPlayer.
func TestModelStopKeyTransitionsToStopped(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying

	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	nm := next.(Model)
	if nm.playback.state != PlaybackStopped {
		t.Fatalf("s key: expected PlaybackStopped, got %v", nm.playback.state)
	}
	if p.stopCalls != 1 {
		t.Fatalf("s key: expected 1 Stop() call, got %d", p.stopCalls)
	}
}

// TestDismissPreviewDoesNotResumeOnPlayError verifies that when the player's
// Play() returns an error after preview dismissal, the playback state stays
// paused instead of incorrectly transitioning to PlaybackPlaying.
func TestDismissPreviewDoesNotResumeOnPlayError(t *testing.T) {
	p := &stubFailingPlayer{playErr: errors.New("mpv disconnected")}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPaused
	m.overlay.previewWasPlaying = true
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}

	nm := m.dismissPreview()

	if nm.playback.state != PlaybackPaused {
		t.Fatalf("expected PlaybackPaused when resume fails, got %v", nm.playback.state)
	}
	if nm.overlay.preview != nil {
		t.Fatal("preview overlay should be cleared after dismiss")
	}
}

// TestVolumeUpNoUpdateOnError verifies that the TUI volume and session are not
// updated when the mpv SetVolumeRelative IPC call fails, preventing drift
// between the displayed volume and mpv's actual volume.
func TestVolumeUpNoUpdateOnError(t *testing.T) {
	p := &stubFailingVolumePlayer{volumeErr: errors.New("mpv disconnected")}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))
	m.playback.volume = 50

	next, _, _ := m.handlePlaybackKeyAction(actionVolumeUp)
	nm := next.(Model)

	if nm.playback.volume != 50 {
		t.Fatalf("volume should stay at 50 on error, got %d", nm.playback.volume)
	}
	if len(sw.volumeCalls) != 0 {
		t.Fatalf("session volume should not be written on error, got %d calls", len(sw.volumeCalls))
	}
}

// TestVolumeDownNoUpdateOnError verifies that a failed SetVolumeRelative for
// volume-down does not update the TUI volume or persist to the session.
func TestVolumeDownNoUpdateOnError(t *testing.T) {
	p := &stubFailingVolumePlayer{volumeErr: errors.New("mpv disconnected")}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))
	m.playback.volume = 50

	next, _, _ := m.handlePlaybackKeyAction(actionVolumeDown)
	nm := next.(Model)

	if nm.playback.volume != 50 {
		t.Fatalf("volume should stay at 50 on error, got %d", nm.playback.volume)
	}
	if len(sw.volumeCalls) != 0 {
		t.Fatalf("session volume should not be written on error, got %d calls", len(sw.volumeCalls))
	}
}
