package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestPasteResumesPlaybackWhenQueueExhausted verifies that pasting tracks after
// the queue was exhausted (PlaybackEndedMsg already processed) auto-resumes
// playback from the first pasted entry and populates all playback bookkeeping
// (currentHash, currentIndex, status bar).
func TestPasteResumesPlaybackWhenQueueExhausted(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithPlayer(p), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()[:2] // [A, B]
	m.queue.cursor = 1                         // cursor on B (last)
	m.playback.state = playback.StateIdle
	m.playback.currentIndex = -1
	m.playback.queueExhausted = true // queue ran out

	m.yankBuffer = []QueueEntry{sampleQueueEntries()[2]} // yank C

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if m.playback.state != playback.StatePlaying {
		t.Fatalf("expected PlaybackPlaying after paste with exhausted queue, got %v", m.playback.state)
	}
	// Should call PlayFromIndex(2) - the insert position.
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 2 {
		t.Fatalf("expected PlayFromIndex(2), got calls=%v", p.playFromIndexCalls)
	}
	if m.playback.queueExhausted {
		t.Fatal("queueExhausted should be cleared after auto-resume")
	}
	// The pasted entry's metadata must be reflected in playback state and
	// status bar so the UI is correct immediately (before TrackChangedMsg).
	pastedEntry := msg.Entries[0]
	if m.playback.currentHash != pastedEntry.Hash {
		t.Errorf("currentHash = %q, want %q", m.playback.currentHash, pastedEntry.Hash)
	}
	if m.playback.currentIndex != 2 {
		t.Errorf("currentIndex = %d, want 2", m.playback.currentIndex)
	}
	if m.statusBar.artist != pastedEntry.Artist {
		t.Errorf("statusBar.artist = %q, want %q", m.statusBar.artist, pastedEntry.Artist)
	}
	if m.statusBar.title != pastedEntry.Title {
		t.Errorf("statusBar.title = %q, want %q", m.statusBar.title, pastedEntry.Title)
	}
}

// TestPasteExhaustedResumeResetsPlaybackFields verifies that pasting after
// queue exhaustion resets stale elapsed/peak/durationCorrected fields so the
// progress bar and duration correction logic start fresh for the new track.
func TestPasteExhaustedResumeResetsPlaybackFields(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithPlayer(p), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()[:2]
	m.queue.cursor = 1
	m.playback.state = playback.StateIdle
	m.playback.currentIndex = -1
	m.playback.queueExhausted = true
	// Simulate stale fields from the previous track.
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.playback.peakElapsedSecs = 100
	m.playback.peakTotalSecs = 200
	m.playback.durationCorrected = true

	m.yankBuffer = []QueueEntry{sampleQueueEntries()[2]}

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if m.playback.state != playback.StatePlaying {
		t.Fatalf("expected PlaybackPlaying, got %v", m.playback.state)
	}
	if m.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0", m.playback.elapsedSecs)
	}
	if m.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0", m.playback.totalSecs)
	}
	if m.playback.peakElapsedSecs != 0 {
		t.Errorf("peakElapsedSecs = %v, want 0", m.playback.peakElapsedSecs)
	}
	if m.playback.peakTotalSecs != 0 {
		t.Errorf("peakTotalSecs = %v, want 0", m.playback.peakTotalSecs)
	}
	if m.playback.durationCorrected {
		t.Error("durationCorrected should be false after exhaustion resume")
	}
}

// TestPasteDoesNotResumeWhenUserStopped verifies that pasting tracks when
// playback is idle but NOT due to queue exhaustion does not auto-resume.
func TestPasteDoesNotResumeWhenUserStopped(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithPlayer(p), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()[:2]
	m.queue.cursor = 1
	m.playback.state = playback.StateIdle
	m.playback.currentIndex = -1
	m.playback.queueExhausted = false // user stopped, not exhausted

	m.yankBuffer = []QueueEntry{sampleQueueEntries()[2]}

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if m.playback.state != playback.StateIdle {
		t.Fatalf("expected PlaybackIdle (user-stopped), got %v", m.playback.state)
	}
	if len(p.playFromIndexCalls) != 0 {
		t.Fatalf("expected no PlayFromIndex calls, got %v", p.playFromIndexCalls)
	}
}
