package tui

import (
	"context"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestPlaybackEndedMsgShowsQueueFinishedWhenEmptyRepeatOff(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = playback.RepeatOff

	nm, _ := updateModel(m, playback.PlaybackEndedMsg{})

	if nm.statusBar.TopNotification().Msg != "Queue finished" {
		t.Fatalf("expected 'Queue finished' notification, got %q", nm.statusBar.TopNotification().Msg)
	}
	if nm.statusBar.TopNotification().ExpiresAt.IsZero() {
		t.Fatal("Queue finished notification must have a non-zero expiry")
	}
	if nm.statusBar.TopNotification().ExpiresAt.Before(time.Now().Add(4 * time.Second)) {
		t.Fatal("Queue finished notification expiry must be ~5 seconds from now")
	}
}

func TestPlaybackEndedMsgShowsNotificationWhenRepeatAllFallsThrough(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = playback.RepeatAll

	nm, _ := updateModel(m, playback.PlaybackEndedMsg{})
	if nm.statusBar.TopNotification().Msg != "Queue finished" {
		t.Fatalf("expected 'Queue finished' when repeat-all falls through, got %q", nm.statusBar.TopNotification().Msg)
	}
}

func TestPlaybackEndedMsgNoSoundWhenRepeatNotOff(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = playback.RepeatAll

	nm, _ := updateModel(m, playback.PlaybackEndedMsg{})
	if nm.statusBar.TopNotification().Msg != "Queue finished" {
		t.Fatalf("expected 'Queue finished', got %q", nm.statusBar.TopNotification().Msg)
	}
}

func TestPlaybackEndedMsgShowsNotificationWithEntriesPresent(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = playback.RepeatOff
	m.queue.entries = []QueueEntry{{ID: 1, Display: "Track 1"}}

	nm, _ := updateModel(m, playback.PlaybackEndedMsg{})
	if nm.statusBar.TopNotification().Msg != "Queue finished" {
		t.Fatalf("expected %q, got %q", "Queue finished", nm.statusBar.TopNotification().Msg)
	}
}
