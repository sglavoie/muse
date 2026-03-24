package tui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestUpdateMissingTrackMsgMarksQueueEntryAndSetsError verifies that
// MissingTrackMsg marks the matching queue entry as missing and sets a
// transient error message in the status bar.
func TestUpdateMissingTrackMsgMarksQueueEntryAndSetsError(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{Path: "/music/song.flac", Display: "Artist – Song"},
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
		{Path: "/music/gone.flac", Display: "Artist – Gone", Missing: true, Hash: "abc123"},
	}
	m.queue.cursor = 0

	// HandleAction with actionSelect on a missing entry should produce
	// MissingTrackSelectedMsg.
	v, cmd := m.queue.HandleAction(actionSelect)
	m.queue = v
	if cmd == nil {
		t.Fatal("expected a command from selecting a missing track")
	}
	msg := cmd()
	if _, ok := msg.(MissingTrackSelectedMsg); !ok {
		t.Fatalf("expected MissingTrackSelectedMsg, got %T", msg)
	}

	// Feed the message through root Update to verify the notification.
	next, _ := m.Update(msg)
	nm := next.(Model)
	if nm.statusBar.notification.msg == "" {
		t.Fatal("MissingTrackSelectedMsg must set a transient notification")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "missing") {
		t.Errorf("unexpected notification: %s", nm.statusBar.notification.msg)
	}
}

// TestQueueViewRendersMissingEntryDimmed verifies that a missing queue entry
// is rendered with dim styling and shows the "[missing]" label.
func TestQueueViewRendersMissingEntryDimmed(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/gone.flac", Display: "Artist – Gone", Missing: true},
		},
	}
	s := styles.NewStyles(true)
	got := q.render(s, "", nil)
	if !strings.Contains(got, "[missing]") {
		t.Fatalf("missing entry must include [missing] label; got:\n%s", got)
	}
	// Use styledText to check dim styling when ANSI is available.
	dimSample := styledText(s.Dim, "x")
	if ansi.Strip(dimSample) == dimSample {
		return // lipgloss produced no ANSI in this environment; skip style check
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(ansi.Strip(line), "[missing]") {
			if line == ansi.Strip(line) {
				t.Error("missing entry must be rendered with dim ANSI styling")
			}
			return
		}
	}
	t.Error("missing entry '[missing]' label not found in any output line")
}

// TestQueueViewHandleActionEnterSendsPlayQueueIndexMsg verifies that
// actionSelect returns a Cmd that emits PlayQueueIndexMsg with the cursor
// index. Root routes Enter → actionSelect via the registry and calls
// HandleAction, so this is the code path exercised in production.
func TestQueueViewHandleActionEnterSendsPlayQueueIndexMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "Artist – A"},
			{Path: "/music/b.flac", Display: "Artist – B"},
		},
		cursor: 1,
	}
	_, cmd := q.HandleAction(actionSelect)
	if cmd == nil {
		t.Fatal("actionSelect must return a non-nil cmd")
	}
	msg := cmd()
	pqi, ok := msg.(PlayQueueIndexMsg)
	if !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
	if pqi.Index != 1 {
		t.Fatalf("expected index 1, got %d", pqi.Index)
	}
}

// TestQueueViewHandleActionEnterOnEmptyQueueIsNoOp verifies that actionSelect
// on an empty queue does not emit a PlayQueueIndexMsg.
func TestQueueViewHandleActionEnterOnEmptyQueueIsNoOp(t *testing.T) {
	q := QueueView{}
	_, cmd := q.HandleAction(actionSelect)
	if cmd != nil {
		t.Fatal("actionSelect on empty queue must return nil cmd")
	}
}

// TestQueueViewUpdateDSendsDequeueTrackMsg verifies that actionDequeue
// dispatches DequeueTrackMsg with the current cursor index.
func TestQueueViewUpdateDSendsDequeueTrackMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 1,
	}
	_, cmd := q.HandleAction(actionDequeue)
	if cmd == nil {
		t.Fatal("actionDequeue must return a non-nil cmd")
	}
	msg, ok := cmd().(DequeueTrackMsg)
	if !ok {
		t.Fatalf("expected DequeueTrackMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Fatalf("expected index 1, got %d", msg.Index)
	}
}

// TestQueueViewUpdateDOnEmptyQueueIsNoOp verifies that actionDequeue on an
// empty queue does not emit a DequeueTrackMsg.
func TestQueueViewUpdateDOnEmptyQueueIsNoOp(t *testing.T) {
	q := QueueView{}
	_, cmd := q.HandleAction(actionDequeue)
	if cmd != nil {
		t.Fatal("actionDequeue on empty queue must return nil cmd")
	}
}

// TestQueueViewUpdateXSendsBulkSelectionToggleMsg verifies that
// actionBulkSelectionToggle dispatches BulkSelectionToggleMsg with the current
// cursor index.
func TestQueueViewUpdateXSendsBulkSelectionToggleMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 0,
	}
	_, cmd := q.HandleAction(actionBulkSelectionToggle)
	if cmd == nil {
		t.Fatal("actionBulkSelectionToggle must return a non-nil cmd")
	}
	msg, ok := cmd().(BulkSelectionToggleMsg)
	if !ok {
		t.Fatalf("expected BulkSelectionToggleMsg, got %T", cmd())
	}
	if msg.Index != 0 {
		t.Fatalf("expected index 0, got %d", msg.Index)
	}
}

// TestQueueViewUpdateCSendsClearQueueMsg verifies that actionClearQueue
// dispatches ClearQueueMsg.
func TestQueueViewUpdateCSendsClearQueueMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
		},
	}
	_, cmd := q.HandleAction(actionClearQueue)
	if cmd == nil {
		t.Fatal("actionClearQueue must return a non-nil cmd")
	}
	if _, ok := cmd().(ClearQueueMsg); !ok {
		t.Fatalf("expected ClearQueueMsg, got %T", cmd())
	}
}

// TestQueueViewUpdateKSendsMoveTrackUpMsg verifies that actionMoveTrackUp
// dispatches MoveTrackUpMsg with the current cursor index.
func TestQueueViewUpdateKSendsMoveTrackUpMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 1,
	}
	_, cmd := q.HandleAction(actionMoveTrackUp)
	if cmd == nil {
		t.Fatal("actionMoveTrackUp must return a non-nil cmd")
	}
	msg, ok := cmd().(MoveTrackUpMsg)
	if !ok {
		t.Fatalf("expected MoveTrackUpMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Fatalf("expected index 1, got %d", msg.Index)
	}
}

// TestQueueViewUpdateKAtTopIsNoOp verifies that actionMoveTrackUp at the
// first entry (boundary) does not emit a MoveTrackUpMsg.
func TestQueueViewUpdateKAtTopIsNoOp(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 0,
	}
	_, cmd := q.HandleAction(actionMoveTrackUp)
	if cmd != nil {
		t.Fatal("actionMoveTrackUp at top entry must return nil cmd")
	}
}

// TestQueueViewUpdateJSendsMoveTrackDownMsg verifies that actionMoveTrackDown
// dispatches MoveTrackDownMsg with the current cursor index.
func TestQueueViewUpdateJSendsMoveTrackDownMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 0,
	}
	_, cmd := q.HandleAction(actionMoveTrackDown)
	if cmd == nil {
		t.Fatal("actionMoveTrackDown must return a non-nil cmd")
	}
	msg, ok := cmd().(MoveTrackDownMsg)
	if !ok {
		t.Fatalf("expected MoveTrackDownMsg, got %T", cmd())
	}
	if msg.Index != 0 {
		t.Fatalf("expected index 0, got %d", msg.Index)
	}
}

// TestQueueViewUpdateJAtBottomIsNoOp verifies that actionMoveTrackDown at the
// last entry (boundary) does not emit a MoveTrackDownMsg.
func TestQueueViewUpdateJAtBottomIsNoOp(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 1,
	}
	_, cmd := q.HandleAction(actionMoveTrackDown)
	if cmd != nil {
		t.Fatal("actionMoveTrackDown at bottom entry must return nil cmd")
	}
}

// TestRootQueueLocalDDispatchesDequeueMsg verifies the end-to-end path: pressing
// 'd' while the queue view is active returns a Cmd that emits DequeueTrackMsg.
func TestRootQueueLocalDDispatchesDequeueMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if cmd == nil {
		t.Fatal("'d' in queue view: expected non-nil cmd")
	}
	msg, ok := cmd().(DequeueTrackMsg)
	if !ok {
		t.Fatalf("'d' in queue view: expected DequeueTrackMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Fatalf("'d' in queue view: expected index 1, got %d", msg.Index)
	}
}

// TestRootQueueLocalXDispatchesBulkSelectionToggleMsg verifies the end-to-end
// path: pressing 'x' while the queue view is active dispatches BulkSelectionToggleMsg.
func TestRootQueueLocalXDispatchesBulkSelectionToggleMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
	}
	m.queue.cursor = 0

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd == nil {
		t.Fatal("'x' in queue view: expected non-nil cmd")
	}
	if _, ok := cmd().(BulkSelectionToggleMsg); !ok {
		t.Fatalf("'x' in queue view: expected BulkSelectionToggleMsg, got %T", cmd())
	}
}

// TestRootQueueLocalCDispatchesClearQueueMsg verifies the end-to-end path:
// pressing 'C' (Shift+c) twice while the queue view is active dispatches
// ClearQueueMsg. The first press shows a confirmation hint; the second
// press (within 2 seconds) actually clears the queue.
func TestRootQueueLocalCDispatchesClearQueueMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
	}

	// First press: confirmation hint, no cmd.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("first 'C' should not dispatch a cmd")
	}
	if !m.undo.clearQueuePending {
		t.Fatal("clearQueuePending should be true after first 'C'")
	}
	if m.statusBar.notification.msg != "Press C again to clear queue" {
		t.Fatalf("expected confirmation hint, got %q", m.statusBar.notification.msg)
	}

	// Second press: dispatches ClearQueueMsg.
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	if cmd == nil {
		t.Fatal("second 'C' in queue view: expected non-nil cmd")
	}
	if _, ok := cmd().(ClearQueueMsg); !ok {
		t.Fatalf("second 'C' in queue view: expected ClearQueueMsg, got %T", cmd())
	}
}

// TestRootQueueLocalKDispatchesMoveTrackUpMsg verifies the end-to-end path:
// pressing 'K' (Shift+k) while the queue view is active dispatches MoveTrackUpMsg.
func TestRootQueueLocalKDispatchesMoveTrackUpMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
	if cmd == nil {
		t.Fatal("'K' in queue view: expected non-nil cmd")
	}
	msg, ok := cmd().(MoveTrackUpMsg)
	if !ok {
		t.Fatalf("'K' in queue view: expected MoveTrackUpMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Fatalf("'K' in queue view: expected index 1, got %d", msg.Index)
	}
}

// TestRootQueueLocalJDispatchesMoveTrackDownMsg verifies the end-to-end path:
// pressing 'J' (Shift+j) while the queue view is active dispatches MoveTrackDownMsg.
func TestRootQueueLocalJDispatchesMoveTrackDownMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 0

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	if cmd == nil {
		t.Fatal("'J' in queue view: expected non-nil cmd")
	}
	msg, ok := cmd().(MoveTrackDownMsg)
	if !ok {
		t.Fatalf("'J' in queue view: expected MoveTrackDownMsg, got %T", cmd())
	}
	if msg.Index != 0 {
		t.Fatalf("'J' in queue view: expected index 0, got %d", msg.Index)
	}
}

// TestKWhileSortedProducesTransientNotification verifies that pressing 'K'
// while the queue is sorted shows a "Cannot reorder" notification instead of
// silently doing nothing.
func TestKWhileSortedProducesTransientNotification(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1
	// Simulate a sorted queue by setting a non-nil displayOrder.
	m.queue.displayOrder = []int{0, 1}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
	nm := next.(Model)
	if cmd == nil {
		t.Fatal("'K' while sorted: expected a cmd producing TransientNotifyMsg")
	}
	// Feed the message through Update so the notification is set.
	next2, _ := nm.Update(cmd())
	nm2 := next2.(Model)
	if nm2.statusBar.notification.msg == "" {
		t.Fatal("'K' while sorted: expected a transient notification to be set")
	}
	if !strings.Contains(nm2.statusBar.notification.msg, "Cannot reorder") {
		t.Fatalf("'K' while sorted: unexpected notification %q", nm2.statusBar.notification.msg)
	}
}

// TestMoveTrackUpMsgCallsReordererWithCorrectIDAndDirection verifies that
// MoveTrackUpMsg swaps the TUI slice and calls ReorderQueueTrack with the
// moved entry's ID and direction -1.
func TestMoveTrackUpMsgCallsReordererWithCorrectIDAndDirection(t *testing.T) {
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithQueueReorderer(r))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	next, cmd := m.Update(MoveTrackUpMsg{Index: 1})
	nm := next.(Model)

	// TUI slice must be swapped.
	if nm.queue.entries[0].ID != 20 || nm.queue.entries[1].ID != 10 {
		t.Fatalf("expected entries swapped: got IDs %d %d", nm.queue.entries[0].ID, nm.queue.entries[1].ID)
	}
	if nm.queue.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", nm.queue.cursor)
	}

	// Execute the cmd to trigger the DB write.
	if cmd == nil {
		t.Fatal("MoveTrackUpMsg with reorderer: expected non-nil cmd")
	}
	cmd()

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 reorderer call, got %d", len(r.calls))
	}
	if r.calls[0].id != 20 {
		t.Fatalf("expected entry ID 20, got %d", r.calls[0].id)
	}
	if r.calls[0].direction != -1 {
		t.Fatalf("expected direction -1, got %d", r.calls[0].direction)
	}
}

// TestMoveTrackDownMsgCallsReordererWithCorrectIDAndDirection verifies that
// MoveTrackDownMsg swaps the TUI slice and calls ReorderQueueTrack with the
// moved entry's ID and direction +1.
func TestMoveTrackDownMsgCallsReordererWithCorrectIDAndDirection(t *testing.T) {
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithQueueReorderer(r))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	next, cmd := m.Update(MoveTrackDownMsg{Index: 1})
	nm := next.(Model)

	// TUI slice must be swapped.
	if nm.queue.entries[1].ID != 30 || nm.queue.entries[2].ID != 20 {
		t.Fatalf("expected entries swapped: got IDs %d %d", nm.queue.entries[1].ID, nm.queue.entries[2].ID)
	}
	if nm.queue.cursor != 2 {
		t.Fatalf("expected cursor 2, got %d", nm.queue.cursor)
	}

	// Execute the cmd to trigger the DB write.
	if cmd == nil {
		t.Fatal("MoveTrackDownMsg with reorderer: expected non-nil cmd")
	}
	cmd()

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 reorderer call, got %d", len(r.calls))
	}
	if r.calls[0].id != 20 {
		t.Fatalf("expected entry ID 20, got %d", r.calls[0].id)
	}
	if r.calls[0].direction != +1 {
		t.Fatalf("expected direction +1, got %d", r.calls[0].direction)
	}
}

// TestUndoMsgRestoresDequeueWithinWindow verifies that receiving UndoMsg within
// the undo window calls UndoDequeue on the DB writer and restores the entry to
// the TUI slice at its original index.
func TestUndoMsgRestoresDequeueWithinWindow(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	// Dequeue the middle entry (index 1).
	next, _ := m.Update(DequeueTrackMsg{Index: 1})
	nm := next.(Model)
	if len(nm.queue.entries) != 2 {
		t.Fatalf("after dequeue: expected 2 entries, got %d", len(nm.queue.entries))
	}

	// UndoMsg within the window must restore the entry.
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 3 {
		t.Fatalf("after undo: expected 3 entries, got %d", len(nm.queue.entries))
	}
	if nm.queue.entries[1].ID != 20 {
		t.Fatalf("after undo: expected restored entry at index 1 with ID=20, got ID=%d", nm.queue.entries[1].ID)
	}
	if len(d.undoDequeueCallIDs) != 1 || d.undoDequeueCallIDs[0] != 20 {
		t.Fatalf("UndoDequeue must be called with ID=20, got calls=%v", d.undoDequeueCallIDs)
	}
	// Undo slot must be cleared after restore.
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after a successful undo")
	}
}

// TestUndoMsgIsNoOpAfterWindowExpiry verifies that UndoMsg received after the
// 5-second window has elapsed is a no-op: the queue is unchanged and UndoDequeue
// is not called.
func TestUndoMsgIsNoOpAfterWindowExpiry(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
	}

	// Dequeue entry 0, then manually expire the undo slot.
	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)
	nm.undo.slot.expiresAt = time.Now().Add(-1 * time.Second) // expired

	// UndoMsg after expiry must be a no-op.
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("undo after expiry: expected 1 entry (no restore), got %d", len(nm.queue.entries))
	}
	if len(d.undoDequeueCallIDs) != 0 {
		t.Fatalf("undo after expiry: UndoDequeue must not be called, got calls=%v", d.undoDequeueCallIDs)
	}
}

// TestNewDestructiveActionReplacesUndoSlot verifies that a second destructive
// action before the first undo window expires overwrites the undo slot.
func TestNewDestructiveActionReplacesUndoSlot(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	// First dequeue: removes index 0 (ID=10).
	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)
	firstSlotID := nm.undo.slot.action.(undoDequeueAction).id

	// Second dequeue: removes index 0 of the remaining entries (ID=20).
	next, _ = nm.Update(DequeueTrackMsg{Index: 0})
	nm = next.(Model)

	if nm.undo.slot.action.(undoDequeueAction).id == firstSlotID {
		t.Fatal("second dequeue must replace the undo slot with the new entry's ID")
	}
	if nm.undo.slot.action.(undoDequeueAction).id != 20 {
		t.Fatalf("undo slot must hold ID=20 after second dequeue, got %d", nm.undo.slot.action.(undoDequeueAction).id)
	}
}

// TestUndoMsgRestoresClearQueueWithinWindow verifies that UndoMsg within the
// window calls UndoClearQueue on the DB writer and restores all entries in the
// TUI slice.
func TestUndoMsgRestoresClearQueueWithinWindow(t *testing.T) {
	d := &stubQueueDequeuer{clearQueueSoftResult: []int64{10, 20}}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)
	if len(nm.queue.entries) != 0 {
		t.Fatalf("after clear: expected 0 entries, got %d", len(nm.queue.entries))
	}

	// UndoMsg within the window must restore both entries.
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 2 {
		t.Fatalf("after undo clear: expected 2 entries, got %d", len(nm.queue.entries))
	}
	if len(d.undoClearQueueCalls) != 1 {
		t.Fatalf("UndoClearQueue must be called once, got %d calls", len(d.undoClearQueueCalls))
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after a successful undo")
	}
}

// TestUndoMsgWithNoSlotIsNoOp verifies that UndoMsg when no destructive action
// has been performed is a no-op.
func TestUndoMsgWithNoSlotIsNoOp(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	// Queue must be unchanged.
	if len(nm.queue.entries) != 1 {
		t.Fatalf("UndoMsg with no slot: expected 1 entry unchanged, got %d", len(nm.queue.entries))
	}
}

// TestQueueCursorDownJKey verifies that pressing "j" while the queue view is
// active advances the cursor by one.
func TestQueueCursorDownJKey(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.queue.cursor != 1 {
		t.Errorf("j key: expected cursor 1, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorUpKKey verifies that pressing "k" while the queue view is
// active moves the cursor up by one.
func TestQueueCursorUpKKey(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.queue.cursor != 0 {
		t.Errorf("k key: expected cursor 0, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorDownArrow verifies that the down arrow key advances the
// cursor in the queue view, behaving identically to "j".
func TestQueueCursorDownArrow(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.queue.cursor != 1 {
		t.Errorf("down arrow: expected cursor 1, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorUpArrow verifies that the up arrow key moves the cursor up
// in the queue view, behaving identically to "k".
func TestQueueCursorUpArrow(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	nm := next.(Model)
	if nm.queue.cursor != 0 {
		t.Errorf("up arrow: expected cursor 0, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorClampAtBottom verifies that the cursor stops at the last
// entry without wrapping when the queue view is active.
func TestQueueCursorClampAtBottom(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1 // already at last item

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.queue.cursor != 1 {
		t.Errorf("j at last: expected cursor 1 (clamped), got %d", nm.queue.cursor)
	}
}

// TestQueueCursorClampAtTop verifies that the cursor stops at the first entry
// without wrapping when the queue view is active.
func TestQueueCursorClampAtTop(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	// cursor starts at 0

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.queue.cursor != 0 {
		t.Errorf("k at first: expected cursor 0 (clamped), got %d", nm.queue.cursor)
	}
}

// TestDequeueTrackMsgCallsPlayerQueueRemoverWithCorrectIndex verifies that
// DequeueTrackMsg calls RemoveFromQueue on the player with the dequeued track's
// index so the player's in-memory queue stays in sync with the TUI queue.
func TestDequeueTrackMsgCallsPlayerQueueRemoverWithCorrectIndex(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 1})
	nm := next.(Model)

	if len(nm.queue.entries) != 2 {
		t.Fatalf("expected 2 queue entries after dequeue, got %d", len(nm.queue.entries))
	}
	if len(p.removeFromQueueCalls) != 1 {
		t.Fatalf("expected 1 RemoveFromQueue call, got %d", len(p.removeFromQueueCalls))
	}
	if p.removeFromQueueCalls[0] != 1 {
		t.Fatalf("expected RemoveFromQueue called with index 1, got %d", p.removeFromQueueCalls[0])
	}
}

// TestDequeueTrackMsgDoesNotCallStop verifies that dequeuing a track never
// calls Stop on the player, regardless of playback state. Playback of the
// currently active track continues uninterrupted; only the queue index list
// is trimmed.
func TestDequeueTrackMsgDoesNotCallStop(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state PlaybackState
	}{
		{"idle", PlaybackIdle},
		{"playing", PlaybackPlaying},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &stubPlayerQueueRemover{}
			m := New(context.Background(), WithPlayer(p))
			m.playback.state = tc.state
			m.queue.entries = []QueueEntry{
				{ID: 10, Path: "/music/a.flac", Display: "A"},
				{ID: 20, Path: "/music/b.flac", Display: "B"},
			}

			m.Update(DequeueTrackMsg{Index: 0})

			if p.stopCalls != 0 {
				t.Fatalf("DequeueTrackMsg must not call Stop, got %d calls", p.stopCalls)
			}
		})
	}
}

// TestUndoDequeueRebuildPlayerQueue verifies that undoing a dequeue calls
// SetQueue on the player so the player's in-memory queue stays in sync with
// the TUI queue after the re-insertion.
func TestUndoDequeueRebuildPlayerQueue(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithPlayer(p), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Hash: "ha", Artist: "A", Title: "Ta"},
		{ID: 20, Path: "/music/b.flac", Hash: "hb", Artist: "B", Title: "Tb"},
		{ID: 30, Path: "/music/c.flac", Hash: "hc", Artist: "C", Title: "Tc"},
	}

	// Dequeue the middle entry.
	next, _ := m.Update(DequeueTrackMsg{Index: 1})
	nm := next.(Model)

	// Undo within the window.
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 3 {
		t.Fatalf("after undo: expected 3 entries, got %d", len(nm.queue.entries))
	}

	// SetQueue must have been called to rebuild the player queue.
	if len(p.setQueueCalls) == 0 {
		t.Fatal("undo dequeue must call SetQueue to rebuild the player queue")
	}
	lastCall := p.setQueueCalls[len(p.setQueueCalls)-1]
	if len(lastCall) != 3 {
		t.Fatalf("SetQueue called with %d tracks, want 3", len(lastCall))
	}
	if lastCall[1].Hash != "hb" {
		t.Errorf("restored player queue entry at index 1: want hash %q, got %q", "hb", lastCall[1].Hash)
	}
}

// TestDequeueTrackMsgClampsScrollOffset verifies that dequeuing a track when
// the queue is scrolled clamps the scrollOffset so it does not point past the
// end of the shortened list.
func TestDequeueTrackMsgClampsScrollOffset(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
		{ID: 30, Path: "/c.flac", Display: "C"},
	}
	// Simulate a scrolled-down state: scrollOffset points to the last entry.
	m.queue.scrollOffset = 2
	m.queue.cursor = 2

	next, _ := m.Update(DequeueTrackMsg{Index: 2})
	nm := next.(Model)

	// After removing the last entry, scrollOffset must be clamped.
	if nm.queue.scrollOffset > len(nm.queue.entries)-1 {
		t.Fatalf("scrollOffset %d exceeds last entry index %d after dequeue",
			nm.queue.scrollOffset, len(nm.queue.entries)-1)
	}
}

// TestDequeueLastEntryResetsScrollOffset verifies that dequeuing the only
// entry resets scrollOffset to 0.
func TestDequeueLastEntryResetsScrollOffset(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
	}
	m.queue.scrollOffset = 0
	m.queue.cursor = 0

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.queue.scrollOffset != 0 {
		t.Fatalf("scrollOffset must be 0 after dequeuing the last entry, got %d",
			nm.queue.scrollOffset)
	}
}

// TestDequeueTrackMsgDoesNotTouchPlaylistTracks verifies that processing a
// DequeueTrackMsg leaves any loaded playlist tracks completely unchanged.
// Queue entries and playlist tracks are independent; dequeuing a queue entry
// must never modify the saved playlist_tracks table view.
func TestDequeueTrackMsgDoesNotTouchPlaylistTracks(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
	}
	playlistTracks := []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Playlist Track", Artist: "Artist", DurationMs: 60000},
	}
	m = drillIntoPlaylistTrackLevel(m, playlistTracks)

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if len(nm.playlists.tracks) != len(playlistTracks) {
		t.Fatalf("playlist tracks must be unchanged after dequeue: expected %d, got %d",
			len(playlistTracks), len(nm.playlists.tracks))
	}
	if nm.playlists.tracks[0].ID != playlistTracks[0].ID {
		t.Fatalf("playlist track ID must be unchanged: expected %d, got %d",
			playlistTracks[0].ID, nm.playlists.tracks[0].ID)
	}
}

// TestUndoEnqueueClampsScrollOffset verifies that undoing a single-track
// enqueue clamps scrollOffset so it does not point past the end of the
// shortened queue.
func TestUndoEnqueueClampsScrollOffset(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}
	// Set up an undoKindEnqueue slot pointing at the last entry (index 1).
	m.undo.slot = undoSlot{
		action:    undoEnqueueAction{id: 20, entry: m.queue.entries[1], idx: 1},
		expiresAt: time.Now().Add(undoWindowDuration),
	}
	// Simulate a scrolled-down state.
	m.queue.scrollOffset = 1
	m.queue.cursor = 1

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("after undo enqueue: expected 1 entry, got %d", len(nm.queue.entries))
	}
	if nm.queue.scrollOffset > 0 {
		t.Fatalf("scrollOffset must be clamped to 0 after removing last entry, got %d",
			nm.queue.scrollOffset)
	}
}

// TestUndoBatchEnqueueClampsScrollOffset verifies that undoing a batch enqueue
// clamps scrollOffset so it does not point past the end of the shortened queue.
func TestUndoBatchEnqueueClampsScrollOffset(t *testing.T) {
	d := &stubQueueDequeuer{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueDequeuer(d), WithBatchEnqueuer(be))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 100, Path: "/b.flac", Display: "B"},
		{ID: 101, Path: "/c.flac", Display: "C"},
	}
	// Set up an undoBatchEnqueueAction slot: entries at indices 1 and 2 were
	// batch-enqueued.
	m.undo.slot = undoSlot{
		action: undoBatchEnqueueAction{
			ids:          []int64{100, 101},
			count:        2,
			startIdx:     1,
			playlistName: "test",
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}
	// Simulate scrolled to the batch entries.
	m.queue.scrollOffset = 2
	m.queue.cursor = 2

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("after undo batch enqueue: expected 1 entry, got %d", len(nm.queue.entries))
	}
	if nm.queue.scrollOffset > 0 {
		t.Fatalf("scrollOffset must be clamped to 0 after batch undo, got %d",
			nm.queue.scrollOffset)
	}
}

// TestMoveTrackUpSyncsPlayerQueue verifies that MoveTrackUpMsg rebuilds the
// player's in-memory queue via SetQueue so that auto-advance uses the new
// ordering after a K reorder.
func TestMoveTrackUpSyncsPlayerQueue(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithPlayer(p), WithQueueReorderer(r))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Hash: "ha", Artist: "A", Title: "Ta"},
		{ID: 20, Path: "/b.flac", Hash: "hb", Artist: "B", Title: "Tb"},
		{ID: 30, Path: "/c.flac", Hash: "hc", Artist: "C", Title: "Tc"},
	}

	next, cmd := m.Update(MoveTrackUpMsg{Index: 1})
	_ = next.(Model)

	// SetQueue must have been called to sync the player queue.
	if len(p.setQueueCalls) != 1 {
		t.Fatalf("MoveTrackUpMsg must call SetQueue once, got %d calls", len(p.setQueueCalls))
	}
	synced := p.setQueueCalls[0]
	if len(synced) != 3 {
		t.Fatalf("SetQueue called with %d tracks, want 3", len(synced))
	}
	// After moving index 1 up, the order should be: B(hb), A(ha), C(hc)
	if synced[0].Hash != "hb" || synced[1].Hash != "ha" || synced[2].Hash != "hc" {
		t.Errorf("player queue order after move-up: got hashes %q %q %q, want hb ha hc",
			synced[0].Hash, synced[1].Hash, synced[2].Hash)
	}

	// The DB reorder cmd should also be returned.
	if cmd == nil {
		t.Fatal("expected non-nil cmd for DB reorder")
	}
}

// TestMoveTrackDownSyncsPlayerQueue verifies that MoveTrackDownMsg rebuilds
// the player's in-memory queue via SetQueue so that auto-advance uses the
// new ordering after a J reorder.
func TestMoveTrackDownSyncsPlayerQueue(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithPlayer(p), WithQueueReorderer(r))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Hash: "ha", Artist: "A", Title: "Ta"},
		{ID: 20, Path: "/b.flac", Hash: "hb", Artist: "B", Title: "Tb"},
		{ID: 30, Path: "/c.flac", Hash: "hc", Artist: "C", Title: "Tc"},
	}

	next, cmd := m.Update(MoveTrackDownMsg{Index: 1})
	_ = next.(Model)

	if len(p.setQueueCalls) != 1 {
		t.Fatalf("MoveTrackDownMsg must call SetQueue once, got %d calls", len(p.setQueueCalls))
	}
	synced := p.setQueueCalls[0]
	if len(synced) != 3 {
		t.Fatalf("SetQueue called with %d tracks, want 3", len(synced))
	}
	// After moving index 1 down, the order should be: A(ha), C(hc), B(hb)
	if synced[0].Hash != "ha" || synced[1].Hash != "hc" || synced[2].Hash != "hb" {
		t.Errorf("player queue order after move-down: got hashes %q %q %q, want ha hc hb",
			synced[0].Hash, synced[1].Hash, synced[2].Hash)
	}

	if cmd == nil {
		t.Fatal("expected non-nil cmd for DB reorder")
	}
}

// TestUndoEnqueueSchedulesPurge verifies that undoing a single-track enqueue
// schedules a purge that eventually fires (i.e. PurgeDeletedQueueEntries is
// called after the undo window elapses, not immediately cancelled).
func TestUndoEnqueueSchedulesPurge(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}
	m.undo.slot = undoSlot{
		action:    undoEnqueueAction{id: 20, entry: m.queue.entries[1], idx: 1},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	// The entry at dequeueIdx (index 1, ID=20) should have been soft-deleted.
	if len(d.dequeueTrackSoftCalls) != 1 || d.dequeueTrackSoftCalls[0] != 20 {
		t.Fatalf("undo enqueue must soft-delete the entry at the stored index (ID=20); got calls=%v",
			d.dequeueTrackSoftCalls)
	}

	// The undo slot must be cleared (action == nil).
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}

	// The purge timer's cancel channel must be preserved (non-nil) and open
	// so the timer can fire. A subsequent destructive action will close it if
	// needed, preventing stale purges from affecting newer undo operations.
	if nm.undo.slot.cancelPurge == nil {
		t.Fatal("cancelPurge must be preserved after undo so the purge timer can be cancelled by a future action")
	}
	// Verify the channel is open (not closed) by attempting a non-blocking receive.
	select {
	case <-nm.undo.slot.cancelPurge:
		t.Fatal("cancelPurge must not be closed — the purge timer should still be able to fire")
	default:
	}
}

// TestUndoBatchEnqueueSchedulesPurge verifies that undoing a batch enqueue
// schedules a purge that is not immediately cancelled.
func TestUndoBatchEnqueueSchedulesPurge(t *testing.T) {
	d := &stubQueueDequeuer{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueDequeuer(d), WithBatchEnqueuer(be))
	m.queue.entries = []QueueEntry{
		{ID: 100, Path: "/a.flac", Display: "A"},
		{ID: 101, Path: "/b.flac", Display: "B"},
	}
	m.undo.slot = undoSlot{
		action: undoBatchEnqueueAction{
			ids:      []int64{100, 101},
			count:    2,
			startIdx: 0,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	// Batch entries must have been soft-deleted.
	if len(be.dequeueTracksSoftIDs) != 1 {
		t.Fatalf("undo batch enqueue must call DequeueTracksSoft once, got %d calls",
			len(be.dequeueTracksSoftIDs))
	}

	// The purge timer's cancel channel must be preserved (non-nil and open)
	// so the timer can fire, and so a future action can cancel it if needed.
	if nm.undo.slot.cancelPurge == nil {
		t.Fatal("cancelPurge must be preserved after undo so the purge timer can be cancelled by a future action")
	}
	select {
	case <-nm.undo.slot.cancelPurge:
		t.Fatal("cancelPurge must not be closed — the purge timer should still be able to fire")
	default:
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
}

// TestClearQueueClearsStatusBarTrackInfo verifies that ClearQueueMsg resets the
// status bar artist, title, playlistSource, and currentTrackHash to empty strings
// so the status bar does not display stale track info after the queue is cleared.
func TestClearQueueClearsStatusBarTrackInfo(t *testing.T) {
	sp := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(sp))
	m.statusBar.artist = "Old Artist"
	m.statusBar.title = "Old Title"
	m.statusBar.playlistSource = "Old Playlist"
	m.playback.currentHash = "oldhash"
	m.playback.state = PlaybackPlaying
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.statusBar.artist != "" {
		t.Errorf("statusBar.artist = %q, want empty after ClearQueueMsg", nm.statusBar.artist)
	}
	if nm.statusBar.title != "" {
		t.Errorf("statusBar.title = %q, want empty after ClearQueueMsg", nm.statusBar.title)
	}
	if nm.statusBar.playlistSource != "" {
		t.Errorf("statusBar.playlistSource = %q, want empty after ClearQueueMsg", nm.statusBar.playlistSource)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentTrackHash = %q, want empty after ClearQueueMsg", nm.playback.currentHash)
	}
}

// TestPlaybackEndedClearsStatusBarTrackInfo verifies that PlaybackEndedMsg
// resets the status bar artist, title, playlistSource, and currentTrackHash to
// empty strings when the queue finishes (non-RepeatAll mode).
func TestPlaybackEndedClearsStatusBarTrackInfo(t *testing.T) {
	m := New(context.Background(), WithPlayer(&stubPlayer{}))
	m.statusBar.artist = "Old Artist"
	m.statusBar.title = "Old Title"
	m.statusBar.playlistSource = "Queue"
	m.playback.currentHash = "oldhash"
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatOff

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.statusBar.artist != "" {
		t.Errorf("statusBar.artist = %q, want empty after PlaybackEndedMsg", nm.statusBar.artist)
	}
	if nm.statusBar.title != "" {
		t.Errorf("statusBar.title = %q, want empty after PlaybackEndedMsg", nm.statusBar.title)
	}
	if nm.statusBar.playlistSource != "" {
		t.Errorf("statusBar.playlistSource = %q, want empty after PlaybackEndedMsg", nm.statusBar.playlistSource)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentTrackHash = %q, want empty after PlaybackEndedMsg", nm.playback.currentHash)
	}
}

// TestUndoDequeueEnsuresCursorVisibility verifies that after undoing a dequeue,
// the scroll offset is adjusted so the restored entry (cursor position) is
// within the visible viewport.
func TestUndoDequeueEnsuresCursorVisibility(t *testing.T) {
	m := New(context.Background(), WithPlayer(&stubPlayer{}))
	m.queue.height = 10 // visibleRows = 10 - 4 (reserve) = 6

	// Build a queue with enough entries to scroll.
	entries := make([]QueueEntry, 20)
	for i := range entries {
		entries[i] = QueueEntry{ID: int64(i + 1), Path: "/a.flac", Display: "Track"}
	}
	// Simulate: entry at index 15 was dequeued, so we have indices 0..14,16..19 (19 entries).
	dequeued := entries[15]
	m.queue.entries = slices.Concat(entries[:15], entries[16:])

	// Simulate user scrolled to the top of the queue.
	m.queue.scrollOffset = 0
	m.queue.cursor = 0

	undoer := &stubQueueDequeuer{}
	m.undoer = undoer
	m.undo.slot = undoSlot{
		action:    undoDequeueAction{id: dequeued.ID, entry: dequeued, idx: 15},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if nm.queue.cursor != 15 {
		t.Fatalf("cursor = %d, want 15 after undo dequeue", nm.queue.cursor)
	}
	// cursor (15) must be visible: scrollOffset <= 15 < scrollOffset + visibleRows (6)
	maxRows := nm.queue.visibleRows()
	if nm.queue.cursor < nm.queue.scrollOffset || nm.queue.cursor >= nm.queue.scrollOffset+maxRows {
		t.Errorf("cursor %d not visible: scrollOffset=%d, visibleRows=%d",
			nm.queue.cursor, nm.queue.scrollOffset, maxRows)
	}
}

// TestDequeuePlayingTrackClearsStatusBar verifies that dequeuing the currently
// playing track stops playback and clears the status bar fields.
func TestDequeuePlayingTrackClearsStatusBar(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.statusBar.playlistSource = "Queue"
	m.playback.elapsedSecs = 42.0
	m.playback.totalSecs = 180.0
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123"},
		{ID: 20, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.statusBar.artist != "" {
		t.Errorf("statusBar.artist = %q, want empty", nm.statusBar.artist)
	}
	if nm.statusBar.title != "" {
		t.Errorf("statusBar.title = %q, want empty", nm.statusBar.title)
	}
	if nm.statusBar.playlistSource != "" {
		t.Errorf("statusBar.playlistSource = %q, want empty", nm.statusBar.playlistSource)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentTrackHash = %q, want empty", nm.playback.currentHash)
	}
	if nm.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0", nm.playback.totalSecs)
	}
	if p.stopCalls != 1 {
		t.Errorf("Stop must be called once, got %d calls", p.stopCalls)
	}
}

// TestTickDoesNotUpdateElapsedWhenIdle verifies that the tick handler does not
// overwrite elapsedSecs/totalSecs when playback is idle.
func TestTickDoesNotUpdateElapsedWhenIdle(t *testing.T) {
	p := &stubElapsedReader{elapsed: 99.0, total: 300.0}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackIdle
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0 when idle", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0 when idle", nm.playback.totalSecs)
	}
}

// TestBulkDequeueCreatesUndoSlot verifies that bulk-dequeueing selected entries
// sets up an undoKindBulkDequeue slot with the correct entries, indices, and IDs,
// and that the notification includes the undo hint.
func TestBulkDequeueCreatesUndoSlot(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B", Selected: true},
		{ID: 30, Path: "/c.flac", Display: "C"},
		{ID: 40, Path: "/d.flac", Display: "D", Selected: true},
	}

	// Press 'd' to trigger bulk dequeue.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if len(nm.queue.entries) != 2 {
		t.Fatalf("after bulk dequeue: expected 2 entries, got %d", len(nm.queue.entries))
	}
	if nm.queue.entries[0].ID != 10 || nm.queue.entries[1].ID != 30 {
		t.Fatalf("remaining entries should be ID=10,30; got %d,%d",
			nm.queue.entries[0].ID, nm.queue.entries[1].ID)
	}
	bulkAction, ok := nm.undo.slot.action.(undoBulkDequeueAction)
	if !ok {
		t.Fatalf("expected undoBulkDequeueAction, got %T", nm.undo.slot.action)
	}
	if len(bulkAction.ids) != 2 || bulkAction.ids[0] != 20 || bulkAction.ids[1] != 40 {
		t.Fatalf("bulkDequeueIDs = %v, want [20 40]", bulkAction.ids)
	}
	if len(bulkAction.indices) != 2 || bulkAction.indices[0] != 1 || bulkAction.indices[1] != 3 {
		t.Fatalf("bulkDequeueIndices = %v, want [1 3]", bulkAction.indices)
	}
	if !strings.Contains(nm.statusBar.notification.msg, "undo") {
		t.Fatalf("notification should contain undo hint, got %q", nm.statusBar.notification.msg)
	}
	// Soft-delete must have been called for both IDs.
	if len(d.dequeueTrackSoftCalls) != 2 {
		t.Fatalf("DequeueTrackSoft calls = %d, want 2", len(d.dequeueTrackSoftCalls))
	}
}

// TestUndoBulkDequeueRestoresEntries verifies that undoing a bulk dequeue
// restores all removed entries at their original indices and calls UndoDequeue
// for each ID.
func TestUndoBulkDequeueRestoresEntries(t *testing.T) {
	d := &stubQueueDequeuer{}
	ps := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(ps), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B", Selected: true},
		{ID: 30, Path: "/c.flac", Display: "C"},
		{ID: 40, Path: "/d.flac", Display: "D", Selected: true},
		{ID: 50, Path: "/e.flac", Display: "E"},
	}

	// Bulk dequeue selected entries (IDs 20 and 40).
	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)
	if len(nm.queue.entries) != 3 {
		t.Fatalf("after bulk dequeue: expected 3 entries, got %d", len(nm.queue.entries))
	}

	// Undo within the window.
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 5 {
		t.Fatalf("after undo: expected 5 entries, got %d", len(nm.queue.entries))
	}
	// Verify entries restored at original positions.
	wantIDs := []int64{10, 20, 30, 40, 50}
	for i, want := range wantIDs {
		if nm.queue.entries[i].ID != want {
			t.Errorf("entry[%d].ID = %d, want %d", i, nm.queue.entries[i].ID, want)
		}
	}
	// UndoDequeue must be called for each removed ID.
	if len(d.undoDequeueCallIDs) != 2 {
		t.Fatalf("UndoDequeue calls = %d, want 2", len(d.undoDequeueCallIDs))
	}
	if d.undoDequeueCallIDs[0] != 20 || d.undoDequeueCallIDs[1] != 40 {
		t.Fatalf("UndoDequeue IDs = %v, want [20 40]", d.undoDequeueCallIDs)
	}
	// Player queue must have been synced.
	if len(ps.setQueueCalls) == 0 {
		t.Fatal("expected SetQueue to be called after undo")
	}
	// Undo slot must be cleared.
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful bulk dequeue undo")
	}
}

// TestUndoBulkDequeueExpiredIsNoOp verifies that undoing a bulk dequeue after
// the undo window has expired shows "Nothing to undo" and does not restore entries.
func TestUndoBulkDequeueExpiredIsNoOp(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B", Selected: true},
	}

	// Bulk dequeue.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)
	// Expire the undo window.
	nm.undo.slot.expiresAt = time.Now().Add(-1 * time.Second)

	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("after expired undo: expected 1 entry, got %d", len(nm.queue.entries))
	}
	if len(d.undoDequeueCallIDs) != 0 {
		t.Fatalf("UndoDequeue must not be called after expiry, got %v", d.undoDequeueCallIDs)
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Nothing to undo") {
		t.Fatalf("expected 'Nothing to undo' notification, got %q", nm.statusBar.notification.msg)
	}
}

// TestFocusNowPlayingMovesCursorToCurrentTrack verifies that pressing "f" in
// the queue view moves the cursor to the entry matching currentTrackHash.
func TestFocusNowPlayingMovesCursorToCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Hash: "aaa", Display: "A"},
		{Path: "/music/b.flac", Hash: "bbb", Display: "B"},
		{Path: "/music/c.flac", Hash: "ccc", Display: "C"},
		{Path: "/music/d.flac", Hash: "ddd", Display: "D"},
		{Path: "/music/e.flac", Hash: "eee", Display: "E"},
	}
	m.playback.currentHash = "ccc"
	m.queue.cursor = 0 // cursor starts at top

	next, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	nm := next.(Model)

	if nm.queue.cursor != 2 {
		t.Errorf("f key: expected cursor 2 (track ccc), got %d", nm.queue.cursor)
	}
}

// TestFocusNowPlayingNoOpWhenNothingPlaying verifies that pressing "f" does
// nothing when no track is currently playing (empty currentTrackHash).
func TestFocusNowPlayingNoOpWhenNothingPlaying(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Hash: "aaa", Display: "A"},
		{Path: "/music/b.flac", Hash: "bbb", Display: "B"},
	}
	m.playback.currentHash = ""
	m.queue.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	nm := next.(Model)

	if nm.queue.cursor != 1 {
		t.Errorf("f key with no playing track: expected cursor unchanged at 1, got %d", nm.queue.cursor)
	}
}

// TestDequeueDuplicateHashKeepsPlayback verifies that removing a queue entry
// whose hash matches the currently-playing track does NOT stop playback when
// another entry with the same hash remains in the queue.
func TestDequeueDuplicateHashKeepsPlayback(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123"},
		{ID: 20, Path: "/music/a.flac", Display: "A (dup)", Hash: "abc123"},
		{ID: 30, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	// Remove the first instance — the second instance with the same hash remains.
	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Errorf("playbackState = %v, want PlaybackPlaying (another entry with same hash remains)", nm.playback.state)
	}
	if nm.playback.currentHash != "abc123" {
		t.Errorf("currentHash = %q, want %q", nm.playback.currentHash, "abc123")
	}
	if nm.statusBar.artist != "Artist" {
		t.Errorf("statusBar.artist = %q, want %q", nm.statusBar.artist, "Artist")
	}
	if p.stopCalls != 0 {
		t.Errorf("Stop should not be called when another instance of the hash remains, got %d calls", p.stopCalls)
	}
}

// TestDequeueLastInstanceOfHashStopsPlayback verifies that removing the only
// queue entry with the currently-playing hash stops playback.
func TestDequeueLastInstanceOfHashStopsPlayback(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123"},
		{ID: 20, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentHash = %q, want empty", nm.playback.currentHash)
	}
}

// TestBulkDequeueDuplicateHashKeepsPlayback verifies that bulk-removing
// selected entries does not stop playback when another entry with the
// currently-playing hash remains.
func TestBulkDequeueDuplicateHashKeepsPlayback(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123", Selected: true},
		{ID: 20, Path: "/music/a.flac", Display: "A (dup)", Hash: "abc123"},
		{ID: 30, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	// Press 'd' to trigger bulk dequeue of selected entries.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Errorf("playbackState = %v, want PlaybackPlaying (another entry with same hash remains)", nm.playback.state)
	}
	if nm.playback.currentHash != "abc123" {
		t.Errorf("currentHash = %q, want %q", nm.playback.currentHash, "abc123")
	}
}

// TestBulkDequeueLastInstanceOfHashStopsPlayback verifies that bulk-removing
// all entries with the currently-playing hash stops playback.
func TestBulkDequeueLastInstanceOfHashStopsPlayback(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123", Selected: true},
		{ID: 20, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentHash = %q, want empty", nm.playback.currentHash)
	}
}

// TestUndoEnqueueFindsEntryByIDAfterReorder verifies that undoing an enqueue
// removes the correct entry even if the queue has been reordered since the
// enqueue (i.e. the entry is no longer at its original index).
func TestUndoEnqueueFindsEntryByIDAfterReorder(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Hash: "aaa", Display: "A"},
		{ID: 20, Path: "/b.flac", Hash: "bbb", Display: "B"},
		{ID: 30, Path: "/c.flac", Hash: "ccc", Display: "C"},
	}

	// Simulate an enqueue of ID=30 at index 2, then a manual reorder so
	// that ID=30 moves to index 0.
	m.setUndoSlot(undoEnqueueAction{
		id:    30,
		entry: m.queue.entries[2],
		idx:   2,
	}, nil)

	// Reorder: move ID=30 to front.
	m.queue.entries = []QueueEntry{
		{ID: 30, Path: "/c.flac", Hash: "ccc", Display: "C"},
		{ID: 10, Path: "/a.flac", Hash: "aaa", Display: "A"},
		{ID: 20, Path: "/b.flac", Hash: "bbb", Display: "B"},
	}

	nm := m.handleUndo()

	// ID=30 should be removed, leaving only ID=10 and ID=20.
	if len(nm.queue.entries) != 2 {
		t.Fatalf("expected 2 entries after undo, got %d", len(nm.queue.entries))
	}
	for _, e := range nm.queue.entries {
		if e.ID == 30 {
			t.Fatal("ID=30 should have been removed by undo enqueue")
		}
	}
	// Verify the DB soft-delete targeted the correct ID.
	if len(d.dequeueTrackSoftCalls) != 1 || d.dequeueTrackSoftCalls[0] != 30 {
		t.Fatalf("expected soft-delete of ID=30, got %v", d.dequeueTrackSoftCalls)
	}
}

// TestUndoBatchEnqueueRemovesByIDNotPosition verifies that undoing a batch
// enqueue removes entries by their database IDs, not by positional index range,
// so that reordering after the enqueue does not cause wrong entries to be removed.
func TestUndoBatchEnqueueRemovesByIDNotPosition(t *testing.T) {
	be := &stubBatchEnqueuer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/old.flac", Hash: "old", Display: "Old"},
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
	}

	// Simulate a batch enqueue of IDs 100, 101 starting at index 1.
	m.setUndoSlot(undoBatchEnqueueAction{
		ids:          []int64{100, 101},
		count:        2,
		startIdx:     1,
		playlistName: "Test",
	}, nil)

	// Reorder: move the batch entries to different positions.
	m.queue.entries = []QueueEntry{
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
		{ID: 1, Path: "/old.flac", Hash: "old", Display: "Old"},
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
	}

	nm := m.handleUndo()

	// Only ID=1 should remain.
	if len(nm.queue.entries) != 1 {
		t.Fatalf("expected 1 entry after batch undo, got %d", len(nm.queue.entries))
	}
	if nm.queue.entries[0].ID != 1 {
		t.Fatalf("expected remaining entry to be ID=1, got ID=%d", nm.queue.entries[0].ID)
	}
	// Verify the DB soft-delete targeted the correct IDs.
	if len(be.dequeueTracksSoftIDs) != 1 {
		t.Fatalf("expected 1 batch soft-delete call, got %d", len(be.dequeueTracksSoftIDs))
	}
}
