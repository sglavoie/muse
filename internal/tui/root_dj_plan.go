package tui

import (
	"errors"
	"hash/fnv"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// djQueueSource is the display label written on DJ-generated queue rows.
// Ownership is never read back from it — that is what queue.Entry.DJOwned and
// the stored provenance column are for.
const djQueueSource = "Radio"

// djMaxTransientRetries bounds how often the live refill re-attempts the same
// inputs after a failed write.
//
// A durable failure that repeats is not going to be fixed by a fourth identical
// attempt, and the one-second tick would otherwise turn it into a write loop
// against a database that is already unhappy. Once the bound is spent the refill
// stays parked until some input actually moves.
const djMaxTransientRetries = 3

// errDJPersistenceUnavailable reports that no writer was injected, so a planned
// batch has nowhere to go.
var errDJPersistenceUnavailable = errors.New("tui: no dj queue writer available")

// djPlanFingerprint names the inputs one asynchronous planning attempt was
// launched from: the data snapshot, the queue rows it had to avoid, the settings
// in force, and the seed.
//
// Every one of these can move while the goroutine runs, and each moves the
// answer: a changed library re-classifies discovery, a changed queue changes
// what may not be selected again, changed settings change the filters, and a
// changed seed changes what the batch is about. Validating the whole tuple
// before acceptance is what stops a result computed for one session from being
// committed into another.
type djPlanFingerprint struct {
	dataRev    uint64
	queueRev   uint64
	settings   radio.Settings
	seedHash   string
	capturedAt time.Time
}

// sameInputs reports whether two attempts describe the same planning inputs.
//
// The hour comparison is not decoration: the time-of-day affinity bonus is keyed
// by the local hour and the ninety-day discovery cutoff slides with the calendar
// day, so a rollover moves inputs that no writer announces.
func (f djPlanFingerprint) sameInputs(other djPlanFingerprint) bool {
	return f.dataRev == other.dataRev &&
		f.queueRev == other.queueRev &&
		f.seedHash == other.seedHash &&
		!djHourChanged(f.capturedAt, other.capturedAt) &&
		f.settings.Equal(other.settings)
}

// djPlanBlock records why live refill is parked and the inputs it was parked at.
//
// Parking is what keeps an empty library, an unresolved tag reference, or a
// broken writer from turning the low-water-mark check into a spin: the guard
// refuses to re-plan until sameInputs says something moved. A transient failure
// is the one case that may re-attempt unchanged inputs, and only
// djMaxTransientRetries times.
type djPlanBlock struct {
	active    bool
	at        djPlanFingerprint
	reasons   radio.Reasons
	err       error
	retriable bool
	attempts  int
}

// djPlanStatus is the explicit record of what the last live attempt produced,
// including the parts a batch cannot express: how far short of the discovery
// target it fell, why it was refused, and which behavioral signals were missing
// when it was scored.
type djPlanStatus struct {
	outcome   radio.PlanOutcome
	shortages radio.Shortages
	reasons   radio.Reasons
	degraded  []djSignalFailure
	accepted  int
	err       error
	at        time.Time
}

// djPreview is the latest draft plan, held for the Preview tab.
//
// A preview is a plan whose NextAllocation the caller threw away. Nothing else
// distinguishes it from a live batch, which is exactly why it is kept in its own
// field, behind its own request counter, and never merged into the live state.
type djPreview struct {
	valid    bool
	settings radio.Settings
	result   radio.PlanResult
	signals  djSignals
	err      error
	at       time.Time
	// fingerprint names the inputs this draft was computed from, so the Preview
	// tab can reuse it while they hold and replace it once they move.
	fingerprint djPlanFingerprint
}

// djPlanState is the Model's bookkeeping for asynchronous Auto-DJ planning.
//
// The live identity lives in radioState.requestID, which the queue-replacement
// boundaries already advance. Preview carries a separate counter on purpose: a
// draft the user is scrubbing through in the Tune tab must never be able to
// retire a refill that is keeping playback alive.
type djPlanState struct {
	previewID      uint64
	previewPending bool
	// previewSettings is the draft the in-flight preview was launched for, so a
	// second gesture arriving while it runs can tell "already being planned"
	// from "planned for something else".
	previewSettings radio.Settings
	preview         djPreview

	block  djPlanBlock
	status djPlanStatus

	// staleRetries bounds the immediate relaunches that follow a result refused
	// for moved inputs, so inputs churning faster than planning end as a stalled
	// refill rather than as a planning loop.
	staleRetries int

	// rejects retains preparations whose compensating RejectDJBatch failed, so
	// the hidden rows cannot outlive the result that disowned them.
	rejects         []db.DJPreparation
	rejectInFlight  bool
	rejectAttempted db.DJPreparation
}

// DJPlanBlocked reports whether live refill is parked on unchanged inputs.
func (m Model) DJPlanBlocked() bool { return m.djPlan.block.active }

// djPlanBlockReasons returns the planner's explanation for a parked refill.
func (m Model) djPlanBlockReasons() radio.Reasons { return m.djPlan.block.reasons }

// djPlanBlockErr returns the failure that parked the refill, or nil.
func (m Model) djPlanBlockErr() error { return m.djPlan.block.err }

// djPlanStatusOf returns the record of the last live attempt.
func (m Model) djPlanStatusOf() djPlanStatus { return m.djPlan.status }

// djPreviewResult returns the latest draft plan.
func (m Model) djPreviewResult() djPreview { return m.djPlan.preview }

// djQueueRevisionOf reduces a protected-hash set to one comparable token.
//
// It is a fingerprint recomputed from the queue rather than a counter bumped by
// every mutation, for the reason internal/player recomputes its refresh
// fingerprint instead of counting commits: the queue slice is written from
// roughly thirty places in this package, and a counter is only as correct as the
// least-maintained of them. Reducing the set itself cannot be forgotten by a new
// call site, and it also ignores the churn that does not matter — a selection
// toggle, a metadata edit, a re-sort — so a plan is not thrown away for a change
// that could not have altered it.
//
// XOR is the right combiner here because the input is a set: equal sets reduce
// equal whatever the map iteration order, and the length is folded in so that a
// subset cannot collide with its superset for free.
func djQueueRevisionOf(protected map[string]struct{}) uint64 {
	var mixed uint64
	for h := range protected {
		x := fnv.New64a()
		_, _ = x.Write([]byte(h))
		mixed ^= x.Sum64()
	}
	return mixed ^ uint64(len(protected))*0x9E3779B97F4A7C15
}

// djQueueRevision returns the current queue half of a planning fingerprint.
func (m Model) djQueueRevision() uint64 {
	return djQueueRevisionOf(m.djProtectedHashes())
}

// djSeedHash returns the hash a plan would be seeded from, without capturing a
// snapshot. The lifecycle seed wins; the legacy Radio seed is the fallback for a
// Model whose DJ session was never started.
func (m Model) djSeedHash() string {
	if !m.dj.seed.IsZero() {
		return m.dj.seed.TrackHash
	}
	if m.radio.seedHash != "" {
		return m.radio.seedHash
	}
	return m.playback.currentHash
}

// djPlanSeed resolves the seed one plan is built from.
//
// The persisted lifecycle seed is authoritative: it is the one that survives a
// restart and the one the user's start gesture chose. The fallback below covers
// a Model that has Radio enabled with no DJ session behind it — a test stub, or
// a session restored from a build that predates the lifecycle — and still yields
// a plannable seed, because radio.PlanRequest.Validate refuses a zero one.
func (m Model) djPlanSeed(now time.Time) radio.SeedSnapshot {
	if !m.dj.seed.IsZero() {
		return m.dj.seed
	}
	clock := radio.FixedClock(now)
	if hash := m.djSeedHash(); hash != "" {
		if t, ok := m.library.Index.FindByHash(hash); ok {
			return radio.CaptureSeed(t, m.trackTags[hash], clock)
		}
	}
	return radio.WholeLibrarySeed(clock)
}

// djFingerprint stamps the settings and seed in force onto revisions the caller
// already has. A launched refill uses the revisions its own snapshot recorded,
// so the fingerprint describes the inputs the plan actually ran on rather than
// whatever they had drifted to by the time the goroutine was built.
func (m Model) djFingerprint(dataRev, queueRev uint64, now time.Time) djPlanFingerprint {
	return djPlanFingerprint{
		dataRev:    dataRev,
		queueRev:   queueRev,
		settings:   m.dj.settings,
		seedHash:   m.djSeedHash(),
		capturedAt: now,
	}
}

// djLiveFingerprint describes the inputs a live refill would be planned from
// right now. It deliberately reads no snapshot: the gate below runs on every
// low-water-mark check and must stay cheap.
func (m Model) djLiveFingerprint(now time.Time) djPlanFingerprint {
	return m.djFingerprint(m.djData.dataRev, m.djQueueRevision(), now)
}

// djLiveAllowed reports whether a live refill may be launched, given whatever
// parked the last one.
func (m Model) djLiveAllowed(now time.Time) bool {
	b := m.djPlan.block
	if !b.active {
		return true
	}
	if !b.at.sameInputs(m.djLiveFingerprint(now)) {
		return true
	}
	return b.retriable && b.attempts < djMaxTransientRetries
}
