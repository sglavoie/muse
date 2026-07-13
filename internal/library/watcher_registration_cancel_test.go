package library

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWatch_RegistrationAbortsOnCancel proves the recursive watch registration
// observes context cancellation: when ctx is cancelled while the tree walk is
// in flight, Watch returns promptly after visiting only the gated directory
// instead of walking the whole tree. This is the exact property that keeps the
// synchronous library.Watch call in runLibraryPipeline (and therefore
// StopAndWait) from blocking until a large tree finishes registering.
func TestWatch_RegistrationAbortsOnCancel(t *testing.T) {
	root := t.TempDir()
	// Create many sibling directories so a full walk would visit far more than
	// the single gated step; aborting early must leave most unvisited.
	const numDirs = 40
	for i := range numDirs {
		if err := os.Mkdir(filepath.Join(root, "d"+strconv.Itoa(i)), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var steps atomic.Int32
	firstStep := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	RegistrationStepHook = func(string) {
		steps.Add(1)
		once.Do(func() {
			// Signal the walk is in flight, then block so the test can cancel
			// before registration proceeds to the next directory.
			close(firstStep)
			<-release
		})
	}
	t.Cleanup(func() { RegistrationStepHook = nil })

	type watchResult struct {
		ch       <-chan WatchEvent
		degraded bool
		err      error
	}
	done := make(chan watchResult, 1)
	go func() {
		ch, degraded, err := Watch(ctx, []string{root}, nil, &fakeExtractor{}, DefaultDebounce)
		done <- watchResult{ch, degraded, err}
	}()

	// Wait until registration is blocked mid-walk, cancel, then release the gate
	// so the walk can observe the cancellation on its next step.
	select {
	case <-firstStep:
	case <-time.After(5 * time.Second):
		t.Fatal("registration never reached the gated step")
	}
	cancel()
	close(release)

	select {
	case res := <-done:
		if !res.degraded {
			t.Fatal("cancelled registration: expected degraded=true")
		}
		if res.err != nil {
			t.Fatalf("cancelled registration: expected nil error, got %v", res.err)
		}
		// The returned channel must be the pre-closed (inactive) channel.
		if _, ok := <-res.ch; ok {
			t.Fatal("cancelled registration: expected pre-closed event channel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not return promptly after cancellation; " +
			"registration is still walking the full tree")
	}

	// The walk must have aborted almost immediately: only the gated directory
	// (and at most a couple of races) may have been visited, never all of them.
	if n := steps.Load(); int(n) >= numDirs {
		t.Fatalf("registration walked %d/%d directories after cancellation; "+
			"cancellation was not observed", n, numDirs)
	}
}
