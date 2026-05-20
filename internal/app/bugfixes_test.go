package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/db"
)

// TestShutdownCloseLogInvokedExactlyOnce verifies that the normal shutdown
// path calls CloseLog exactly once. Combined with the sync.Once guard added
// in Shutdown, this prevents the timeout path from double-closing the log
// when it races shutdownSequence's own step-6 call.
func TestShutdownCloseLogInvokedExactlyOnce(t *testing.T) {
	installLogger(t)

	var count int32
	deps := app.ShutdownDeps{
		CloseLog: func() { atomic.AddInt32(&count, 1) },
	}
	app.Shutdown(deps, app.ShutdownTimeout)
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Errorf("CloseLog calls = %d, want 1", got)
	}
}

// TestShutdownNilCloseLogDoesNotPanicWithGuard verifies that the sync.Once
// guard around CloseLog is robust to a nil CloseLog field — the guarded
// wrapper must short-circuit before calling through to a nil function value.
func TestShutdownNilCloseLogDoesNotPanicWithGuard(t *testing.T) {
	installLogger(t)
	// Must not panic even though CloseLog is nil.
	app.Shutdown(app.ShutdownDeps{CloseLog: nil}, app.ShutdownTimeout)
}

// TestRunDurationAudit_CtxCancelStopsWorkers verifies that worker goroutines
// exit promptly when the context is cancelled instead of burning CPU running
// probeFn on every remaining work item. The fix adds a ctx.Err() check at the
// top of each worker loop iteration; without it, all queued probes run to
// completion even though the audit is being torn down.
func TestRunDurationAudit_CtxCancelStopsWorkers(t *testing.T) {
	var probes int32
	probeStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})

	restore := app.SetProbeFn(func(ctx context.Context, _ string) (float64, error) {
		atomic.AddInt32(&probes, 1)
		// Signal that at least one probe has started, then block every probe
		// until the test releases. This forces all currently-running workers
		// to be in-flight when cancel() is called, so the regression check
		// counts only the first batch (one per worker) rather than racing
		// against fast non-blocking probes that drain the backlog before
		// cancellation lands.
		select {
		case probeStarted <- struct{}{}:
		default:
		}
		select {
		case <-releaseFirst:
		case <-ctx.Done():
		}
		return 1.0, nil
	})
	t.Cleanup(restore)

	// Queue enough tracks that workers would still have work to do after the
	// first one finishes. With NumCPU workers, NumCPU*4 tracks gives plenty
	// of headroom for the regression.
	n := runtime.NumCPU() * 4
	tracks := make([]db.UnverifiedTrack, n)
	for i := range tracks {
		tracks[i] = db.UnverifiedTrack{
			Hash:       "h",
			Path:       "/p",
			DurationMs: 1000, // matches probe → would be verified
		}
	}
	reader := &stubReader{tracks: tracks}
	writer := newStubVerifier()

	ctx, cancel := context.WithCancel(context.Background())
	ch := app.RunDurationAudit(ctx, reader, writer)

	// Wait until at least one probe has started, then cancel before
	// releasing it. Workers should observe ctx.Err() on the next iteration
	// and stop, instead of probing the entire backlog.
	<-probeStarted
	cancel()
	close(releaseFirst)

	// Drain the channel so the audit goroutine can exit.
	for range ch {
	}

	// At most one probe per worker can sneak past the ctx check (the one
	// already in-flight when cancellation occurred). Allow that slack.
	got := int(atomic.LoadInt32(&probes))
	maxAllowed := runtime.NumCPU()
	if got > maxAllowed {
		t.Errorf("probeFn called %d times after cancellation; want <= %d (workers should bail on ctx.Done)", got, maxAllowed)
	}
}

// TestRunStartupSkipsRestoreForMissingFile verifies that Run does not invoke
// deps.RestorePlayer when the current track's file is missing on disk. Loading
// a missing file in mpv would surface a startup error that the user can't act
// on; the TUI already renders the entry as missing.
func TestRunStartupSkipsRestoreForMissingFile(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	// Seed a session pointing at a track whose file does not exist on disk.
	database := openTestDB(t, dirs)
	missingPath := filepath.Join(dirs.Runtime, "gone.mp3")
	insertTrack(t, database, "hash-missing", missingPath, "Gone", "Track")
	insertQueueEntry(t, database, "hash-missing", 1000)
	setSessionRow(t, database, "hash-missing", 5000, false, "none")
	database.Close()

	restoreCalled := false
	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
		RestorePlayer: func(_ string, _ int64) error {
			restoreCalled = true
			return nil
		},
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	closeRunResult(t, result)

	if restoreCalled {
		t.Error("RestorePlayer must NOT be called when the current track's file is missing on disk")
	}
	// Sanity: the queue entry should be flagged Missing after RestoreSession.
	if len(result.Restored.QueueEntries) != 1 || !result.Restored.QueueEntries[0].Missing {
		t.Errorf("expected restored queue to flag the entry missing; got %+v", result.Restored.QueueEntries)
	}
}

func TestRunStartupSkipsRestoreForUnreadableCurrentTrack(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	database := openTestDB(t, dirs)
	blocker := filepath.Join(dirs.Runtime, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	unreadablePath := filepath.Join(blocker, "gone.mp3")
	insertTrack(t, database, "hash-unreadable", unreadablePath, "Gone", "Track")
	insertQueueEntry(t, database, "hash-unreadable", 1000)
	setSessionRow(t, database, "hash-unreadable", 5000, false, "none")
	database.Close()

	restoreCalled := false
	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
		RestorePlayer: func(_ string, _ int64) error {
			restoreCalled = true
			return nil
		},
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	closeRunResult(t, result)

	if restoreCalled {
		t.Error("RestorePlayer must NOT be called when the current track's path is unreachable")
	}
	if len(result.Restored.QueueEntries) != 1 || !result.Restored.QueueEntries[0].Missing {
		t.Errorf("expected restored queue to flag the entry missing; got %+v", result.Restored.QueueEntries)
	}
}

// TestRunStartupRestoreStillCalledForReadableFile guards against the
// missing-file skip regressing into "never restore at all". When the file is
// present on disk the player must still receive the path/position to seek to.
func TestRunStartupRestoreStillCalledForReadableFile(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	database := openTestDB(t, dirs)
	audioPath := filepath.Join(dirs.Runtime, "ok.mp3")
	if err := os.WriteFile(audioPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-ok", audioPath, "Ok", "Track")
	insertQueueEntry(t, database, "hash-ok", 1000)
	setSessionRow(t, database, "hash-ok", 9000, false, "none")
	database.Close()

	var gotPath string
	var gotPos int64
	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
		RestorePlayer: func(path string, pos int64) error {
			gotPath = path
			gotPos = pos
			return nil
		},
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	closeRunResult(t, result)

	if gotPath != audioPath || gotPos != 9000 {
		t.Errorf("RestorePlayer args: got (%q, %d), want (%q, 9000)", gotPath, gotPos, audioPath)
	}
}

// TestRunDrainsAllParallelErrors verifies that when BOTH parallel tasks fail,
// the first error is returned AND the second is not silently dropped (logged
// at Warn). Before the fix the loop returned on the first error, leaving the
// other error stuck in the closed channel and never surfaced anywhere.
func TestRunDrainsAllParallelErrors(t *testing.T) {
	dirs := setupDirs(t)
	cap := installLogger(t)

	mpvErr := errors.New("mpv boom")
	scanErr := errors.New("scan boom")

	// Use latches so both errors are emitted deterministically before
	// the parent goroutine's wg.Wait() observes either.
	gate := make(chan struct{})
	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { return nil },
		LaunchMPV: func(_ context.Context, _ string) error {
			<-gate
			return mpvErr
		},
		RunScan: func(_ context.Context, _ *db.DB) error {
			<-gate
			return scanErr
		},
	}

	// Release both goroutines simultaneously so both errors land in the
	// channel before Run's loop runs.
	go func() { close(gate) }()

	_, err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected an error from parallel failures")
	}

	// Look for evidence that the additional error was at least logged.
	// The wrapped error message should mention either mpv or scan.
	msgs := cap.messages()
	if !hasMsg(msgs, "additional parallel-task error") {
		t.Errorf("expected log of additional parallel-task error; messages: %v", msgs)
	}
}
