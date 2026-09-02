package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
)

func TestVisualModeQueueExtend(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	m = doNav(m, keys.ActionCursorDown)

	assertSelectedQueueEntriesExact(t, m.queue.Entries, 0, 1, 2)
}

func TestVisualModeQueueExitPreservesSelections(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode inactive after exit")
	}
	assertSelectedQueueEntriesExact(t, m.queue.Entries, 0, 1)
}
