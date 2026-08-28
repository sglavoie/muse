package app_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
)

// attrCapture is a slog.Handler that records each record's message together
// with its attributes rendered as "key=value" pairs. logCapture keeps only the
// message, but safego.Recover names the failing goroutine in the "goroutine"
// attribute, which is what identifies the shutdown step that panicked.
type attrCapture struct {
	mu    sync.Mutex
	lines []string
}

func (a *attrCapture) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (a *attrCapture) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(at slog.Attr) bool {
		b.WriteString(" ")
		b.WriteString(at.Key)
		b.WriteString("=")
		b.WriteString(at.Value.String())
		return true
	})
	a.mu.Lock()
	a.lines = append(a.lines, b.String())
	a.mu.Unlock()
	return nil
}

func (a *attrCapture) WithAttrs(_ []slog.Attr) slog.Handler { return a }
func (a *attrCapture) WithGroup(_ string) slog.Handler      { return a }

func (a *attrCapture) has(sub string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, l := range a.lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

// installAttrLogger swaps in attrCapture as the slog default for the test.
func installAttrLogger(t *testing.T) *attrCapture {
	t.Helper()
	prev := slog.Default()
	cap := &attrCapture{}
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return cap
}

// TestShutdown_PanicStillRunsRemainingSteps verifies that a panic raised by an
// early teardown callback does not abort the rest of the sequence: every later
// step must still run, and each exactly once. Before per-step recovery, the
// single outer recover ended shutdownSequence at the panic point while still
// closing done, so Shutdown returned normally with the DB open, the lockfile
// held, the mpv socket on disk and the log unflushed.
func TestShutdown_PanicStillRunsRemainingSteps(t *testing.T) {
	logs := installAttrLogger(t)

	dir := shortTestDir(t)
	lf := config.NewLockfile(dir)
	if err := lf.Acquire(); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	socketPath := filepath.Join(dir, "mpv.sock")
	createSocketFile(t, socketPath)

	rec := &stepRecorder{}

	deps := app.ShutdownDeps{
		// Step 1 panics. Everything below it must still run.
		ClosePlayer: func() error {
			rec.record("closePlayer")
			panic("boom: player teardown")
		},
		StopLibraryPipeline: func() { rec.record("stopPipeline") },
		CancelDBWriter:      func() { rec.record("cancelDB") },
		WaitDBWriter:        func() { rec.record("waitDB") },
		CloseDB:             func() { rec.record("closeDB") },
		Lockfile:            lf,
		SocketPath:          socketPath,
		CloseLog:            func() { rec.record("closeLog") },
	}

	app.Shutdown(deps, app.ShutdownTimeout)

	got := rec.snapshot()

	// Every later step ran, and exactly once.
	want := []string{"closePlayer", "stopPipeline", "cancelDB", "waitDB", "closeDB", "closeLog"}
	for _, name := range want {
		n := 0
		for _, s := range got {
			if s == name {
				n++
			}
		}
		if n != 1 {
			t.Errorf("step %q ran %d time(s), want exactly 1; got %v", name, n, got)
		}
	}

	// Order is preserved across the panic.
	for i := 1; i < len(want); i++ {
		bi, ai := indexIn(got, want[i-1]), indexIn(got, want[i])
		if bi == -1 || ai == -1 {
			continue // already reported above
		}
		if bi >= ai {
			t.Errorf("expected %q (index %d) before %q (index %d) in %v",
				want[i-1], bi, want[i], ai, got)
		}
	}

	// Steps with no callback still had their side effects applied: the mpv
	// socket is gone and the advisory lock was released.
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("mpv socket still present after a panicking teardown step")
	}
	reacquired := config.NewLockfile(dir)
	if err := reacquired.Acquire(); err != nil {
		t.Errorf("lockfile should be reacquirable after a panicking teardown step: %v", err)
	} else {
		reacquired.Release()
	}

	// The recovered panic log names the step that failed.
	if !logs.has("recovered panic in background goroutine") {
		t.Error("no recovered-panic log line was emitted")
	}
	if !logs.has("goroutine=app.shutdownSequence step 1: close player") {
		t.Error("recovered-panic log does not identify the failed shutdown step")
	}
}
