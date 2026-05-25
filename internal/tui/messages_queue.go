package tui

// DequeueTrackMsg is dispatched when the user presses 'd' in the queue view
// to remove the entry at Index from the playback queue.
type DequeueTrackMsg struct {
	Index int
}

// BulkSelectionToggleMsg is dispatched when the user presses 'x' in the queue
// view to toggle bulk-selection on the entry at Index.
type BulkSelectionToggleMsg struct {
	Index int
}

// ClearQueueMsg is dispatched when the user presses 'C' (Shift+c) in the
// queue view to remove all entries from the playback queue.
type ClearQueueMsg struct{}

// MoveTrackUpMsg is dispatched when the user presses 'K' (Shift+k) in the
// queue view to move the entry at Index one position earlier.
type MoveTrackUpMsg struct {
	Index int
}

// MoveTrackDownMsg is dispatched when the user presses 'J' (Shift+j) in the
// queue view to move the entry at Index one position later.
type MoveTrackDownMsg struct {
	Index int
}

// BatchEnqueuedMsg carries the result of a batch enqueue operation.
type BatchEnqueuedMsg struct {
	Entries      []QueueEntry
	QueueIDs     []int64
	WasEmpty     bool
	PlaylistName string
	Err          error
}

// QueuePastedMsg carries the result of a paste operation.
type QueuePastedMsg struct {
	Entries   []QueueEntry
	InsertPos int     // position in entries slice to insert at
	CutIDs    []int64 // original DB IDs to remove (non-nil for cut-paste)
	Err       error
}

// TransientNotifyMsg carries a short message to display in the status bar.
type TransientNotifyMsg string

// RadioBatchEnqueuedMsg carries the result of a radio auto-enqueue batch.
type RadioBatchEnqueuedMsg struct {
	Entries  []QueueEntry
	QueueIDs []int64
	WasEmpty bool
	Err      error
}
