package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// radioResultCancelled reports whether the user has withdrawn the request this
// result answers. Relaunching after either of these would undo a gesture, so
// they are kept apart from the staleness check that does relaunch.
func (m Model) radioResultCancelled(msg RadioBatchEnqueuedMsg) bool {
	// Radio was toggled off while the goroutine was in flight; publishing now
	// would resurrect the mode the user just cancelled.
	if !m.radio.enabled {
		return true
	}
	// Playback was deliberately stopped while the goroutine was in flight. The
	// batch was admitted by a low-water-mark check the stop has since answered,
	// so publishing it would grow the queue after the user ended playback — and
	// would hand the tick a fresh reason to keep going. Retiring it here is what
	// makes the stop boundary cover work that was already running, not just work
	// the gate can still refuse to launch.
	if m.radio.stoppedByUser {
		return true
	}
	// Saved DJ state was refused, so the settings this result was planned
	// against are the stock stand-ins rather than the user's. The gate above
	// already declines to launch such work; answering for it here too is what
	// makes the refusal hold for a result that reached the update loop by any
	// other route, and it retires the hidden rows rather than stranding them.
	if m.djRestoreBlocked() {
		return true
	}
	// Queue-cleared-in-flight guard: when the radio batch was launched the
	// queue had remaining tracks (low-water-mark trigger), so msg.WasEmpty is
	// false. If the queue is empty by the time the batch arrives the user might
	// have cleared it; silently appending the batch would then resurrect a queue
	// the user just emptied. Distinguish natural exhaustion (queueExhausted=true,
	// set by handlePlaybackEndedMsg) from a user-initiated clear (queueExhausted
	// reset by resetPlaybackState). Only the former should auto-resume by
	// appending the in-flight batch.
	return !msg.WasEmpty && len(m.queue.Entries) == 0 && !m.playback.queueExhausted
}

// radioInputsCurrent reports whether the inputs a batch was computed from are
// still the inputs in force, so publishing it means what it meant when it was
// planned.
//
// Options, data, and seed must all still match. Applied settings that changed
// mean the filters and the discovery target changed; a moved seed means the
// batch is about something else; a moved data revision means the classification
// or the eligibility it was selected by may already be wrong.
func (m Model) radioInputsCurrent(msg RadioBatchEnqueuedMsg, now time.Time) bool {
	cur := m.djLiveFingerprint(now)
	if msg.fingerprint.dataRev != cur.dataRev ||
		djHourChanged(msg.fingerprint.capturedAt, now) ||
		!msg.fingerprint.settings.Equal(cur.settings) {
		return false
	}
	if !m.radioSeedCurrent(msg.fingerprint.seedHash, cur.seedHash) {
		return false
	}
	return m.radioQueueAdmissible(msg, msg.fingerprint.queueRev)
}

// radioSeedCurrent reports whether a batch is still about the seed in force.
//
// A pinned seed is the whole of what an anchored session is, so nothing but a
// deliberate reseed can move it and any move is a change of subject. A seed that
// follows playback is the opposite: it is meant to drift, and a batch planned
// from the row that was playing when the refill launched is exactly what
// following means. Refusing that would leave the DJ unable to refill whenever the
// queue advances while planning runs — which, at the low-water mark, is most of
// the time. Deliberate seed changes are not left to this check either way: the
// lifecycle transitions supersede in-flight work outright, so what reaches here
// on a following seed is only ordinary drift.
func (m Model) radioSeedCurrent(planned, current string) bool {
	return planned == current || !m.radioSeedPinned()
}

// radioSeedPinned reports whether the seed can only move deliberately. Only an
// anchored session pins it; a following session re-anchors on every confirmed
// track change, and a Model with Radio on and no session behind it seeds from
// the playing track, which is the same drift by another route.
func (m Model) radioSeedPinned() bool {
	return m.dj.active && m.dj.settings.Anchor == radio.AnchorSeed
}

// radioQueueAdmissible validates the queue half of the fingerprint.
//
// An unchanged queue revision is the fast answer: nothing the plan was told to
// avoid has moved. A changed one is deliberately NOT a rejection, because the
// commonest way for it to change is the queue draining — which is exactly the
// exhaustion this batch was launched to refill, so rejecting there would stall
// continuous playback at the one moment it matters. What decides is the
// direction of the change: rows leaving the protected set cannot invalidate a
// selection that already avoided them, while rows that entered it can, so the
// fallback asks whether any selection now collides with a row that is playing or
// still ahead.
func (m Model) radioQueueAdmissible(msg RadioBatchEnqueuedMsg, capturedRev uint64) bool {
	protected := m.djProtectedHashes()
	if capturedRev == djQueueRevisionOf(protected) {
		return true
	}
	for _, e := range msg.Entries {
		if _, clash := protected[e.Hash]; clash {
			return false
		}
	}
	return true
}

// applyRadioBatch adopts a published batch: only here do the recent-selection
// ring and the discovery allocation advance, because only here is the batch
// something the user will actually hear.
func (m Model) applyRadioBatch(msg RadioBatchEnqueuedMsg, now time.Time) (Model, tea.Cmd) {
	for _, e := range msg.Entries {
		m = m.radioNoteRecent(e.Hash)
	}
	m = m.adoptRadioAllocation(msg)
	m.djPlan.block = djPlanBlock{}
	m.djPlan.staleRetries = 0
	m.djPlan.status = djPlanStatus{
		outcome:   msg.plan.Outcome,
		shortages: msg.plan.Shortages,
		reasons:   msg.plan.Reasons,
		degraded:  msg.signals.Failures(),
		accepted:  len(msg.Entries),
		at:        now,
	}

	batchStartIdx := len(m.queue.Entries)
	m.queue.Entries = append(m.queue.Entries, msg.Entries...)
	m = m.preserveQueueSort()
	if appender, ok := m.player.(playerQueueAppender); ok {
		for _, entry := range msg.Entries {
			appender.AppendToQueue(queueTrackFromEntry(entry))
		}
	}

	// Auto-resume when the queue had run dry. A failed resume leaves the batch
	// in the queue and the model exhausted; nothing schedules another enqueue
	// here, so a player that cannot resume never turns into a refill loop.
	if m.playback.queueExhausted && m.playback.state == playback.StateIdle {
		m, _ = m.resumeExtendedQueue(batchStartIdx)
	} else if msg.WasEmpty && m.playback.state == playback.StateIdle {
		entry := msg.Entries[0]
		m = m.tryAutoPlay(entry.Hash, entry.Artist, entry.Title, djQueueSource)
	}

	return m, nil
}

// radioCommittedAllocation returns the discovery carry the accept transaction
// should write, or nil when it should write none.
//
// The carry belongs to a session: it is the remainder of a rounded discovery
// split, accumulated against one discovery target. Committing it with no session
// running would leave it on the stopped session row for the next start to
// inherit, biasing a first batch for a reason the user cannot see.
func (m Model) radioCommittedAllocation(msg RadioBatchEnqueuedMsg) *radio.Allocation {
	if !m.dj.active || !msg.plan.CanContinue() {
		return nil
	}
	alloc := msg.plan.NextAllocation
	return &alloc
}

// adoptRadioAllocation records the discovery rounding carry an accepted batch
// earned. A DJ writer already committed it inside the accept transaction, so
// re-issuing the write there would rewrite the whole session row — filters
// included — for a column that is already correct.
func (m Model) adoptRadioAllocation(msg RadioBatchEnqueuedMsg) Model {
	if !msg.plan.CanContinue() {
		return m
	}
	if msg.prep.Kind != "" {
		return m.djNoteAllocation(msg.plan.NextAllocation)
	}
	return m.djAdoptAllocation(msg.plan.NextAllocation)
}

// recordRadioNoBatch files a live result that selected nothing.
//
// Nothing was prepared, so there is nothing to retire; what matters is that the
// refill parks on these exact inputs. "No candidates pauses additions with an
// explanation" is a product decision, and a decision to stop is only honoured if
// the guard refuses to ask the same question again a second later.
func (m Model) recordRadioNoBatch(msg RadioBatchEnqueuedMsg) Model {
	m.djPlan.status = djPlanStatus{
		outcome:   msg.plan.Outcome,
		shortages: msg.plan.Shortages,
		reasons:   msg.plan.Reasons,
		degraded:  msg.signals.Failures(),
		at:        msg.fingerprint.capturedAt,
	}
	m.djPlan.block = djPlanBlock{
		active:  true,
		at:      msg.fingerprint,
		reasons: msg.plan.Reasons,
	}
	m.djPlan.staleRetries = 0
	return m
}

// recordRadioFailure files a live result that failed outright.
//
// A transient failure — a refused write — may be re-attempted against unchanged
// inputs, but only djMaxTransientRetries times: a database that refuses three
// identical writes is not going to accept the fourth, and the one-second tick
// would otherwise turn it into a write loop. Anything else parks until an input
// moves.
func (m Model) recordRadioFailure(msg RadioBatchEnqueuedMsg, now time.Time) Model {
	slog.Warn("radio enqueue failed", "err", msg.Err, "transient", msg.transient)
	m.djPlan.status = djPlanStatus{
		outcome:  msg.plan.Outcome,
		degraded: msg.signals.Failures(),
		err:      msg.Err,
		at:       now,
	}
	b := m.djPlan.block
	if b.active && b.retriable && msg.transient && b.at.sameInputs(msg.fingerprint) {
		b.attempts++
		b.err = msg.Err
		m.djPlan.block = b
		return m
	}
	m.djPlan.block = djPlanBlock{
		active:    true,
		at:        msg.fingerprint,
		err:       msg.Err,
		retriable: msg.transient,
		attempts:  1,
	}
	return m
}

// retryRadioAfterStale relaunches a refill whose result was refused because its
// inputs moved under it.
//
// Without this the DJ can go silent exactly when it is needed: the natural
// triggers are a confirmed track change and a queue-end, so a batch refused at
// the moment the queue ran dry would leave nothing scheduled to wake the refill
// again. The relaunch is bounded, because inputs that keep moving under every
// attempt must end as a stalled refill rather than as a planning loop.
func (m Model) retryRadioAfterStale() (Model, tea.Cmd) {
	if m.djPlan.staleRetries >= djMaxTransientRetries {
		return m, nil
	}
	m.djPlan.staleRetries++
	return m.startRadioEnqueueCmd()
}

// djWakeParkedRefill relaunches a refill that parked, once whatever parked it
// has stopped being a reason to stay parked.
//
// Every other automatic trigger is an event in the playback stream: a confirmed
// track change or a queue end. A parked refill is precisely the state in which
// no further such event is coming — playback is exhausted, so nothing will
// advance and nothing will end — so without a periodic wakeup a refused write or
// an empty filtered result silences continuous playback for good, however
// quickly the writer recovers and however many eligible tracks arrive
// afterwards. The one-second tick is the only message that still arrives, which
// is what makes it the wakeup.
//
// It is deliberately narrow, and the narrowness is the whole safety argument.
// Only a parked refill is woken, and a park is recorded by nothing but a live
// attempt that the guards already admitted and that came back with nothing;
// every planning replacement retires it through invalidateRadioBatch, including
// an automatic Follow reseed. An explicit Stop or a completed stop-after-current
// retires it through
// radioNoteDeliberateStop, which also suspends the gate so a park cannot simply
// be earned again. So a queue the user emptied stays empty, a stop stays
// stopped, and a paused startup that never launched a refill does not acquire
// one from the tick.
//
// What bounds it is the ordinary gate: radioCanEnqueue re-checks every guard and
// ends at djLiveAllowed, which re-attempts unchanged inputs only after a
// transient write failure and only djMaxTransientRetries times. Anything else —
// an empty result, an unresolved tag — waits for an input to actually move, and
// a move is what makes the next attempt a different question rather than a loop.
func (m Model) djWakeParkedRefill() (Model, tea.Cmd) {
	if !m.djPlan.block.active {
		return m, nil
	}
	return m.startRadioEnqueueCmd()
}

// discardRadioResult reconciles durable state for a result that will not be
// represented in the TUI queue, using whichever writer created the rows: a DJ
// preparation is rejected as a unit, while the pre-DJ paths are reconciled by a
// compensating soft delete of the exact row IDs.
func (m Model) discardRadioResult(msg RadioBatchEnqueuedMsg) Model {
	if msg.prep.Kind != "" {
		return m.rejectDJPreparation(msg.prep)
	}
	return m.discardRadioBatch(msg.QueueIDs)
}

// discardRadioBatch reconciles durable state for a result that will not be
// represented in the TUI queue. The rows remain soft-deleted for normal queue
// maintenance to purge later. A failed delete retains the exact batch for the
// periodic retry path so ReadQueue cannot restore it permanently.
func (m Model) discardRadioBatch(ids []int64) Model {
	if len(ids) == 0 || m.batchEnqueuer == nil {
		return m
	}
	if err := m.batchEnqueuer.DequeueTracksSoft(ids); err != nil {
		slog.Warn("radio discarded batch cleanup failed", "err", err)
		m.radio.cleanupBatches = append(m.radio.cleanupBatches, append([]int64(nil), ids...))
	}
	return m
}

// startRadioBatchCleanupCmd starts at most one retained cleanup retry. The
// existing one-second tick calls it, bounding repeated database failures while
// ensuring every failed batch remains live until its compensating delete wins.
func (m Model) startRadioBatchCleanupCmd() (Model, tea.Cmd) {
	if m.radio.cleanupInFlight || len(m.radio.cleanupBatches) == 0 || m.batchEnqueuer == nil {
		return m, nil
	}
	ids := append([]int64(nil), m.radio.cleanupBatches[0]...)
	be := m.batchEnqueuer
	m.radio.cleanupInFlight = true
	return m, func() tea.Msg {
		return radioBatchCleanupMsg{QueueIDs: ids, Err: be.DequeueTracksSoft(ids)}
	}
}

// handleRadioBatchCleanupMsg removes only the exact front batch after its
// compensating delete succeeds. Failures leave it queued for the next tick;
// neither outcome touches the identity or pending bit of a newer Radio request.
func (m Model) handleRadioBatchCleanupMsg(msg radioBatchCleanupMsg) Model {
	if !m.radio.cleanupInFlight || len(m.radio.cleanupBatches) == 0 ||
		!sameRadioQueueIDs(m.radio.cleanupBatches[0], msg.QueueIDs) {
		return m
	}
	m.radio.cleanupInFlight = false
	if msg.Err != nil {
		slog.Warn("radio discarded batch cleanup retry failed", "err", msg.Err)
		return m
	}
	m.radio.cleanupBatches[0] = nil
	m.radio.cleanupBatches = m.radio.cleanupBatches[1:]
	if len(m.radio.cleanupBatches) == 0 {
		m.radio.cleanupBatches = nil
	}
	return m
}

func sameRadioQueueIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
