package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
)

// TestRunOrder verifies that all startup steps execute in the correct sequence
// and that expected slog messages are emitted for each lifecycle event.
func TestRunOrder(t *testing.T) {
	dirs := setupDirs(t)
	dirs.Runtime = shortTestDir(t)
	log := installLogger(t)

	// Place a stale socket file to confirm it gets removed.
	staleSocket := filepath.Join(dirs.Runtime, "mpv.sock")
	createSocketFile(t, staleSocket)

	var mu sync.Mutex
	var parallelSteps []string

	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { return nil },
		LaunchMPV: func(_ context.Context, _ string) error {
			mu.Lock()
			parallelSteps = append(parallelSteps, "mpv")
			mu.Unlock()
			return nil
		},
		RunScan: func(_ context.Context, _ *db.DB) error {
			mu.Lock()
			parallelSteps = append(parallelSteps, "scan")
			mu.Unlock()
			return nil
		},
	}

	if _, err := app.Run(context.Background(), dirs, deps); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	if _, err := os.Stat(staleSocket); !os.IsNotExist(err) {
		t.Error("stale mpv socket was not removed")
	}

	mu.Lock()
	gotMPV := contains(parallelSteps, "mpv")
	gotScan := contains(parallelSteps, "scan")
	mu.Unlock()
	if !gotMPV {
		t.Error("LaunchMPV was not called")
	}
	if !gotScan {
		t.Error("RunScan was not called")
	}

	msgs := log.messages()
	checks := []struct {
		before string
		after  string
	}{
		{"acquiring lockfile", "removing stale mpv socket"},
		{"removing stale mpv socket", "running database migrations"},
		{"running database migrations", "checking prerequisites"},
		{"checking prerequisites", "launching mpv and library scan"},
	}
	for _, c := range checks {
		bi := msgIndex(msgs, c.before)
		ai := msgIndex(msgs, c.after)
		if bi == -1 {
			t.Errorf("log message %q not found", c.before)
			continue
		}
		if ai == -1 {
			t.Errorf("log message %q not found", c.after)
			continue
		}
		if bi >= ai {
			t.Errorf("expected %q before %q, got indices %d >= %d", c.before, c.after, bi, ai)
		}
	}

	for _, expected := range []string{
		"acquiring lockfile",
		"lockfile acquired",
		"removing stale mpv socket",
		"stale mpv socket removed",
		"running database migrations",
		"database migrations complete",
		"checking prerequisites",
		"prerequisites verified",
		"launching mpv and library scan",
	} {
		if !hasMsg(msgs, expected) {
			t.Errorf("expected log message containing %q, not found in: %v", expected, msgs)
		}
	}
}

// TestRunLockfileFailureStopsSequence verifies that a lockfile conflict causes
// Run to return an error and no subsequent steps execute.
func TestRunLockfileFailureStopsSequence(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	owner := config.NewLockfile(dirs.Runtime)
	if err := owner.Acquire(); err != nil {
		t.Fatalf("owner Acquire: %v", err)
	}
	t.Cleanup(owner.Release)
	prereqCalled := false
	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { prereqCalled = true; return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}

	_, err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected error from lockfile conflict, got nil")
	}
	if !strings.Contains(err.Error(), "another instance is running") {
		t.Errorf("unexpected error: %v", err)
	}
	if prereqCalled {
		t.Error("CheckPrereqs should not have been called after lockfile failure")
	}
}

// TestRunPrereqFailureStopsParallelLaunch verifies that a prereq check failure
// prevents mpv and the scan from starting.
func TestRunPrereqFailureStopsParallelLaunch(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	prereqErr := errors.New("mpv not found")
	launchCalled := false

	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { return prereqErr },
		LaunchMPV:    func(_ context.Context, _ string) error { launchCalled = true; return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}

	_, err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected error from prereq failure, got nil")
	}
	if !strings.Contains(err.Error(), prereqErr.Error()) {
		t.Errorf("unexpected error: %v", err)
	}
	if launchCalled {
		t.Error("LaunchMPV should not have been called after prereq failure")
	}
}

// TestRunMigrationFailureStopsPrereqs verifies that a migration failure
// prevents the prerequisite check from running.
func TestRunMigrationFailureStopsPrereqs(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	blockPath := filepath.Join(dirs.Data, "block")
	if err := os.WriteFile(blockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dirs.Data = blockPath

	prereqCalled := false
	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { prereqCalled = true; return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}

	_, err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected error from DB open failure, got nil")
	}
	if prereqCalled {
		t.Error("CheckPrereqs should not have been called after migration failure")
	}
}

// TestRunStaleSocketRemovedBeforeMigration verifies that the socket file is
// removed after the lockfile is held but before the DB is opened.
func TestRunStaleSocketRemovedBeforeMigration(t *testing.T) {
	dirs := setupDirs(t)
	dirs.Runtime = shortTestDir(t)
	installLogger(t)

	staleSocket := filepath.Join(dirs.Runtime, "mpv.sock")
	createSocketFile(t, staleSocket)

	socketRemovedBeforeScan := false
	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan: func(_ context.Context, _ *db.DB) error {
			_, err := os.Stat(staleSocket)
			socketRemovedBeforeScan = os.IsNotExist(err)
			return nil
		},
	}

	if _, err := app.Run(context.Background(), dirs, deps); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	if !socketRemovedBeforeScan {
		t.Error("stale socket was not removed before scan launched")
	}
}

func TestRun_LeavesRegularFileAtSocketPathUntouched(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	socketPath := filepath.Join(dirs.Runtime, "mpv.sock")
	want := []byte("keep me")
	if err := os.WriteFile(socketPath, want, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := app.Run(context.Background(), dirs, app.Deps{
		Config:       config.Config{MPVSocketPath: socketPath},
		CheckPrereqs: func(context.Context) error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	})
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	closeRunResult(t, result)

	got, err := os.ReadFile(socketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("socket path regular file contents = %q, want %q", got, want)
	}
}

// TestRunNoStaleSocket verifies that Run succeeds even when no socket file exists.
func TestRunNoStaleSocket(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	if _, err := app.Run(context.Background(), dirs, noopDeps()); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
}

// TestRunMissingLaunchMPVReturnsError verifies a missing LaunchMPV dependency
// fails fast with a regular error (not a panic).
func TestRunMissingLaunchMPVReturnsError(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { return nil },
	}

	_, err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected error when LaunchMPV is nil, got nil")
	}
	if !strings.Contains(err.Error(), "LaunchMPV is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRunParallelLaunchBothErrorsReturned verifies that if both parallel tasks
// fail, at least one error is surfaced.
func TestRunParallelLaunchBothErrorsReturned(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	mpvErr := errors.New("mpv failed")
	scanErr := errors.New("scan failed")

	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return mpvErr },
		RunScan:      func(_ context.Context, _ *db.DB) error { return scanErr },
	}

	_, err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected error from parallel task failures, got nil")
	}
}

// TestRunConcurrentLaunchAndScan proves that LaunchMPV and RunScan execute
// concurrently: RunScan must start before the deliberately delayed LaunchMPV returns.
func TestRunConcurrentLaunchAndScan(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	gate := make(chan struct{})

	var mu sync.Mutex
	var scanStarted, mpvReturned time.Time

	deps := app.Deps{
		CheckPrereqs: func(context.Context) error { return nil },
		LaunchMPV: func(_ context.Context, _ string) error {
			<-gate
			mu.Lock()
			mpvReturned = time.Now()
			mu.Unlock()
			return nil
		},
		RunScan: func(_ context.Context, _ *db.DB) error {
			mu.Lock()
			scanStarted = time.Now()
			mu.Unlock()
			return nil
		},
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		close(gate)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := app.Run(ctx, dirs, deps); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	mu.Lock()
	started := scanStarted
	returned := mpvReturned
	mu.Unlock()

	if started.IsZero() {
		t.Fatal("RunScan was never called")
	}
	if returned.IsZero() {
		t.Fatal("LaunchMPV never returned")
	}
	if !started.Before(returned) {
		t.Errorf("RunScan started (%v) was not before LaunchMPV returned (%v); goroutines are not running concurrently", started, returned)
	}
}
