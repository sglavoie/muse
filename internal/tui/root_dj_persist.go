package tui

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// djPrepared is one planned batch persisted as hidden rows, not yet visible.
type djPrepared struct {
	prep db.DJPreparation
	ids  []int64
	// needsActivation is true when publication is a separate call that can
	// fail, which is also what makes the rows discardable without a trace.
	needsActivation bool
}

// djPersistence is the durable half of a live refill.
//
// Three implementations in descending order of fidelity. The DJ queue writer
// records provenance and commits the discovery carry in the same transaction
// that publishes the rows, so a batch and the state it advanced can never
// disagree. The pending radio enqueuer is the pre-DJ two-phase path, which keeps
// a batch invisible to startup restoration until it is accepted but records no
// ownership. A plain batch enqueuer publishes immediately and is the floor for
// test stubs.
type djPersistence struct {
	dj      djQueueWriter
	pending pendingRadioBatchEnqueuer
	batch   batchEnqueuer
}

// djPersister selects the best writer the Model was built with.
func (m Model) djPersister() djPersistence {
	p := djPersistence{dj: m.djQueue, batch: m.batchEnqueuer}
	if pe, ok := m.batchEnqueuer.(pendingRadioBatchEnqueuer); ok {
		p.pending = pe
	}
	return p
}

// available reports whether a planned batch has anywhere to go.
func (p djPersistence) available() bool { return p.dj != nil || p.batch != nil }

// recordsProvenance reports whether the writer stores DJ ownership and reasons
// on the rows it creates. Only rows that carry it may be claimed as DJ-owned in
// the TUI, or a refresh would try to replace a row the database protects.
func (p djPersistence) recordsProvenance() bool { return p.dj != nil }

// prepare writes the batch as hidden rows. It runs off the update loop.
func (p djPersistence) prepare(picked []radio.PlannedTrack) (djPrepared, error) {
	switch {
	case p.dj != nil:
		prep, err := p.dj.PrepareDJBatch(djQueueEntriesFor(picked))
		if err != nil {
			return djPrepared{}, err
		}
		return djPrepared{prep: prep, ids: prep.IDs, needsActivation: true}, nil
	case p.pending != nil:
		ids, err := p.pending.AppendRadioTracksPending(plannedLibraryTracks(picked))
		if err != nil {
			return djPrepared{}, err
		}
		return djPrepared{ids: ids, needsActivation: true}, nil
	case p.batch != nil:
		ids, err := p.batch.AppendTracks(plannedLibraryTracks(picked), djQueueSource)
		if err != nil {
			return djPrepared{}, err
		}
		return djPrepared{ids: ids}, nil
	}
	return djPrepared{}, errDJPersistenceUnavailable
}

// accept publishes prepared rows together with the state the batch advanced. A
// nil allocation means the batch advanced none, so the session row is left alone.
func (p djPersistence) accept(prep db.DJPreparation, ids []int64, alloc *radio.Allocation) error {
	switch {
	case p.dj != nil && prep.Kind != "":
		_, err := p.dj.AcceptDJBatch(prep, db.DJCommit{Allocation: alloc})
		return err
	case p.pending != nil && prep.Kind == "" && len(ids) > 0:
		return p.pending.ActivateRadioTracks(ids)
	}
	return nil
}

// djQueueEntriesFor pairs each selection with the reasons it was chosen, so the
// explanation is persisted with the row and survives a restart.
func djQueueEntriesFor(picked []radio.PlannedTrack) []db.DJQueueEntry {
	entries := make([]db.DJQueueEntry, len(picked))
	for i, p := range picked {
		entries[i] = db.DJQueueEntry{Track: p.Track, Reasons: p.Reasons}
	}
	return entries
}

// plannedLibraryTracks drops the planner's annotations for the writers that have
// nowhere to record them.
func plannedLibraryTracks(picked []radio.PlannedTrack) []library.Track {
	tracks := make([]library.Track, len(picked))
	for i, p := range picked {
		tracks[i] = p.Track
	}
	return tracks
}

// newDJQueueEntries builds the in-memory rows for an accepted batch. Ownership
// and reasons are only claimed when the writer actually recorded them, so the
// TUI never shows a refreshable row the database would refuse to retire.
func newDJQueueEntries(picked []radio.PlannedTrack, ids []int64, owned bool) []queue.Entry {
	entries := newQueueEntries(plannedLibraryTracks(picked), ids, djQueueSource)
	if !owned {
		return entries
	}
	for i := range entries {
		entries[i].DJOwned = true
		entries[i].DJReasons = picked[i].Reasons
	}
	return entries
}
