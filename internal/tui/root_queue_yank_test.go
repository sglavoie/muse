package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
)

// TestYankSingleTrack verifies that ctrl+y with no selection yanks the focused track.
func TestYankSingleTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 1

	rm, _, handled := m.handleQueueKeyAction(keys.ActionYank)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionYank should be handled")
	}
	if len(m.yankBuffer) != 1 {
		t.Fatalf("expected 1 yanked entry, got %d", len(m.yankBuffer))
	}
	if m.yankBuffer[0].Hash != "h2" {
		t.Errorf("expected yanked hash 'h2', got %q", m.yankBuffer[0].Hash)
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "1 track") {
		t.Errorf("expected notification about 1 track, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestYankBulkSelection verifies that ctrl+y with selected entries yanks all
// selected entries in queue order.
func TestYankBulkSelection(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Entries[0].Selected = true
	m.queue.Entries[2].Selected = true
	m.queue.Cursor = 0

	rm, _, handled := m.handleQueueKeyAction(keys.ActionYank)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionYank should be handled")
	}
	if len(m.yankBuffer) != 2 {
		t.Fatalf("expected 2 yanked entries, got %d", len(m.yankBuffer))
	}
	if m.yankBuffer[0].Hash != "h1" || m.yankBuffer[1].Hash != "h3" {
		t.Errorf("yanked entries out of order: %q, %q", m.yankBuffer[0].Hash, m.yankBuffer[1].Hash)
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "2 track") {
		t.Errorf("expected notification about 2 tracks, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestYankReplacesBuffer verifies that yanking again replaces the previous buffer.
func TestYankReplacesBuffer(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 0

	rm, _, _ := m.handleQueueKeyAction(keys.ActionYank)
	m = rm.(Model)
	if m.yankBuffer[0].Hash != "h1" {
		t.Fatal("first yank should capture h1")
	}

	m.queue.Cursor = 3
	rm, _, _ = m.handleQueueKeyAction(keys.ActionYank)
	m = rm.(Model)
	if len(m.yankBuffer) != 1 || m.yankBuffer[0].Hash != "h4" {
		t.Errorf("second yank should replace buffer with h4, got %v", m.yankBuffer)
	}
}

// TestYankEmptyQueue verifies yank is a no-op on an empty queue.
func TestYankEmptyQueue(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue

	rm, _, handled := m.handleQueueKeyAction(keys.ActionYank)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionYank should be handled even on empty queue")
	}
	if len(m.yankBuffer) != 0 {
		t.Errorf("expected empty yank buffer, got %d entries", len(m.yankBuffer))
	}
}
