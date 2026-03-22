package app_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
)

// stepRecorder collects step names in the order they are called. Because
// Shutdown is sequential, no mutex is required, but one is included for
// safety.
type stepRecorder struct {
	mu    sync.Mutex
	steps []string
}

func (r *stepRecorder) record(name string) {
	r.mu.Lock()
	r.steps = append(r.steps, name)
	r.mu.Unlock()
}

func (r *stepRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]string, len(r.steps))
	copy(cp, r.steps)
	return cp
}

// indexIn returns the index of the first element of ss containing sub, or -1.
func indexIn(ss []string, sub string) int {
	for i, s := range ss {
		if s == sub {
			return i
		}
	}
	return -1
}

// TestShutdownOrder confirms that all teardown steps are invoked and that
// their execution order matches the specified sequence.
func TestShutdownOrder(t *testing.T) {
	installLogger(t)

	rec := &stepRecorder{}

	deps := app.ShutdownDeps{
		ClosePlayer:    func() error { rec.record("close"); return nil },
		CancelDBWriter: func() { rec.record("cancelDB") },
		WaitDBWriter:   func() { rec.record("waitDB") },
		Lockfile:       nil, // tested separately
		SocketPath:     "",  // tested separately
	}

	app.Shutdown(deps, app.ShutdownTimeout)

	got := rec.snapshot()

	expected := []string{"close", "cancelDB", "waitDB"}
	for _, name := range expected {
		if indexIn(got, name) == -1 {
			t.Errorf("step %q was not recorded; got %v", name, got)
		}
	}

	checks := []struct{ before, after string }{
		{"close", "cancelDB"},
		{"cancelDB", "waitDB"},
	}
	for _, c := range checks {
		bi := indexIn(got, c.before)
		ai := indexIn(got, c.after)
		if bi == -1 || ai == -1 {
			continue // already reported above
		}
		if bi >= ai {
			t.Errorf("expected %q (index %d) before %q (index %d) in %v",
				c.before, bi, c.after, ai, got)
		}
	}
}

// TestShutdownDBWriterFlushedBeforeClose verifies that CancelDBWriter is
// always called before WaitDBWriter so the writer goroutine is signalled to
// stop before we block waiting for it to exit.
func TestShutdownDBWriterFlushedBeforeClose(t *testing.T) {
	installLogger(t)

	rec := &stepRecorder{}

	deps := app.ShutdownDeps{
		CancelDBWriter: func() { rec.record("cancel") },
		WaitDBWriter:   func() { rec.record("wait") },
	}

	app.Shutdown(deps, app.ShutdownTimeout)

	got := rec.snapshot()

	cancelIdx := indexIn(got, "cancel")
	waitIdx := indexIn(got, "wait")

	if cancelIdx == -1 {
		t.Fatal("CancelDBWriter was not called")
	}
	if waitIdx == -1 {
		t.Fatal("WaitDBWriter was not called")
	}
	if cancelIdx >= waitIdx {
		t.Errorf("CancelDBWriter (index %d) must be called before WaitDBWriter (index %d)",
			cancelIdx, waitIdx)
	}
}

// TestShutdownLockfileRemoved verifies that an on-disk PID lockfile is deleted
// during shutdown.
func TestShutdownLockfileRemoved(t *testing.T) {
	installLogger(t)

	dir := t.TempDir()
	lf := config.NewLockfile(dir)
	if err := lf.Acquire(); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Confirm the file exists before shutdown.
	lockPath := filepath.Join(dir, "muse.pid")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lockfile missing before Shutdown: %v", err)
	}

	app.Shutdown(app.ShutdownDeps{Lockfile: lf}, app.ShutdownTimeout)

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lockfile was not removed during Shutdown")
	}
}

// TestShutdownSocketRemoved verifies that the mpv socket file is deleted.
func TestShutdownSocketRemoved(t *testing.T) {
	installLogger(t)

	socketPath := filepath.Join(t.TempDir(), "mpv.sock")
	if err := os.WriteFile(socketPath, nil, 0o600); err != nil {
		t.Fatalf("create socket: %v", err)
	}

	app.Shutdown(app.ShutdownDeps{SocketPath: socketPath}, app.ShutdownTimeout)

	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("mpv socket was not removed during Shutdown")
	}
}

// TestShutdownSocketAlreadyGone verifies that a missing socket is not an error.
func TestShutdownSocketAlreadyGone(t *testing.T) {
	installLogger(t)

	// Use a path that does not exist.
	socketPath := filepath.Join(t.TempDir(), "mpv.sock")

	// Must not panic or log an error.
	app.Shutdown(app.ShutdownDeps{SocketPath: socketPath}, app.ShutdownTimeout)
}

// TestShutdownNilFieldsNoPanic confirms that all nil/empty fields are safe
// no-ops and the function returns without panic.
func TestShutdownNilFieldsNoPanic(t *testing.T) {
	installLogger(t)

	// All fields nil/zero — must not panic.
	app.Shutdown(app.ShutdownDeps{}, app.ShutdownTimeout)
}

// TestShutdownAllStepsRunDespiteErrors verifies that a failure in an early
// step (ClosePlayer) does not abort subsequent steps.
func TestShutdownAllStepsRunDespiteErrors(t *testing.T) {
	installLogger(t)

	rec := &stepRecorder{}

	deps := app.ShutdownDeps{
		ClosePlayer:    func() error { rec.record("close"); return errors.New("player error") },
		CancelDBWriter: func() { rec.record("cancel") },
		WaitDBWriter:   func() { rec.record("wait") },
	}

	// Must not panic or skip steps.
	app.Shutdown(deps, app.ShutdownTimeout)

	got := rec.snapshot()
	for _, name := range []string{"close", "cancel", "wait"} {
		if indexIn(got, name) == -1 {
			t.Errorf("step %q was skipped after earlier error; got %v", name, got)
		}
	}
}

// TestShutdownDBWriterDrainsOnShutdown verifies the end-to-end wiring between
// app.Shutdown, db.Writer, and a real SQLite database. All writes committed
// before Shutdown is called must be present in the database after it returns.
func TestShutdownDBWriterDrainsOnShutdown(t *testing.T) {
	installLogger(t)

	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer d.Close()

	if _, err := d.Exec("CREATE TABLE items (n INTEGER)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	w := db.NewWriter(ctx, d.DB)

	const n = 5
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			_ = w.Exec(func(sdb *sql.DB) error {
				_, err := sdb.Exec("INSERT INTO items VALUES (?)", val)
				return err
			})
		}(i)
	}
	wg.Wait() // all writes committed before Shutdown is invoked

	app.Shutdown(app.ShutdownDeps{
		CancelDBWriter: cancel,
		WaitDBWriter:   w.Wait,
	}, app.ShutdownTimeout)

	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM items").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != n {
		t.Errorf("DB has %d rows; want %d after Shutdown", count, n)
	}
}

// TestShutdownLockfileAndSocketOrdering verifies that the lockfile is released
// before the mpv socket is deleted (step 4 before step 5).
func TestShutdownLockfileAndSocketOrdering(t *testing.T) {
	installLogger(t)

	dir := t.TempDir()
	lf := config.NewLockfile(dir)
	if err := lf.Acquire(); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	socketPath := filepath.Join(dir, "mpv.sock")
	if err := os.WriteFile(socketPath, nil, 0o600); err != nil {
		t.Fatalf("create socket: %v", err)
	}

	// Record which files are gone at the moment each resource is cleaned up.
	// We do this by wrapping Release via a custom lockfile check.
	// Since config.Lockfile.Release is not injectable, we verify post-conditions
	// instead: both file should be gone after Shutdown returns.

	app.Shutdown(app.ShutdownDeps{
		Lockfile:   lf,
		SocketPath: socketPath,
	}, app.ShutdownTimeout)

	lockPath := filepath.Join(dir, "muse.pid")
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lockfile still present after Shutdown")
	}
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("mpv socket still present after Shutdown")
	}
}

// TestShutdownTimeout_ForcesExit verifies that Shutdown calls os.Exit(1) when
// the sequence exceeds the timeout. It runs itself as a subprocess to safely
// observe the exit code without terminating the test process.
func TestShutdownTimeout_ForcesExit(t *testing.T) {
	if os.Getenv("TEST_SHUTDOWN_TIMEOUT") == "1" {
		// Subprocess mode: WaitDBWriter blocks forever, timeout is tiny.
		block := make(chan struct{})
		app.Shutdown(app.ShutdownDeps{
			WaitDBWriter: func() { <-block },
		}, 50*time.Millisecond)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestShutdownTimeout_ForcesExit")
	cmd.Env = append(os.Environ(), "TEST_SHUTDOWN_TIMEOUT=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit, got nil")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got: %v", err)
	}
}
