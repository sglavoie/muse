package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// These are integration tests for the Apply gesture: they drive the real
// orchestration — the play-request gate, the staged commit, the DJ request
// identity, and the handler that files the result — against a player that models
// the guarded sequence and a writer that models the two-phase batch.
//
// What they are checking is not that a refresh works. It is that everything the
// refresh must not disturb survives it: the row playing, played history, the
// forward history a Previous left behind, manual entries however many there are,
// and every gesture the user made while the refresh was in flight.

// refreshPlayerStub models the four-stage commit sequence closely enough to
// exercise the caller's half of it: a preparation that can run several times, a
// durable commit that happens at most once, and a publication that adopts the
// queue the caller built.
//
// It is deliberately not a mock. The classification it hands to Prepare is
// derived from its own queue the way the Manager derives it, so a test that
// moves the queue moves what the refresh is allowed to touch.
type refreshPlayerStub struct {
	tracks  []player.QueueTrack
	current int
	// forward names the rows a Previous left unreplayed, which the traversal
	// protects. unscheduled names the rows the cycle has given up on.
	forward     map[int64]bool
	unscheduled map[int64]bool

	// deferAll makes every attempt report an unsettled playback window.
	deferAll bool
	// staleBefore rejects the first N preparations as stale, which is what an
	// intervening Next, Previous, or gapless advance does to a live refresh.
	staleBefore int
	// beforePrepare runs on the refresh goroutine before each capture, so a
	// test can move the queue underneath the attempt it is about to make.
	beforePrepare func(attempt int, p *refreshPlayerStub)

	prepares  int
	commits   int
	published []player.QueueRefresh
	appended  []player.QueueTrack
}

func (p *refreshPlayerStub) Play() error             { return nil }
func (p *refreshPlayerStub) Pause() error            { return nil }
func (p *refreshPlayerStub) PlayFromIndex(int) error { return nil }

func (p *refreshPlayerStub) AppendToQueue(track player.QueueTrack) {
	p.appended = append(p.appended, track)
	p.tracks = append(p.tracks, track)
}

func (p *refreshPlayerStub) QueueTraversal() player.QueueTraversal {
	order := make([]int, 0, len(p.tracks))
	tail := make([]int, 0, len(p.tracks))
	for i, tr := range p.tracks {
		if p.unscheduled[tr.QueueID] {
			tail = append(tail, i)
			continue
		}
		order = append(order, i)
	}
	pos := -1
	for i, idx := range order {
		if idx == p.current {
			pos = i
		}
	}
	return player.QueueTraversal{
		Order:           append(order, tail...),
		CurrentPosition: pos,
		Remaining:       len(order) - pos - 1,
	}
}

// capture classifies the live queue the way RefreshSnapshot documents it: one
// class per row, and only the upcoming rows the cycle still schedules are
// eligible.
func (p *refreshPlayerStub) capture() player.RefreshSnapshot {
	var snap player.RefreshSnapshot
	for i, tr := range p.tracks {
		row := player.RefreshRow{
			Identity: player.QueueRowIdentity(tr.QueueID),
			Index:    i,
			QueueID:  tr.QueueID,
			Hash:     tr.Hash,
		}
		switch {
		case p.unscheduled[tr.QueueID]:
			snap.Unscheduled = append(snap.Unscheduled, row)
		case i == p.current:
			snap.Current = row
		case i < p.current:
			snap.Played = append(snap.Played, row)
		case p.forward[tr.QueueID]:
			snap.Protected = append(snap.Protected, row)
		default:
			snap.Eligible = append(snap.Eligible, row)
		}
	}
	return snap
}

// publish adopts the committed queue and reports what the handover did, which
// is the post-commit half the caller may not refuse.
func (p *refreshPlayerStub) publish(plan player.QueueRefresh) player.RefreshResult {
	var currentID int64
	if p.current >= 0 && p.current < len(p.tracks) {
		currentID = p.tracks[p.current].QueueID
	}
	p.tracks = append([]player.QueueTrack(nil), plan.Queue...)
	res := player.RefreshResult{CurrentIndex: -1}
	for i, tr := range p.tracks {
		if tr.QueueID == currentID {
			res.CurrentIndex = i
		}
	}
	p.current = res.CurrentIndex
	for _, rep := range plan.Replacements {
		if rep.Successor == "" {
			res.Retired++
		} else {
			res.Replaced++
		}
	}
	p.published = append(p.published, plan)
	return res
}

func (p *refreshPlayerStub) CommitQueueRefresh(
	_ context.Context, rc player.RefreshCommit,
) (player.RefreshCommitResult, error) {
	var res player.RefreshCommitResult
	if p.deferAll {
		res.Attempts = 1
		return res, &player.RefreshCommitError{
			Stage: player.RefreshStagePrepare, Err: player.ErrRefreshDeferred,
		}
	}
	var last error
	for attempt := 1; attempt <= 3; attempt++ {
		res.Attempts = attempt
		if p.beforePrepare != nil {
			p.beforePrepare(attempt, p)
		}
		p.prepares++
		plan, err := rc.Prepare(p.capture())
		if err != nil {
			last = err
			if !errors.Is(err, player.ErrRefreshStale) {
				break
			}
			continue
		}
		if attempt <= p.staleBefore {
			// The timeline moved between the capture and the plan, which the
			// real sequence rechecks for before it commits anything.
			last = player.ErrRefreshStale
			continue
		}
		if err := rc.Commit(plan); err != nil {
			return res, &player.RefreshCommitError{Stage: player.RefreshStageCommit, Err: err}
		}
		p.commits++
		res.Committed = true
		res.Result = p.publish(plan)
		return res, nil
	}
	return res, &player.RefreshCommitError{Stage: player.RefreshStagePrepare, Err: last}
}

// ids returns the queue IDs of the player's live queue, which is what every
// assertion about "what is playing next" is phrased in.
func (p *refreshPlayerStub) ids() []int64 {
	out := make([]int64, len(p.tracks))
	for i, tr := range p.tracks {
		out[i] = tr.QueueID
	}
	return out
}

// stubDJReplacementWriter adds the replacement preparation to the two-phase
// writer the refill tests already use, so both paths share one notion of
// "prepared but not published".
type stubDJReplacementWriter struct {
	*stubDJQueueWriter
	replacing  [][]int64
	replaceErr error
}

func newDJReplacementWriter() *stubDJReplacementWriter {
	return &stubDJReplacementWriter{stubDJQueueWriter: &stubDJQueueWriter{}}
}

func (s *stubDJReplacementWriter) PrepareDJReplacement(
	entries []db.DJQueueEntry, replacing []int64,
) (db.DJPreparation, error) {
	s.replacing = append(s.replacing, append([]int64(nil), replacing...))
	s.prepared = append(s.prepared, append([]db.DJQueueEntry(nil), entries...))
	if s.replaceErr != nil {
		return db.DJPreparation{}, s.replaceErr
	}
	if s.prepareErr != nil {
		return db.DJPreparation{}, s.prepareErr
	}
	ids := make([]int64, len(entries))
	for i := range entries {
		s.nextID++
		ids[i] = 500 + s.nextID
	}
	return db.DJPreparation{
		Kind:      db.DJPrepareReplace,
		IDs:       ids,
		Replacing: append([]int64(nil), replacing...),
	}, nil
}

// djRefreshRow builds one queue row, DJ-owned or not. The hash decides what the
// planner may re-pick, so rows are named after the planning fixture.
func djRefreshRow(id int64, hash string, owned bool) queue.Entry {
	return queue.Entry{
		ID:             id,
		Hash:           hash,
		Path:           "/" + hash + ".flac",
		Artist:         "Artist " + hash,
		Title:          hash,
		SourcePlaylist: "Radio",
		DJOwned:        owned,
	}
}

// djRefreshModel returns an active session over the planning fixture with the
// given queue, the first row playing, and both halves of the refresh wired up.
func djRefreshModel(entries []queue.Entry, writer *stubDJReplacementWriter) (Model, *refreshPlayerStub) {
	p := &refreshPlayerStub{
		current:     0,
		forward:     map[int64]bool{},
		unscheduled: map[int64]bool{},
	}
	for _, e := range entries {
		p.tracks = append(p.tracks, queueTrackFromEntry(e))
	}
	m := newDJModel(
		WithRecommendationReader(newDJRecReader()),
		WithDJQueueWriter(writer),
		WithPlayer(p),
	)
	m.queue.Entries = append([]queue.Entry(nil), entries...)
	m.radio.enabled = true
	m.playback.currentIndex = 0
	m.playback.currentHash = entries[0].Hash
	m.playback.state = playback.StatePlaying
	m.dj.active = true
	m.dj.seed = djTestRequest().Seed
	return m, p
}

// djDefaultQueue is the standard fixture: a manual row playing, then three
// DJ-owned rows the refresh may take over.
func djDefaultQueue() []queue.Entry {
	return []queue.Entry{
		djRefreshRow(1, "f1", false),
		djRefreshRow(2, "f2", true),
		djRefreshRow(3, "f3", true),
		djRefreshRow(4, "d1", true),
	}
}

// runDJRefresh applies settings and runs the refresh command, returning the
// message the runtime would deliver. It deliberately stops short of delivering
// it, which is what lets a test move the world underneath a result in flight.
func runDJRefresh(t *testing.T, m Model, settings radio.Settings) (Model, djRefreshDoneMsg) {
	t.Helper()
	m, cmd, err := m.djApply(settings, djFixedNow)
	if err != nil {
		t.Fatalf("apply rejected valid settings: %v", err)
	}
	if cmd == nil {
		t.Fatal("apply launched no refresh")
	}
	msg, ok := cmd().(djRefreshDoneMsg)
	if !ok {
		t.Fatalf("apply produced %T, want a refresh result", cmd())
	}
	return m, msg
}

// queueIDs returns the visible queue as row IDs.
func queueIDs(m Model) []int64 {
	out := make([]int64, len(m.queue.Entries))
	for i, e := range m.queue.Entries {
		out[i] = e.ID
	}
	return out
}

func sameIDs(a, b []int64) bool { return slices.Equal(a, b) }

// exploreSettings is a valid snapshot that differs from the default, so an
// Apply carrying it is a real change rather than a no-op.
func exploreSettings() radio.Settings {
	s := radio.DefaultSettings()
	s.DiscoveryPercent = 80
	return s
}

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
