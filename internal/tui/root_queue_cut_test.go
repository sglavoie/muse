package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
)

// TestCutSingleTrack verifies that ctrl+x with no selection cuts the focused
// track and sets yankIsCut.
func TestCutSingleTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 1

	rm, _, handled := m.handleQueueKeyAction(keys.ActionCut)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionCut should be handled")
	}
	if len(m.yankBuffer) != 1 {
		t.Fatalf("expected 1 cut entry, got %d", len(m.yankBuffer))
	}
	if m.yankBuffer[0].Hash != "h2" {
		t.Errorf("expected cut hash 'h2', got %q", m.yankBuffer[0].Hash)
	}
	if !m.yankIsCut {
		t.Error("yankIsCut should be true after cut")
	}
	if !strings.Contains(m.statusBar.topNotification().msg, "Cut") {
		t.Errorf("expected 'Cut' in notification, got %q", m.statusBar.topNotification().msg)
	}
	// Queue should not be modified yet (deferred to paste).
	if len(m.queue.entries) != 4 {
		t.Errorf("queue should still have 4 entries, got %d", len(m.queue.entries))
	}
}

// TestCutBulkSelection verifies that ctrl+x with selected entries cuts all
// selected entries.
func TestCutBulkSelection(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.entries[0].Selected = true
	m.queue.entries[2].Selected = true
	m.queue.cursor = 0

	rm, _, _ := m.handleQueueKeyAction(keys.ActionCut)
	m = rm.(Model)

	if len(m.yankBuffer) != 2 {
		t.Fatalf("expected 2 cut entries, got %d", len(m.yankBuffer))
	}
	if m.yankBuffer[0].Hash != "h1" || m.yankBuffer[1].Hash != "h3" {
		t.Errorf("cut entries out of order: %q, %q", m.yankBuffer[0].Hash, m.yankBuffer[1].Hash)
	}
	if !m.yankIsCut {
		t.Error("yankIsCut should be true")
	}
}

// TestYankAfterCutClearsCutFlag verifies that yanking after a cut downgrades
// the buffer to a copy operation.
func TestYankAfterCutClearsCutFlag(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0

	// Cut.
	rm, _, _ := m.handleQueueKeyAction(keys.ActionCut)
	m = rm.(Model)
	if !m.yankIsCut {
		t.Fatal("yankIsCut should be true after cut")
	}

	// Now yank a different track.
	m.queue.cursor = 1
	rm, _, _ = m.handleQueueKeyAction(keys.ActionYank)
	m = rm.(Model)

	if m.yankIsCut {
		t.Error("yankIsCut should be false after yank")
	}
	if m.yankBuffer[0].Hash != "h2" {
		t.Errorf("yank buffer should have h2, got %q", m.yankBuffer[0].Hash)
	}
}
