package tui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestSpacePauseResumeFromAnyView verifies that pressing space while paused
// calls Play() and while playing calls Pause(), regardless of view.
func TestSpacePauseResumeFromAnyView(t *testing.T) {
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
			m.playback.state = playback.StatePaused
			m.queue.entries = []QueueEntry{{ID: 1, Display: "Track 1"}}

			// Space while paused -> Play() called.
			next, _ := m.Update(spaceMsg)
			nm := next.(Model)
			if p.playCalls != 1 || p.pauseCalls != 0 {
				t.Fatalf("first space (%s): expected 1 Play call, got play=%d pause=%d", tc.name, p.playCalls, p.pauseCalls)
			}

			// Space while playing -> Pause() called.
			_, _ = nm.Update(spaceMsg)
			if p.playCalls != 1 || p.pauseCalls != 1 {
				t.Fatalf("second space (%s): expected 1 Pause call, got play=%d pause=%d", tc.name, p.playCalls, p.pauseCalls)
			}
		})
	}
}

// TestSpaceFromIdleDispatchesPlayQueueIndex verifies that pressing space in
// PlaybackIdle with queue entries dispatches a PlayQueueIndexMsg instead of
// calling Play() directly, so the track is properly loaded via PlayFromIndex.
func TestSpaceFromIdleDispatchesPlayQueueIndex(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Display: "Track 1"},
		{ID: 2, Path: "/b.flac", Display: "Track 2"},
	}
	m.playback.currentIndex = 1

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	if p.playCalls != 0 {
		t.Fatalf("Play() should not be called from idle, got %d calls", p.playCalls)
	}
	if cmd == nil {
		t.Fatal("expected a Cmd dispatching PlayQueueIndexMsg, got nil")
	}
	msg := cmd()
	pqi, ok := msg.(playback.PlayQueueIndexMsg)
	if !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
	if pqi.Index != 1 {
		t.Fatalf("expected index 1, got %d", pqi.Index)
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

	if m.playback.state != playback.StateIdle {
		t.Fatalf("initial: expected PlaybackIdle, got %v", m.playback.state)
	}

	// PlayQueueIndexMsg -> playing.
	next, _ := m.Update(playback.PlayQueueIndexMsg{Index: 0})
	nm := next.(Model)
	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("after PlayQueueIndexMsg: expected PlaybackPlaying, got %v", nm.playback.state)
	}

	// Space while playing -> paused.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm = next.(Model)
	if nm.playback.state != playback.StatePaused {
		t.Fatalf("space (playing->paused): expected PlaybackPaused, got %v", nm.playback.state)
	}

	// Space while paused -> playing.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm = next.(Model)
	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("space (paused->playing): expected PlaybackPlaying, got %v", nm.playback.state)
	}

	// PlaybackEndedMsg -> idle.
	next, _ = nm.Update(playback.PlaybackEndedMsg{})
	nm = next.(Model)
	if nm.playback.state != playback.StateIdle {
		t.Fatalf("after PlaybackEndedMsg: expected PlaybackIdle, got %v", nm.playback.state)
	}
}

// TestModelStopKeyTransitionsToStopped verifies that the s key transitions
// the playback state to stopped when the player implements commandPlayer.
func TestModelStopKeyTransitionsToStopped(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying

	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	nm := next.(Model)
	if nm.playback.state != playback.StateStopped {
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
	m.playback.state = playback.StatePaused
	m.overlay.previewWasPlaying = true
	m.overlay.preview = &overlay.PreviewOverlay{Artist: "A", Title: "T"}

	nm := m.dismissPreview()

	if nm.playback.state != playback.StatePaused {
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

	next, _, _ := m.handlePlaybackKeyAction(keys.ActionVolumeUp)
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

	next, _, _ := m.handlePlaybackKeyAction(keys.ActionVolumeDown)
	nm := next.(Model)

	if nm.playback.volume != 50 {
		t.Fatalf("volume should stay at 50 on error, got %d", nm.playback.volume)
	}
	if len(sw.volumeCalls) != 0 {
		t.Fatalf("session volume should not be written on error, got %d calls", len(sw.volumeCalls))
	}
}
