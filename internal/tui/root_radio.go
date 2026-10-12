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
	enabled        bool
	seedHash       string
	syntheticSeed  *radio.SeedTrack // non-nil = use directly instead of hash lookup
	enqueuePending bool
	requestID      uint64
	recentHashes   []string // ring buffer, cap RadioRecentCap
	// stoppedByUser records that playback ended because the user asked it to —
	// an explicit Stop, or a stop-after-current that ran to completion — rather
	// than because the queue ran out. It suspends automatic refills until the
	// user deliberately starts something again; see radioNoteDeliberateStop.
	stoppedByUser bool
	// A successful command result must answer the current request before it
	// can lift a deliberate-stop suspension. Track starts also need a report;
	// unpausing the loaded file completes with the command itself.
	start           radioStartIntent
	cleanupBatches  [][]int64
	cleanupInFlight bool
}

// Reports do not carry command IDs. Remember a report received while this
// request is current, but never let it authorize refills before the call succeeds.
type radioStartIntent struct {
	request   uint64
	succeeded bool
	trackSeen bool
	unpause   bool
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
// playback not deliberately stopped, stopAfterCurrent not set, repeatMode !=
// RepeatAll, library non-empty, remaining <= low-water-mark, and the planner not
// parked on inputs that have not moved since it last came back with nothing.
// That last guard is what keeps an empty candidate set, an unresolved tag
// reference, or a failing writer from turning the low-water-mark check into a
// loop that re-asks the same question forever.
//
// The deliberate-stop guard covers the window the stopAfterCurrent flag cannot:
// once a stop-after-current has run to completion the flag is already back to
// false, and an explicit Stop never sets it at all, so both would otherwise
// leave an exhausted-looking queue that every later wakeup is free to refill.
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
	if m.radio.stoppedByUser {
		return false
	}
	if m.playback.stopAfterCurrent {
		return false
	}
	if m.playback.repeatMode == playback.RepeatAll {
		return false
	}
	if m.library.Index.Len() == 0 {
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

// radioNoteDeliberateStop retires automatic refill work at a deliberate stop
// boundary: an explicit Stop, or a stop-after-current the player ran out.
//
// Both are the user ending playback, and neither leaves an event behind that
// would ask the refill again — which is precisely why the park has to be retired
// here rather than left for some later guard to notice. A park records inputs
// the planner came back empty-handed from, or a write the database refused; the
// tick wakeup exists to re-ask exactly those once the writer recovers or an
// eligible track appears. After a deliberate stop that recovery is no longer
// something the user asked for, so the park is dropped and the suspension below
// keeps a fresh one from being launched in its place.
//
// The suspension is lifted by radioResumeAfterStop, from the gestures that mean
// the user wants audio again: a start they asked for and the player took, and
// any deliberate session boundary that passes through supersedeRadioBatch.
func (m Model) radioNoteDeliberateStop() Model {
	m.radio.stoppedByUser = true
	// A start dispatched before the stop is not a start the user still wants: the
	// stop is the newer gesture, so it withdraws the request as well as the
	// playback, and the report that request may still produce lifts nothing.
	m.radio.start = radioStartIntent{}
	m.djPlan.block = djPlanBlock{}
	m.djPlan.staleRetries = 0
	return m
}

// radioNoteStartRequested pairs the user's gesture with the shared player
// request ID. A newer start gesture replaces both halves of an older confirmation;
// unrelated controls may advance requestSeq without replacing this intent.
func (m Model) radioNoteStartRequested(request uint64) Model {
	m.radio.start = radioStartIntent{request: request}
	return m
}

// radioNoteStartResult records only the current request's outcome. In particular,
// a selection result can still pass playRequestSeq after a newer transport
// command; it must not authorize or withdraw that transport's start intent.
func (m Model) radioNoteStartResult(request uint64, err error) Model {
	if request == 0 || request != m.radio.start.request {
		return m
	}
	if err != nil {
		m.radio.start = radioStartIntent{}
		return m
	}
	m.radio.start.succeeded = true
	return m.radioConfirmStart()
}

// radioConfirmStartedTrack remembers an accepted report. It may precede the
// result, including when it is just a reconnect echo of the loaded occurrence.
// The result must still succeed before that report can lift the suspension.
func (m Model) radioConfirmStartedTrack() Model {
	if m.radio.start.request == 0 {
		return m
	}
	m.radio.start.trackSeen = true
	return m.radioConfirmStart()
}

func (m Model) radioConfirmStart() Model {
	if !m.radio.start.succeeded || (!m.radio.start.unpause && !m.radio.start.trackSeen) {
		return m
	}
	return m.radioResumeAfterStop()
}

// radioResumeAfterStop lifts the refill suspension a deliberate stop put in
// place. It says nothing about what the refill may plan — the ordinary gate
// still decides that — only that the user has asked for playback again and got
// it. The outstanding request is consumed with the suspension: it has been
// answered, so a later report must arm its own.
func (m Model) radioResumeAfterStop() Model {
	m.radio.stoppedByUser = false
	m.radio.start = radioStartIntent{}
	return m
}

// supersedeRadioBatch invalidates DJ work at a deliberate session or queue
// boundary and lifts any earlier stop suspension. Automatic Follow reseeding
// uses invalidateRadioBatch instead: a new seed is not a request for playback.
func (m Model) supersedeRadioBatch() Model {
	return m.radioResumeAfterStop().invalidateRadioBatch()
}

// invalidateRadioBatch retires every result created from the current Radio
// state without changing playback intent. The generation advances even when
// no batch is pending so each replacement has an unambiguous identity.
func (m Model) invalidateRadioBatch() Model {
	m.radio.requestID++
	m.radio.enqueuePending = false
	// Neither the park nor the stale-retry budget earned under the old planning
	// inputs applies to the new ones, whether the change was automatic or asked
	// for by the user.
	m.djPlan.block = djPlanBlock{}
	m.djPlan.staleRetries = 0
	// The immediate refresh is DJ work against the same boundary, so it is
	// withdrawn here too. This is the single point every transition that must not
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
