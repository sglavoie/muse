package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestDJApplyRefreshesUpcomingRowsInPlace is the whole gesture end to end: the
// DJ-owned rows ahead of the listener are handed over, and everything else in
// the queue is exactly where it was.
func TestDJApplyRefreshesUpcomingRowsInPlace(t *testing.T) {
	writer := newDJReplacementWriter()
	m, p := djRefreshModel(djDefaultQueue(), writer)
	// An unrelated reversible action is armed: a refresh is not a destructive
	// gesture of the user's, so it has no business consuming their undo.
	undo := undoSlot{
		action:      undoDequeueAction{id: 77, entry: djRefreshRow(77, "d2", false), idx: 3},
		cancelPurge: make(chan struct{}),
	}
	m.undo.slot = undo

	m, msg := runDJRefresh(t, m, exploreSettings())
	if !msg.valid {
		t.Fatalf("the refresh committed nothing: err = %v", msg.err)
	}
	if len(writer.replacing) != 1 {
		t.Fatalf("prepared %d replacements, want exactly 1", len(writer.replacing))
	}
	if want := []int64{2, 3, 4}; !sameIDs(writer.replacing[0], want) {
		t.Fatalf("replaced rows %v, want the DJ-owned upcoming rows %v", writer.replacing[0], want)
	}
	if len(writer.accepted) != 1 {
		t.Fatalf("accepted %d batches, want exactly 1", len(writer.accepted))
	}
	if len(writer.rejected) != 0 {
		t.Fatalf("a committed refresh also rejected preparations: %v", writer.rejected)
	}

	m, _ = m.handleDJRefreshDoneMsg(msg)

	if got := queueIDs(m); len(got) == 0 || got[0] != 1 {
		t.Fatalf("visible queue %v no longer starts with the row that is playing", got)
	}
	for _, id := range []int64{2, 3, 4} {
		for _, live := range queueIDs(m) {
			if live == id {
				t.Errorf("retired DJ row %d is still in the visible queue %v", id, queueIDs(m))
			}
		}
	}
	if got := len(m.queue.Entries); got != 1+len(msg.committed.entries) {
		t.Fatalf("visible queue has %d rows, want 1 manual + %d successors",
			got, len(msg.committed.entries))
	}
	if p.ids()[0] != 1 {
		t.Errorf("the player's queue %v no longer starts with the playing row", p.ids())
	}
	if m.playback.currentIndex != 0 {
		t.Errorf("current index moved to %d; the playing row must keep its place", m.playback.currentIndex)
	}
	if !m.dj.settings.Equal(exploreSettings().Normalized()) {
		t.Errorf("applied settings are %+v, want the ones Apply carried", m.dj.settings)
	}
	if m.djRefresh.status.replaced == 0 {
		t.Error("the status reports no replaced slots after a refresh that replaced some")
	}
	if m.undo.slot.action == nil || m.undo.slot.action.kind() != undoKindDequeue ||
		m.undo.slot.cancelPurge != undo.cancelPurge {
		t.Error("the refresh consumed an unrelated reversible action")
	}
	for _, e := range msg.committed.entries {
		if !e.DJOwned {
			t.Errorf("successor %q was published without DJ provenance", e.Hash)
		}
	}
}

// TestDJApplyNeverBypassesAManualQueue is the long-manual-queue case from the
// shared contract: the refresh replaces the DJ rows where they stand and adds
// nothing in front of the queue the user built.
func TestDJApplyNeverBypassesAManualQueue(t *testing.T) {
	entries := []queue.Entry{djRefreshRow(1, "f1", false)}
	for i := int64(2); i <= 9; i++ {
		entries = append(entries, djRefreshRow(i, fmt.Sprintf("m%d", i), false))
	}
	entries = append(entries, djRefreshRow(10, "f2", true), djRefreshRow(11, "f3", true))
	writer := newDJReplacementWriter()
	m, _ := djRefreshModel(entries, writer)

	m, msg := runDJRefresh(t, m, exploreSettings())
	if !msg.valid {
		t.Fatalf("the refresh committed nothing: err = %v", msg.err)
	}
	if want := []int64{10, 11}; !sameIDs(writer.replacing[0], want) {
		t.Fatalf("replaced rows %v, want only the DJ-owned rows %v", writer.replacing[0], want)
	}
	if writer.accepted[0].Kind != db.DJPrepareReplace {
		t.Fatalf("published a %q batch; a refresh must never append past a manual queue",
			writer.accepted[0].Kind)
	}

	m, _ = m.handleDJRefreshDoneMsg(msg)

	for i, id := range queueIDs(m)[:9] {
		if id != int64(i+1) {
			t.Fatalf("manual queue was reordered: %v", queueIDs(m))
		}
	}
}

// TestDJRefreshLeavesPlayedAndForwardHistoryAlone covers the Previous case: the
// forward history a Previous left unreplayed is ahead of the cursor but is not
// the cycle's to give up, and neither is anything already played.
func TestDJRefreshLeavesPlayedAndForwardHistoryAlone(t *testing.T) {
	entries := []queue.Entry{
		djRefreshRow(1, "f1", true),
		djRefreshRow(2, "f2", true),
		djRefreshRow(3, "f3", true),
		djRefreshRow(4, "d1", true),
	}
	writer := newDJReplacementWriter()
	m, p := djRefreshModel(entries, writer)
	// A Previous moved the cursor back to row 2 and left row 3 as forward
	// history; row 1 is played.
	p.current = 1
	p.forward[3] = true
	m.playback.currentIndex = 1
	m.playback.currentHash = "f2"

	_, msg := runDJRefresh(t, m, exploreSettings())
	if !msg.valid {
		t.Fatalf("the refresh committed nothing: err = %v", msg.err)
	}
	if want := []int64{4}; !sameIDs(writer.replacing[0], want) {
		t.Fatalf("replaced rows %v, want only %v: played history, the playing row, "+
			"and forward history are all off limits", writer.replacing[0], want)
	}
}

// TestDJRefreshRechecksAfterAManualEnqueue covers a manual enqueue and a reorder
// landing between the Apply and the capture the refresh plans against. A row the
// refresh never captured is a row it cannot describe, so it refuses the plan
// rather than publishing a queue that would drop the user's track — and the
// recheck that follows sees the new row and leaves it alone.
func TestDJRefreshRechecksAfterAManualEnqueue(t *testing.T) {
	writer := newDJReplacementWriter()
	m, p := djRefreshModel(djDefaultQueue(), writer)
	manual := djRefreshRow(9, "d3", false)
	p.beforePrepare = func(attempt int, p *refreshPlayerStub) {
		if attempt != 1 {
			return
		}
		// The user enqueues a track by hand and drags it in front of the DJ
		// rows while the Apply is on its way to the player.
		tr := queueTrackFromEntry(manual)
		p.tracks = append([]player.QueueTrack{p.tracks[0], tr}, p.tracks[1:]...)
	}

	m, msg := runDJRefresh(t, m, exploreSettings())
	if msg.valid {
		t.Fatal("the refresh published a queue built without the row the user enqueued")
	}
	m, _ = m.handleDJRefreshDoneMsg(msg)
	if len(writer.accepted) != 0 {
		t.Fatalf("published %d batches against a queue it could not describe", len(writer.accepted))
	}
	if len(writer.rejected) != len(writer.prepared) {
		t.Fatalf("retired %d of %d preparations the refused attempts wrote",
			len(writer.rejected), len(writer.prepared))
	}
	if !m.djRefresh.deferred {
		t.Fatal("a queue that moved underneath the refresh did not schedule a recheck")
	}

	// The Update loop that appended the row has since reached the Model, so the
	// recheck plans against a queue that holds it.
	m.queue.Entries = append([]queue.Entry{m.queue.Entries[0], manual}, m.queue.Entries[1:]...)
	m, cmd := m.djResumeDeferredRefresh(djFixedNow)
	if cmd == nil {
		t.Fatal("the recheck was never launched")
	}
	resumed, ok := cmd().(djRefreshDoneMsg)
	if !ok || !resumed.valid {
		t.Fatalf("the recheck committed nothing: %+v", resumed)
	}
	for _, id := range writer.replacing[len(writer.replacing)-1] {
		if id == 9 {
			t.Fatal("the recheck claimed a row the user enqueued by hand")
		}
	}

	m, _ = m.handleDJRefreshDoneMsg(resumed)

	for _, id := range queueIDs(m) {
		if id == 9 {
			for _, tr := range p.tracks {
				if tr.QueueID == 9 {
					return
				}
			}
			t.Fatalf("the manual row is gone from the player's queue %v", p.ids())
		}
	}
	t.Fatalf("the manually enqueued row is gone from the visible queue %v", queueIDs(m))
}

// TestDJRefreshRetriesWhileTheTimelineMoves covers Next, Previous, and gapless
// advancement racing the preparation: every abandoned preparation is retired,
// and exactly one is published.
func TestDJRefreshRetriesWhileTheTimelineMoves(t *testing.T) {
	writer := newDJReplacementWriter()
	m, p := djRefreshModel(djDefaultQueue(), writer)
	p.staleBefore = 2

	m, msg := runDJRefresh(t, m, exploreSettings())
	if !msg.valid {
		t.Fatalf("the refresh committed nothing after the timeline settled: err = %v", msg.err)
	}
	if p.prepares != 3 {
		t.Fatalf("prepared %d times, want 3: two losses to the moving timeline and one win", p.prepares)
	}
	if len(writer.prepared) != 3 {
		t.Fatalf("wrote %d preparations, want one per attempt", len(writer.prepared))
	}
	if len(writer.accepted) != 1 {
		t.Fatalf("published %d batches, want exactly 1", len(writer.accepted))
	}
	if len(msg.discarded) != 2 {
		t.Fatalf("reported %d abandoned preparations, want 2", len(msg.discarded))
	}

	_, _ = m.handleDJRefreshDoneMsg(msg)

	if len(writer.rejected) != 2 {
		t.Fatalf("retired %d abandoned preparations, want 2: hidden rows that outlive "+
			"their result would be republished by the next accept naming them", len(writer.rejected))
	}
	for _, prep := range writer.rejected {
		if sameIDs(prep.IDs, writer.accepted[0].IDs) {
			t.Fatal("the published batch was also retired")
		}
	}
}

// TestDJRefreshDefersUntilPlaybackSettles covers a gapless advance in progress:
// no plan can be checked against a timeline that is still moving, so the refresh
// waits for the settlement and the confirmed track change resumes it.
func TestDJRefreshDefersUntilPlaybackSettles(t *testing.T) {
	writer := newDJReplacementWriter()
	m, p := djRefreshModel(djDefaultQueue(), writer)
	p.deferAll = true

	m, msg := runDJRefresh(t, m, exploreSettings())
	if msg.valid {
		t.Fatal("a deferred refresh committed something")
	}
	if len(writer.prepared) != 0 {
		t.Fatalf("a deferred refresh wrote %d preparations, want none", len(writer.prepared))
	}
	m, _ = m.handleDJRefreshDoneMsg(msg)
	if !m.djRefresh.deferred {
		t.Fatal("an unsettled playback window did not leave the refresh deferred")
	}

	// The settlement arrives, and the resumed attempt finds a settled window.
	p.deferAll = false
	next, cmd := m.djResumeDeferredRefresh(djFixedNow)
	if cmd == nil {
		t.Fatal("the settlement did not resume the deferred refresh")
	}
	if next.djRefresh.deferred {
		t.Error("the deferral survived its own relaunch")
	}
	resumed, ok := cmd().(djRefreshDoneMsg)
	if !ok || !resumed.valid {
		t.Fatalf("the resumed refresh committed nothing: %+v", resumed)
	}
	next, _ = next.handleDJRefreshDoneMsg(resumed)
	if next.djRefresh.retries != 0 {
		t.Errorf("retry budget is %d after an adopted refresh, want it refunded", next.djRefresh.retries)
	}
}

// TestDJRefreshDeferralIsBounded proves a queue that never settles ends as a
// stalled refresh rather than as a relaunch loop.
func TestDJRefreshDeferralIsBounded(t *testing.T) {
	writer := newDJReplacementWriter()
	m, p := djRefreshModel(djDefaultQueue(), writer)
	p.deferAll = true

	m, msg := runDJRefresh(t, m, exploreSettings())
	m, _ = m.handleDJRefreshDoneMsg(msg)

	relaunches := 0
	for i := 0; i < djRefreshMaxRetries+3; i++ {
		var cmd tea.Cmd
		m, cmd = m.djResumeDeferredRefresh(djFixedNow)
		if cmd == nil {
			break
		}
		relaunches++
		out, ok := cmd().(djRefreshDoneMsg)
		if !ok {
			t.Fatalf("relaunch produced %T", cmd())
		}
		m, _ = m.handleDJRefreshDoneMsg(out)
	}
	if relaunches != djRefreshMaxRetries {
		t.Fatalf("relaunched %d times, want the %d-attempt budget", relaunches, djRefreshMaxRetries)
	}
	if m.djRefresh.deferred {
		t.Error("an exhausted budget left the refresh still asking to be resumed")
	}
}

// TestDJApplyRapidSupersedesTheOlderRefresh covers rapid Apply: the older result
// arrives after the newer settings are in force and must not put them back.
func TestDJApplyRapidSupersedesTheOlderRefresh(t *testing.T) {
	writer := newDJReplacementWriter()
	m, _ := djRefreshModel(djDefaultQueue(), writer)

	first := exploreSettings()
	m, firstCmd, err := m.djApply(first, djFixedNow)
	if err != nil {
		t.Fatalf("first apply rejected: %v", err)
	}

	// The user applies again before the first result comes back.
	second := radio.DefaultSettings()
	second.DiscoveryPercent = 20
	second.Anchor = radio.AnchorFollow
	m, secondCmd, err := m.djApply(second, djFixedNow)
	if err != nil {
		t.Fatalf("second apply rejected: %v", err)
	}

	firstMsg, ok := firstCmd().(djRefreshDoneMsg)
	if !ok {
		t.Fatalf("first apply produced %T", firstCmd())
	}
	if firstMsg.valid {
		t.Fatal("a superseded refresh committed its batch; the withdrawal did not reach it")
	}
	if len(writer.accepted) != 0 {
		t.Fatalf("a superseded refresh published %d batches", len(writer.accepted))
	}

	m, _ = m.handleDJRefreshDoneMsg(firstMsg)
	if !m.dj.settings.Equal(second.Normalized()) {
		t.Fatalf("the late result put settings back to %+v", m.dj.settings)
	}
	if len(writer.rejected) != len(writer.prepared) {
		t.Fatalf("retired %d of %d preparations the superseded refresh wrote",
			len(writer.rejected), len(writer.prepared))
	}

	secondMsg, ok := secondCmd().(djRefreshDoneMsg)
	if !ok {
		t.Fatalf("second apply produced %T", secondCmd())
	}
	if !secondMsg.valid {
		t.Fatalf("the newest refresh committed nothing: err = %v", secondMsg.err)
	}
	m, _ = m.handleDJRefreshDoneMsg(secondMsg)
	if !m.dj.settings.Equal(second.Normalized()) {
		t.Fatalf("settings are %+v after adopting the newest refresh", m.dj.settings)
	}
}
