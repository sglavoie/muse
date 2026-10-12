package tui

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// refreshSlots returns the upcoming rows this refresh may take over, in the
// order the traversal will reach them.
//
// Two filters, and both matter. The player's eligible set already excludes the
// row playing, played history, the forward history a Previous left behind, and
// anything the cycle no longer schedules. DJ ownership excludes the rest of what
// the user built: a manual entry is theirs, and a row written before provenance
// existed cannot be proven otherwise, so neither is replaceable.
func (r djRefreshRequest) refreshSlots(snap player.RefreshSnapshot) []player.RefreshRow {
	slots := make([]player.RefreshRow, 0, len(snap.Eligible))
	for _, row := range snap.Eligible {
		if row.QueueID == 0 {
			continue
		}
		if entry, ok := r.rows[row.QueueID]; !ok || !entry.DJOwned {
			continue
		}
		slots = append(slots, row)
	}
	return slots
}

// protectedFor returns the hashes the plan must not select again: the row
// playing now plus every row still ahead of it, minus the slots being replaced.
//
// Excluding the replaced slots is the difference between a refresh and a refill.
// Their tracks are on their way out, so protecting them would forbid the planner
// from ever re-picking a track the new settings still favour — and with a small
// filtered library that is most of what it has to work with.
func (r djRefreshRequest) protectedFor(
	snap player.RefreshSnapshot, slots []player.RefreshRow,
) map[string]struct{} {
	replaced := make(map[int64]bool, len(slots))
	for _, slot := range slots {
		replaced[slot.QueueID] = true
	}
	protected := make(map[string]struct{}, radioBatchSize)
	add := func(row player.RefreshRow) {
		if row.Hash == "" || replaced[row.QueueID] {
			return
		}
		protected[row.Hash] = struct{}{}
	}
	if snap.HasCurrent() {
		add(snap.Current)
	}
	for _, row := range snap.Protected {
		add(row)
	}
	for _, row := range snap.Eligible {
		add(row)
	}
	return protected
}

// refreshedQueue rebuilds the whole queue the refresh publishes: the snapshot's
// rows in visible order minus the retired slots, then the successors at the
// tail, which is where the accept transaction puts them.
//
// A live row this Model has no entry for is reported as staleness rather than
// guessed at. It means the queue moved between the capture the Model made and
// the one the player just took, and the next attempt captures both again.
func (r djRefreshRequest) refreshedQueue(
	snap player.RefreshSnapshot, slots []player.RefreshRow, added []queue.Entry,
) ([]player.QueueTrack, error) {
	retired := make(map[int64]bool, len(slots))
	for _, slot := range slots {
		retired[slot.QueueID] = true
	}
	rows := djRefreshOrderedRows(snap)
	tracks := make([]player.QueueTrack, 0, len(rows)+len(added))
	for _, row := range rows {
		if row.QueueID != 0 && retired[row.QueueID] {
			continue
		}
		entry, ok := r.rows[row.QueueID]
		if !ok {
			return nil, fmt.Errorf("%w: queue row %d is not one this refresh captured",
				player.ErrRefreshStale, row.QueueID)
		}
		tracks = append(tracks, queueTrackFromEntry(entry))
	}
	for _, entry := range added {
		tracks = append(tracks, queueTrackFromEntry(entry))
	}
	return tracks, nil
}

// djRefreshOrderedRows flattens a snapshot back into visible queue order. The
// classification splits the queue by role; publishing it needs the order the
// durable rows are actually in.
func djRefreshOrderedRows(snap player.RefreshSnapshot) []player.RefreshRow {
	rows := make([]player.RefreshRow, 0,
		1+len(snap.Played)+len(snap.Protected)+len(snap.Eligible)+len(snap.Unscheduled))
	if snap.HasCurrent() {
		rows = append(rows, snap.Current)
	}
	rows = append(rows, snap.Played...)
	rows = append(rows, snap.Protected...)
	rows = append(rows, snap.Eligible...)
	rows = append(rows, snap.Unscheduled...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Index < rows[j].Index })
	return rows
}

// djRefreshReplacements pairs each slot with the row that takes it over, in the
// order the accept transaction pairs them. A slot past the end of the batch is
// retired with an empty successor, which is what a plan shorter than the queue
// it displaced reports.
func djRefreshReplacements(slots []player.RefreshRow, ids []int64) []player.RowReplacement {
	reps := make([]player.RowReplacement, len(slots))
	for i, slot := range slots {
		reps[i] = player.RowReplacement{Replaced: slot.Identity}
		if i < len(ids) {
			reps[i].Successor = player.QueueRowIdentity(ids[i])
		}
	}
	return reps
}

// withProtected returns a copy of the snapshot planning against a different
// protected set, with the revision token that describes it. The two must move
// together or a fingerprint would claim inputs the plan never used.
func (in djInputs) withProtected(protected map[string]struct{}) djInputs {
	in.protected = protected
	in.queueRev = djQueueRevisionOf(protected)
	return in
}

// handleDJRefreshDoneMsg files the outcome of one immediate refresh.
//
// The order is the argument. Abandoned preparations are retired first, because
// hidden rows outliving their result is the one failure that compounds: the next
// accept naming them would republish a batch nobody asked for. What follows
// splits on the only question that matters — whether a durable transaction
// landed — and then on whether this result is still the one in force.
func (m Model) handleDJRefreshDoneMsg(msg djRefreshDoneMsg) (Model, tea.Cmd) {
	for _, prep := range msg.discarded {
		m = m.rejectDJPreparation(prep)
	}
	if msg.RequestID != m.djRefresh.id {
		return m.reconcileLateDJRefresh(msg), nil
	}

	m.djRefresh.pending = false
	m.djRefresh.withdraw = nil
	if !msg.valid {
		m, cmd := m.recordDJRefreshFailure(msg, time.Now())
		// A failure that relaunched is not an answer yet. Acknowledging it here
		// would release the Tune panel's wait on a refresh still in flight and
		// let a second Apply race it; the relaunched refresh answers instead,
		// and the retry budget is what guarantees one eventually does.
		if m.djRefresh.pending {
			return m, cmd
		}
		return m.ackAutoDJRefresh(msg.err), cmd
	}
	m = m.adoptDJRefreshRows(msg.committed)
	return m.adoptDJRefreshState(msg, time.Now()).ackAutoDJRefresh(nil), nil
}

// reconcileLateDJRefresh answers for a result a disable, a clear, a reseed, or a
// newer Apply has already superseded.
//
// Nothing here touches settings, the seed, the discovery carry, the recent ring,
// or the planning block: superseding is the user's newer decision, and a late
// result must not be able to undo it. What it does have to answer for is the
// durable state a committed transaction left behind, which the supersession
// could not withdraw because it arrived too late to.
func (m Model) reconcileLateDJRefresh(msg djRefreshDoneMsg) Model {
	if !msg.valid {
		return m
	}
	// A queue that no longer holds anything this refresh replaced is not the
	// queue it described — it was cleared or rebuilt after the commit. Adopting
	// the successors there would resurrect rows into a queue the user emptied,
	// so they are reconciled away instead.
	if !m.queueHoldsAnySlot(msg.committed.slots) {
		m = m.discardRadioBatch(msg.committed.prep.IDs)
	} else {
		m = m.adoptDJRefreshRows(msg.committed)
		for _, entry := range msg.committed.entries {
			m = m.radioNoteRecent(entry.Hash)
		}
	}
	// The accept transaction may have written a discovery carry belonging to
	// the settings that have since been replaced. Reasserting the session row
	// from the state actually in force is what puts the newer decision back on
	// disk, whichever order the two writes landed in.
	if m.dj.active {
		return m.djPersist()
	}
	return m.djPersistStop()
}

// queueHoldsAnySlot reports whether the queue still holds a row this refresh
// intended to replace.
func (m Model) queueHoldsAnySlot(slots []player.RefreshRow) bool {
	if len(slots) == 0 {
		return false
	}
	held := make(map[int64]bool, len(slots))
	for _, slot := range slots {
		held[slot.QueueID] = true
	}
	for _, entry := range m.queue.Entries {
		if entry.ID != 0 && held[entry.ID] {
			return true
		}
	}
	return false
}

// adoptDJRefreshRows brings the visible queue in line with the queue the player
// and the database now hold: the retired rows out, the successors at the tail.
//
// The playing index is corrected by counting what left in front of it rather
// than by trusting the player's report, because the two lists are only the same
// list while nothing else has touched either — and a manual enqueue between the
// publication and this message is enough to make them differ by one.
func (m Model) adoptDJRefreshRows(att djRefreshAttempt) Model {
	retired := make(map[int64]bool, len(att.slots))
	for _, slot := range att.slots {
		retired[slot.QueueID] = true
	}
	kept := make([]queue.Entry, 0, len(m.queue.Entries)+len(att.entries))
	before := 0
	for i, entry := range m.queue.Entries {
		if entry.ID != 0 && retired[entry.ID] {
			if i < m.playback.currentIndex {
				before++
			}
			continue
		}
		kept = append(kept, entry)
	}
	kept = append(kept, att.entries...)
	m.queue.Entries = kept
	if m.playback.currentIndex >= 0 {
		m.playback.currentIndex -= before
	}
	m.clampQueueCursorAndScroll()
	return m.preserveQueueSort()
}

// adoptDJRefreshState records what an adopted refresh advanced.
//
// Only here do the recent-selection ring and the discovery carry move, for the
// same reason they only move in applyRadioBatch: this is the point at which the
// selections are something the user will actually hear. The carry is noted
// rather than persisted — the accept transaction already committed it in the
// same statement that published the rows.
func (m Model) adoptDJRefreshState(msg djRefreshDoneMsg, now time.Time) Model {
	att := msg.committed
	for _, entry := range att.entries {
		m = m.radioNoteRecent(entry.Hash)
	}
	if att.plan.CanContinue() {
		m = m.djNoteAllocation(att.plan.NextAllocation)
	}
	m.djRefresh.retries = 0
	// Counted from the committed attempt rather than from the publication,
	// because the durable transaction is what the refresh is: a recovery reports
	// only its current index, having adopted the committed queue wholesale, and
	// the publication counts the same pairing this attempt named anyway.
	replaced := min(len(att.entries), len(att.slots))
	m.djRefresh.status = djRefreshStatus{
		outcome:   att.plan.Outcome,
		shortages: att.plan.Shortages,
		reasons:   att.plan.Reasons,
		degraded:  att.signals.Failures(),
		replaced:  replaced,
		retired:   len(att.slots) - replaced,
		recovered: msg.res.Recovered,
		at:        now,
	}
	if msg.res.Recovered {
		slog.Warn("dj refresh: publication recovered onto the committed queue", "err", msg.res.Cause)
	}
	m.djPlan.status = djPlanStatus{
		outcome:   att.plan.Outcome,
		shortages: att.plan.Shortages,
		reasons:   att.plan.Reasons,
		degraded:  att.signals.Failures(),
		accepted:  len(att.entries),
		at:        now,
	}

	// A refresh that retired its slots and added nothing has just told the user
	// their settings admit no candidates. Parking the refill on the queue as it
	// now stands is what stops the next tick from asking the same question a
	// second later and appending the answer the refresh declined to add.
	if !att.plan.CanContinue() {
		m.djPlan.block = djPlanBlock{
			active:  true,
			at:      m.djLiveFingerprint(now),
			reasons: att.plan.Reasons,
		}
		m.djPlan.staleRetries = 0
		return m
	}
	m.djPlan.block = djPlanBlock{}
	m.djPlan.staleRetries = 0
	return m
}

// recordDJRefreshFailure files a refresh that committed nothing.
//
// Nothing durable happened, so the applied settings are unaffected either way;
// what differs is whether asking again is worth anything. Data that moved under
// the attempt is worth asking again immediately, a timeline that moved is worth
// waiting out, an empty queue of DJ rows is not a failure at all, and everything
// else is recorded and left alone until the user or the refill moves the state
// on.
func (m Model) recordDJRefreshFailure(msg djRefreshDoneMsg, now time.Time) (Model, tea.Cmd) {
	switch {
	case msg.cancelled, errors.Is(msg.err, errDJRefreshCancelled):
		return m, nil
	case errors.Is(msg.err, errDJRefreshInputsChanged):
		// The library moved while the attempt was in flight, so its selections
		// were judged against data that no longer describes the collection. The
		// answer is the same question asked again, and asking it from here is
		// what makes the new snapshot fresh: djStartRefreshCmd captures on the
		// update loop, after every invalidation that caused this has landed.
		//
		// The retry budget is the bound. Without it a scan bumping the revision
		// on every file it touches would keep a refresh relaunching itself for
		// as long as the scan runs.
		m.djRefresh.status = djRefreshStatus{err: msg.err, at: now}
		if m.djRefresh.retries >= djRefreshMaxRetries {
			slog.Warn("dj refresh: giving up after repeated input changes", "err", msg.err)
			return m, nil
		}
		m.djRefresh.retries++
		return m.djStartRefreshCmd(now)
	case errors.Is(msg.err, errDJRefreshNoSlots):
		// The queue holds nothing the DJ owns — a hand-built queue, or one
		// whose DJ rows have all been played. The settings are applied and the
		// ordinary refill will pick against them.
		m.djRefresh.status = djRefreshStatus{at: now}
		return m, nil
	case errors.Is(msg.err, player.ErrRefreshDeferred), errors.Is(msg.err, player.ErrRefreshStale):
		m.djRefresh.status = djRefreshStatus{err: msg.err, at: now}
		if m.djRefresh.retries >= djRefreshMaxRetries {
			slog.Warn("dj refresh: giving up after repeated staleness", "err", msg.err)
			return m, nil
		}
		m.djRefresh.deferred = true
		return m, nil
	}
	slog.Warn("dj refresh failed", "err", msg.err)
	m.djRefresh.status = djRefreshStatus{err: msg.err, at: now}
	return m, nil
}

// djResumeDeferredRefresh relaunches a refresh that never found a settled
// playback window.
//
// Deferral is the one refusal that says nothing about the plan: an end-file was
// unresolved, so no plan could have been checked against a timeline that was
// still moving. The two callers are the settlement itself — a confirmed track
// change, which is the transition resolving — and the periodic tick, which is
// the backstop for a deferral no track change follows.
func (m Model) djResumeDeferredRefresh(now time.Time) (Model, tea.Cmd) {
	if !m.djRefresh.deferred || m.djRefresh.pending {
		return m, nil
	}
	if m.djRefresh.retries >= djRefreshMaxRetries {
		m.djRefresh.deferred = false
		return m, nil
	}
	m.djRefresh.retries++
	return m.djStartRefreshCmd(now)
}
