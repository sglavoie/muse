package player

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// These tests are about the seam between the two systems a refresh changes: what
// happens when the database transaction succeeds and the player does not, and
// what happens when the player moves while the transaction is being built.
//
// The asymmetry is the contract. Before the commit, anything may call the
// refresh off and the previously committed batch keeps playing. After it, the
// refreshed queue is what the database holds, so the player converges onto it
// rather than reverting to a queue that no longer exists anywhere.

// refreshCommitManager returns a manager playing the first row of a four-row
// sequential queue, which leaves rows 2, 3, and 4 replaceable.
func refreshCommitManager(t *testing.T, name string) (*Manager, []QueueTrack) {
	t.Helper()
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, name, &mu, &received)

	q := refreshQueue(t, 1, 2, 3, 4)
	m.mu.Lock()
	m.queue = q
	m.currentIndex = 0
	m.mu.Unlock()
	assertEligible(t, m, 2, 3, 4)
	return m, q
}

// stagedCommit records what each stage of a refresh saw, and is the
// RefreshCommit the tests below submit.
type stagedCommit struct {
	mu sync.Mutex
	// snaps holds the snapshot every preparation was handed, in order.
	snaps []RefreshSnapshot
	// plans holds the plan every commit was handed, in order.
	plans []QueueRefresh
	// build turns a snapshot into a plan; a nil error means the preparation
	// succeeded.
	build func(int, RefreshSnapshot) (QueueRefresh, error)
	// onCommit runs inside the durable transaction, which is where a test
	// perturbs the player to force a post-commit failure.
	onCommit func()
	// commitErr is returned by every commit when non-nil.
	commitErr error
}

func (s *stagedCommit) refresh(t *testing.T, m *Manager) RefreshCommit {
	t.Helper()
	return RefreshCommit{
		Prepare: func(snap RefreshSnapshot) (QueueRefresh, error) {
			assertPlayerLocksFree(t, m, "preparation")
			s.mu.Lock()
			s.snaps = append(s.snaps, snap)
			n := len(s.snaps)
			s.mu.Unlock()
			return s.build(n, snap)
		},
		Commit: func(plan QueueRefresh) error {
			assertPlayerLocksFree(t, m, "the durable commit")
			s.mu.Lock()
			s.plans = append(s.plans, plan)
			s.mu.Unlock()
			if s.onCommit != nil {
				s.onCommit()
			}
			return s.commitErr
		},
	}
}

func (s *stagedCommit) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.snaps), len(s.plans)
}

func (s *stagedCommit) lastSnapshot(t *testing.T) RefreshSnapshot {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.snaps) == 0 {
		t.Fatal("no preparation ever ran")
	}
	return s.snaps[len(s.snaps)-1]
}

// assertPlayerLocksFree fails when a caller callback runs with a player lock
// held.
//
// This is the constraint that keeps the stages callable at all: preparation and
// commit do database work, and a caller that touched the queue from inside one
// of them — to read the row that is playing, say — would deadlock against a lock
// the refresh is already holding. Taking the pair in the production order proves
// both are free.
func assertPlayerLocksFree(t *testing.T, m *Manager, stage string) {
	t.Helper()
	probed := make(chan int, 1)
	go func() {
		m.playlistMu.Lock()
		m.mu.Lock()
		n := len(m.queue)
		m.mu.Unlock()
		m.playlistMu.Unlock()
		probed <- n
	}()
	select {
	case <-probed:
	case <-time.After(2 * time.Second):
		t.Errorf("%s ran with a player lock held: a callback that reached back into the "+
			"queue from there would deadlock", stage)
	}
}

// TestCommitQueueRefresh_CommitsOnceAndPublishes is the whole sequence on a
// timeline nobody disturbs.
func TestCommitQueueRefresh_CommitsOnceAndPublishes(t *testing.T) {
	m, q := refreshCommitManager(t, "refcom-ok")
	fresh := refreshQueue(t, 10)
	staged := &stagedCommit{
		build: func(_ int, snap RefreshSnapshot) (QueueRefresh, error) {
			return buildRefresh(snap, q, []refreshSwap{swap(3, fresh[0])}), nil
		},
	}

	res, err := m.CommitQueueRefresh(context.Background(), staged.refresh(t, m))
	if err != nil {
		t.Fatalf("CommitQueueRefresh: %v", err)
	}
	if !res.Committed || res.Recovered || res.Attempts != 1 {
		t.Fatalf("result = %+v, want one attempt committed and published", res)
	}
	if prepares, commits := staged.counts(); prepares != 1 || commits != 1 {
		t.Fatalf("%d preparations and %d commits, want one of each", prepares, commits)
	}
	if res.Result.Replaced != 1 || res.Result.CurrentIndex != 0 {
		t.Fatalf("publication = %+v, want one replaced slot and an unmoved cursor", res.Result)
	}
	assertEligible(t, m, 2, 4, 10)
	if got := currentQueueID(m); got != 1 {
		t.Fatalf("current row = %d, want 1: a refresh must not move playback", got)
	}
}

// TestCommitQueueRefresh_RetriesWhenTheTimelineMovesBeforeTheCommit is the
// required pre-commit check doing its job.
//
// Building a plan is a database round trip, and playback keeps running through
// it. Without the re-check the sequence would commit a batch built against a
// queue that had already moved and only find out at publication — past the point
// of no return, with nothing to do but recover. Catching it first makes it an
// ordinary retry that costs nothing: the previously committed batch is still the
// authoritative one, and the next capture describes wherever playback got to.
func TestCommitQueueRefresh_RetriesWhenTheTimelineMovesBeforeTheCommit(t *testing.T) {
	m, _ := refreshCommitManager(t, "refcom-retry")
	fresh := refreshQueue(t, 10, 11)
	staged := &stagedCommit{
		build: func(attempt int, snap RefreshSnapshot) (QueueRefresh, error) {
			live := managerQueue(m)
			if attempt == 1 {
				// The queue grows while this batch is being built, exactly as a
				// user enqueue would.
				m.AppendToQueue(fresh[1])
				return buildRefresh(snap, live, []refreshSwap{swap(3, fresh[0])}), nil
			}
			return buildRefresh(snap, live, []refreshSwap{swap(3, fresh[0])}), nil
		},
	}

	res, err := m.CommitQueueRefresh(context.Background(), staged.refresh(t, m))
	if err != nil {
		t.Fatalf("CommitQueueRefresh: %v", err)
	}
	if !res.Committed || res.Attempts != 2 {
		t.Fatalf("result = %+v, want the second attempt to commit", res)
	}
	if prepares, commits := staged.counts(); prepares != 2 || commits != 1 {
		t.Fatalf("%d preparations and %d commits, want the stale attempt to have committed nothing",
			prepares, commits)
	}
	// Row 11 rode along as an ordinary append; row 3 was replaced by row 10.
	assertEligible(t, m, 2, 4, 11, 10)
}

// TestCommitQueueRefresh_GivesUpWithoutCommittingWhenTheTimelineKeepsMoving is
// the bounded end of that retry.
//
// A queue being mutated faster than a batch can be built is not a transient
// race, and spinning on it would keep a database transaction permanently in
// flight. Reporting the staleness leaves the caller to decide, and leaves the
// committed state exactly as it was.
func TestCommitQueueRefresh_GivesUpWithoutCommittingWhenTheTimelineKeepsMoving(t *testing.T) {
	m, _ := refreshCommitManager(t, "refcom-give")
	fresh := refreshQueue(t, 10, 20, 21, 22)
	staged := &stagedCommit{
		build: func(attempt int, snap RefreshSnapshot) (QueueRefresh, error) {
			live := managerQueue(m)
			m.AppendToQueue(fresh[attempt])
			return buildRefresh(snap, live, []refreshSwap{swap(3, fresh[0])}), nil
		},
	}

	res, err := m.CommitQueueRefresh(context.Background(), staged.refresh(t, m))
	if err == nil {
		t.Fatal("CommitQueueRefresh accepted a plan whose timeline never held still")
	}
	var stageErr *RefreshCommitError
	if !errors.As(err, &stageErr) || stageErr.Stage != RefreshStagePrepare {
		t.Fatalf("error = %v, want a preparation-stage failure", err)
	}
	if !errors.Is(err, ErrRefreshStale) {
		t.Fatalf("error = %v, want ErrRefreshStale", err)
	}
	if res.Committed || res.Attempts != refreshCommitAttempts {
		t.Fatalf("result = %+v, want %d attempts and nothing committed", res, refreshCommitAttempts)
	}
	if _, commits := staged.counts(); commits != 0 {
		t.Fatalf("%d commits ran for a refresh that never passed its pre-commit check", commits)
	}
	// Row 3 is still upcoming: the refresh changed nothing.
	assertEligible(t, m, 2, 3, 4, 20, 21, 22)
}

// TestCommitQueueRefresh_ReportsACommitFailureWithoutTouchingThePlayer is the
// other pre-commit failure: the transaction itself was rejected.
func TestCommitQueueRefresh_ReportsACommitFailureWithoutTouchingThePlayer(t *testing.T) {
	m, q := refreshCommitManager(t, "refcom-dberr")
	fresh := refreshQueue(t, 10)
	dbErr := errors.New("accept batch: database is locked")
	staged := &stagedCommit{
		build: func(_ int, snap RefreshSnapshot) (QueueRefresh, error) {
			return buildRefresh(snap, q, []refreshSwap{swap(3, fresh[0])}), nil
		},
		commitErr: dbErr,
	}

	res, err := m.CommitQueueRefresh(context.Background(), staged.refresh(t, m))
	if !errors.Is(err, dbErr) {
		t.Fatalf("error = %v, want the database failure", err)
	}
	var stageErr *RefreshCommitError
	if !errors.As(err, &stageErr) || stageErr.Stage != RefreshStageCommit {
		t.Fatalf("error = %v, want a commit-stage failure", err)
	}
	if res.Committed || res.Recovered {
		t.Fatalf("result = %+v, want nothing committed and nothing recovered", res)
	}
	assertEligible(t, m, 2, 3, 4)
}

// TestCommitQueueRefresh_RecoversOntoTheCommittedQueueAfterAPublicationRejection
// is the post-commit case, and the one the staged design exists for.
//
// The guard protects a plan's assumptions, and here they really did stop
// holding: the queue moved inside the transaction. But the transaction landed,
// so the refreshed queue is what the database has and what a restart would load.
// Refusing to publish it would leave the player playing rows that no longer
// exist and would write them back on the next save. Recovery adopts the
// committed queue instead — and the rejection is reported, not swallowed.
func TestCommitQueueRefresh_RecoversOntoTheCommittedQueueAfterAPublicationRejection(t *testing.T) {
	m, q := refreshCommitManager(t, "refcom-recov")
	fresh := refreshQueue(t, 10, 11)
	staged := &stagedCommit{
		build: func(_ int, snap RefreshSnapshot) (QueueRefresh, error) {
			return buildRefresh(snap, q, []refreshSwap{swap(3, fresh[0])}), nil
		},
		// The queue moves while the transaction is open, which is the one window
		// the pre-commit check cannot cover.
		onCommit: func() { m.AppendToQueue(fresh[1]) },
	}

	res, err := m.CommitQueueRefresh(context.Background(), staged.refresh(t, m))
	if err != nil {
		t.Fatalf("CommitQueueRefresh reported a failure for a committed batch: %v", err)
	}
	if !res.Committed || !res.Recovered {
		t.Fatalf("result = %+v, want a committed batch recovered onto", res)
	}
	if !errors.Is(res.Cause, ErrRefreshStale) {
		t.Fatalf("recovery cause = %v, want the staleness the publication reported", res.Cause)
	}
	var causeErr *RefreshCommitError
	if !errors.As(res.Cause, &causeErr) || causeErr.Stage != RefreshStagePublish {
		t.Fatalf("recovery cause = %v, want it attributed to the publication stage", res.Cause)
	}
	if prepares, commits := staged.counts(); prepares != 1 || commits != 1 {
		t.Fatalf("%d preparations and %d commits, want the commit to have run exactly once",
			prepares, commits)
	}

	// The committed queue wins: row 3 is gone, row 10 took its slot, and the row
	// appended inside the transaction — which the transaction did not include —
	// is not resurrected.
	assertEligible(t, m, 2, 4, 10)
	if got := currentQueueID(m); got != 1 {
		t.Fatalf("current row = %d, want 1: recovery must not move playback", got)
	}
	if res.Result.CurrentIndex != 0 {
		t.Fatalf("recovered current index = %d, want 0", res.Result.CurrentIndex)
	}
}

// TestCommitQueueRefresh_RejectsARefreshWithoutBothStages pins that a caller
// cannot submit half a sequence.
func TestCommitQueueRefresh_RejectsARefreshWithoutBothStages(t *testing.T) {
	m, _ := refreshCommitManager(t, "refcom-half")
	_, err := m.CommitQueueRefresh(context.Background(), RefreshCommit{
		Prepare: func(snap RefreshSnapshot) (QueueRefresh, error) { return QueueRefresh{}, nil },
	})
	if !errors.Is(err, ErrRefreshPlanInvalid) {
		t.Fatalf("error = %v, want ErrRefreshPlanInvalid", err)
	}
}

// TestCommitQueueRefresh_WaitsOutAnUnresolvedEndOfFile is the EOF case.
//
// A refill runs when a track is ending, which is precisely when the end-file it
// has to wait for is in flight. Rejecting during that window would be correct
// and useless — the caller would recapture into the same window. So the
// preparation parks until the transition resolves, and the plan it then builds
// describes the row mpv actually moved to rather than the one that finished.
func TestCommitQueueRefresh_WaitsOutAnUnresolvedEndOfFile(t *testing.T) {
	w := openEOFPendingWindow(t, "refcom-eof")
	fresh := refreshQueue(t, 10)
	staged := &stagedCommit{
		build: func(_ int, snap RefreshSnapshot) (QueueRefresh, error) {
			return buildRefresh(snap, managerQueue(w.m), []refreshSwap{swap(3, fresh[0])}), nil
		},
	}

	type outcome struct {
		res RefreshCommitResult
		err error
	}
	done := make(chan outcome, 1)
	w.m.CommitQueueRefreshAsync(context.Background(), staged.refresh(t, w.m),
		func(res RefreshCommitResult, err error) { done <- outcome{res, err} })

	// Nothing may be prepared while the successor question is open: the queue
	// still reads as if the finished row were playing.
	time.Sleep(20 * time.Millisecond)
	if prepares, _ := staged.counts(); prepares != 0 {
		t.Fatalf("%d preparations ran during the pending end-file", prepares)
	}
	select {
	case out := <-done:
		t.Fatalf("the refresh finished inside the pending window: %+v %v", out.res, out.err)
	default:
	}

	// The transition resolves onto the row mpv had already started.
	w.m.dispatchEOF(w.state)

	var out outcome
	select {
	case out = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the deferred refresh never resumed after the transition settled")
	}
	if out.err != nil {
		t.Fatalf("CommitQueueRefresh after the settled transition: %v", out.err)
	}
	if !out.res.Committed || out.res.Recovered {
		t.Fatalf("result = %+v, want a clean commit after the wait", out.res)
	}
	if _, commits := staged.counts(); commits != 1 {
		t.Fatalf("%d commits, want exactly one", commits)
	}
	// The plan was built against the promoted row, not the one that finished.
	if got := staged.lastSnapshot(t).Current.QueueID; got != 2 {
		t.Fatalf("prepared against current row %d, want the promoted row 2", got)
	}
	if got := currentQueueID(w.m); got != 2 {
		t.Fatalf("current row = %d, want the promoted row 2", got)
	}
	if got := w.f.count(isLoadfileReplace); got != 1 {
		t.Fatalf("mpv received %d loadfile replace commands, want only the one that started "+
			"playback: the refresh must not reload the track that is playing: %v", got, w.f.commands())
	}
	if got := w.f.playingPath(); got != w.queue[1].Path {
		t.Fatalf("mpv is playing %q, want the uninterrupted successor %q", got, w.queue[1].Path)
	}
}

// TestCommitQueueRefresh_ReportsADeferralWhenTheTransitionNeverSettles bounds
// that wait. A transition handed to the restart supervisor can stay open, and a
// refresh that blocked on it forever would hold its caller with it.
func TestCommitQueueRefresh_ReportsADeferralWhenTheTransitionNeverSettles(t *testing.T) {
	w := openEOFPendingWindow(t, "refcom-stuck")
	staged := &stagedCommit{
		build: func(_ int, snap RefreshSnapshot) (QueueRefresh, error) {
			return buildRefresh(snap, managerQueue(w.m), nil), nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	res, err := w.m.CommitQueueRefresh(ctx, staged.refresh(t, w.m))
	if !errors.Is(err, ErrRefreshDeferred) {
		t.Fatalf("error = %v, want ErrRefreshDeferred", err)
	}
	if res.Committed {
		t.Fatalf("result = %+v, want nothing committed", res)
	}
	if prepares, commits := staged.counts(); prepares != 0 || commits != 0 {
		t.Fatalf("%d preparations and %d commits ran for a refresh that never got a window",
			prepares, commits)
	}
	w.assertSuccessorIntact(t)
}
