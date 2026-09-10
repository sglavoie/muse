package db

import (
	"errors"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// radioSourceLabel is the display label written into source_playlist for
// radio-generated rows. It is presentation only: ownership lives in dj_owned,
// because a user may name a playlist "Radio" and every row written before
// migration 024 carries the label with no ownership record at all.
const radioSourceLabel = "Radio"

// Preparation states stored in queue.pending_kind. They mirror the values
// migration 024 documents.
const (
	pendingKindNone = ""
)

// DJPreparationKind distinguishes an ordinary batch appended to the end of the
// queue from a refresh that displaces DJ-owned rows when it is accepted.
type DJPreparationKind string

const (
	// DJPrepareAppend adds tracks and removes nothing.
	DJPrepareAppend DJPreparationKind = "append"
	// DJPrepareReplace adds tracks and retires the DJ-owned rows named by
	// DJPreparation.Replacing in the same transaction.
	DJPrepareReplace DJPreparationKind = "replace"
)

// Queue provenance errors. Callers match them with errors.Is.
var (
	// ErrQueueBatchStale reports a prepared batch that is no longer pending:
	// it was already accepted, explicitly rejected, or converted into an
	// ordinary tombstone by restart recovery. Nothing is changed.
	ErrQueueBatchStale = errors.New("db: prepared queue batch is no longer pending")

	// ErrQueueRowProtected reports an attempt to retire a queue row the DJ does
	// not own — a manual entry, or a row written before provenance existed. The
	// whole accept is refused rather than partially applied.
	ErrQueueRowProtected = errors.New("db: queue row is not DJ-owned")

	// ErrQueuePreparationInvalid reports a preparation the caller built wrong,
	// such as an append that also lists rows to replace.
	ErrQueuePreparationInvalid = errors.New("db: invalid queue preparation")
)

// DJQueueEntry is one planner-selected track together with the structured
// reasons it was chosen. The reasons are persisted with the row so the Preview
// and Queue views can explain a selection after a restart.
type DJQueueEntry struct {
	Track   library.Track
	Reasons radio.Reasons
}

// DJPreparation is the handle for a batch that has been persisted but is not
// yet visible. It is returned by PrepareDJBatch and PrepareDJReplacement and
// consumed by exactly one of AcceptDJBatch or RejectDJBatch.
//
// A preparation is not durable state the caller has to reconcile: if the
// process dies while holding one, restart recovery converts its rows into
// ordinary tombstones and a later accept fails with ErrQueueBatchStale.
type DJPreparation struct {
	// Kind is DJPrepareAppend or DJPrepareReplace.
	Kind DJPreparationKind
	// IDs are the hidden rows, in queue order.
	IDs []int64
	// Replacing are the active DJ-owned rows this batch retires on accept, in
	// the order the caller wants them paired with IDs. Always empty for an
	// append.
	Replacing []int64
}

// DJReplacement pairs a retired queue row with the row that took its place.
// NewID is 0 when the retired row had no replacement, which happens when a
// refresh returns fewer tracks than it displaced.
type DJReplacement struct {
	OldID int64
	NewID int64
}

// DJCommit is the DJ state that must land atomically with an accepted batch.
//
// Session and Allocation are alternatives, not layers: a batch produced by
// newly applied settings commits the whole snapshot, and an ordinary batch
// commits only the discovery carry it advanced. Leave both nil to publish the
// queue rows without touching DJ state.
type DJCommit struct {
	// Session, when non-nil, replaces the active session snapshot — settings,
	// seed, allocation, and both filter tables — in the accept transaction.
	Session *DJSession
	// Allocation, when non-nil and Session is nil, writes only the discovery
	// rounding carry. Pass radio.PlanResult.NextAllocation here; adopting it
	// is what makes a batch "accepted" as far as the planner is concerned.
	Allocation *radio.Allocation
}

// DJAcceptance reports what an accepted batch changed, in queue order.
type DJAcceptance struct {
	// Activated are the rows that became visible.
	Activated []int64
	// Removed are the DJ-owned rows that were soft-deleted. Rows the caller
	// asked to replace that had already been dequeued are not listed: they were
	// gone before the accept and are left alone.
	Removed []int64
	// Replacements pairs each entry of Removed with the row that took its
	// place, so player integration can swap upcoming tracks rather than
	// rebuilding its traversal from visible queue indices.
	Replacements []DJReplacement
}

// DJQueueWriter prepares, accepts, and rejects DJ batches. Preparation persists
// hidden rows; acceptance publishes them together with the DJ state the batch
// advanced, in one transaction.
type DJQueueWriter interface {
	PrepareDJBatch(entries []DJQueueEntry) (DJPreparation, error)
	PrepareDJReplacement(entries []DJQueueEntry, replacing []int64) (DJPreparation, error)
	AcceptDJBatch(prep DJPreparation, commit DJCommit) (DJAcceptance, error)
	RejectDJBatch(prep DJPreparation) error
}

var _ DJQueueWriter = (*Writer)(nil)
