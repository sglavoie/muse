package tui

import (
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/sglavoie/muse/internal/radio"
)

// djNoteAllocation records a carry the accept transaction already persisted.
//
// The DJ queue writer commits the allocation in the same transaction that
// publishes the batch, so re-issuing djPersist here would rewrite the whole
// session row — recipe, filters and all — for a column that is already correct,
// and would do it from a Model whose settings may have moved on since.
func (m Model) djNoteAllocation(alloc radio.Allocation) Model {
	if !m.dj.active {
		return m
	}
	m.dj.allocation = alloc.Normalized()
	return m
}

// djNoteTrackChanged re-anchors a Follow session on a confirmed track change.
//
// Confirmed is the operative word. This is called from handleTrackChangedMsg
// after every stale-event guard has passed, so the seed moves only for the row
// the player says it is actually on — never for a start-file a newer selection
// has already superseded, and never for an optimistic index the TUI guessed.
// Under AnchorSeed nothing here fires at all: the captured starting context is
// what an anchored session is.
func (m Model) djNoteTrackChanged(hash string, now time.Time) Model {
	if !m.dj.active || m.dj.settings.Anchor != radio.AnchorFollow || hash == "" {
		return m
	}
	if hash == m.dj.seed.TrackHash {
		return m
	}
	track, ok := m.library.Index.FindByHash(hash)
	if !ok {
		// A row outside the library index — a queue entry whose file has gone
		// missing — carries no metadata to score against. Keeping the previous
		// seed scores the next batch against something real instead of against
		// a blank.
		return m
	}
	return m.djReseed(radio.CaptureSeed(track, m.trackTags[hash], radio.FixedClock(now)))
}

// djNoteTagMerged re-points the live session's tag references after a merge,
// and writes them back.
//
// The merge remapped the durable rows inside its own transaction
// (db.remapDJTagRefs), and the Model is the second copy of the same references —
// the one every later whole-session write is rebuilt from. A session left
// holding the merged-away id would plan against a tag the catalog no longer
// offers, parking the refill as unresolved, so the live remap above is the first
// half of the repair.
//
// The write is the other half, and it is needed because the merge transaction
// commits off the update loop, strictly ahead of the message that lands here.
// Anything that persists the whole row in that window — a Follow reseed, an
// Apply, the tick reconciling an outstanding refusal — rebuilds dj_tag_refs from
// a Model that still names the source tag, and replaceDJFilters takes a positive
// id whether or not a tag still carries it. The durable remap is then gone, with
// nothing left to notice: the live state here is correct, so no unsaved marker is
// raised, and the row stays stale until some unrelated later write happens to fix
// it — or until a restart restores an unresolved filter.
//
// What is written is the state now in force, not anything captured earlier, so a
// settings, seed, or allocation change made in that same window is preserved
// rather than rolled back. A refusal takes the ordinary path: recorded, reported,
// and re-attempted by djReconcilePersist under its bounded budget, which rewrites
// this remapped snapshot.
//
// A merge that moved no reference this session holds writes nothing. The durable
// rows can only have gone stale if this Model was the one handing the source id
// back, so a session that never named it has nothing to reconcile — which covers
// a refused restore too, whose stock defaults hold no tag references at all.
//
// Nothing is superseded from here either. The merge moves the data revision
// through djInvalidate and it moves the applied settings, and either one retires
// a batch in flight through the ordinary acceptance check, which relaunches. A
// supersede would cancel instead, and a tag edit is not the user asking the DJ
// to stop.
func (m Model) djNoteTagMerged(srcID, destID int64) Model {
	settings := m.dj.settings.RemapTag(srcID, destID)
	seed := m.dj.seed.RemapTag(srcID, destID)
	moved := !settings.Equal(m.dj.settings) || !slices.Equal(seed.TagIDs, m.dj.seed.TagIDs)
	m.dj.settings, m.dj.seed = settings, seed
	if !moved {
		return m
	}
	return m.djPersist()
}

// djRepairMergedTagRefs re-points every stored DJ reference the merge moved,
// once more, after the completion has landed.
//
// The merge transaction already did this, and for the session row djNoteTagMerged
// above is enough: it rewrites that one row from the state now in force. What
// neither covers is the *recipe* rows. The confirmation is answered off the
// update loop, so MergeTags commits strictly before the message that lands here,
// and a Save-as, a Rename, or an Update submitted in that gap hands storage a
// settings snapshot frozen when its prompt opened — before the merge. Storage
// rewrites a recipe's whole filter set from that snapshot, and replaceDJFilters
// stores a positive id whether or not a tag still carries it, so the durable
// remap is replaced by the merged-away reference and nothing notices: the write
// succeeded, the panel is told so, and the recipe reopens on a filter that
// resolves to nothing.
//
// Repairing rather than refusing is deliberate. The user's gesture is their own
// and lands whole — the typed name, the recipe it named, the unrelated filters —
// and only the reference the merge has already decided about is moved, which is
// what the merge means. It is also what covers every other write that could land
// in that gap, including ones added later, because it re-points the rows as they
// now are instead of guarding one route into them.
func (m Model) djRepairMergedTagRefs(srcID, destID int64) Model {
	if srcID == destID || srcID <= 0 || destID <= 0 {
		return m
	}
	s := m.dj.staleRefs
	// A newer merge queues behind any repair still owed, so the replay stays in
	// the order the merges happened, and it is owed the same fresh budget a
	// deliberate gesture gets.
	s.merges = append(append([]djTagMerge(nil), s.merges...), djTagMerge{src: srcID, dest: destID})
	s.attempts, s.err = 0, nil
	m.dj.staleRefs = s
	return m.djFlushTagRefRepairs(false)
}

// djFlushTagRefRepairs replays the outstanding repairs, oldest first, and stops
// at the first refusal so the order is never broken.
//
// retry marks the automatic path. A refusal there is reported only once the
// bounded budget runs out, because the tick is not a gesture the user made and a
// transient lock would otherwise raise a notice every second.
func (m Model) djFlushTagRefRepairs(retry bool) Model {
	s := m.dj.staleRefs
	if !s.pending() {
		return m
	}
	if m.djWriter == nil {
		// No writer means no stored references to repair: this build keeps none.
		m.dj.staleRefs = djStaleTagRefs{}
		return m
	}
	for len(s.merges) > 0 {
		merge := s.merges[0]
		if err := m.djWriter.RemapDJTagRefs(merge.src, merge.dest); err != nil {
			s.err, s.attempts = err, s.attempts+1
			m.dj.staleRefs = s
			slog.Warn("dj session: repair merged tag refs", "err", err, "attempt", s.attempts)
			if !retry || s.exhausted() {
				m.statusBar.NotifyError("Auto-DJ filters still name the merged tag: " + err.Error())
			}
			return m
		}
		s.merges = s.merges[1:]
	}
	m.dj.staleRefs = djStaleTagRefs{}
	if retry {
		m.statusBar.NotifyInfo("Auto-DJ filters updated for the merged tag")
	}
	return m
}

// djReconcileTagRefs re-attempts a refused reference repair on the tick.
//
// It is the bounded path the refusal is kept for, and it runs here for the same
// reason djReconcilePersist does: the divergence is invisible until a restart,
// and waiting for the user's next merge is waiting for a gesture that may never
// come.
func (m Model) djReconcileTagRefs() Model {
	s := m.dj.staleRefs
	if !s.pending() || s.exhausted() {
		return m
	}
	return m.djFlushTagRefRepairs(true)
}

// djPersist writes the whole snapshot. The in-memory state is kept on failure —
// refusing the transition would leave the user looking at a DJ that ignored
// their keypress — but the refusal is recorded rather than only logged, because
// the settings in force and the settings on disk have just diverged and nothing
// else would notice until the next launch restored the old ones.
func (m Model) djPersist() Model {
	if m.djWriter == nil {
		return m
	}
	if err := m.djWriter.WriteDJSession(m.djSnapshot()); err != nil {
		slog.Warn("dj session: write", "err", err)
		return m.noteDJWriteRefused(err, true)
	}
	return m.noteDJWriteLanded()
}

// djPersistStop clears the active flag and the stored seed in one statement.
//
// A stop that lands does not reconcile a snapshot write that did not: it leaves
// the settings columns alone, so a stale settings row stays stale and stays
// outstanding for the reconciliation to rewrite whole.
func (m Model) djPersistStop() Model {
	if m.djWriter == nil {
		return m
	}
	if err := m.djWriter.StopDJSession(); err != nil {
		slog.Warn("dj session: stop", "err", err)
		return m.noteDJWriteRefused(err, m.dj.unsaved.snapshot)
	}
	if m.dj.unsaved.snapshot {
		return m
	}
	return m.noteDJWriteLanded()
}

// noteDJWriteRefused records a refused lifecycle write and says so.
//
// The status bar is where a start, a stop, or a reseed reports: those gestures
// have no panel open to answer to, and a DJ whose session silently stopped being
// saved is exactly the failure the user finds out about after a restart.
func (m Model) noteDJWriteRefused(err error, snapshot bool) Model {
	m.dj.unsaved = djUnsavedWrite{err: err, snapshot: snapshot}
	m.statusBar.NotifyError("Auto-DJ not saved: " + err.Error())
	return m
}

// noteDJWriteLanded clears an outstanding refusal after a write that succeeded.
func (m Model) noteDJWriteLanded() Model {
	if !m.dj.unsaved.pending() {
		return m
	}
	m.dj.unsaved = djUnsavedWrite{}
	m.statusBar.NotifyInfo("Auto-DJ session saved")
	return m
}

// djUnsavedErr reports state this Model holds and the database does not, or nil.
//
// This is what stops an Apply from being acknowledged as a durable one. The
// wording is the panel's, which renders the error verbatim.
func (m Model) djUnsavedErr() error {
	if !m.dj.unsaved.pending() {
		return nil
	}
	return fmt.Errorf("%w: %w", errDJUnsaved, m.dj.unsaved.err)
}

// djReconcilePersist re-attempts a refused lifecycle write.
//
// This is the recoverable path the refusal is kept for, and it runs on the tick
// rather than on the user's next lifecycle gesture: that gesture may never come,
// and until it does the divergence is invisible — the session plays on the
// settings the user chose and only a restart reveals that the row never took
// them.
//
// The whole row is rewritten from the state actually in force, which reconciles
// a refused stop as well: a stopped snapshot clears the same seed and carry
// columns StopDJSession would have, and rewrites the settings a stop leaves
// behind.
func (m Model) djReconcilePersist() Model {
	u := m.dj.unsaved
	if !u.pending() || u.exhausted() || m.djWriter == nil {
		return m
	}
	if err := m.djWriter.WriteDJSession(m.djSnapshot()); err != nil {
		u.err, u.attempts = err, u.attempts+1
		u.snapshot = true
		m.dj.unsaved = u
		slog.Warn("dj session: reconcile", "err", err, "attempt", u.attempts)
		if u.exhausted() {
			m.statusBar.NotifyError("Auto-DJ session could not be saved: " + err.Error())
		}
		return m
	}
	return m.noteDJWriteLanded()
}
