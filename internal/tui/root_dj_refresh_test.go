package tui

import (
	"context"
	"errors"
	"slices"
	"testing"

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
	// beforeCommit runs after a preparation has been accepted and before the
	// durable transaction, which is the one window in which a plan already
	// written as hidden rows has still committed nothing.
	beforeCommit func(attempt int, p *refreshPlayerStub)

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
		if p.beforeCommit != nil {
			p.beforeCommit(attempt, p)
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
