package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestTickMsgUpdatesElapsedFields(t *testing.T) {
	p := &stubElapsedReader{elapsed: 75.5, total: 240.0}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying

	nm, _ := updateModel(m, tickMsg{})

	if nm.playback.elapsedSecs != 75.5 {
		t.Fatalf("elapsedSecs: expected 75.5, got %v", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 240.0 {
		t.Fatalf("totalSecs: expected 240.0, got %v", nm.playback.totalSecs)
	}
}

func TestTickMsgReschedulesCmd(t *testing.T) {
	m := New(context.Background())
	_, cmd := updateModel(m, tickMsg{})
	if cmd == nil {
		t.Fatal("tickMsg must return a follow-up cmd to reschedule the tick")
	}
}

func TestInitSchedulesTick(t *testing.T) {
	m := New(context.Background())
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init must return a non-nil cmd (at least the 1-second tick)")
	}
}

func TestPlaybackEndedMsgResetsElapsed(t *testing.T) {
	p := &stubElapsedReader{elapsed: 60.0, total: 180.0}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying

	nm, _ := updateModel(m, tickMsg{})
	if nm.playback.elapsedSecs == 0 {
		t.Fatal("expected non-zero elapsedSecs after tickMsg")
	}

	nm, _ = updateModel(nm, playback.PlaybackEndedMsg{})
	if nm.playback.elapsedSecs != 0 {
		t.Fatalf("elapsedSecs must be 0 after PlaybackEndedMsg, got %v", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Fatalf("totalSecs must be 0 after PlaybackEndedMsg, got %v", nm.playback.totalSecs)
	}
}
