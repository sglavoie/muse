package tui

import (
	"context"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/keys"
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
	m.ActiveView = viewLibrary
	// library.index is empty by default

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	nm := next.(Model)
	if cmd != nil {
		t.Fatal("expected nil command when no track to preview")
	}
	if nm.statusBar.TopNotification().Msg != "No track to preview" {
		t.Fatalf("expected 'No track to preview' notification, got %q", nm.statusBar.TopNotification().Msg)
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

	if nm.statusBar.TopNotification().Msg != "Preview error: boom" {
		t.Fatalf("expected error notification, got %q", nm.statusBar.TopNotification().Msg)
	}
}

// TestPreviewStartErrorResumesPlayback verifies that a previewStartErrorMsg
// resumes main playback if it was paused for the preview.
func TestPreviewStartErrorResumesPlayback(t *testing.T) {
	sp := &stubPlayer{}
	m := New(context.Background())
	m.player = sp
	m.overlay.previewWasPlaying = true

	next, cmd := m.Update(previewStartErrorMsg{err: errTest("fail")})
	nm := next.(Model)
	if cmd == nil {
		t.Fatal("expected ordered resume command")
	}
	if sp.playCalls != 0 {
		t.Fatal("resume blocked the Update loop")
	}
	next, _ = nm.Update(playbackControlResult(t, cmd()))
	nm = next.(Model)

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

	next, cmd := m.Update(previewStartErrorMsg{err: errTest("preview fail")})
	nm := next.(Model)
	if cmd == nil {
		t.Fatal("expected ordered resume command")
	}
	next, _ = nm.Update(playbackControlResult(t, cmd()))
	nm = next.(Model)

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

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	nm := next.(Model)

	if cmd == nil {
		t.Fatal("expected ordered pause command")
	}
	if sp.pauseCalls != 0 {
		t.Fatal("pause blocked the Update loop")
	}
	pauseMsg := cmd()
	pauseResult, ok := pauseMsg.(previewPauseResultMsg)
	if !ok {
		t.Fatalf("expected previewPauseResultMsg, got %T", pauseMsg)
	}
	if sp.pauseCalls != 1 {
		t.Fatalf("expected 1 Pause call, got %d", sp.pauseCalls)
	}
	if !nm.overlay.previewWasPlaying {
		t.Fatal("expected previewWasPlaying to be set")
	}
	if nm.playback.state != playback.StatePaused {
		t.Fatal("expected playback state to be paused")
	}
	_, startCmd := nm.Update(pauseResult)
	if startCmd == nil {
		t.Fatal("expected preview start only after the pause result")
	}
}

// TestPreviewOrdersAfterPendingSelection covers FR-015's dispatch windows.
// Preview pause and both resume paths are ordering barriers: none may pass a
// selection that the Update loop dispatched first, and a resume must also wait
// for an intervening transport control.
func TestPreviewOrdersAfterPendingSelection(t *testing.T) {
	t.Run("activation pauses after selection before starting preview", func(t *testing.T) {
		m, p := stoppableReplayModel([]int{3, 1, 4, 0, 2}, 2)
		m, selection := m.handlePlayQueueIndexMsg(playback.PlayQueueIndexMsg{Index: 2})
		m.ActiveView = viewQueue
		m.queue.Cursor = 0

		next, pauseCmd, handled := m.handlePlaybackKeyAction(keys.ActionPreview)
		if !handled || pauseCmd == nil {
			t.Fatal("preview activation did not queue its pause")
		}
		m = next.(Model)
		pauseReq := m.playback.requestSeq
		pauseDone := make(chan tea.Msg, 1)
		go func() { pauseDone <- pauseCmd() }()
		waitForPlayRequestWait(t, m.playback.requests, pauseReq)
		if len(p.calls) != 0 {
			t.Fatalf("player calls before selection release = %v", p.calls)
		}

		selectionResult := runPlayCmd(t, selection)
		pauseResult, ok := (<-pauseDone).(previewPauseResultMsg)
		if !ok {
			t.Fatal("ordered pause returned the wrong message type")
		}
		if selectionResult.Err != nil || pauseResult.err != nil || !pauseResult.ran {
			t.Fatalf("ordered calls failed: selection=%v pause=%v ran=%t",
				selectionResult.Err, pauseResult.err, pauseResult.ran)
		}
		if !slices.Equal(p.calls, []string{"PlayFromIndex", "Pause"}) {
			t.Fatalf("player calls = %v, want [PlayFromIndex Pause]", p.calls)
		}

		_, startCmd := m.Update(pauseResult)
		if startCmd == nil {
			t.Fatal("preview did not become startable after the ordered pause")
		}
	})

	t.Run("dismissal resumes after selection and transport", func(t *testing.T) {
		m, p := stoppableReplayModel([]int{0, 1, 2, 3}, 0)
		m, selection := m.handlePlayQueueIndexMsg(playback.PlayQueueIndexMsg{Index: 2})
		next, nextCmd, _ := m.handlePlaybackKeyAction(keys.ActionNext)
		m = next.(Model)
		nextReq := m.playback.requestSeq
		m.playback.state = playback.StatePaused
		m.overlay.previewWasPlaying = true
		m.overlay.preview = &overlay.PreviewOverlay{Artist: "A", Title: "T"}
		m.overlay.previewPlayer = &stubPreviewer{}

		m, resumeCmd, handled := m.handlePreviewKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
		if !handled || resumeCmd == nil {
			t.Fatal("preview dismissal did not queue its resume")
		}
		resumeReq := m.playback.requestSeq
		nextDone := make(chan tea.Msg, 1)
		resumeDone := make(chan tea.Msg, 1)
		go func() { nextDone <- nextCmd() }()
		go func() { resumeDone <- resumeCmd() }()
		waitForPlayRequestWait(t, m.playback.requests, nextReq)
		waitForPlayRequestWait(t, m.playback.requests, resumeReq)

		selectionResult := runPlayCmd(t, selection)
		nextResult := playbackControlResult(t, <-nextDone)
		resumeResult := playbackControlResult(t, <-resumeDone)
		if selectionResult.Err != nil || nextResult.err != nil || resumeResult.err != nil {
			t.Fatalf("ordered calls failed: selection=%v next=%v resume=%v",
				selectionResult.Err, nextResult.err, resumeResult.err)
		}
		if !slices.Equal(p.calls, []string{"PlayFromIndex", "Next", "Play"}) {
			t.Fatalf("player calls = %v, want [PlayFromIndex Next Play]", p.calls)
		}
	})

	t.Run("start error resumes after selection", func(t *testing.T) {
		m, p := stoppableReplayModel([]int{0, 1, 2, 3}, 0)
		m, selection := m.handlePlayQueueIndexMsg(playback.PlayQueueIndexMsg{Index: 2})
		m.playback.state = playback.StatePaused
		m.overlay.previewWasPlaying = true

		next, resumeCmd := m.Update(previewStartErrorMsg{err: errTest("preview failed")})
		m = next.(Model)
		if resumeCmd == nil {
			t.Fatal("preview start error did not queue its resume")
		}
		resumeReq := m.playback.requestSeq
		resumeDone := make(chan tea.Msg, 1)
		go func() { resumeDone <- resumeCmd() }()
		waitForPlayRequestWait(t, m.playback.requests, resumeReq)

		selectionResult := runPlayCmd(t, selection)
		resumeResult := playbackControlResult(t, <-resumeDone)
		if selectionResult.Err != nil || resumeResult.err != nil {
			t.Fatalf("ordered calls failed: selection=%v resume=%v",
				selectionResult.Err, resumeResult.err)
		}
		if !slices.Equal(p.calls, []string{"PlayFromIndex", "Play"}) {
			t.Fatalf("player calls = %v, want [PlayFromIndex Play]", p.calls)
		}
	})
}
