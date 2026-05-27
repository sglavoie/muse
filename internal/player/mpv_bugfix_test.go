package player

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

// TestWatchAndRestart_ResetsFailureCounterBeforeBackoff verifies that a
// successful reconnect (which sets backoffReset) clears the consecutive-
// failure counter BEFORE the next disconnect's backoff is computed. Before
// the fix, the counter was only reset after handleRestart returned, so the
// first disconnect after a successful reconnect would incorrectly trigger
// an exponential-backoff delay.
func TestWatchAndRestart_ResetsFailureCounterBeforeBackoff(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)

	launchCh := make(chan struct{}, 16)

	// initialDelay is set very high so that if the reset is broken the test
	// would take orders of magnitude longer than the asserted upper bound.
	const initialDelay = 500 * time.Millisecond

	m := &Manager{
		socketPath:       shortSockPath(t, "restart-reset-before"),
		rootCtx:          rootCtx,
		rootCancel:       rootCancel,
		ctx:              ctx,
		cancel:           cancel,
		disconnectCh:     make(chan error, 4),
		callbacksReady:   make(chan struct{}),
		restartBaseDelay: initialDelay,
		cmdQueue:         []func() error{},
	}
	defer m.Close()

	m.launchFn = func(_ context.Context, _ string) (*exec.Cmd, error) {
		select {
		case launchCh <- struct{}{}:
		default:
		}
		return nil, fmt.Errorf("simulated launch failure")
	}

	done := make(chan struct{})
	go func() {
		m.watchAndRestart()
		close(done)
	}()
	defer func() {
		rootCancel()
		<-done
	}()

	// Simulate the first disconnect → handleRestart runs, fails to launch,
	// returns. consecutiveFailures = 1, no backoff applied yet.
	m.disconnectCh <- fmt.Errorf("disconnect 1")

	// Wait for the first launch attempt to be observed.
	select {
	case <-launchCh:
	case <-time.After(2 * time.Second):
		t.Fatal("first launch was not observed within 2s")
	}

	// Now simulate that pollUntilReady (asynchronously) succeeded by setting
	// backoffReset to true.
	m.backoffReset.Store(true)

	// Send the next disconnect. With the fix, the Swap-before-increment
	// clears the counter so this counts as failure 1 and there is NO delay
	// before handleRestart runs again. Without the fix, this would count as
	// failure 2 and a 500ms delay would be applied before handleRestart.
	start := time.Now()
	m.disconnectCh <- fmt.Errorf("disconnect 2")

	select {
	case <-launchCh:
	case <-time.After(2 * time.Second):
		t.Fatal("second launch was not observed within 2s")
	}
	elapsed := time.Since(start)

	// Allow generous slack for scheduling jitter but stay well below the
	// 500ms backoff that the unfixed code would apply.
	if elapsed > 250*time.Millisecond {
		t.Errorf("second launch took %v; expected < 250ms (the backoff should have been skipped after backoffReset)", elapsed)
	}
}
