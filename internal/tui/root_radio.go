package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// radioState holds all mutable radio-mode fields for the root Model.
type radioState struct {
	enabled         bool
	seedHash        string
	syntheticSeed   *radio.SeedTrack // non-nil = use directly instead of hash lookup
	enqueuePending  bool
	requestID       uint64
	recentHashes    []string // ring buffer, cap RadioRecentCap
	cleanupBatches  [][]int64
	cleanupInFlight bool
}

const (
	radioBatchSize    = 5
	radioLowWaterMark = 2
)

// RadioRecentCap is the capacity of the radio recent-hashes ring buffer.
// Exported so startup can ask the database for exactly that many recently
// played hashes when seeding the ring.
const RadioRecentCap = 30

// radioToggle flips radio mode on or off, notifies the user, and kicks off an
// initial enqueue when enabling.
func (m Model) radioToggle() (tea.Model, tea.Cmd, bool) {
	m = m.radioSetEnabled(!m.radio.enabled)
	// The DJ session lifecycle moves with the toggle. An ordinary `A` press is
	// the least specific start there is, so it seeds from the current track
	// (and from the whole library when nothing is playing) whatever view the
	// user happens to be in; the contextual entry points below pass a different
	// request. Restoring is not a start: a session resumed at launch keeps the
	// seed it was captured with, and only a deliberate press captures a fresh
	// one.
	if m.radio.enabled {
		m.radio.seedHash = m.playback.currentHash
		m, _ = m.djStartFor(djSeedCurrent, time.Now())
	} else {
		m.radio.syntheticSeed = nil
		m = m.supersedeRadioBatch()
		m = m.djStop()
	}
	m.statusBar.PushMsg(m.modeToggleMsg("Radio mode", playback.ModeIndexRadio), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, statusbar.NotifInfo)
	if m.radio.enabled {
		var cmd tea.Cmd
		m, cmd = m.startRadioEnqueueCmd()
		return m, cmd, true
	}
	return m, nil, true
}

// radioRemainingCount returns the number of queue entries still to be played.
//
// When the player exposes its traversal, that snapshot is the answer: it counts
// the rows the Manager will actually still visit, which under shuffle has
// nothing to do with the visible position of the playing row, and which a sort
// or a filter of the queue view cannot move. The visible-index arithmetic below
// is only the fallback for players without the snapshot; it reads the durable
// queue order and so mistakes a shuffle cursor sitting near the persisted tail
// for a nearly exhausted queue (and one near the head for a full one).
func (m Model) radioRemainingCount() int {
	if t, ok := m.playerTraversal(); ok {
		return t.Remaining
	}
	if m.playback.currentIndex < 0 {
		return len(m.queue.Entries)
	}
	if m.playback.currentIndex >= len(m.queue.Entries) {
		return len(m.queue.Entries)
	}
	return len(m.queue.Entries) - m.playback.currentIndex - 1
}

// radioCanEnqueue reports whether a refill may be launched.
//
// Guards: radio enabled, saved DJ state not refused, no pending enqueue,
// stopAfterCurrent not set, repeatMode != RepeatAll, library non-empty,
// remaining <= low-water-mark, and the planner not parked on inputs that have
// not moved since it last came back with nothing. That last guard is what keeps
// an empty candidate set, an unresolved tag reference, or a failing writer from
// turning the low-water-mark check into a loop that re-asks the same question
// forever.
//
// The refused-state guard is the one that has nothing to do with the queue.
// Radio mode is restored from its own column, so a launch that could not read
// the DJ row comes up with the refill armed and stock settings behind it; the
// first low-water-mark check would then add tracks the saved filters excluded.
// Radio stays on and the user keeps their queue — only the automatic additions
// wait for settings they actually chose.
func (m Model) radioCanEnqueue() bool {
	if !m.radio.enabled {
		return false
	}
	if m.djRestoreBlocked() {
		return false
	}
	if m.radio.enqueuePending {
		return false
	}
	if m.playback.stopAfterCurrent {
		return false
	}
	if m.playback.repeatMode == playback.RepeatAll {
		return false
	}
	if len(m.library.Index.Tracks()) == 0 {
		return false
	}
	if m.radioRemainingCount() > radioLowWaterMark {
		return false
	}
	return m.djLiveAllowed(time.Now())
}

// startRadioEnqueueCmd gives a newly admitted batch its identity and marks it
// pending before returning the work. Keeping both mutations here prevents a
// caller from launching work whose result cannot be distinguished from an
// older batch invalidated by a clear or disable.
func (m Model) startRadioEnqueueCmd() (Model, tea.Cmd) {
	if !m.radioCanEnqueue() {
		return m, nil
	}
	m.radio.requestID++
	m.radio.enqueuePending = true
	return m, m.radioEnqueueCmd()
}

// supersedeRadioBatch invalidates every result created from the current Radio
// state. The generation advances even when no batch is pending so all queue
// replacement boundaries have an unambiguous identity.
func (m Model) supersedeRadioBatch() Model {
	m.radio.requestID++
	m.radio.enqueuePending = false
	// A replacement boundary is a deliberate change of what the DJ is refilling
	// into, so neither the park nor the stale-retry budget earned under the old
	// state applies to the new one.
	m.djPlan.block = djPlanBlock{}
	m.djPlan.staleRetries = 0
	// The immediate refresh is DJ work against the same boundary, so it is
	// withdrawn here too. This is the single point every gesture that must not
	// be undone by a delayed result already goes through: the radio toggle, a
	// queue clear, and every session transition including a newly applied
	// settings snapshot.
	m = m.supersedeDJRefresh()
	m.djRefresh.retries = 0
	return m
}

// radioEnqueueCmd captures the current planning inputs and returns a tea.Cmd
// that plans one batch and persists it as hidden rows, returning
// RadioBatchEnqueuedMsg.
//
// Everything the goroutine reads is copied here, on the update loop: the data
// snapshot, the applied settings, the seed, the discovery carry, and the
// fingerprint acceptance will validate the result against. Reading any of it
// from the Model inside the closure would both race the update loop and defeat
// the point of the fingerprint, which is to describe one fixed set of inputs.
func (m Model) radioEnqueueCmd() tea.Cmd {
	persist := m.djPersister()
	if !persist.available() {
		return nil
	}
	now := time.Now()
	in := m.captureDJInputs(now)
	req := radio.PlanRequest{
		Settings:    m.dj.settings,
		Seed:        m.djPlanSeed(now),
		BatchSize:   radioBatchSize,
		RequestedAt: now,
	}
	msg := RadioBatchEnqueuedMsg{
		RequestID:   m.radio.requestID,
		WasEmpty:    len(m.queue.Entries) == 0,
		fingerprint: m.djFingerprint(in.dataRev, in.queueRev, now),
	}
	alloc := m.dj.allocation
	reader := m.recReader
	return func() tea.Msg {
		return djPlanAndPrepare(msg, in, req, alloc, reader, persist, now)
	}
}

// djPlanAndPrepare runs one live planning attempt off the update loop: plan the
// batch, then write it as rows nothing can see until acceptance publishes them.
//
// The two failure shapes are kept apart because they need different answers.
// Missing eligibility data means no verdict about what the filters admit is
// available, and no amount of retrying changes that until the tag cache is
// rebuilt — which advances the data revision and unparks the refill on its own.
// A failed write is transient and worth a bounded retry against the very same
// inputs.
func djPlanAndPrepare(msg RadioBatchEnqueuedMsg, in djInputs, req radio.PlanRequest,
	alloc radio.Allocation, reader recommendationReader, persist djPersistence, now time.Time,
) RadioBatchEnqueuedMsg {
	result, signals, err := djPlanBatch(in, req, alloc, reader, now)
	msg.plan = result
	msg.signals = signals
	if err != nil {
		msg.Err = err
		return msg
	}
	for _, f := range signals.Failures() {
		slog.Warn("radio: behavioral signal unavailable", "signal", f.Signal, "err", f.Err)
	}
	if !result.CanContinue() {
		return msg
	}
	prepared, err := persist.prepare(result.Tracks)
	if err != nil {
		msg.Err = err
		msg.transient = true
		return msg
	}
	msg.prep = prepared.prep
	msg.QueueIDs = prepared.ids
	msg.PendingActivation = prepared.needsActivation
	msg.Entries = newDJQueueEntries(result.Tracks, prepared.ids, persist.recordsProvenance())
	return msg
}

// radioNoteRecent records hash as just-heard in the recent-history ring,
// moving it to the newest position if it is already there so a repeat cannot
// consume several of the RadioRecentCap slots. Called for radio-enqueued
// tracks and for manually played ones alike: a hand-picked track is just as
// recently heard as a radio-picked one, and radio should avoid both.
func (m Model) radioNoteRecent(hash string) Model {
	if hash == "" {
		return m
	}
	ring := make([]string, 0, len(m.radio.recentHashes)+1)
	for _, h := range m.radio.recentHashes {
		if h != hash {
			ring = append(ring, h)
		}
	}
	m.radio.recentHashes = trimRadioRecent(append(ring, hash))
	return m
}

// radioRecentSet returns the recent-history ring as a set, the form
// radio.ScoreAll takes to apply its recently-played penalty.
func (m Model) radioRecentSet() map[string]struct{} {
	recentSet := make(map[string]struct{}, len(m.radio.recentHashes))
	for _, h := range m.radio.recentHashes {
		recentSet[h] = struct{}{}
	}
	return recentSet
}

// trimRadioRecent drops the oldest entries so the ring holds at most
// RadioRecentCap hashes, keeping the most recently appended ones.
func trimRadioRecent(hashes []string) []string {
	if len(hashes) <= RadioRecentCap {
		return hashes
	}
	return hashes[len(hashes)-RadioRecentCap:]
}

// buildTimeGenreBonus converts hour-genre affinity rows into a bonus map for
// the given hour. The top genre for the hour receives +3 and the runner-up
// receives +1. Affinities must be pre-sorted by (hour, count DESC) as returned
// by ReadHourGenreAffinities.
func buildTimeGenreBonus(affinities []db.HourGenreAffinity, hour int) map[string]float64 {
	bonus := make(map[string]float64)
	bonusValues := [2]float64{3, 1}
	rank := 0
	for _, a := range affinities {
		if a.Hour != hour {
			continue
		}
		bonus[a.Genre] = bonusValues[rank]
		rank++
		if rank >= len(bonusValues) {
			break
		}
	}
	return bonus
}

// handleRadioBatchEnqueuedMsg appends radio-picked tracks to the queue,
// triggers auto-resume if the queue was exhausted, and schedules the next
// low-water-mark check.
func (m Model) handleRadioBatchEnqueuedMsg(msg RadioBatchEnqueuedMsg) (Model, tea.Cmd) {
	// A disable, clear, replacement, or newer enqueue supersedes this result
	// regardless of what the queue looks like now. The rows were committed
	// before the message was delivered, so retire those exact rows before
	// discarding the in-memory result. Do not clear enqueuePending: it may
	// belong to the newer request whose identity caused this mismatch.
	if msg.RequestID != m.radio.requestID {
		return m.discardRadioResult(msg), nil
	}
	m.radio.enqueuePending = false

	now := time.Now()
	if msg.Err != nil {
		return m.recordRadioFailure(msg, now), nil
	}
	if len(msg.Entries) == 0 {
		return m.recordRadioNoBatch(msg), nil
	}
	// A cancelled result and a stale one are both discarded, but they are asked
	// for in opposite ways. Cancellation is the user saying stop, so it ends
	// there; staleness is the answer arriving too late to a question still worth
	// asking, so it is asked again.
	if m.radioResultCancelled(msg) {
		return m.discardRadioResult(msg), nil
	}
	if !m.radioInputsCurrent(msg, now) {
		return m.discardRadioResult(msg).retryRadioAfterStale()
	}
	if err := m.djPersister().accept(msg.prep, msg.QueueIDs, m.radioCommittedAllocation(msg)); err != nil {
		slog.Warn("radio batch publication failed", "err", err)
		m = m.discardRadioResult(msg)
		msg.Err, msg.transient = err, true
		return m.recordRadioFailure(msg, now), nil
	}
	return m.applyRadioBatch(msg, now)
}

// radioResultCancelled reports whether the user has withdrawn the request this
// result answers. Relaunching after either of these would undo a gesture, so
// they are kept apart from the staleness check that does relaunch.
func (m Model) radioResultCancelled(msg RadioBatchEnqueuedMsg) bool {
	// Radio was toggled off while the goroutine was in flight; publishing now
	// would resurrect the mode the user just cancelled.
	if !m.radio.enabled {
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
