package tui

import (
	"github.com/sglavoie/muse/internal/tui/queue"
)

// BatchEnqueuedMsg carries the result of a batch enqueue operation.
type BatchEnqueuedMsg struct {
	Entries      []queue.Entry
	QueueIDs     []int64
	WasEmpty     bool
	PlaylistName string
	Err          error
}

// QueuePastedMsg carries the result of a paste operation.
type QueuePastedMsg struct {
	Entries   []queue.Entry
	InsertPos int     // position in entries slice to insert at
	CutIDs    []int64 // original DB IDs to remove (non-nil for cut-paste)
	Err       error
}

// RadioBatchEnqueuedMsg carries the result of a radio auto-enqueue batch.
type RadioBatchEnqueuedMsg struct {
	Entries   []queue.Entry
	QueueIDs  []int64
	WasEmpty  bool
	RequestID uint64
	Err       error
}
