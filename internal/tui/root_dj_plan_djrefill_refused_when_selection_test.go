package tui

import (
	"errors"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestDJRefillRefusedWhenSelectionIsNowUpcoming is the other half of that
// leniency: rows entering the protected set can invalidate a selection, because
// the batch would then duplicate a track the user is about to hear anyway.
func TestDJRefillRefusedWhenSelectionIsNowUpcoming(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)

	m, msg := runRefill(t, m)

	// The user hand-queues one of the tracks the batch had picked.
	picked := msg.Entries[0]
	m.queue.Entries = append(m.queue.Entries, queue.Entry{
		ID: 99, Hash: picked.Hash, Path: picked.Path, Artist: picked.Artist, Title: picked.Title,
	})

	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	if len(writer.accepted) != 0 {
		t.Error("a batch duplicating an upcoming row was published")
	}
	if len(writer.rejected) != 1 {
		t.Errorf("rejected %d batches, want the colliding one retired", len(writer.rejected))
	}
}

// TestDJAccountingAdvancesOnlyOnAcceptance is the accepted-only rule. A batch the
// user never hears must not consume the discovery carry or burn its tracks into
// the avoidance ring, or a run of discarded batches would silently bias the
// session away from music it never actually played.
func TestDJAccountingAdvancesOnlyOnAcceptance(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)
	m.dj.allocation = radio.Allocation{CarryUnits: 2}

	m, msg := runRefill(t, m)
	if msg.plan.NextAllocation == (radio.Allocation{CarryUnits: 2}) {
		t.Skip("the fixture's plan left the carry unchanged; the assertion below cannot discriminate")
	}
	recentBefore := len(m.radioRecentSet())

	discarded := m.supersedeRadioBatch()
	discarded, _ = discarded.handleRadioBatchEnqueuedMsg(msg)

	if discarded.dj.allocation != (radio.Allocation{CarryUnits: 2}) {
		t.Errorf("a discarded batch spent the carry: %v, want the original 2 units",
			discarded.dj.allocation)
	}
	if got := len(discarded.radioRecentSet()); got != recentBefore {
		t.Errorf("a discarded batch added %d hashes to the avoidance ring", got-recentBefore)
	}

	accepted, _ := m.handleRadioBatchEnqueuedMsg(msg)
	if accepted.dj.allocation != msg.plan.NextAllocation.Normalized() {
		t.Errorf("an accepted batch left the carry at %v, want %v",
			accepted.dj.allocation, msg.plan.NextAllocation.Normalized())
	}
	if got := len(accepted.radioRecentSet()); got != recentBefore+len(msg.Entries) {
		t.Errorf("the avoidance ring holds %d hashes, want %d", got, recentBefore+len(msg.Entries))
	}
}

// TestDJAcceptedAllocationIsNotWrittenTwice pins where the carry is persisted:
// the accept transaction already wrote it, and a second write from the update
// loop would rewrite the whole session row — filters included — from a Model
// whose settings may have moved on.
func TestDJAcceptedAllocationIsNotWrittenTwice(t *testing.T) {
	writer := &stubDJQueueWriter{}
	sessions := &djStubWriter{}
	m := djRefillModel(writer)
	m.djWriter = sessions

	m, msg := runRefill(t, m)
	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	if len(writer.commits) != 1 || writer.commits[0].Allocation == nil {
		t.Fatalf("the accept transaction did not carry the allocation: %+v", writer.commits)
	}
	if *writer.commits[0].Allocation != msg.plan.NextAllocation {
		t.Errorf("committed allocation %v, want the plan's %v",
			*writer.commits[0].Allocation, msg.plan.NextAllocation)
	}
	if len(sessions.writes) != 0 {
		t.Errorf("accepting a batch also rewrote the session row %d times", len(sessions.writes))
	}
}

// TestDJCarryIsNotStampedWithoutASession guards the other end of the same
// column. The carry is the remainder of one session's rounded discovery split;
// the write is unconditional on the single session row, so committing one from
// plain Radio would leave it on a stopped session for the next start to
// inherit — biasing a first batch for a reason the user cannot see.
func TestDJCarryIsNotStampedWithoutASession(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)
	m.dj.active = false
	m.radio.seedHash = "f1"

	m, msg := runRefill(t, m)
	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	if len(writer.commits) != 1 {
		t.Fatalf("the batch was committed %d times, want exactly 1", len(writer.commits))
	}
	if writer.commits[0].Allocation != nil {
		t.Errorf("a sessionless refill stamped carry %v on the session row",
			*writer.commits[0].Allocation)
	}
	if m.dj.allocation != (radio.Allocation{}) {
		t.Errorf("a sessionless refill moved the in-memory carry to %v", m.dj.allocation)
	}
}

// TestDJEmptyPlanParksUntilInputsMove is the no-spin contract for the ordinary
// blocked state. "No candidates pauses additions with an explanation" is only
// honoured if the guard then refuses to ask the same question a second later.
func TestDJEmptyPlanParksUntilInputsMove(t *testing.T) {
	m := djRefillModel(&stubDJQueueWriter{})

	m.radio.enqueuePending = false
	m, _ = m.handleRadioBatchEnqueuedMsg(RadioBatchEnqueuedMsg{
		RequestID:   m.radio.requestID,
		WasEmpty:    false,
		fingerprint: m.djLiveFingerprint(time.Now()),
		plan: radio.PlanResult{
			Outcome: radio.PlanOutcomeEmpty,
			Reasons: radio.Reasons{{Code: radio.ReasonNoCandidates}},
		},
	})

	if !m.DJPlanBlocked() {
		t.Fatal("an empty plan left the refill unparked, so the next tick re-asks it")
	}
	if len(m.djPlanBlockReasons()) == 0 {
		t.Error("the refill parked without recording why")
	}
	if m.radioCanEnqueue() {
		t.Error("a parked refill was re-admitted against inputs that had not moved")
	}

	moved := m
	moved.djInvalidate()
	if !moved.radioCanEnqueue() {
		t.Error("a refill stayed parked after the library moved underneath it")
	}
}

// TestDJWriteFailuresAreBoundedNotEndless covers the transient blocked state. A
// refused write is worth re-attempting against the same inputs, but a database
// that refuses three identical writes will not accept the fourth, and the
// one-second tick would otherwise turn it into a write loop.
func TestDJWriteFailuresAreBoundedNotEndless(t *testing.T) {
	writer := &stubDJQueueWriter{prepareErr: errors.New("disk full")}
	m := djRefillModel(writer)

	attempts := 0
	for i := 0; i < djMaxTransientRetries+3; i++ {
		next, cmd := m.startRadioEnqueueCmd()
		if cmd == nil {
			break
		}
		attempts++
		m = next
		m, _ = m.handleRadioBatchEnqueuedMsg(cmd().(RadioBatchEnqueuedMsg))
	}

	if attempts != djMaxTransientRetries {
		t.Errorf("a permanently failing writer was attempted %d times, want %d",
			attempts, djMaxTransientRetries)
	}
	if !m.DJPlanBlocked() || m.djPlanBlockErr() == nil {
		t.Error("a failing writer did not park the refill with its error")
	}
	if m.radioCanEnqueue() {
		t.Error("the refill kept re-admitting itself after exhausting its retries")
	}
	moved := m
	moved.djInvalidate()
	if !moved.radioCanEnqueue() {
		t.Error("a moved library did not clear the exhausted retry budget")
	}
}

// TestDJFailedRejectionIsRetriedUntilItWins proves hidden rows cannot outlive the
// result that disowned them. A preparation left pending would be republished by
// the next accept that names it, so a refused rejection is retained and retried
// on the tick rather than logged and forgotten.
func TestDJFailedRejectionIsRetriedUntilItWins(t *testing.T) {
	writer := &stubDJQueueWriter{rejectErr: errors.New("database is locked")}
	m := djRefillModel(writer)

	m, msg := runRefill(t, m)
	m = m.supersedeRadioBatch()
	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	if len(m.djPlan.rejects) != 1 {
		t.Fatalf("retained %d failed rejections, want the one that was refused", len(m.djPlan.rejects))
	}

	m, cmd := m.startDJRejectRetryCmd()
	if cmd == nil {
		t.Fatal("no retry was scheduled for a refused rejection")
	}
	if _, again := m.startDJRejectRetryCmd(); again != nil {
		t.Error("a second retry was scheduled while one was already in flight")
	}
	m = m.handleDJRejectRetryMsg(cmd().(djRejectRetryMsg))
	if len(m.djPlan.rejects) != 1 {
		t.Error("a rejection that failed again was dropped from the retry queue")
	}

	writer.rejectErr = nil
	m, cmd = m.startDJRejectRetryCmd()
	if cmd == nil {
		t.Fatal("no retry was scheduled after the first one failed")
	}
	m = m.handleDJRejectRetryMsg(cmd().(djRejectRetryMsg))
	if len(m.djPlan.rejects) != 0 {
		t.Errorf("a successful rejection stayed in the retry queue: %v", m.djPlan.rejects)
	}
}

// TestDJRejectionOfAlreadyRecoveredRowsIsSettled records that restart recovery
// doing the work first is success, not failure: rows converted into ordinary
// tombstones are exactly what the rejection wanted.
func TestDJRejectionOfAlreadyRecoveredRowsIsSettled(t *testing.T) {
	writer := &stubDJQueueWriter{rejectErr: db.ErrQueueBatchStale}
	m := djRefillModel(writer)

	m, msg := runRefill(t, m)
	m = m.supersedeRadioBatch()
	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	if len(m.djPlan.rejects) != 0 {
		t.Errorf("a batch recovery had already retired was queued for retry: %v", m.djPlan.rejects)
	}
}

// TestDJRefillDegradesWithoutAProvenanceWriter keeps the pre-DJ writers usable:
// a Model built without the queue writer still refills, but must not claim
// ownership the database never recorded, or a later refresh would try to replace
// a row that is not DJ-owned.
func TestDJRefillDegradesWithoutAProvenanceWriter(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := djRefillModel(nil)
	m.djQueue = nil
	m.batchEnqueuer = be

	m, msg := runRefill(t, m)
	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	if len(be.appendTracksCalls) != 1 {
		t.Fatalf("appended %d batches through the fallback writer, want 1", len(be.appendTracksCalls))
	}
	if got := be.appendTracksSources[0]; got != djQueueSource {
		t.Errorf("batch appended under source %q, want %q", got, djQueueSource)
	}
	for _, e := range m.queue.Entries[1:] {
		if e.DJOwned {
			t.Errorf("row %q claims ownership the writer never recorded", e.Hash)
		}
	}
	if m.dj.allocation != msg.plan.NextAllocation.Normalized() {
		t.Errorf("carry = %v, want the accepted plan's %v", m.dj.allocation, msg.plan.NextAllocation)
	}
}

// TestDJRefillWithoutAnyWriterIsRefused proves the planner is never run for a
// batch that has nowhere to go: planning is the expensive half, and a Model with
// no writer cannot publish whatever it produced.
func TestDJRefillWithoutAnyWriterIsRefused(t *testing.T) {
	m := djRefillModel(nil)
	m.djQueue = nil
	m.batchEnqueuer = nil

	if _, cmd := m.startRadioEnqueueCmd(); cmd != nil {
		t.Error("a refill was launched with no writer to publish it")
	}
}

// TestDJPlanSeedPrefersTheSessionSeed pins seed resolution: the persisted
// lifecycle seed is what the user's start gesture chose and what survives a
// restart, so a legacy Radio hash must not quietly outrank it.
func TestDJPlanSeedPrefersTheSessionSeed(t *testing.T) {
	m := djRefillModel(&stubDJQueueWriter{})
	m.radio.seedHash = "d3"

	if got := m.djPlanSeed(time.Now()).TrackHash; got != "f1" {
		t.Errorf("planned from seed %q, want the session's %q", got, "f1")
	}

	m.dj.seed = radio.SeedSnapshot{}
	if got := m.djPlanSeed(time.Now()).TrackHash; got != "d3" {
		t.Errorf("without a session seed the plan used %q, want the radio seed %q", got, "d3")
	}

	m.radio.seedHash = ""
	m.playback.currentHash = ""
	if seed := m.djPlanSeed(time.Now()); seed.IsZero() {
		t.Error("a seedless Model produced a zero seed, which the planner refuses outright")
	}
}

// TestDJQueueRevisionIgnoresOrderAndTracksMembership pins the fingerprint's queue
// half: it must reduce a set, so a re-sort does not throw a plan away, while a
// row genuinely entering or leaving it does move the revision.
func TestDJQueueRevisionIgnoresOrderAndTracksMembership(t *testing.T) {
	a := map[string]struct{}{"x": {}, "y": {}, "z": {}}
	b := map[string]struct{}{"z": {}, "y": {}, "x": {}}
	if djQueueRevisionOf(a) != djQueueRevisionOf(b) {
		t.Error("the same protected rows in a different order reduced differently")
	}
	if djQueueRevisionOf(a) == djQueueRevisionOf(map[string]struct{}{"x": {}, "y": {}}) {
		t.Error("dropping a protected row left the revision unchanged")
	}
	if djQueueRevisionOf(nil) == djQueueRevisionOf(map[string]struct{}{"x": {}}) {
		t.Error("an empty protected set matched a populated one")
	}
}

// TestDJPlanFingerprintNoticesTheHourRolling covers the input no writer
// announces: the time-of-day bonus is keyed by the local hour and the discovery
// cutoff slides with the calendar day, so a Muse left running overnight must not
// treat an old plan as current.
func TestDJPlanFingerprintNoticesTheHourRolling(t *testing.T) {
	m := djRefillModel(&stubDJQueueWriter{})
	at := m.djLiveFingerprint(djFixedNow)

	if !at.sameInputs(m.djLiveFingerprint(djFixedNow.Add(20 * time.Minute))) {
		t.Error("inputs were called different within the same hour")
	}
	if at.sameInputs(m.djLiveFingerprint(djFixedNow.Add(time.Hour))) {
		t.Error("the hour rolled over and the inputs were still called the same")
	}
}

// plannedFixtureTracks is the shape djPlanAndPrepare hands the writers when the
// planner produced annotated selections.
func plannedFixtureTracks() []radio.PlannedTrack {
	return []radio.PlannedTrack{
		{Track: library.Track{Hash: "d1", Path: "/d1.flac"}, Reasons: radio.Reasons{{Code: radio.ReasonDiscoveryNever}}},
		{Track: library.Track{Hash: "f2", Path: "/f2.flac"}},
	}
}

// TestDJQueueEntriesCarryOnlyRecordedProvenance keeps the in-memory rows honest
// about what was persisted, which is what stops the TUI offering a refresh the
// database would refuse.
func TestDJQueueEntriesCarryOnlyRecordedProvenance(t *testing.T) {
	picked := plannedFixtureTracks()
	ids := []int64{11, 12}

	owned := newDJQueueEntries(picked, ids, true)
	if len(owned) != len(picked) {
		t.Fatalf("built %d entries for %d selections", len(owned), len(picked))
	}
	if !owned[0].DJOwned || len(owned[0].DJReasons) != 1 {
		t.Errorf("a recorded selection lost its provenance: %+v", owned[0])
	}
	if !owned[1].DJOwned {
		t.Error("a selection with no reasons also lost its ownership")
	}

	plain := newDJQueueEntries(picked, ids, false)
	if plain[0].DJOwned || len(plain[0].DJReasons) != 0 {
		t.Errorf("an unrecorded selection claimed provenance: %+v", plain[0])
	}
}
