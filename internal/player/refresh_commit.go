package player

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sglavoie/muse/internal/safego"
)

// This file spells out the sequence a queue refresh runs through, and it exists
// because the two systems a refresh changes cannot be changed together. SQLite
// owns the queue rows and mpv owns what is coming out of the speakers; there is
// no transaction spanning both, and pretending otherwise is how a batch ends up
// committed to the database while the player is still playing the rows it
// replaced. So "apply this batch" is four named stages, each with an explicit
// answer for its own failure:
//
//	Preparation   Wait for a settled playback window, capture the timeline, and
//	              build the durable plan against it. Nothing is authoritative
//	              yet, so a rejection here leaves the previously committed batch
//	              exactly as it was — still in the database, still playing.
//	Commit        One database transaction makes the refreshed queue the
//	              authoritative one. It runs off every player lock and exactly
//	              once, and it is the point of no return.
//	Publication   The player adopts the committed queue in memory, under the
//	              guard ApplyQueueRefresh enforces. A rejection here is a
//	              post-commit failure: the durable queue stands, so the player
//	              recovers onto it rather than reverting to a queue the database
//	              no longer holds.
//	Maintenance   mpv's playlist is brought back in line — an obsolete gapless
//	              successor dropped, a fresh one scheduled. Best effort by
//	              construction, and never at the cost of the current track.
//
// The asymmetry between the first two stages and the last two is the whole
// design. Everything that can still be called off is done before the commit, and
// everything after it is required to converge onto the committed queue rather
// than to succeed.

const (
	// refreshCommitAttempts bounds how many times the preparation stage is
	// retried when the timeline moves underneath it. A refresh races ordinary
	// playback, so losing once is expected and losing three times in a row means
	// the queue is being mutated faster than a batch can be built — at which
	// point returning the staleness is more useful than spinning.
	refreshCommitAttempts = 3

	// refreshSettleWait bounds the wait for an unresolved end-file. The usual
	// pending window is a single event round trip, but a transition that has to
	// go through a process restart to resolve takes seconds, and a refresh that
	// gave up immediately would be indistinguishable from one that raced.
	refreshSettleWait = 5 * time.Second

	// refreshSettlePoll is how often the wait re-reads the liveness state. There
	// is no condition variable on the EOF transition, and adding one would put a
	// refresh's wakeup on the event reader's critical path.
	refreshSettlePoll = 2 * time.Millisecond
)

// ErrRefreshDeferred means the refresh never found a settled playback window to
// run in: an end-file stayed unresolved for the whole wait. Nothing was
// committed and nothing was published.
var ErrRefreshDeferred = errors.New("player: refresh deferred: playback did not settle")

// RefreshStage names the stage a refresh reached.
type RefreshStage int

const (
	// RefreshStagePrepare is everything before the database transaction: the
	// wait for a settled window, the capture, the caller's plan building, and
	// the required check that the timeline still matches.
	RefreshStagePrepare RefreshStage = iota

	// RefreshStageCommit is the durable transaction that makes the refreshed
	// queue authoritative.
	RefreshStageCommit

	// RefreshStagePublish is the in-memory adoption of the committed queue. It
	// names the rejection a recovery absorbed rather than a failure the caller
	// gets back, because by then the batch is committed.
	RefreshStagePublish

	// RefreshStageMaintain is the post-commit reconciliation of mpv's playlist.
	// It never produces an error: its outcome is what RefreshResult's prefetch
	// fields report, since a playlist entry that could not be taken back is a
	// retry rather than a failure of the refresh.
	RefreshStageMaintain
)

func (s RefreshStage) String() string {
	switch s {
	case RefreshStagePrepare:
		return "preparation"
	case RefreshStageCommit:
		return "commit"
	case RefreshStagePublish:
		return "publication"
	case RefreshStageMaintain:
		return "maintenance"
	default:
		return fmt.Sprintf("stage(%d)", int(s))
	}
}

// RefreshCommit is the caller's half of the sequence: the two steps only the
// owner of the durable queue can perform.
//
// Prepare turns a captured timeline into the plan that describes the refreshed
// queue. It may reject the snapshot by returning an error, and it must not touch
// the player — it is called with no player lock held precisely so that it can do
// database work, and reaching back into the queue from here is what the staged
// design exists to avoid.
//
// Prepare can run more than once. A preparation whose commit never happens must
// therefore leave nothing behind that a later attempt would trip over; the
// hidden pending batch the Auto-DJ writes is already recoverable in exactly this
// way.
//
// Commit performs the durable transaction and is called at most once, with no
// player lock held. Returning an error from it means nothing was committed.
type RefreshCommit struct {
	Prepare func(RefreshSnapshot) (QueueRefresh, error)
	Commit  func(QueueRefresh) error
}

// RefreshCommitResult reports how far a refresh got and what it took to get
// there.
type RefreshCommitResult struct {
	// Result is what the publication did. When Recovered is set, only
	// CurrentIndex is meaningful — recovery adopts the durable queue wholesale
	// rather than applying a substitution it can count.
	Result RefreshResult

	// Committed reports that the durable transaction succeeded, which makes the
	// refreshed queue authoritative regardless of anything that follows.
	Committed bool

	// Recovered reports that the publication was rejected after the commit and
	// the player was moved onto the committed queue by the recovery path
	// instead. Cause holds the rejection.
	Recovered bool

	// Cause is the publication rejection a recovery absorbed, as a
	// *RefreshCommitError naming RefreshStagePublish, or nil.
	Cause error

	// Attempts counts the preparation attempts, including the one that
	// committed.
	Attempts int
}

// RefreshCommitError reports which stage rejected a refresh, so a caller can
// tell "your batch was never applied" from "your batch is live but mpv is
// catching up" without inspecting sentinels.
type RefreshCommitError struct {
	Stage RefreshStage
	Err   error
}

func (e *RefreshCommitError) Error() string {
	return fmt.Sprintf("player: refresh %s failed: %v", e.Stage, e.Err)
}

func (e *RefreshCommitError) Unwrap() error { return e.Err }

// CommitQueueRefresh runs a refresh through preparation, durable commit,
// publication, and maintenance.
//
// It performs database and IPC work and blocks while it waits for a playback
// transition to settle, so it must not be called from the Bubble Tea update
// path; CommitQueueRefreshAsync is that caller's entry point.
//
// The error it returns is a *RefreshCommitError naming the stage that rejected
// the refresh, and it is returned only for the two stages that can still call
// the refresh off. Once the commit succeeds the refresh has happened: a
// publication the player rejects is absorbed by recovery and reported through
// Recovered and Cause, not as a failure, because there is no state left to
// return to.
func (m *Manager) CommitQueueRefresh(ctx context.Context, rc RefreshCommit) (RefreshCommitResult, error) {
	var res RefreshCommitResult
	if rc.Prepare == nil || rc.Commit == nil {
		return res, &RefreshCommitError{
			Stage: RefreshStagePrepare,
			Err:   fmt.Errorf("%w: the refresh has no preparation or no commit step", ErrRefreshPlanInvalid),
		}
	}

	var last error
	for attempt := 1; attempt <= refreshCommitAttempts; attempt++ {
		res.Attempts = attempt
		plan, err := m.prepareRefresh(ctx, rc)
		if err == nil {
			return m.commitPreparedRefresh(rc, plan, res)
		}
		last = err
		// Staleness is the timeline having moved while the batch was being
		// built. It is the one failure worth repeating, because the next
		// capture describes wherever playback got to.
		if !errors.Is(err, ErrRefreshStale) {
			break
		}
	}
	return res, &RefreshCommitError{Stage: RefreshStagePrepare, Err: last}
}

// CommitQueueRefreshAsync runs CommitQueueRefresh on its own goroutine and hands
// the outcome to done, which is called exactly once.
//
// This is what the UI calls. The whole sequence — a settle wait, a database
// transaction, and mpv IPC — has no business on the Bubble Tea update path, and
// done runs on the refresh goroutine rather than under any player lock, so a
// handler that turns the result into a message cannot reenter the locks the
// refresh just released.
func (m *Manager) CommitQueueRefreshAsync(ctx context.Context, rc RefreshCommit, done func(RefreshCommitResult, error)) {
	safego.Go("player.commitQueueRefresh", func() {
		res, err := m.CommitQueueRefresh(ctx, rc)
		if done != nil {
			done(res, err)
		}
	})
}

// prepareRefresh runs the whole pre-commit stage: settle, capture, build, and
// re-check.
//
// The re-check at the end is required rather than an optimisation. Building a
// plan means a database round trip, and the timeline can move during it; without
// the check, the sequence would commit a batch and only discover at publication
// — past the point of no return — that it described a queue nobody is playing
// any more. Checking first turns that into an ordinary retry that leaves the
// previously committed state untouched.
func (m *Manager) prepareRefresh(ctx context.Context, rc RefreshCommit) (QueueRefresh, error) {
	if err := m.awaitRefreshWindow(ctx); err != nil {
		return QueueRefresh{}, err
	}
	snap := m.CaptureQueueRefresh()
	plan, err := rc.Prepare(snap)
	if err != nil {
		return QueueRefresh{}, err
	}
	// The snapshot the plan is checked against is the one it was handed, so a
	// caller cannot accidentally submit a plan carrying an older capture.
	plan.Snapshot = snap
	if err := m.checkRefreshWindow(snap); err != nil {
		return QueueRefresh{}, err
	}
	return plan, nil
}

// commitPreparedRefresh performs the durable transaction and everything that
// follows it.
func (m *Manager) commitPreparedRefresh(rc RefreshCommit, plan QueueRefresh, res RefreshCommitResult) (RefreshCommitResult, error) {
	if err := rc.Commit(plan); err != nil {
		return res, &RefreshCommitError{Stage: RefreshStageCommit, Err: err}
	}
	res.Committed = true

	out, err := m.ApplyQueueRefresh(plan)
	if err == nil {
		res.Result = out
		return res, nil
	}
	res.Recovered = true
	res.Cause = &RefreshCommitError{Stage: RefreshStagePublish, Err: err}
	res.Result = RefreshResult{CurrentIndex: m.recoverOntoRefresh(plan), PrefetchInvalidated: true}
	return res, nil
}

// recoverOntoRefresh moves the player onto a queue the database has already
// committed, after the guarded publication refused it.
//
// The guard is there to protect a plan's assumptions, and those assumptions are
// gone the moment the transaction lands: the refreshed queue is what the
// database holds and what a restart would load, so refusing to adopt it would
// leave the player playing rows that no longer exist and would resurrect them on
// the next save. Recovery therefore does not retry the substitution — that plan
// is unusable, which is what the rejection said — it adopts the committed queue
// through the ordinary replacement path, which carries the current row across by
// identity, reconciles the traversal, and reschedules the prefetch.
//
// The narrowing this accepts is real and deliberate: a substitution keeps each
// successor in its predecessor's turn, while reconciliation appends the new rows
// at the end of the cycle. A recovered refresh plays the right rows in a less
// carefully preserved order, which is the correct trade against playing rows
// that are not in the queue.
func (m *Manager) recoverOntoRefresh(plan QueueRefresh) int {
	slog.Warn("player: refresh publication rejected after commit; recovering onto the committed queue",
		"rows", len(plan.Queue), "replacements", len(plan.Replacements))
	m.SetQueue(plan.Queue)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentIndex
}

// awaitRefreshWindow blocks until no playback transition is in flight, so the
// capture that follows describes a timeline that has stopped moving.
//
// A refresh that merely rejected during the pending end-file window would be
// correct and useless: the window is short, but the Auto-DJ refills precisely
// when a track is ending, which is when the window is open. Waiting it out turns
// the most likely collision into a couple of milliseconds of latency, and the
// retry loop above only ever sees the transitions that opened after the capture.
//
// The wait is bounded twice over — by the caller's context and by
// refreshSettleWait — because an end-file that cannot resolve is a real state:
// a discard that could not be confirmed hands the transition to the restart
// supervisor, and nothing should block on that indefinitely.
func (m *Manager) awaitRefreshWindow(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, refreshSettleWait)
	defer cancel()

	// A manager with no lifetime context (the hand-built ones in tests) gets a
	// nil channel, which a select simply never selects.
	var closing <-chan struct{}
	if m.ctx != nil {
		closing = m.ctx.Done()
	}

	ticker := time.NewTicker(refreshSettlePoll)
	defer ticker.Stop()
	for {
		if err := m.refreshWindowLocked(); err == nil {
			return nil
		} else if !errors.Is(err, ErrRefreshTransitionPending) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ErrRefreshDeferred, ctx.Err())
		case <-closing:
			return fmt.Errorf("%w: the player is shutting down", ErrRefreshDeferred)
		case <-ticker.C:
		}
	}
}

// refreshWindowLocked reports whether a refresh may run right now.
func (m *Manager) refreshWindowLocked() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.checkRefreshTransitionLocked()
}

// checkRefreshWindow re-runs, before the commit, the two rejections the
// publication would raise after it: an unresolved transition and a timeline that
// no longer matches the snapshot.
//
// It is deliberately the same predicate rather than an approximation of it.
// Anything it lets through can still be rejected at publication — the queue is
// only locked for the publication itself, and playback keeps running in between
// — but every rejection it catches is one that would otherwise have cost a
// committed batch and a recovery.
func (m *Manager) checkRefreshWindow(snap RefreshSnapshot) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := m.checkRefreshTransitionLocked(); err != nil {
		return err
	}
	if live := m.captureRefreshLocked(); !live.guard.equal(snap.guard) {
		return fmt.Errorf("%w: the queue or playback moved while the batch was being prepared", ErrRefreshStale)
	}
	return nil
}
