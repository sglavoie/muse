package main

import (
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestCommandListener_TransientAcceptError(t *testing.T) {
	sockPath := filepath.Join(shortTempDir(t), "test.sock")
	realLn, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer realLn.Close()

	received := make(chan playback.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	fake := &fakeListener{real: realLn, errCount: 2}
	go acceptLoop(p, fake)

	conn, err := net.DialTimeout("unix", sockPath, time.Second)
	if err != nil {
		t.Fatal("accept loop stopped after transient errors:", err)
	}
	_, _ = conn.Write([]byte("next\n"))
	conn.Close()

	select {
	case msg := <-received:
		if msg.ActionName != "next" {
			t.Errorf("got %q, want %q", msg.ActionName, "next")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out: accept loop did not survive transient errors")
	}
}

func TestAcceptLoop_CircuitBreaker(t *testing.T) {
	sockPath := filepath.Join(shortTempDir(t), "circuit.sock")
	realLn, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	addr := realLn.Addr()
	realLn.Close()

	received := make(chan playback.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	done := make(chan struct{})
	go func() {
		acceptLoopWithBackoff(p, &alwaysErrListener{addr: addr}, 0)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("acceptLoop did not exit after reaching the circuit-breaker limit")
	}
}

// TestAcceptLoop_CircuitBreakerClosesListener verifies that when the circuit
// breaker trips, the listener is closed so the Unix socket file is removed and
// subsequent dials fail fast instead of hanging against a dead listener.
func TestAcceptLoop_CircuitBreakerClosesListener(t *testing.T) {
	sockPath := filepath.Join(shortTempDir(t), "cb-close.sock")
	realLn, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	ln := &unixSocketListener{Listener: realLn, path: sockPath}

	received := make(chan playback.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	tracking := &alwaysErrListenerWithClose{inner: ln}

	done := make(chan struct{})
	go func() {
		acceptLoopWithBackoff(p, tracking, 0)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("acceptLoop did not exit after circuit breaker")
	}

	if atomic.LoadInt32(&tracking.closeCalls) == 0 {
		t.Error("expected listener.Close() to be invoked when the circuit breaker trips")
	}
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Errorf("socket file should be removed after circuit breaker; got stat err=%v", err)
	}
}
