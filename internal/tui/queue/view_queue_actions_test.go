package queue

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// stubFileExistsTrue replaces the package-level fileExists check with
// one that always returns true for the duration of the test. Tests that
// exercise actionSelect with synthetic paths use this helper so the
// pre-flight stat does not short-circuit Enter → PlayQueueIndexMsg into
// the missing-track removal path.
func stubFileExistsTrue(t *testing.T) {
	t.Helper()
	saved := FileExists
	FileExists = func(string) bool { return true }
	t.Cleanup(func() { FileExists = saved })
}

// TestMarkMissingFlagsAllDuplicateEntries verifies that markMissing marks
// every entry with the matching path, not just the first one.
func TestMarkMissingFlagsAllDuplicateEntries(t *testing.T) {
	q := View{
		Entries: []Entry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
			{Path: "/music/a.flac", Display: "A (dup)"},
		},
	}
	q.MarkMissing("/music/a.flac")

	if !q.Entries[0].Missing {
		t.Error("first entry with matching path must be marked missing")
	}
	if q.Entries[1].Missing {
		t.Error("entry with different path must not be marked missing")
	}
	if !q.Entries[2].Missing {
		t.Error("second entry with matching path must be marked missing")
	}
}

// TestQueueSelectNoPanicWhenCursorOutOfBounds verifies that the actionSelect
// handler in View does not panic when the cursor exceeds the entries slice.
func TestQueueSelectNoPanicWhenCursorOutOfBounds(t *testing.T) {
	q := View{Entries: []Entry{{Path: "/a.flac", Display: "A"}}, Cursor: 5}
	_, cmd := q.HandleAction(keys.ActionSelect)
	if cmd != nil {
		t.Fatal("actionSelect with out-of-bounds cursor must return nil cmd")
	}
}

// TestQueueDequeueNoPanicWhenCursorOutOfBounds verifies that the actionDequeue
// handler in View does not panic when the cursor exceeds the entries slice.
func TestQueueDequeueNoPanicWhenCursorOutOfBounds(t *testing.T) {
	q := View{Entries: []Entry{{Path: "/a.flac", Display: "A"}}, Cursor: 5}
	_, cmd := q.HandleAction(keys.ActionDequeue)
	if cmd != nil {
		t.Fatal("actionDequeue with out-of-bounds cursor must return nil cmd")
	}
}

func TestQueueHandleActionCursorUpDown(t *testing.T) {
	q := View{Entries: make([]Entry, 5), Cursor: 0, Width: 80}
	q, _ = q.HandleAction(keys.ActionCursorDown)
	if q.Cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", q.Cursor)
	}
	q, _ = q.HandleAction(keys.ActionCursorDown)
	if q.Cursor != 2 {
		t.Errorf("CursorDown: expected 2, got %d", q.Cursor)
	}
	q, _ = q.HandleAction(keys.ActionCursorUp)
	if q.Cursor != 1 {
		t.Errorf("CursorUp: expected 1, got %d", q.Cursor)
	}
}

func TestQueueHandleActionCursorClampsAtBounds(t *testing.T) {
	q := View{Entries: make([]Entry, 3), Cursor: 0, Width: 80}
	q, _ = q.HandleAction(keys.ActionCursorUp)
	if q.Cursor != 0 {
		t.Errorf("CursorUp at 0: expected 0, got %d", q.Cursor)
	}
	q.Cursor = 2
	q, _ = q.HandleAction(keys.ActionCursorDown)
	if q.Cursor != 2 {
		t.Errorf("CursorDown at end: expected 2, got %d", q.Cursor)
	}
}

func TestQueueHandleActionGoToTopBottom(t *testing.T) {
	q := View{Entries: make([]Entry, 10), Cursor: 5, Width: 80}
	q, _ = q.HandleAction(keys.ActionGoToTop)
	if q.Cursor != 0 || q.ScrollOffset != 0 {
		t.Errorf("GoToTop: cursor=%d scroll=%d, want 0,0", q.Cursor, q.ScrollOffset)
	}
	q, _ = q.HandleAction(keys.ActionGoToBottom)
	if q.Cursor != 9 {
		t.Errorf("GoToBottom: expected cursor=9, got %d", q.Cursor)
	}
}

func TestQueueHandleActionHalfPageDownUp(t *testing.T) {
	// visibleRows = height(14) - reserve(3) = 11; HalfPageDown = 11/2 = 5
	q := View{Entries: make([]Entry, 20), Cursor: 0, Height: 14, Width: 80}
	q, _ = q.HandleAction(keys.ActionHalfPageDown)
	if q.Cursor != 5 {
		t.Errorf("HalfPageDown: expected 5, got %d", q.Cursor)
	}
	q, _ = q.HandleAction(keys.ActionHalfPageUp)
	if q.Cursor != 0 {
		t.Errorf("HalfPageUp: expected 0, got %d", q.Cursor)
	}
}

func TestQueueHandleActionFullPageDownUp(t *testing.T) {
	// visibleRows = height(14) - reserve(3) = 11; FullPageDown = 11
	q := View{Entries: make([]Entry, 20), Cursor: 0, Height: 14, Width: 80}
	q, _ = q.HandleAction(keys.ActionFullPageDown)
	if q.Cursor != 11 {
		t.Errorf("FullPageDown: expected 11, got %d", q.Cursor)
	}
	q, _ = q.HandleAction(keys.ActionFullPageUp)
	if q.Cursor != 0 {
		t.Errorf("FullPageUp: expected 0, got %d", q.Cursor)
	}
}

func TestQueueHandleActionClearQueue(t *testing.T) {
	q := View{Entries: make([]Entry, 3)}
	_, cmd := q.HandleAction(keys.ActionClearQueue)
	if cmd == nil {
		t.Fatal("ClearQueue: expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(ClearQueueMsg); !ok {
		t.Errorf("ClearQueue: expected ClearQueueMsg, got %T", msg)
	}
}

func TestQueueHandleActionMoveTrackUp(t *testing.T) {
	q := View{Entries: make([]Entry, 3), Cursor: 1}
	_, cmd := q.HandleAction(keys.ActionMoveTrackUp)
	if cmd == nil {
		t.Fatal("MoveTrackUp: expected non-nil cmd")
	}
	msg := cmd()
	if m, ok := msg.(MoveTrackUpMsg); !ok || m.Index != 1 {
		t.Errorf("MoveTrackUp: got %+v", msg)
	}
}

func TestQueueHandleActionMoveTrackDown(t *testing.T) {
	q := View{Entries: make([]Entry, 3), Cursor: 1}
	_, cmd := q.HandleAction(keys.ActionMoveTrackDown)
	if cmd == nil {
		t.Fatal("MoveTrackDown: expected non-nil cmd")
	}
	msg := cmd()
	if m, ok := msg.(MoveTrackDownMsg); !ok || m.Index != 1 {
		t.Errorf("MoveTrackDown: got %+v", msg)
	}
}

// TestQueueBulkSelectionToggleNoPanicWhenCursorOutOfBounds verifies that the
// actionBulkSelectionToggle handler does not panic when cursor is out of bounds.
func TestQueueBulkSelectionToggleNoPanicWhenCursorOutOfBounds(t *testing.T) {
	q := View{Entries: []Entry{{Path: "/a.flac", Display: "A"}}, Cursor: 5}
	_, cmd := q.HandleAction(keys.ActionBulkSelectionToggle)
	if cmd != nil {
		t.Fatal("actionBulkSelectionToggle with out-of-bounds cursor must return nil cmd")
	}
}

// TestHandleActionSelectUsesOrigIdx verifies that actionSelect dispatches
// PlayQueueIndexMsg with the correct underlying entry index when a display
// order is active.
func TestHandleActionSelectUsesOrigIdx(t *testing.T) {
	stubFileExistsTrue(t)
	v := View{
		Entries: []Entry{
			{ID: 1, Path: "/a", Hash: "ha", Title: "A", Artist: "X"},
			{ID: 2, Path: "/b", Hash: "hb", Title: "B", Artist: "Y"},
			{ID: 3, Path: "/c", Hash: "hc", Title: "C", Artist: "Z"},
		},
		DisplayOrder: []int{2, 1, 0},
		Cursor:       0,
		Height:       20,
	}
	v2, cmd := v.HandleAction(keys.ActionSelect)
	_ = v2
	if cmd == nil {
		t.Fatal("actionSelect should return a command")
	}
	msg := cmd()
	pqm, ok := msg.(playback.PlayQueueIndexMsg)
	if !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
	if pqm.Index != 2 {
		t.Errorf("PlayQueueIndexMsg.Index = %d, want 2 (underlying entry index)", pqm.Index)
	}
}

// TestQueueViewHandleActionEnterSendsPlayQueueIndexMsg verifies that
// actionSelect returns a Cmd that emits PlayQueueIndexMsg with the cursor
// index. Root routes Enter -> actionSelect via the registry and calls
// HandleAction, so this is the code path exercised in production.
func TestQueueViewHandleActionEnterSendsPlayQueueIndexMsg(t *testing.T) {
	stubFileExistsTrue(t)
	q := View{
		Entries: []Entry{
			{Path: "/music/a.flac", Display: "Artist - A"},
			{Path: "/music/b.flac", Display: "Artist - B"},
		},
		Cursor: 1,
	}
	_, cmd := q.HandleAction(keys.ActionSelect)
	if cmd == nil {
		t.Fatal("actionSelect must return a non-nil cmd")
	}
	msg := cmd()
	pqi, ok := msg.(playback.PlayQueueIndexMsg)
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
	q := View{}
	_, cmd := q.HandleAction(keys.ActionSelect)
	if cmd != nil {
		t.Fatal("actionSelect on empty queue must return nil cmd")
	}
}

// TestQueueViewUpdateDSendsDequeueTrackMsg verifies that actionDequeue
// dispatches DequeueTrackMsg with the current cursor index.
func TestQueueViewUpdateDSendsDequeueTrackMsg(t *testing.T) {
	q := View{
		Entries: []Entry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		Cursor: 1,
	}
	_, cmd := q.HandleAction(keys.ActionDequeue)
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
	q := View{}
	_, cmd := q.HandleAction(keys.ActionDequeue)
	if cmd != nil {
		t.Fatal("actionDequeue on empty queue must return nil cmd")
	}
}

// TestQueueViewUpdateXSendsBulkSelectionToggleMsg verifies that
// actionBulkSelectionToggle dispatches BulkSelectionToggleMsg with the current
// cursor index.
func TestQueueViewUpdateXSendsBulkSelectionToggleMsg(t *testing.T) {
	q := View{
		Entries: []Entry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		Cursor: 0,
	}
	_, cmd := q.HandleAction(keys.ActionBulkSelectionToggle)
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
	q := View{
		Entries: []Entry{
			{Path: "/music/a.flac", Display: "A"},
		},
	}
	_, cmd := q.HandleAction(keys.ActionClearQueue)
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
	q := View{
		Entries: []Entry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		Cursor: 1,
	}
	_, cmd := q.HandleAction(keys.ActionMoveTrackUp)
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
	q := View{
		Entries: []Entry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		Cursor: 0,
	}
	_, cmd := q.HandleAction(keys.ActionMoveTrackUp)
	if cmd != nil {
		t.Fatal("actionMoveTrackUp at top entry must return nil cmd")
	}
}

// TestQueueViewUpdateJSendsMoveTrackDownMsg verifies that actionMoveTrackDown
// dispatches MoveTrackDownMsg with the current cursor index.
func TestQueueViewUpdateJSendsMoveTrackDownMsg(t *testing.T) {
	q := View{
		Entries: []Entry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		Cursor: 0,
	}
	_, cmd := q.HandleAction(keys.ActionMoveTrackDown)
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
	q := View{
		Entries: []Entry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		Cursor: 1,
	}
	_, cmd := q.HandleAction(keys.ActionMoveTrackDown)
	if cmd != nil {
		t.Fatal("actionMoveTrackDown at bottom entry must return nil cmd")
	}
}
