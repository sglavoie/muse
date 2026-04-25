package tui

import (
	"context"
	"testing"
)

// TestPlaybackEndedReturnsIncrementCmd verifies that handlePlaybackEndedMsg
// returns a tea.Cmd for persisting the play count instead of using a bare
// goroutine. This ensures the DB write is managed by the Bubble Tea runtime.
func TestPlaybackEndedReturnsIncrementCmd(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := New(context.Background(), WithHistoryRecorder(hr))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"
	m.playback.elapsedSecs = 60
	m.playback.totalSecs = 120
	m.playback.peakElapsedSecs = 60
	m.playback.peakTotalSecs = 120

	next, cmd := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Fatalf("expected PlaybackIdle, got %v", nm.playback.state)
	}
	if cmd == nil {
		t.Fatal("handlePlaybackEndedMsg must return a non-nil cmd for play count persistence")
	}
	// Execute the cmd to trigger RecordPlay.
	cmd()
	if len(hr.calls) != 1 {
		t.Fatalf("expected 1 RecordPlay call, got %d", len(hr.calls))
	}
	if hr.calls[0].hash != "h1" {
		t.Errorf("RecordPlay hash = %q, want %q", hr.calls[0].hash, "h1")
	}
}

// TestPlaybackEndedNoCmdWhenBelowThreshold verifies that no increment command
// is returned when the listen duration is below the threshold.
func TestPlaybackEndedNoCmdWhenBelowThreshold(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := New(context.Background(), WithHistoryRecorder(hr))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"
	m.playback.elapsedSecs = 5
	m.playback.totalSecs = 300 // 5 out of 300 = 1.7%, well below 50% and < 30s
	m.playback.peakElapsedSecs = 5
	m.playback.peakTotalSecs = 300

	next, cmd := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Fatalf("expected PlaybackIdle, got %v", nm.playback.state)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when listen duration is below threshold")
	}
}

// TestPlaybackEndedUsesPeakForPlayCount verifies that handlePlaybackEndedMsg
// uses the peak elapsed values for the play count check.
func TestPlaybackEndedUsesPeakForPlayCount(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := New(context.Background(), WithHistoryRecorder(hr))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"
	// Simulate race: current values clobbered, peak intact.
	m.playback.elapsedSecs = 0.2
	m.playback.totalSecs = 300.0
	m.playback.peakElapsedSecs = 60.0
	m.playback.peakTotalSecs = 120.0

	_, cmd := m.Update(PlaybackEndedMsg{})

	if cmd == nil {
		t.Fatal("expected non-nil cmd for play count increment")
	}
	cmd()
	if len(hr.calls) != 1 || hr.calls[0].hash != "h1" {
		t.Fatalf("expected RecordPlay for h1, got %+v", hr.calls)
	}
}
