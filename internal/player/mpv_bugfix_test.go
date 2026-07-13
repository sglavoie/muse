package player

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

// TestRecoverOutage_FirstAttemptOfEachOutageIsImmediate verifies that the very
// first launch-and-connect attempt of an outage never incurs a backoff delay,
// no matter how many outages preceded it. Backoff applies only between
// successive attempts within a single outage. Because recoverOutage owns a
// fresh attempt counter per call, a prior outage's failures cannot bleed a
// spurious delay into the next outage's first attempt (the regression this
// guards). initialDelay is set high so a broken first-attempt path would blow
// past the asserted upper bound.
func TestRecoverOutage_FirstAttemptOfEachOutageIsImmediate(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)

	launchCh := make(chan struct{}, 16)
	const initialDelay = 500 * time.Millisecond

	m := &Manager{
		socketPath:                 shortSockPath(t, "restart-first-immediate"),
		rootCtx:                    rootCtx,
		rootCancel:                 rootCancel,
		ctx:                        ctx,
		cancel:                     cancel,
		disconnectCh:               make(chan error, 4),
		callbacksReady:             make(chan struct{}),
		restartBaseDelay:           initialDelay,
		restartMaxAttemptsOverride: 1, // one attempt per outage, then give up
		cmdQueue:                   []func() error{},
	}
	defer m.Close()
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(string) {}})

	m.launchFn = func(_ context.Context, _ string) (*exec.Cmd, error) {
		select {
		case launchCh <- struct{}{}:
		default:
		}
		return nil, fmt.Errorf("simulated launch failure")
	}

	for outage := 0; outage < 2; outage++ {
		start := time.Now()
		// maxAttempts=1 → a single immediate attempt, then give up.
		m.recoverOutage()

		select {
		case <-launchCh:
		case <-time.After(2 * time.Second):
			t.Fatalf("outage %d: launch was not observed within 2s", outage)
		}

		if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
			t.Errorf("outage %d: first attempt took %v; expected immediate (< 250ms, base=%v)", outage, elapsed, initialDelay)
		}

		// recoverOutage nils the queue on give-up; restore it so the next
		// outage starts from the same post-disconnect state.
		m.mu.Lock()
		m.cmdQueue = []func() error{}
		m.mu.Unlock()
	}
}
