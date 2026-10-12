package tui

import (
	"context"
	"errors"
	"fmt"
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

	// errDJRefreshInputsChanged reports a refresh withdrawn because the data its
	// selections were judged eligible by moved while it was in flight.
	//
	// It is deliberately not errDJRefreshCancelled. A cancellation is the user's
	// newer decision and the answer to it is to stop; this is the same decision
	// asked of a library that has since changed, so the answer is to ask again
	// from a fresh snapshot. Keeping the two apart is what stops a tag edit from
	// reading as a disable, and a disable from being replanned around.
	errDJRefreshInputsChanged = errors.New("tui: auto-dj refresh inputs changed before its commit")
)

// djRefreshWithdrawal is the shared half of one refresh in flight: flags the
// update loop raises and the refresh goroutine reads.
//
// Two of them rather than one, because the two withdrawals mean opposite things.
// A supersession is a newer gesture — a disable, a clear, a reseed, a second
// Apply — and the answer is to stop. An invalidation is the eligibility data the
// plan was computed from having moved, and the answer is to plan again against
// the library as it now is.
//
// They are atomics rather than Model fields because they are written on the
// update loop and read on the refresh goroutine. That is also the whole of what
// the goroutine may consult: the snapshot it carries is immutable, and no live
// Model map is ever reachable from here.
type djRefreshWithdrawal struct {
	superseded  atomic.Bool
	invalidated atomic.Bool
}

// supersede calls the refresh off for a newer gesture.
func (w *djRefreshWithdrawal) supersede() {
	if w != nil {
		w.superseded.Store(true)
	}
}

// invalidate calls the refresh off because the library it planned against
// changed. Nil-safe on purpose: every Auto-DJ invalidation funnels through
// djInvalidate, and almost all of them happen with no refresh in flight.
func (w *djRefreshWithdrawal) invalidate() {
	if w != nil {
		w.invalidated.Store(true)
	}
}

// reason returns the error the stage reading it must decline with, or nil to
// carry on. Supersession outranks invalidation: a superseded refresh is filed as
// late whatever it says, so replanning it would be work for a question nobody is
// asking any more.
func (w *djRefreshWithdrawal) reason() error {
	switch {
	case w == nil:
		return nil
	case w.superseded.Load():
		return errDJRefreshCancelled
	case w.invalidated.Load():
		return errDJRefreshInputsChanged
	}
	return nil
}

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

	// withdraw holds the in-flight refresh's withdrawal flags, read by its
	// preparation and commit steps on the refresh goroutine. It is nil when
	// nothing is in flight.
	withdraw *djRefreshWithdrawal

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
	// A Tune-panel commit waiting on the refresh being withdrawn is answered
	// here: its result will be filed as late, so nothing downstream would ever
	// report back to the panel.
	if m.djRefresh.pending {
		m = m.withdrawAutoDJPanelWait()
	}
	m.djRefresh.withdraw.supersede()
	m.djRefresh.id++
	m.djRefresh.withdraw = nil
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
	// Refused saved state is checked alongside the two ordinary preconditions
	// rather than left to them. A refresh only follows an Apply today, and an
	// Apply clears the refusal on its way through — but a refresh is a durable
	// replacement of the queue's upcoming rows, so the one route that must never
	// run against stand-in settings states that for itself.
	if !m.radio.enabled || !m.dj.active || m.djRestoreBlocked() {
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

	withdraw := &djRefreshWithdrawal{}
	m.djRefresh.withdraw = withdraw
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
			withdraw: withdraw,
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
	withdraw *djRefreshWithdrawal
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
//
// The withdrawal check leads because this stage can run several times: the
// player retries a preparation whose timeline moved, and every retry would
// otherwise re-plan from the same retired snapshot. Refusing here also means an
// invalidated attempt writes no hidden rows at all.
func (r *djRefreshRun) prepare(snap player.RefreshSnapshot) (player.QueueRefresh, error) {
	if err := r.req.withdraw.reason(); err != nil {
		return player.QueueRefresh{}, err
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
// The withdrawal check is the last moment a disable, a clear, a newer Apply, or
// a library change can still call the refresh off for free. Past it the
// transaction is the point of no return, and the late-acknowledgement path in
// the handler is what reconciles a result that got here a moment too early.
// That boundary is also what separates the two outcomes a caller must tell
// apart: declining here leaves nothing committed and every prepared row still
// hidden, whereas an invalidation that lands after the transaction cannot
// un-commit it and is answered by the ordinary refill's freshness check instead.
func (r *djRefreshRun) commit(player.QueueRefresh) error {
	if err := r.req.withdraw.reason(); err != nil {
		return err
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
