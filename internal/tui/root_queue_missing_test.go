package tui

import (
	"context"
	"strings"
	"testing"
)

// TestUpdateMissingTrackMsgMarksQueueEntryAndSetsError verifies that
// MissingTrackMsg marks the matching queue entry as missing and sets a
// transient error message in the status bar.
func TestUpdateMissingTrackMsgMarksQueueEntryAndSetsError(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{Path: "/music/song.flac", Display: "Artist - Song"},
	}

	next, _ := m.Update(MissingTrackMsg{Path: "/music/song.flac"})
	nm := next.(Model)

	if nm.statusBar.notification.msg == "" {
		t.Fatal("MissingTrackMsg must set a transient status bar error message")
	}
	if !nm.queue.entries[0].Missing {
		t.Fatal("MissingTrackMsg must mark the matching queue entry as missing")
	}
}

// TestSelectMissingTrackEmitsMissingTrackSelectedMsg verifies that pressing
// enter on a missing queue entry emits MissingTrackSelectedMsg and that root
// handles it by setting a transient notification.
func TestSelectMissingTrackEmitsMissingTrackSelectedMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/gone.flac", Display: "Artist - Gone", Missing: true, Hash: "abc123"},
	}
	m.queue.cursor = 0

	v, cmd := m.queue.HandleAction(actionSelect)
	m.queue = v
	if cmd == nil {
		t.Fatal("expected a command from selecting a missing track")
	}
	msg := cmd()
	if _, ok := msg.(MissingTrackSelectedMsg); !ok {
		t.Fatalf("expected MissingTrackSelectedMsg, got %T", msg)
	}

	next, _ := m.Update(msg)
	nm := next.(Model)
	if nm.statusBar.notification.msg == "" {
		t.Fatal("MissingTrackSelectedMsg must set a transient notification")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "missing") {
		t.Errorf("unexpected notification: %s", nm.statusBar.notification.msg)
	}
}