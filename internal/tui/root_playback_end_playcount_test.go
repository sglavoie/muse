package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestPlaybackEndedReturnsIncrementCmd verifies that handlePlaybackEndedMsg
// returns a tea.Cmd for persisting the listen instead of using a bare
// goroutine. This ensures the DB write is managed by the Bubble Tea runtime.
func TestPlaybackEndedReturnsIncrementCmd(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := New(context.Background(), WithHistoryRecorder(hr))
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "h1"
	m = m.armPendingListen("h1", "A")
	m.playback.elapsedSecs = 60
	m.playback.totalSecs = 120
	m.playback.listenedSecs = 60
	m.playback.peakTotalSecs = 120

	next, cmd := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Fatalf("expected PlaybackIdle, got %v", nm.playback.state)
	}
	if cmd == nil {
		t.Fatal("handlePlaybackEndedMsg must return a non-nil cmd for listen persistence")
	}
	// Execute the cmd to trigger RecordListen.
	result := cmd()
	if len(hr.calls) != 1 {
		t.Fatalf("expected 1 RecordListen call, got %d", len(hr.calls))
	}
	if hr.calls[0].Hash != "h1" {
		t.Errorf("RecordListen hash = %q, want %q", hr.calls[0].Hash, "h1")
	}
	if !hr.calls[0].Counted {
		t.Error("expected Counted=true for a 60s listen of a 120s track")
	}
	prm, ok := result.(playback.PlayRecordedMsg)
	if !ok {
		t.Fatalf("expected playRecordedMsg, got %T", result)
	}
	if prm.Hash != "h1" {
		t.Errorf("playRecordedMsg.Hash = %q, want %q", prm.Hash, "h1")
	}
}

// TestPlaybackEndedRecordsUncountedBelowThreshold verifies that a listen above
// the capture floor but below the play-count threshold is still persisted, just
// flagged uncounted so it feeds skip analysis without inflating play counts.
func TestPlaybackEndedRecordsUncountedBelowThreshold(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := New(context.Background(), WithHistoryRecorder(hr))
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "h1"
	m = m.armPendingListen("h1", "A")
	// 5 out of 300 = 1.7%, well below 50% and < 30s.
	m.playback.elapsedSecs = 5
	m.playback.totalSecs = 300
	m.playback.listenedSecs = 5
	m.playback.peakTotalSecs = 300

	next, cmd := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Fatalf("expected PlaybackIdle, got %v", nm.playback.state)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil cmd: the listen is above the capture floor")
	}
	cmd()
	if len(hr.calls) != 1 {
		t.Fatalf("expected 1 RecordListen call, got %d", len(hr.calls))
	}
	if hr.calls[0].Counted {
		t.Error("expected Counted=false for a 5s listen of a 300s track")
	}
}

// TestPlaybackEndedUsesAccumulatedListenTime verifies that the count decision
// reads the accumulated listen time rather than the live elapsed position,
// which a gapless transition can clobber to near zero before the end message
// arrives.
func TestPlaybackEndedUsesAccumulatedListenTime(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := New(context.Background(), WithHistoryRecorder(hr))
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "h1"
	m = m.armPendingListen("h1", "A")
	// Simulate race: current values clobbered, accumulated total intact.
	m.playback.elapsedSecs = 0.2
	m.playback.totalSecs = 300.0
	m.playback.listenedSecs = 60.0
	m.playback.peakTotalSecs = 120.0

	_, cmd := m.Update(playback.PlaybackEndedMsg{})

	if cmd == nil {
		t.Fatal("expected non-nil cmd for listen persistence")
	}
	result := cmd()
	if len(hr.calls) != 1 || hr.calls[0].Hash != "h1" {
		t.Fatalf("expected RecordListen for h1, got %+v", hr.calls)
	}
	if !hr.calls[0].Counted {
		t.Error("expected Counted=true for 60s of a 120s track")
	}
	if _, ok := result.(playback.PlayRecordedMsg); !ok {
		t.Fatalf("expected playRecordedMsg, got %T", result)
	}
}
