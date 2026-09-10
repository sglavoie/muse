package tui

import (
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
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
//
// The unexported fields carry the Auto-DJ half of the result: the plan the batch
// came from, the behavioral evidence it was scored with, the hidden-row handle
// that has to be published or retired, and the fingerprint of the inputs it was
// computed against. They are unexported because their types are, and because
// nothing outside this package has any business constructing one.
type RadioBatchEnqueuedMsg struct {
	Entries           []queue.Entry
	QueueIDs          []int64
	WasEmpty          bool
	RequestID         uint64
	PendingActivation bool
	Err               error

	// plan is the planner result this batch was selected by. Its
	// NextAllocation is adopted only when the batch is accepted.
	plan radio.PlanResult
	// signals records which optional behavioral queries were unavailable, so a
	// degraded score is reported rather than passed off as a full one.
	signals djSignals
	// prep is the DJ preparation handle. Zero when the batch went through one
	// of the pre-DJ writers, which have no handle to hold.
	prep db.DJPreparation
	// fingerprint names the inputs the plan was computed from; acceptance
	// refuses a result whose inputs have moved.
	fingerprint djPlanFingerprint
	// transient marks a failure worth a bounded retry against unchanged
	// inputs, as opposed to one that must wait for an input to move.
	transient bool
}

// djPreviewPlannedMsg carries a draft plan for the Preview tab.
//
// It is a separate message type behind a separate request counter for a reason
// the shared contract makes explicit: previews are drafts, and a draft must not
// be able to retire a live refill. Nothing here reaches the queue, the database,
// or the discovery allocation.
type djPreviewPlannedMsg struct {
	RequestID uint64
	settings  radio.Settings
	result    radio.PlanResult
	signals   djSignals
	err       error
	at        time.Time
}

// radioBatchCleanupMsg reports one compensating delete attempt for a stale
// Radio batch. QueueIDs identifies the exact retained batch being reconciled.
type radioBatchCleanupMsg struct {
	QueueIDs []int64
	Err      error
}

// djRejectRetryMsg reports one compensating RejectDJBatch attempt for a
// preparation whose result was discarded. Prep identifies the exact retained
// preparation being reconciled.
type djRejectRetryMsg struct {
	Prep db.DJPreparation
	Err  error
}
