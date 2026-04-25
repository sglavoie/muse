package tui

import (
	"context"
	"testing"
	"time"
)

// TestTickMsgClearsBulkRemovePendingAfterWindow verifies that the tick handler
// clears the bulkRemovePending flag once the confirmation window has elapsed,
// consistent with how clearQueuePending is cleared on tick.
func TestTickMsgClearsBulkRemovePendingAfterWindow(t *testing.T) {
	m := New(context.Background())
	// Simulate pressing the bulk-remove key once - set the flag and a past timestamp.
	m.undo.bulkRemovePending = true
	m.undo.bulkRemovePendingAt = time.Now().Add(-5 * time.Second) // well past the 2s window

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.undo.bulkRemovePending {
		t.Fatal("expected bulkRemovePending=false after tick past confirmation window")
	}
}

// TestTickMsgKeepsBulkRemovePendingWithinWindow verifies that the tick handler
// does NOT clear bulkRemovePending while the confirmation window is still open.
func TestTickMsgKeepsBulkRemovePendingWithinWindow(t *testing.T) {
	m := New(context.Background())
	m.undo.bulkRemovePending = true
	m.undo.bulkRemovePendingAt = time.Now() // just set, well within 2s window

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if !nm.undo.bulkRemovePending {
		t.Fatal("expected bulkRemovePending=true while within confirmation window")
	}
}