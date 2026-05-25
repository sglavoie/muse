package tui

import (
	"context"
	"testing"
	"time"
)

func TestPlaybackEndedMsgShowsQueueFinishedWhenEmptyRepeatOff(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = RepeatOff

	nm, _ := updateModel(m, PlaybackEndedMsg{})

	if nm.statusBar.topNotification().msg != "Queue finished" {
		t.Fatalf("expected 'Queue finished' notification, got %q", nm.statusBar.topNotification().msg)
	}
	if nm.statusBar.topNotification().expiresAt.IsZero() {
		t.Fatal("Queue finished notification must have a non-zero expiry")
	}
	if nm.statusBar.topNotification().expiresAt.Before(time.Now().Add(4 * time.Second)) {
		t.Fatal("Queue finished notification expiry must be ~5 seconds from now")
	}
}

func TestPlaybackEndedMsgShowsNotificationWhenRepeatAllFallsThrough(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = RepeatAll

	nm, _ := updateModel(m, PlaybackEndedMsg{})
	if nm.statusBar.topNotification().msg != "Queue finished" {
		t.Fatalf("expected 'Queue finished' when repeat-all falls through, got %q", nm.statusBar.topNotification().msg)
	}
}

func TestPlaybackEndedMsgNoSoundWhenRepeatNotOff(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = RepeatAll

	nm, _ := updateModel(m, PlaybackEndedMsg{})
	if nm.statusBar.topNotification().msg != "Queue finished" {
		t.Fatalf("expected 'Queue finished', got %q", nm.statusBar.topNotification().msg)
	}
}

func TestPlaybackEndedMsgShowsNotificationWithEntriesPresent(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = RepeatOff
	m.queue.entries = []QueueEntry{{ID: 1, Display: "Track 1"}}

	nm, _ := updateModel(m, PlaybackEndedMsg{})
	if nm.statusBar.topNotification().msg != "Queue finished" {
		t.Fatalf("expected %q, got %q", "Queue finished", nm.statusBar.topNotification().msg)
	}
}
