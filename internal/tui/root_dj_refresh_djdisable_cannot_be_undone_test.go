package tui

import (
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestDJDisableCannotBeUndoneByARefreshInFlight covers the disable gesture: the
// toggle withdraws the refresh, so nothing it prepared is ever published.
func TestDJDisableCannotBeUndoneByARefreshInFlight(t *testing.T) {
	writer := newDJReplacementWriter()
	m, p := djRefreshModel(djDefaultQueue(), writer)

	m, cmd, err := m.djApply(exploreSettings(), djFixedNow)
	if err != nil {
		t.Fatalf("apply rejected: %v", err)
	}
	next, _, _ := m.radioToggle()
	m = next.(Model)
	if m.radio.enabled || m.dj.active {
		t.Fatal("the toggle did not turn the DJ off")
	}

	msg, ok := cmd().(djRefreshDoneMsg)
	if !ok {
		t.Fatalf("apply produced %T", cmd())
	}
	if msg.valid {
		t.Fatal("a refresh committed after the DJ was disabled")
	}
	if len(writer.accepted) != 0 {
		t.Fatalf("a disabled DJ published %d batches", len(writer.accepted))
	}
	if len(p.published) != 0 {
		t.Fatal("a disabled DJ still refreshed the player's queue")
	}

	m, _ = m.handleDJRefreshDoneMsg(msg)
	if m.radio.enabled || m.dj.active {
		t.Fatal("the late result re-enabled the DJ")
	}
	if len(writer.rejected) != len(writer.prepared) {
		t.Fatalf("retired %d of %d preparations after a disable",
			len(writer.rejected), len(writer.prepared))
	}
}

// TestDJClearReconcilesACommittedRefreshAwayCovers the acknowledgement that
// arrives too late to be withdrawn: the commit already landed, so the successors
// are reconciled away rather than resurrected into a queue the user emptied.
func TestDJClearReconcilesACommittedRefreshAway(t *testing.T) {
	writer := newDJReplacementWriter()
	m, _ := djRefreshModel(djDefaultQueue(), writer)
	enq := &stubBatchEnqueuer{}
	m.batchEnqueuer = enq

	m, msg := runDJRefresh(t, m, exploreSettings())
	if !msg.valid {
		t.Fatalf("the refresh committed nothing: err = %v", msg.err)
	}

	// The clear lands after the commit but before the acknowledgement.
	m = m.handleClearQueueMsg()
	if len(m.queue.Entries) != 0 {
		t.Fatalf("the clear left %d rows behind", len(m.queue.Entries))
	}

	m, _ = m.handleDJRefreshDoneMsg(msg)
	if len(m.queue.Entries) != 0 {
		t.Fatalf("a late refresh resurrected %d rows into a cleared queue: %v",
			len(m.queue.Entries), queueIDs(m))
	}
	if len(enq.dequeueTracksSoftIDs) != 1 {
		t.Fatalf("issued %d compensating deletes, want exactly 1", len(enq.dequeueTracksSoftIDs))
	}
	if !sameIDs(enq.dequeueTracksSoftIDs[0], msg.committed.prep.IDs) {
		t.Fatalf("compensated %v, want the committed rows %v",
			enq.dequeueTracksSoftIDs[0], msg.committed.prep.IDs)
	}
}

// TestDJLateRefreshReassertsTheSessionRow proves a committed acknowledgement
// arriving after newer settings cannot leave the stale discovery carry it
// committed on disk.
func TestDJLateRefreshReassertsTheSessionRow(t *testing.T) {
	writer := newDJReplacementWriter()
	sessions := &djStubWriter{}
	m, _ := djRefreshModel(djDefaultQueue(), writer)
	m.djWriter = sessions

	m, cmd, err := m.djApply(exploreSettings(), djFixedNow)
	if err != nil {
		t.Fatalf("apply rejected: %v", err)
	}
	msg, ok := cmd().(djRefreshDoneMsg)
	if !ok {
		t.Fatalf("apply produced %T", cmd())
	}
	if !msg.valid {
		t.Fatalf("the refresh committed nothing: err = %v", msg.err)
	}

	// Newer settings are applied and persisted before the acknowledgement is
	// delivered. The committed accept transaction already wrote the carry the
	// old settings earned.
	newer := radio.DefaultSettings()
	newer.DiscoveryPercent = 20
	m, err = m.djApplySettings(newer)
	if err != nil {
		t.Fatalf("newer settings rejected: %v", err)
	}
	writes := len(sessions.writes)

	m, _ = m.handleDJRefreshDoneMsg(msg)

	if len(sessions.writes) != writes+1 {
		t.Fatalf("the late result did not reassert the session row: %d writes, want %d",
			len(sessions.writes), writes+1)
	}
	last := sessions.writes[len(sessions.writes)-1]
	if !last.Settings.Equal(newer.Normalized()) {
		t.Fatalf("reasserted settings %+v, want the newest applied ones", last.Settings)
	}
	if last.Allocation != (radio.Allocation{}) {
		t.Fatalf("the stale carry %+v survived on the session row", last.Allocation)
	}
	if !m.dj.settings.Equal(newer.Normalized()) {
		t.Fatalf("in-memory settings are %+v after a late result", m.dj.settings)
	}
}

// TestDJRefreshStorageFailureChangesNothing covers a refused accept: the durable
// transaction is the only thing that makes a refresh real, so a refusal must
// leave the queue, the player, and the applied settings exactly as they were.
func TestDJRefreshStorageFailureChangesNothing(t *testing.T) {
	writer := newDJReplacementWriter()
	writer.acceptErr = errors.New("database is locked")
	m, p := djRefreshModel(djDefaultQueue(), writer)
	before := queueIDs(m)

	m, msg := runDJRefresh(t, m, exploreSettings())
	if msg.valid {
		t.Fatal("a refused accept was reported as committed")
	}
	if len(p.published) != 0 {
		t.Fatal("the player adopted a queue the database refused")
	}

	m, _ = m.handleDJRefreshDoneMsg(msg)

	if !sameIDs(queueIDs(m), before) {
		t.Fatalf("visible queue is %v after a refused accept, want %v", queueIDs(m), before)
	}
	if !sameIDs(p.ids(), before) {
		t.Fatalf("the player's queue is %v after a refused accept, want %v", p.ids(), before)
	}
	if !m.dj.settings.Equal(exploreSettings().Normalized()) {
		t.Error("a refused refresh also discarded the applied settings")
	}
	if m.djRefresh.status.err == nil {
		t.Error("the failure was not recorded for the user to see")
	}
	if len(writer.rejected) != len(writer.prepared) {
		t.Fatalf("retired %d of %d preparations after a refused accept",
			len(writer.rejected), len(writer.prepared))
	}
}

// TestDJRefreshPreparationFailureIsRecorded covers a refused preparation: no
// hidden rows exist, so there is nothing to reconcile and nothing to retry.
func TestDJRefreshPreparationFailureIsRecorded(t *testing.T) {
	writer := newDJReplacementWriter()
	writer.replaceErr = errors.New("disk is full")
	m, _ := djRefreshModel(djDefaultQueue(), writer)

	m, msg := runDJRefresh(t, m, exploreSettings())
	m, _ = m.handleDJRefreshDoneMsg(msg)

	if len(writer.accepted) != 0 {
		t.Fatal("a refused preparation was published")
	}
	if len(writer.rejected) != 0 {
		t.Fatalf("retired %d preparations that were never written", len(writer.rejected))
	}
	if m.djRefresh.status.err == nil {
		t.Error("the failure was not recorded")
	}
	if m.djRefresh.deferred {
		t.Error("a storage failure was queued for a relaunch that cannot help")
	}
}

// TestDJRefreshWithNoCandidatesRetiresAndPauses is the product decision from the
// shared contract: settings that admit nothing retire the DJ rows and pause
// additions with an explanation, while current and manual playback continue.
func TestDJRefreshWithNoCandidatesRetiresAndPauses(t *testing.T) {
	writer := newDJReplacementWriter()
	m, p := djRefreshModel(djDefaultQueue(), writer)

	settings := radio.DefaultSettings()
	settings.Exclude = radio.FilterSet{Genres: []string{"Rock", "Jazz", "Folk"}}
	m, msg := runDJRefresh(t, m, settings)
	if !msg.valid {
		t.Fatalf("a no-candidate refresh committed nothing: err = %v", msg.err)
	}
	if len(writer.accepted[0].IDs) != 0 {
		t.Fatalf("published %d rows for settings that admit none", len(writer.accepted[0].IDs))
	}
	if want := []int64{2, 3, 4}; !sameIDs(writer.accepted[0].Replacing, want) {
		t.Fatalf("retired %v, want every DJ-owned upcoming row %v", writer.accepted[0].Replacing, want)
	}
	if writer.commits[0].Allocation != nil {
		t.Error("a batch that selected nothing advanced the discovery carry")
	}

	m, _ = m.handleDJRefreshDoneMsg(msg)

	if want := []int64{1}; !sameIDs(queueIDs(m), want) {
		t.Fatalf("visible queue is %v, want only the manual row that is playing %v", queueIDs(m), want)
	}
	if !sameIDs(p.ids(), []int64{1}) {
		t.Fatalf("the player's queue is %v, want only the playing row", p.ids())
	}
	if m.playback.currentIndex != 0 || m.playback.currentHash != "f1" {
		t.Error("the row that was playing did not survive a no-candidate refresh")
	}
	if m.djRefresh.status.replaced != 0 || m.djRefresh.status.retired != 3 {
		t.Errorf("status reports %d replaced and %d retired, want 0 and 3",
			m.djRefresh.status.replaced, m.djRefresh.status.retired)
	}
	if !m.DJPlanBlocked() {
		t.Fatal("additions were not paused after a refresh that found no candidates")
	}
	if len(m.djPlanBlockReasons()) == 0 {
		t.Error("additions paused with no explanation")
	}
	if _, cmd := m.startRadioEnqueueCmd(); cmd != nil {
		t.Error("the refill resumed immediately after the pause")
	}
}

// TestDJRefreshIsOrderedAsATransportBarrier proves Apply joins the existing
// dispatch order rather than racing it: a teardown retires the refresh before it
// reaches the player, and the result says so.
func TestDJRefreshIsOrderedAsATransportBarrier(t *testing.T) {
	writer := newDJReplacementWriter()
	m, p := djRefreshModel(djDefaultQueue(), writer)

	m, cmd, err := m.djApply(exploreSettings(), djFixedNow)
	if err != nil {
		t.Fatalf("apply rejected: %v", err)
	}
	// A teardown — abandoning playback — outranks every request still queued.
	m.supersedePlayRequests()

	msg, ok := cmd().(djRefreshDoneMsg)
	if !ok {
		t.Fatalf("apply produced %T", cmd())
	}
	if !msg.cancelled {
		t.Fatal("a retired refresh still reached the player")
	}
	if p.prepares != 0 || len(writer.prepared) != 0 {
		t.Fatal("a retired refresh prepared work anyway")
	}

	m, _ = m.handleDJRefreshDoneMsg(msg)
	if m.djRefresh.pending {
		t.Error("a cancelled refresh is still reported as in flight")
	}
	if m.djRefresh.deferred {
		t.Error("a cancelled refresh was queued for a relaunch")
	}
}

// TestDJApplyWithoutARefreshableQueueStillApplies covers the two states with no
// refresh to run: a queue holding nothing the DJ owns, and a Model whose player
// or writer cannot refresh at all. The settings are durable either way.
func TestDJApplyWithoutARefreshableQueueStillApplies(t *testing.T) {
	t.Run("no dj-owned rows", func(t *testing.T) {
		writer := newDJReplacementWriter()
		entries := []queue.Entry{djRefreshRow(1, "f1", false), djRefreshRow(2, "f2", false)}
		m, _ := djRefreshModel(entries, writer)

		m, msg := runDJRefresh(t, m, exploreSettings())
		if msg.valid {
			t.Fatal("a queue with no DJ rows was refreshed anyway")
		}
		if !errors.Is(msg.err, errDJRefreshNoSlots) {
			t.Fatalf("refusal was %v, want the no-slots outcome", msg.err)
		}
		if len(writer.prepared) != 0 {
			t.Fatal("a refresh with no slots wrote hidden rows")
		}

		m, _ = m.handleDJRefreshDoneMsg(msg)
		if !m.dj.settings.Equal(exploreSettings().Normalized()) {
			t.Error("the applied settings did not survive a queue with nothing to refresh")
		}
		if m.djRefresh.status.err != nil {
			t.Errorf("an empty DJ queue was recorded as a failure: %v", m.djRefresh.status.err)
		}
	})

	t.Run("player cannot refresh", func(t *testing.T) {
		writer := newDJReplacementWriter()
		m, _ := djRefreshModel(djDefaultQueue(), writer)
		m.player = &stubPlayer{}
		// Drain the queue to the low-water mark so the fallback refill fires.
		m.queue.Entries = m.queue.Entries[:1]

		m, cmd, err := m.djApply(exploreSettings(), djFixedNow)
		if err != nil {
			t.Fatalf("apply rejected: %v", err)
		}
		if !m.dj.settings.Equal(exploreSettings().Normalized()) {
			t.Fatal("the applied settings were not adopted")
		}
		if cmd == nil {
			t.Fatal("apply produced no work at all; the settings never reach the queue")
		}
		if _, ok := cmd().(RadioBatchEnqueuedMsg); !ok {
			t.Fatalf("apply fell back to %T, want the ordinary refill", cmd())
		}
	})
}
