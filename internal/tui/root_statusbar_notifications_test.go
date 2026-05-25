package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestPlayPauseOnEmptyQueueStaysIdle(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))

	nm, _ := updateModel(m, tea.KeyPressMsg{Code: ' ', Text: " "})

	if nm.playback.state != PlaybackIdle {
		t.Fatalf("playback state must remain idle, got %v", nm.playback.state)
	}
	if nm.statusBar.topNotification().msg != "Queue is empty" {
		t.Fatalf("expected 'Queue is empty' notification, got %q", nm.statusBar.topNotification().msg)
	}
}

func TestPlayPauseWithTracksInQueuePlaysNormally(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.entries = []QueueEntry{{ID: 1, Display: "Track 1"}}
	m.playback.state = PlaybackIdle

	nm, cmd := updateModel(m, tea.KeyPressMsg{Code: ' ', Text: " "})

	if nm.statusBar.topNotification().msg == "Queue is empty — enqueue tracks from Library (key 1)" {
		t.Fatal("empty-queue hint must not appear when queue has entries")
	}
	if cmd == nil {
		t.Fatal("expected a Cmd dispatching PlayQueueIndexMsg, got nil")
	}
	msg := cmd()
	if _, ok := msg.(PlayQueueIndexMsg); !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
}

func TestTransientNotificationClearsAfterTickPastExpiry(t *testing.T) {
	m := New(context.Background())
	m.statusBar.notifications = []transientNotification{{msg: "some transient message", expiresAt: time.Now().Add(-1 * time.Second)}}

	nm, _ := updateModel(m, tickMsg{})
	if nm.statusBar.topNotification().msg != "" {
		t.Fatalf("notification must be cleared after tick past expiry, got %q", nm.statusBar.topNotification().msg)
	}
}

func TestTransientNotificationPersistsBeforeExpiry(t *testing.T) {
	m := New(context.Background())
	m.statusBar.notifications = []transientNotification{{msg: "fresh message", expiresAt: time.Now().Add(10 * time.Second)}}

	nm, _ := updateModel(m, tickMsg{})
	if nm.statusBar.topNotification().msg != "fresh message" {
		t.Fatalf("notification must not be cleared before expiry, got %q", nm.statusBar.topNotification().msg)
	}
}

func TestNewerNotificationOverwritesOlderResettingExpiry(t *testing.T) {
	m := New(context.Background())
	oldExpiry := time.Now().Add(500 * time.Millisecond)
	m.statusBar.notifications = []transientNotification{{msg: "old message", expiresAt: oldExpiry}}

	nm, _ := updateModel(m, PlayerStatusMsg{Text: "new message"})
	if nm.statusBar.topNotification().msg != "new message" {
		t.Fatalf("new notification must overwrite old, got %q", nm.statusBar.topNotification().msg)
	}
	if !nm.statusBar.topNotification().expiresAt.After(oldExpiry) {
		t.Fatal("new notification must reset expiry to a later time than the old one")
	}
}

func TestDequeueShowsUndoHintInStatusBar(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{ID: 10, Path: "/music/a.flac", Display: "A"}}

	nm, _ := updateModel(m, DequeueTrackMsg{Index: 0})
	if nm.statusBar.topNotification().msg == "" {
		t.Fatal("dequeue: status bar must show an undo hint immediately after DequeueTrackMsg")
	}
	assertViewContains(t, nm, "undo")
}

func TestClearQueueShowsUndoHintInStatusBar(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{ID: 10, Path: "/music/a.flac", Display: "A"}, {ID: 20, Path: "/music/b.flac", Display: "B"}}

	nm, _ := updateModel(m, ClearQueueMsg{})
	if nm.statusBar.topNotification().msg == "" {
		t.Fatal("clear queue: status bar must show an undo hint immediately after ClearQueueMsg")
	}
	assertViewContains(t, nm, "undo")
}
