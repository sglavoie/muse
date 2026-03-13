package app_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
)

// logCapture is a minimal slog.Handler that records message strings.
type logCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (l *logCapture) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (l *logCapture) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	l.msgs = append(l.msgs, r.Message)
	l.mu.Unlock()
	return nil
}

func (l *logCapture) WithAttrs(_ []slog.Attr) slog.Handler  { return l }
func (l *logCapture) WithGroup(_ string) slog.Handler        { return l }

func (l *logCapture) messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := make([]string, len(l.msgs))
	copy(cp, l.msgs)
	return cp
}

// setupDirs creates isolated temp directories and returns a config.Dirs that
// points to them.
func setupDirs(t *testing.T) config.Dirs {
	t.Helper()
	return config.Dirs{
		Runtime: t.TempDir(),
		Data:    t.TempDir(),
	}
}

// noopDeps returns a Deps where prereqs always pass and both parallel tasks
// succeed immediately.
func noopDeps() app.Deps {
	return app.Deps{
		CheckPrereqs: func() error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}
}

// installLogger swaps in logCapture as the slog default and restores the
// previous default when the test ends.
func installLogger(t *testing.T) *logCapture {
	t.Helper()
	prev := slog.Default()
	cap := &logCapture{}
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return cap
}

// hasMsg returns true if any captured message contains sub.
func hasMsg(msgs []string, sub string) bool {
	for _, m := range msgs {
		if strings.Contains(m, sub) {
			return true
		}
	}
	return false
}

// msgIndex returns the position of the first message containing sub, or -1.
func msgIndex(msgs []string, sub string) int {
	for i, m := range msgs {
		if strings.Contains(m, sub) {
			return i
		}
	}
	return -1
}

// TestRunOrder verifies that all startup steps execute in the correct sequence
// and that expected slog messages are emitted for each lifecycle event.
func TestRunOrder(t *testing.T) {
	dirs := setupDirs(t)
	log := installLogger(t)

	// Place a stale socket file to confirm it gets removed.
	staleSocket := filepath.Join(dirs.Runtime, "mpv.sock")
	if err := os.WriteFile(staleSocket, nil, 0o600); err != nil {
		t.Fatalf("create stale socket: %v", err)
	}

	var mu sync.Mutex
	var parallelSteps []string

	deps := app.Deps{
		CheckPrereqs: func() error { return nil },
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

	if err := app.Run(context.Background(), dirs, deps); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	// Stale socket must have been removed.
	if _, err := os.Stat(staleSocket); !os.IsNotExist(err) {
		t.Error("stale mpv socket was not removed")
	}

	// Both parallel tasks must have run.
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

	// Verify log message ordering.
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

	// Verify all expected lifecycle log messages are present.
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

	// Write a lockfile that claims a live PID (use current process PID).
	lockPath := filepath.Join(dirs.Runtime, "muse.pid")
	if err := os.MkdirAll(dirs.Runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("999999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// PID 999999999 is almost certainly not alive, so acquire will reclaim it.
	// To force a real conflict we write the actual running PID.
	if err := os.WriteFile(lockPath, []byte(pidStr(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}

	prereqCalled := false
	deps := app.Deps{
		CheckPrereqs: func() error { prereqCalled = true; return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}

	err := app.Run(context.Background(), dirs, deps)
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
		CheckPrereqs: func() error { return prereqErr },
		LaunchMPV:    func(_ context.Context, _ string) error { launchCalled = true; return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}

	err := app.Run(context.Background(), dirs, deps)
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

	// Point Data to a path where the parent directory does not exist so that
	// sqlite cannot create the database file.
	dirs.Data = filepath.Join(dirs.Data, "nonexistent", "nested")

	prereqCalled := false
	deps := app.Deps{
		CheckPrereqs: func() error { prereqCalled = true; return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}

	err := app.Run(context.Background(), dirs, deps)
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
	installLogger(t)

	staleSocket := filepath.Join(dirs.Runtime, "mpv.sock")
	if err := os.WriteFile(staleSocket, nil, 0o600); err != nil {
		t.Fatalf("create stale socket: %v", err)
	}

	// Verify socket is gone by the time RunScan executes (after migration).
	socketRemovedBeforeScan := false
	deps := app.Deps{
		CheckPrereqs: func() error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan: func(_ context.Context, _ *db.DB) error {
			_, err := os.Stat(staleSocket)
			socketRemovedBeforeScan = os.IsNotExist(err)
			return nil
		},
	}

	if err := app.Run(context.Background(), dirs, deps); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	if !socketRemovedBeforeScan {
		t.Error("stale socket was not removed before scan launched")
	}
}

// TestRunNoStaleSocket verifies that Run succeeds even when no socket file exists.
func TestRunNoStaleSocket(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	if err := app.Run(context.Background(), dirs, noopDeps()); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
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
		CheckPrereqs: func() error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return mpvErr },
		RunScan:      func(_ context.Context, _ *db.DB) error { return scanErr },
	}

	err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected error from parallel task failures, got nil")
	}
}

// helpers

func contains(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

func pidStr(pid int) string {
	return strconv.Itoa(pid) + "\n"
}
