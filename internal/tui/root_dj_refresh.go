package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// This file is what makes Apply mean something immediately.
//
// Applying settings while a session is running has to change what the user is
// about to hear, not merely what the next refill will pick — but the queue it
// changes is the one playing, so "immediately" cannot mean "now, from the update
// loop". The refresh therefore participates in three existing mechanisms rather
// than inventing a fourth:
//
//   - the play-request gate, so a refresh is ordered against the selections and
//     transport commands around it and a teardown can retire it before it ever
//     reaches the player;
//   - the guarded four-stage commit, so the durable queue and mpv can disagree
//     only in the one direction the recovery path converges;
//   - the DJ request identity, so a result that arrives after a disable, a
//     clear, a reseed, or a newer Apply is reconciled rather than adopted.
//
// What it replaces is deliberately narrow: the upcoming rows the DJ owns, in
// place. Never the row playing, never played history, never a manual entry, and
// never by appending — a user with a long hand-built queue ahead of them gets
// their DJ rows refreshed where they stand rather than a batch pushed in front
// of the queue they built.

const (
	// djRefreshMaxRetries bounds how many times a deferred or repeatedly stale
	// refresh is relaunched. CommitQueueRefresh already retries the preparation
	// stage internally, so reaching this means the timeline has been moving
	// under every attempt for several seconds — at which point the applied
	// settings reaching the queue through the ordinary refill is the better
	// outcome than a refresh that keeps re-planning.
	djRefreshMaxRetries = 3
)

var (
	// errDJRefreshNoSlots reports a queue with nothing the refresh may take
	// over. It is an ordinary outcome, not a failure: a queue holding only
	// manual entries is exactly the queue a refresh must leave alone.
	errDJRefreshNoSlots = errors.New("tui: no replaceable auto-dj queue rows")

	// errDJRefreshCancelled reports a refresh withdrawn between its preparation
	// and its commit. Returning it from the commit step is what keeps a disable,
	// a clear, or a newer Apply from being undone by work already in flight:
	// nothing is committed, so there is nothing to reconcile afterwards.
	errDJRefreshCancelled = errors.New("tui: auto-dj refresh was superseded before its commit")
)

// djRefreshState tracks the single immediate refresh a Model may have in
// flight.
//
// One at a time is the whole design. A refresh is a queue-wide replacement, so
// two of them racing would each be planned against a queue the other is about to
// change; rapid Apply therefore supersedes rather than queues, and only the
// newest identity is adopted.
type djRefreshState struct {
	// id names the newest refresh. A result carrying anything else is late.
	id uint64

	// pending reports that a refresh is in flight.
	pending bool

	// cancel is the in-flight refresh's withdrawal flag, read by its commit
	// step on the refresh goroutine. It is nil when nothing is in flight.
	cancel *atomic.Bool

	// deferred reports a refresh that found no settled playback window and is
	// waiting to be asked again — by the EOF settlement that resolves the
	// transition, or by the periodic tick as a backstop.
	deferred bool

	// retries counts the relaunches spent on the current applied settings. It
	// is reset by an adopted refresh and by every deliberate replacement
	// boundary, so a bounded budget cannot leak across unrelated gestures.
	retries int

	// status is what the last refresh did, for the Tune tab to explain.
	status djRefreshStatus
}

// djRefreshStatus is the user-facing outcome of one refresh.
type djRefreshStatus struct {
	outcome   radio.PlanOutcome
	shortages radio.Shortages
	reasons   radio.Reasons
	degraded  []djSignalFailure

	// replaced and retired are what the player actually did: slots handed over
	// to a successor, and slots dropped with nothing to put in them.
	replaced int
	retired  int

	// recovered reports that the publication was rejected after the commit and
	// the player was moved onto the committed queue instead. The refresh still
	// happened; only the route to it was the recovery path.
	recovered bool

	err error
	at  time.Time
}

// djApply adopts a settings snapshot and pushes it into the queue at once.
//
// This is the single entry point for the Apply gesture. Persisting the settings
// comes first and is independent of everything after it: an applied snapshot is
// durable even when no refresh is possible, because the alternative is a user
// whose Apply silently did nothing because the queue happened to hold no DJ
// rows. The refresh is then best effort on top, and a Model that cannot run one
// falls back to the ordinary low-water-mark refill, which is where the new
// settings take effect for everyone else.
func (m Model) djApply(settings radio.Settings, now time.Time) (Model, tea.Cmd, error) {
	m, err := m.djApplySettings(settings)
	if err != nil {
		return m, nil, err
	}
	m, cmd := m.djStartRefreshCmd(now)
	if cmd != nil {
		return m, cmd, nil
	}
	m, refill := m.startRadioEnqueueCmd()
	return m, refill, nil
}

// supersedeDJRefresh withdraws whatever refresh is in flight.
//
// Two halves, both required. The identity advances so a result already on its
// way is recognised as late, and the withdrawal flag is raised so a refresh that
// has not reached its commit yet declines to make one. Without the flag a
// disable or a clear could be undone by a transaction that was a microsecond
// behind it; without the identity, one that was not could be adopted as if it
// were current.
//
// The retry budget is deliberately not touched here: djStartRefreshCmd calls
// this on its way in, and a relaunch must not refund its own budget.
func (m Model) supersedeDJRefresh() Model {
	if m.djRefresh.cancel != nil {
		m.djRefresh.cancel.Store(true)
	}
	m.djRefresh.id++
	m.djRefresh.cancel = nil
	m.djRefresh.pending = false
	m.djRefresh.deferred = false
	return m
}

// djStartRefreshCmd launches one immediate refresh, superseding any earlier one.
//
// Everything the goroutine reads is copied here, on the update loop: the applied
// settings, the seed, the discovery carry, the planning snapshot, and the queue
// rows the plan will rebuild the player's queue from. Reading any of it from the
// Model inside the closure would race the update loop.
//
// A nil command means no refresh was launched, which is a supported state rather
// than a failure: no session running, radio off, a player that does not expose
// the guarded commit, or a writer that cannot prepare a replacement.
func (m Model) djStartRefreshCmd(now time.Time) (Model, tea.Cmd) {
	m = m.supersedeDJRefresh()
	if !m.radio.enabled || !m.dj.active {
		return m, nil
	}
	committer, ok := m.player.(queueRefreshCommitter)
	if !ok {
		return m, nil
	}
	writer, ok := m.djQueue.(djReplacementWriter)
	if !ok {
		return m, nil
	}

	cancel := &atomic.Bool{}
	m.djRefresh.cancel = cancel
	m.djRefresh.pending = true
	run := &djRefreshRun{
		committed: -1,
		req: djRefreshRequest{
			id:       m.djRefresh.id,
			settings: m.dj.settings,
			seed:     m.djPlanSeed(now),
			alloc:    m.dj.allocation,
			inputs:   m.captureDJInputs(now),
			reader:   m.recReader,
			writer:   writer,
			rows:     m.djRefreshRows(),
			cancel:   cancel,
			now:      now,
		},
	}

	// A transport barrier rather than a selection: a refresh must not be
	// coalesced away by a later keypress, and it must not supersede the
	// selection in front of it either. The wait it takes is one the player
	// would have taken anyway inside ApplyQueueRefresh's playlist lock.
	gate := m.playback.requests
	req := gate.dispatchControl(m.playback.requestSeq)
	m.playback.requestSeq = req

	return m, func() tea.Msg {
		return run.exec(context.Background(), gate, req, committer)
	}
}

// djRefreshRows copies the queue rows the refresh may need to rebuild, keyed by
// the durable row ID the player's snapshot names them with.
//
// A row with no ID is not persisted and so has no identity the player and the
// database agree on; it is left out, and a refresh that meets one in the live
// queue rejects its own plan as stale rather than publishing a queue it cannot
// describe.
func (m Model) djRefreshRows() map[int64]queue.Entry {
	rows := make(map[int64]queue.Entry, len(m.queue.Entries))
	for _, e := range m.queue.Entries {
		if e.ID == 0 {
			continue
		}
		rows[e.ID] = e
	}
	return rows
}

// djRefreshRequest is the immutable copy of everything one refresh reads.
type djRefreshRequest struct {
	id       uint64
	settings radio.Settings
	seed     radio.SeedSnapshot
	alloc    radio.Allocation
	inputs   djInputs
	reader   recommendationReader
	writer   djReplacementWriter
	rows     map[int64]queue.Entry
	cancel   *atomic.Bool
	now      time.Time
}

// djRefreshAttempt is one preparation: the hidden rows it wrote, the slots it
// intends them for, and the plan that chose them.
type djRefreshAttempt struct {
	prep    db.DJPreparation
	entries []queue.Entry
	slots   []player.RefreshRow
	plan    radio.PlanResult
	signals djSignals
}

// djRefreshRun accumulates the attempts one refresh makes.
//
// The preparation stage can run several times, and each run writes hidden rows
// that only an accept or a reject retires. Keeping every attempt is what lets
// the result account for all of them: one committed, the rest rejected.
type djRefreshRun struct {
	req       djRefreshRequest
	attempts  []djRefreshAttempt
	committed int
}

// exec runs the whole refresh on the tea.Cmd goroutine and reports what
// happened.
//
// The gate wrapper is the ordering contract: the refresh waits for every earlier
// live request, and a teardown that retires it before it runs reports ran=false,
// which is a cancellation rather than a failure — nothing was prepared, so there
// is nothing to reconcile.
func (r *djRefreshRun) exec(
	ctx context.Context, gate *playRequestGate, req uint64, committer queueRefreshCommitter,
) djRefreshDoneMsg {
	var (
		res player.RefreshCommitResult
		err error
	)
	_, ran := gate.run(req, func() error {
		res, err = committer.CommitQueueRefresh(ctx, player.RefreshCommit{
			Prepare: r.prepare,
			Commit:  r.commit,
		})
		return nil
	})

	msg := djRefreshDoneMsg{RequestID: r.req.id, cancelled: !ran, res: res, err: err}
	if !ran {
		return msg
	}
	for i, att := range r.attempts {
		if i == r.committed && res.Committed {
			msg.committed, msg.valid = att, true
			continue
		}
		if len(att.prep.IDs) > 0 {
			msg.discarded = append(msg.discarded, att.prep)
		}
	}
	return msg
}

// prepare turns one captured timeline into a durable replacement plan.
//
// It runs off every player lock, which is what lets it do the planning and the
// database write here. What it must not do is leave the player able to publish
// something the plan does not describe, so the refreshed queue is rebuilt from
// the snapshot itself — the surviving rows in the order the snapshot saw them,
// then the successors at the tail, which is where the accept transaction places
// them in the durable queue.
func (r *djRefreshRun) prepare(snap player.RefreshSnapshot) (player.QueueRefresh, error) {
	if r.req.cancel != nil && r.req.cancel.Load() {
		return player.QueueRefresh{}, errDJRefreshCancelled
	}
	slots := r.req.refreshSlots(snap)
	if len(slots) == 0 {
		return player.QueueRefresh{}, errDJRefreshNoSlots
	}

	in := r.req.inputs.withProtected(r.req.protectedFor(snap, slots))
	result, signals, err := djPlanBatch(in, radio.PlanRequest{
		Settings:    r.req.settings,
		Seed:        r.req.seed,
		BatchSize:   len(slots),
		RequestedAt: r.req.now,
	}, r.req.alloc, r.req.reader, r.req.now)
	if err != nil {
		return player.QueueRefresh{}, err
	}

	// No candidates is not a reason to abandon the refresh. The settings the
	// user applied exclude everything the DJ had lined up, so the honest answer
	// is to retire those rows and add nothing — current playback and every
	// manual entry continue, and the refill parks with the same explanation.
	picked := result.Tracks
	if !result.CanContinue() {
		picked = nil
	}
	if len(picked) > len(slots) {
		picked = picked[:len(slots)]
	}

	replacing := make([]int64, len(slots))
	for i, slot := range slots {
		replacing[i] = slot.QueueID
	}
	prep, err := r.req.writer.PrepareDJReplacement(djQueueEntriesFor(picked), replacing)
	if err != nil {
		return player.QueueRefresh{}, err
	}
	entries := newDJQueueEntries(picked, prep.IDs, true)

	tracks, err := r.req.refreshedQueue(snap, slots, entries)
	if err != nil {
		// The hidden rows are already written; recording the attempt is what
		// gets them rejected when the result is filed.
		r.attempts = append(r.attempts, djRefreshAttempt{prep: prep, entries: entries, slots: slots})
		return player.QueueRefresh{}, err
	}

	r.attempts = append(r.attempts, djRefreshAttempt{
		prep:    prep,
		entries: entries,
		slots:   slots,
		plan:    result,
		signals: signals,
	})
	return player.QueueRefresh{
		Snapshot:     snap,
		Queue:        tracks,
		Replacements: djRefreshReplacements(slots, prep.IDs),
	}, nil
}

// commit publishes the newest attempt, or declines to.
//
// The withdrawal check is the last moment a disable, a clear, or a newer Apply
// can still call the refresh off for free. Past it the transaction is the point
// of no return, and the late-acknowledgement path in the handler is what
// reconciles a result that got here a moment too early.
func (r *djRefreshRun) commit(player.QueueRefresh) error {
	if r.req.cancel != nil && r.req.cancel.Load() {
		return errDJRefreshCancelled
	}
	idx := len(r.attempts) - 1
	if idx < 0 {
		return fmt.Errorf("%w: no preparation to commit", player.ErrRefreshPlanInvalid)
	}
	att := r.attempts[idx]
	if _, err := r.req.writer.AcceptDJBatch(att.prep, db.DJCommit{Allocation: att.committedAllocation()}); err != nil {
		return err
	}
	r.committed = idx
	return nil
}

// committedAllocation returns the discovery carry this attempt earned, or nil
// when it earned none. A batch that selected nothing advanced no rounding.
func (a djRefreshAttempt) committedAllocation() *radio.Allocation {
	if !a.plan.CanContinue() {
		return nil
	}
	alloc := a.plan.NextAllocation
	return &alloc
}

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
	m.djRefresh.cancel = nil
	if !msg.valid {
		return m.recordDJRefreshFailure(msg, time.Now())
	}
	m = m.adoptDJRefreshRows(msg.committed)
	return m.adoptDJRefreshState(msg, time.Now()), nil
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
// what differs is whether asking again is worth anything. A timeline that moved
// under the attempt is worth waiting out, an empty queue of DJ rows is not a
// failure at all, and everything else is recorded and left alone until the user
// or the refill moves the state on.
func (m Model) recordDJRefreshFailure(msg djRefreshDoneMsg, now time.Time) (Model, tea.Cmd) {
	switch {
	case msg.cancelled, errors.Is(msg.err, errDJRefreshCancelled):
		return m, nil
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
