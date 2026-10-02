package player

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newShutdownTestManager builds a Manager whose launchFn only counts calls, so
// a test can assert the restart supervisor never relaunched mpv.
func newShutdownTestManager(t *testing.T) (*Manager, *atomic.Int32, *statusLog) {
	t.Helper()
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	m := &Manager{
		socketPath:       shortSockPath(t, "shutdown"),
		rootCtx:          rootCtx,
		rootCancel:       rootCancel,
		ctx:              ctx,
		cancel:           cancel,
		disconnectCh:     make(chan error, 1),
		callbacksReady:   make(chan struct{}),
		restartBaseDelay: time.Millisecond,
		cmdQueue:         []func() error{},
	}
	t.Cleanup(func() { _ = m.Close() })
	statuses := &statusLog{}
	m.SetEventCallbacks(EventCallbacks{OnStatus: statuses.add})

	launches := &atomic.Int32{}
	m.launchFn = func(context.Context, string) (*exec.Cmd, error) {
		launches.Add(1)
		return nil, errors.New("fake launch")
	}
	return m, launches, statuses
}

type statusLog struct {
	mu   sync.Mutex
	msgs []string
}

func (l *statusLog) add(s string) {
	l.mu.Lock()
	l.msgs = append(l.msgs, s)
	l.mu.Unlock()
}

func (l *statusLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.msgs...)
}

// TestBeginShutdown_SupervisorDoesNotRelaunchAfterDisconnect: once shutdown has
// begun, a disconnect (mpv killed by the same SIGHUP that is ending muse) ends
// the supervisor without a single launch attempt.
func TestBeginShutdown_SupervisorDoesNotRelaunchAfterDisconnect(t *testing.T) {
	m, launches, statuses := newShutdownTestManager(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.watchAndRestart()
	}()

	m.BeginShutdown()
	m.disconnectCh <- errors.New("mpv died")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor kept running after a disconnect during shutdown")
	}
	if n := launches.Load(); n != 0 {
		t.Fatalf("launchFn called %d times after BeginShutdown, want 0", n)
	}
	if got := statuses.snapshot(); len(got) != 0 {
		t.Fatalf("status messages during shutdown = %q, want none", got)
	}
}

// TestBeginShutdown_AttemptRestartRefusesToLaunch: an attempt already past the
// supervisor's check still re-checks the flag before launchFn.
func TestBeginShutdown_AttemptRestartRefusesToLaunch(t *testing.T) {
	m, launches, _ := newShutdownTestManager(t)
	m.BeginShutdown()

	if m.attemptRestart(m.snapshotRestore()) {
		t.Fatal("attemptRestart reported success after BeginShutdown")
	}
	if n := launches.Load(); n != 0 {
		t.Fatalf("launchFn called %d times after BeginShutdown, want 0", n)
	}
}

// TestBeginShutdown_RecoverOutageStopsWithoutGivingUpNotice: an outage that
// begins recovering and then meets shutdown stops retrying without telling the
// user recovery was abandoned.
func TestBeginShutdown_RecoverOutageStopsWithoutGivingUpNotice(t *testing.T) {
	m, launches, statuses := newShutdownTestManager(t)
	m.restartMaxAttemptsOverride = 50
	m.launchFn = func(context.Context, string) (*exec.Cmd, error) {
		if launches.Add(1) == 2 {
			m.BeginShutdown()
		}
		return nil, errors.New("fake launch")
	}

	if m.recoverOutage() {
		t.Fatal("recoverOutage reported success after BeginShutdown")
	}
	if n := launches.Load(); n != 2 {
		t.Fatalf("launchFn called %d times, want 2 (none after BeginShutdown)", n)
	}
	for _, s := range statuses.snapshot() {
		if s == "mpv: gave up after too many restart attempts" {
			t.Fatalf("give-up notice sent during shutdown: %q", statuses.snapshot())
		}
	}
}

func TestBeginShutdown_Idempotent(t *testing.T) {
	m, _, _ := newShutdownTestManager(t)
	if m.shutdownBegun() {
		t.Fatal("shutdownBegun before BeginShutdown")
	}
	m.BeginShutdown()
	m.BeginShutdown()
	if !m.shutdownBegun() {
		t.Fatal("shutdownBegun false after BeginShutdown")
	}
}
