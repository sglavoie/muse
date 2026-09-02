package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestPreviewKeyActivatesPreview verifies that pressing P when a track is
// focused in the library returns a non-nil command to start the preview.
func TestPreviewKeyActivatesPreview(t *testing.T) {
	m := setupLibraryWithTrack(t)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	if cmd == nil {
		t.Fatal("expected non-nil command from P key with focused track")
	}
}

// TestPreviewKeyNoTrackShowsNotification verifies that pressing P with no
// focused track shows a notification.
func TestPreviewKeyNoTrackShowsNotification(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary
	// library.index is empty by default

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	nm := next.(Model)
	if cmd != nil {
		t.Fatal("expected nil command when no track to preview")
	}
	if nm.statusBar.topNotification().msg != "No track to preview" {
		t.Fatalf("expected 'No track to preview' notification, got %q", nm.statusBar.topNotification().msg)
	}
}

// TestPreviewStartedMsgSetsOverlay verifies that receiving a
// previewStartedMsg sets the preview overlay and player.
func TestPreviewStartedMsgSetsOverlay(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{}

	next, _ := m.Update(previewStartedMsg{
		player: sp,
		overlay: overlay.PreviewOverlay{
			Artist: "Test",
			Title:  "Song",
		},
	})
	nm := next.(Model)

	if nm.overlay.preview == nil {
		t.Fatal("expected preview overlay to be set")
	}
	if nm.overlay.preview.Artist != "Test" || nm.overlay.preview.Title != "Song" {
		t.Fatalf("unexpected overlay content: %+v", *nm.overlay.preview)
	}
	if nm.overlay.previewPlayer != sp {
		t.Fatal("expected previewPlayer to be set")
	}
}

// TestPreviewStartErrorMsgShowsNotification verifies that a
// previewStartErrorMsg shows an error notification.
func TestPreviewStartErrorMsgShowsNotification(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(previewStartErrorMsg{err: errTest("boom")})
	nm := next.(Model)

	if nm.statusBar.topNotification().msg != "Preview error: boom" {
		t.Fatalf("expected error notification, got %q", nm.statusBar.topNotification().msg)
	}
}

// TestPreviewStartErrorResumesPlayback verifies that a previewStartErrorMsg
// resumes main playback if it was paused for the preview.
func TestPreviewStartErrorResumesPlayback(t *testing.T) {
	sp := &stubPlayer{}
	m := New(context.Background())
	m.player = sp
	m.overlay.previewWasPlaying = true

	next, _ := m.Update(previewStartErrorMsg{err: errTest("fail")})
	nm := next.(Model)

	if sp.playCalls != 1 {
		t.Fatalf("expected 1 Play call to resume, got %d", sp.playCalls)
	}
	if nm.overlay.previewWasPlaying {
		t.Fatal("expected previewWasPlaying reset")
	}
}

// TestPreviewStartErrorPlayResumeFailureKeepsPaused verifies that when
// preview start fails and the resume Play() call also fails, the playback
// state stays PlaybackPaused (not incorrectly set to PlaybackPlaying).
func TestPreviewStartErrorPlayResumeFailureKeepsPaused(t *testing.T) {
	fp := &stubFailingPlayer{playErr: errTest("resume failed")}
	m := New(context.Background())
	m.player = fp
	m.playback.state = playback.StatePaused
	m.overlay.previewWasPlaying = true

	next, _ := m.Update(previewStartErrorMsg{err: errTest("preview fail")})
	nm := next.(Model)

	if nm.playback.state != playback.StatePaused {
		t.Fatalf("expected PlaybackPaused when resume fails, got %v", nm.playback.state)
	}
	if nm.overlay.previewWasPlaying {
		t.Fatal("expected previewWasPlaying reset even on resume failure")
	}
}

// TestPreviewPausesMainPlayback verifies that pressing P while main
// player is playing pauses it.
func TestPreviewPausesMainPlayback(t *testing.T) {
	sp := &stubPlayer{}
	m := setupLibraryWithTrack(t)
	m.player = sp
	m.playback.state = playback.StatePlaying

	next, _ := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	nm := next.(Model)

	if sp.pauseCalls != 1 {
		t.Fatalf("expected 1 Pause call, got %d", sp.pauseCalls)
	}
	if !nm.overlay.previewWasPlaying {
		t.Fatal("expected previewWasPlaying to be set")
	}
	if nm.playback.state != playback.StatePaused {
		t.Fatal("expected playback state to be paused")
	}
}
