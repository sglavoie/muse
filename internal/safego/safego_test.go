package safego

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureLog installs a text logger writing into the returned buffer for the
// duration of the test. The mutex guards the buffer against the concurrent
// writes that come from the goroutines under test.
func captureLog(t *testing.T) (*bytes.Buffer, *sync.Mutex) {
	t.Helper()
	var buf bytes.Buffer
	var mu sync.Mutex
	handler := slog.NewTextHandler(&lockedWriter{mu: &mu, buf: &buf}, &slog.HandlerOptions{Level: slog.LevelDebug})
	orig := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf, &mu
}

type lockedWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func read(buf *bytes.Buffer, mu *sync.Mutex) string {
	mu.Lock()
	defer mu.Unlock()
	return buf.String()
}

// TestRecover_PanickingGoroutineIsLoggedAndDoesNotCrash is the core guarantee of
// this package: a goroutine that panics is recovered and logged with a stack
// trace instead of taking the whole process (here, the test binary) down. If the
// recover were missing or wrongly nested, this test would not fail — it would
// abort the entire package run, which is itself the signal.
func TestRecover_PanickingGoroutineIsLoggedAndDoesNotCrash(t *testing.T) {
	buf, mu := captureLog(t)

	// The panicking body sits in an inner func so that close(done) is deferred
	// *outside* the recovering frame and therefore runs strictly after Recover
	// has logged; deferring both in the same frame would make the read below
	// race the log write.
	done := make(chan struct{})
	survived := false
	go func() {
		defer close(done)
		func() {
			defer Recover("safego.test.panicker")
			panic("boom from a background goroutine")
		}()
		survived = true
	}()
	<-done

	if !survived {
		t.Error("the goroutine did not resume after the recovered panic")
	}

	log := read(buf, mu)
	if !strings.Contains(log, "recovered panic in background goroutine") {
		t.Errorf("expected the recovery message in the log; got: %s", log)
	}
	if !strings.Contains(log, "safego.test.panicker") {
		t.Errorf("expected the goroutine name in the log; got: %s", log)
	}
	if !strings.Contains(log, "boom from a background goroutine") {
		t.Errorf("expected the panic value in the log; got: %s", log)
	}
	if !strings.Contains(log, "stack=") {
		t.Errorf("expected a stack field in the log; got: %s", log)
	}
	// The stack must be the panicking goroutine's, not an unrelated frame.
	if !strings.Contains(log, "TestRecover_PanickingGoroutineIsLoggedAndDoesNotCrash") {
		t.Errorf("expected the panicking frame in the stack; got: %s", log)
	}
}

// TestRecover_DeferredChannelCloseStillRuns pins the property the whole audit
// leans on: recovering must not swallow a site's existing deferred channel
// close, or a recovered pipeline stage would leave its consumers blocked
// forever — a hang, which is worse than the crash it replaced.
func TestRecover_DeferredChannelCloseStillRuns(t *testing.T) {
	captureLog(t)

	out := make(chan int)
	go func() {
		defer Recover("safego.test.producer")
		defer close(out)
		out <- 1
		panic("producer died mid-stream")
	}()

	var got []int
	for v := range out {
		got = append(got, v)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("expected to receive the pre-panic value; got %v", got)
	}
	// Reaching here at all means range terminated, i.e. close(out) ran during
	// the panic unwind.
}

// TestRecover_NoPanicIsSilent verifies the helper is inert on the normal path.
func TestRecover_NoPanicIsSilent(t *testing.T) {
	buf, mu := captureLog(t)

	done := make(chan struct{})
	go func() {
		defer Recover("safego.test.quiet")
		close(done)
	}()
	<-done

	if log := read(buf, mu); strings.Contains(log, "recovered panic") {
		t.Errorf("expected no recovery log without a panic; got: %s", log)
	}
}

// TestRecoverTo_ReportsPanicAsError verifies the error-reporting variant used by
// startup tasks, where swallowing a panic would otherwise report success.
func TestRecoverTo_ReportsPanicAsError(t *testing.T) {
	captureLog(t)

	errs := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer RecoverTo("safego.test.task", func(err error) { errs <- err })
		panic("task exploded")
	}()
	<-done

	select {
	case err := <-errs:
		if !strings.Contains(err.Error(), "safego.test.task") {
			t.Errorf("expected the goroutine name in the error; got %v", err)
		}
		if !strings.Contains(err.Error(), "task exploded") {
			t.Errorf("expected the panic value in the error; got %v", err)
		}
	default:
		t.Fatal("expected RecoverTo to report the panic to the handler")
	}
}

// TestRecoverTo_HandlerNotCalledWithoutPanic guards against a spurious error
// being reported on the success path.
func TestRecoverTo_HandlerNotCalledWithoutPanic(t *testing.T) {
	captureLog(t)

	called := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer RecoverTo("safego.test.ok", func(error) { called = true })
	}()
	<-done

	if called {
		t.Error("handler must not be called when no panic occurred")
	}
}

// TestGo_RecoversPanicInSpawnedFunction covers the single-call spawn helper.
func TestGo_RecoversPanicInSpawnedFunction(t *testing.T) {
	buf, mu := captureLog(t)

	Go("safego.test.spawned", func() { panic("spawned boom") })

	// The recover lives in Go's wrapper, after fn returns, so there is nothing
	// inside fn that can signal completion; poll the log instead.
	deadline := time.Now().Add(2 * time.Second)
	var log string
	for time.Now().Before(deadline) {
		log = read(buf, mu)
		if strings.Contains(log, "safego.test.spawned") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if !strings.Contains(log, "safego.test.spawned") {
		t.Errorf("expected the spawned goroutine name in the log; got: %s", log)
	}
	if !strings.Contains(log, "spawned boom") {
		t.Errorf("expected the panic value in the log; got: %s", log)
	}
}
