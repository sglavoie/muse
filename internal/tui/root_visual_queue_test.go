package tui

import (
	"context"
	"testing"
)

func TestVisualModeQueueExtend(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown)
	m = doNav(m, actionCursorDown)

	assertSelectedQueueEntriesExact(t, m.queue.entries, 0, 1, 2)
}

func TestVisualModeQueueExitPreservesSelections(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown)
	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode inactive after exit")
	}
	assertSelectedQueueEntriesExact(t, m.queue.entries, 0, 1)
}
