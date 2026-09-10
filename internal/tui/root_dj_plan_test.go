package tui

import (
	"errors"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// stubDJQueueWriter records the two-phase publication a DJ batch goes through,
// so a test can tell "written but hidden" apart from "published" — the whole
// point of the boundary, and the only way to prove a discarded batch left
// nothing visible behind.
type stubDJQueueWriter struct {
	prepared [][]db.DJQueueEntry
	accepted []db.DJPreparation
	commits  []db.DJCommit
	rejected []db.DJPreparation

	prepareErr error
	acceptErr  error
	rejectErr  error

	nextID int64
}

func (s *stubDJQueueWriter) PrepareDJBatch(entries []db.DJQueueEntry) (db.DJPreparation, error) {
	s.prepared = append(s.prepared, append([]db.DJQueueEntry(nil), entries...))
	if s.prepareErr != nil {
		return db.DJPreparation{}, s.prepareErr
	}
	ids := make([]int64, len(entries))
	for i := range entries {
		s.nextID++
		ids[i] = 500 + s.nextID
	}
	return db.DJPreparation{Kind: db.DJPrepareAppend, IDs: ids}, nil
}

func (s *stubDJQueueWriter) AcceptDJBatch(prep db.DJPreparation, commit db.DJCommit) (db.DJAcceptance, error) {
	s.accepted = append(s.accepted, prep)
	s.commits = append(s.commits, commit)
	if s.acceptErr != nil {
		return db.DJAcceptance{}, s.acceptErr
	}
	return db.DJAcceptance{Activated: prep.IDs}, nil
}

func (s *stubDJQueueWriter) RejectDJBatch(prep db.DJPreparation) error {
	s.rejected = append(s.rejected, prep)
	return s.rejectErr
}

// djRefillModel returns a Model that a live refill will actually fire from: an
// active session over the planning fixture, radio on, and a queue drained to the
// low-water mark with the seed still playing.
func djRefillModel(writer *stubDJQueueWriter) Model {
	m := newDJModel(WithRecommendationReader(newDJRecReader()), WithDJQueueWriter(writer))
	m.radio.enabled = true
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "f1", Path: "/f1.flac", Artist: "Alice", Title: "F1"}}
	m.playback.currentIndex = 0
	m.playback.currentHash = "f1"
	m.playback.state = playback.StatePlaying
	m.dj.active = true
	m.dj.seed = djTestRequest().Seed
	return m
}

// runRefill launches a live refill and runs its command, returning the message
// the real runtime would deliver. It deliberately does not deliver it: holding
// the result is what lets a test move the world underneath it.
func runRefill(t *testing.T, m Model) (Model, RadioBatchEnqueuedMsg) {
	t.Helper()
	next, cmd := m.startRadioEnqueueCmd()
	if cmd == nil {
		t.Fatalf("no refill launched: remaining = %d, blocked = %v",
			next.radioRemainingCount(), next.DJPlanBlocked())
	}
	msg, ok := cmd().(RadioBatchEnqueuedMsg)
	if !ok {
		t.Fatal("the refill command did not produce a batch message")
	}
	if msg.Err != nil {
		t.Fatalf("planning the fixture failed: %v", msg.Err)
	}
	if len(msg.Entries) == 0 {
		t.Fatal("the fixture produced no batch, so the assertions below prove nothing")
	}
	return next, msg
}

// TestDJRefillPublishesThroughPlanner is the happy path end to end: the batch is
// written hidden, published only once the result is accepted, and lands in the
// queue carrying the provenance that makes it refreshable later.
func TestDJRefillPublishesThroughPlanner(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)

	m, msg := runRefill(t, m)
	if len(writer.prepared) != 1 {
		t.Fatalf("prepared %d batches, want exactly 1 before acceptance", len(writer.prepared))
	}
	if len(writer.accepted) != 0 {
		t.Fatal("the batch was published before its result was accepted")
	}

	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	if len(writer.accepted) != 1 {
		t.Fatalf("accepted %d batches, want exactly 1", len(writer.accepted))
	}
	if len(writer.rejected) != 0 {
		t.Errorf("an accepted batch was also rejected: %v", writer.rejected)
	}
	if got := len(m.queue.Entries); got != 1+len(msg.Entries) {
		t.Errorf("queue holds %d entries, want %d", got, 1+len(msg.Entries))
	}
	for _, e := range m.queue.Entries[1:] {
		if !e.DJOwned {
			t.Errorf("row %q was appended without DJ ownership, so no refresh can reach it", e.Hash)
		}
		if len(e.DJReasons) == 0 {
			t.Errorf("row %q was appended with no explanation of why it was chosen", e.Hash)
		}
	}
	if m.radio.enqueuePending {
		t.Error("enqueuePending survived an accepted batch")
	}
	if m.DJPlanBlocked() {
		t.Error("an accepted batch left the refill parked")
	}
}

// TestDJStatusRecordsWhatABatchCannotSay covers the explicit-representation
// rule. A batch of five tracks looks identical whether it was planned from
// complete data or from a library whose behavioral signals were unavailable and
// whose discovery quota came up short, so those facts are recorded beside it
// rather than inferred from its length.
func TestDJStatusRecordsWhatABatchCannotSay(t *testing.T) {
	writer := &stubDJQueueWriter{}
	reader := newDJRecReader()
	reader.completionErr = errors.New("play events unavailable")
	m := djRefillModel(writer)
	m.recReader = reader

	m, msg := runRefill(t, m)
	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	status := m.djPlanStatusOf()
	if status.accepted != len(msg.Entries) {
		t.Errorf("status counted %d accepted tracks, want %d", status.accepted, len(msg.Entries))
	}
	if len(status.degraded) == 0 {
		t.Error("a batch scored without completion rates was recorded as fully informed")
	}
	if status.err != nil {
		t.Errorf("a published batch recorded an error: %v", status.err)
	}

	// A failed write is the other thing the queue cannot show: nothing changed,
	// and the reason has to survive somewhere the UI can reach.
	writer.acceptErr = errors.New("disk full")
	failing := djRefillModel(writer)
	failing.recReader = reader
	failing, failMsg := runRefill(t, failing)
	failing, _ = failing.handleRadioBatchEnqueuedMsg(failMsg)

	if failing.djPlanStatusOf().err == nil {
		t.Error("a refused write left no record of why nothing was added")
	}
	if !failing.DJPlanBlocked() {
		t.Error("a refused write did not park the refill")
	}
	if len(writer.rejected) == 0 {
		t.Error("a batch that could not be published was left hidden in the database")
	}
}

// TestDJPreviewTouchesNothingLive is the purity contract. A draft is a plan whose
// allocation the caller threw away; if any of these move, the Tune tab is
// silently spending the session's state to render a preview.
func TestDJPreviewTouchesNothingLive(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)
	m = m.radioNoteRecent("already-heard")
	m.dj.allocation = radio.Allocation{CarryUnits: 3}

	before := m
	next, cmd := m.djStartPreviewCmd(radio.DefaultSettings(), time.Now())
	if cmd == nil {
		t.Fatal("no preview command was returned")
	}
	msg, ok := cmd().(djPreviewPlannedMsg)
	if !ok {
		t.Fatal("the preview command did not produce a preview message")
	}
	next = next.handleDJPreviewPlannedMsg(msg)

	if len(writer.prepared) != 0 || len(writer.accepted) != 0 || len(writer.rejected) != 0 {
		t.Errorf("a preview wrote to the queue: prepared=%v accepted=%v rejected=%v",
			writer.prepared, writer.accepted, writer.rejected)
	}
	if len(next.queue.Entries) != len(before.queue.Entries) {
		t.Errorf("a preview changed the queue: %d entries, want %d",
			len(next.queue.Entries), len(before.queue.Entries))
	}
	if next.dj.allocation != before.dj.allocation {
		t.Errorf("a preview spent the discovery carry: %v, want %v",
			next.dj.allocation, before.dj.allocation)
	}
	if len(next.radioRecentSet()) != len(before.radioRecentSet()) {
		t.Errorf("a preview advanced the recent ring: %v", next.radioRecentSet())
	}
	if next.radio.requestID != before.radio.requestID {
		t.Error("a preview advanced the live request identity")
	}
	if next.radio.enqueuePending != before.radio.enqueuePending {
		t.Error("a preview changed whether a live refill is pending")
	}
	if !next.djPreviewResult().valid || len(next.djPreviewResult().result.Tracks) == 0 {
		t.Error("the preview produced nothing to show")
	}
}

// TestDJPreviewCannotSupersedeLiveRefill pins the reason preview carries its own
// counter: a draft the user scrubs up in the Tune tab must not retire the refill
// that is keeping playback alive.
func TestDJPreviewCannotSupersedeLiveRefill(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)

	m, live := runRefill(t, m)
	m, _ = m.djStartPreviewCmd(radio.DefaultSettings(), time.Now())
	m = m.djSupersedePreview()

	m, _ = m.handleRadioBatchEnqueuedMsg(live)

	if len(writer.accepted) != 1 {
		t.Fatalf("the live batch was not published after preview activity: accepted=%v rejected=%v",
			writer.accepted, writer.rejected)
	}
	if len(m.queue.Entries) == 1 {
		t.Error("preview activity discarded the live refill")
	}
}

// TestDJPreviewResultsAreOrdered proves a slow draft cannot overwrite the fast
// one that superseded it, which is what a user scrubbing the discovery slider
// produces: several drafts in flight, finishing in any order.
func TestDJPreviewResultsAreOrdered(t *testing.T) {
	m := djRefillModel(&stubDJQueueWriter{})

	first, firstCmd := m.djStartPreviewCmd(radio.DefaultSettings(), time.Now())
	settings := radio.DefaultSettings()
	settings.DiscoveryPercent = 80
	second, secondCmd := first.djStartPreviewCmd(settings, time.Now())

	// The newer draft resolves first; the older one arrives afterwards.
	second = second.handleDJPreviewPlannedMsg(secondCmd().(djPreviewPlannedMsg))
	second = second.handleDJPreviewPlannedMsg(firstCmd().(djPreviewPlannedMsg))

	if got := second.djPreviewResult().settings.DiscoveryPercent; got != 80 {
		t.Errorf("preview shows discovery %d%%, want the newest draft's 80%%", got)
	}
	if second.djPlan.previewPending {
		t.Error("a superseded draft cleared the pending flag of the newer one")
	}
}

// TestDJSupersededResultRetiresItsRows is the cancellation contract: a clear, a
// disable, or a replacement moves the request identity, and the rows the
// abandoned goroutine already wrote must not be left hidden in the database.
func TestDJSupersededResultRetiresItsRows(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)

	m, msg := runRefill(t, m)
	m = m.supersedeRadioBatch()
	m.radio.enqueuePending = true // the newer request is the one now pending

	m, cmd := m.handleRadioBatchEnqueuedMsg(msg)

	if len(writer.rejected) != 1 {
		t.Fatalf("rejected %d batches, want the superseded one retired", len(writer.rejected))
	}
	if len(writer.accepted) != 0 {
		t.Error("a superseded batch was published")
	}
	if len(m.queue.Entries) != 1 {
		t.Errorf("a superseded batch reached the queue: %d entries", len(m.queue.Entries))
	}
	if !m.radio.enqueuePending {
		t.Error("a superseded result cleared the pending flag of the request that replaced it")
	}
	if cmd != nil {
		t.Error("a superseded result scheduled more work")
	}
}

// TestDJDisabledResultDoesNotRelaunch separates the two ways a result is thrown
// away: the user turning Radio off is an answer, not a lost race, so nothing may
// ask the question again.
func TestDJDisabledResultDoesNotRelaunch(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)

	m, msg := runRefill(t, m)
	m.radio.enabled = false

	m, cmd := m.handleRadioBatchEnqueuedMsg(msg)

	if cmd != nil {
		t.Error("disabling radio relaunched the refill it cancelled")
	}
	if len(writer.rejected) != 1 {
		t.Errorf("rejected %d batches, want the cancelled one retired", len(writer.rejected))
	}
	if len(m.queue.Entries) != 1 {
		t.Errorf("a cancelled batch reached the queue: %d entries", len(m.queue.Entries))
	}
}

// TestDJStaleResultIsRetiredAndReplanned covers the delayed result. Inputs that
// moved mean the answer is about a library that no longer exists, but the
// question is still live, so the rows go and the refill is asked again — without
// that relaunch a batch refused at the moment the queue ran dry would leave
// nothing scheduled to wake the DJ.
func TestDJStaleResultIsRetiredAndReplanned(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)

	m, msg := runRefill(t, m)
	m.djInvalidate() // a scan landed while the goroutine was planning

	m, cmd := m.handleRadioBatchEnqueuedMsg(msg)

	if len(writer.rejected) != 1 {
		t.Fatalf("rejected %d batches, want the stale one retired", len(writer.rejected))
	}
	if len(writer.accepted) != 0 {
		t.Error("a batch planned against a moved library was published")
	}
	if len(m.queue.Entries) != 1 {
		t.Errorf("a stale batch reached the queue: %d entries", len(m.queue.Entries))
	}
	if cmd == nil {
		t.Fatal("a stale result left nothing scheduled, so the DJ would go silent")
	}
	if !m.radio.enqueuePending {
		t.Error("the relaunched refill was not marked pending")
	}
}

// TestDJStaleRetriesAreBounded proves the relaunch above cannot become a loop:
// inputs churning faster than planning must end as a stalled refill, not as a
// planner spinning against a library that never settles.
func TestDJStaleRetriesAreBounded(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)

	// Follow the relaunch chain itself rather than re-admitting from the top:
	// each discarded result hands back the command for its own retry, and a
	// library that keeps moving makes every one of them go stale too.
	m, cmd := m.startRadioEnqueueCmd()
	if cmd == nil {
		t.Fatal("no refill launched")
	}
	discarded := 0
	for cmd != nil {
		msg, ok := cmd().(RadioBatchEnqueuedMsg)
		if !ok || len(msg.Entries) == 0 {
			t.Fatal("the refill command did not produce a usable batch")
		}
		m.djInvalidate() // a scan lands under every single attempt
		m, cmd = m.handleRadioBatchEnqueuedMsg(msg)
		discarded++
		if discarded > djMaxTransientRetries+2 {
			t.Fatal("the refill kept relaunching itself against inputs that never settle")
		}
	}

	if want := djMaxTransientRetries + 1; discarded != want {
		t.Errorf("the chain ran %d attempts, want the first plus %d bounded retries (%d)",
			discarded, djMaxTransientRetries, want)
	}
	if len(writer.accepted) != 0 {
		t.Error("a batch was published despite every attempt going stale")
	}
	if len(writer.rejected) != discarded {
		t.Errorf("rejected %d batches for %d discarded results", len(writer.rejected), discarded)
	}
	if m.radio.enqueuePending {
		t.Error("the stalled refill was left marked pending, so no later trigger can retry it")
	}
}

// TestDJRefillSurvivesTheQueueDraining is the continuous-playback case. The
// protected set shrinks as the queue plays out, which moves the queue revision at
// exactly the moment the refill exists to serve; a strict revision match here
// would stall the DJ every time it mattered most.
func TestDJRefillSurvivesTheQueueDraining(t *testing.T) {
	writer := &stubDJQueueWriter{}
	p := &stubCommandPlayer{}
	m := djRefillModel(writer)
	m.player = p

	m, msg := runRefill(t, m)

	// The last track finishes while the batch is still being planned.
	m.queue.Entries = nil
	m.playback.queueExhausted = true
	m.playback.state = playback.StateIdle
	m.playback.currentIndex = -1

	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	if len(writer.accepted) != 1 {
		t.Fatalf("the refill was refused as the queue ran dry: accepted=%v rejected=%v",
			writer.accepted, writer.rejected)
	}
	if len(m.queue.Entries) != len(msg.Entries) {
		t.Fatalf("queue holds %d entries, want the batch's %d", len(m.queue.Entries), len(msg.Entries))
	}
	if m.playback.state != playback.StatePlaying {
		t.Errorf("state = %v, want playback resumed into the refill", m.playback.state)
	}
	if m.playback.queueExhausted {
		t.Error("queueExhausted survived a resume into the refill")
	}
}

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
