package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/library"
)

// TestLoadLibrary_StopAndWaitDoesNotWaitForWatchRegistration proves FR-007:
// when the pipeline context is cancelled (as it is on shutdown) while recursive
// watcher registration is still walking the music tree, StopAndWait returns
// promptly without waiting for the full tree walk to finish. runLibraryPipeline
// calls library.Watch synchronously, so before the fix a cancellation during
// registration would block StopAndWait until the entire tree was registered.
func TestLoadLibrary_StopAndWaitDoesNotWaitForWatchRegistration(t *testing.T) {
	dirs := setupDirs(t)
	musicDir := t.TempDir()

	// Many sibling directories so a full registration walk would visit far more
	// than the single gated step.
	const numDirs = 40
	for i := range numDirs {
		if err := os.Mkdir(filepath.Join(musicDir, "d"+strconv.Itoa(i)), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	result, err := app.Run(t.Context(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer result.DB.Close()

	// Gate registration mid-walk: the first visited directory blocks until the
	// test releases it, so the pipeline context can be cancelled while
	// registration is in flight.
	var steps atomic.Int32
	firstStep := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	library.RegistrationStepHook = func(string) {
		steps.Add(1)
		once.Do(func() {
			close(firstStep)
			<-release
		})
	}
	t.Cleanup(func() { library.RegistrationStepHook = nil })

	// Own the context so cancellation is deterministic: cancelling this parent
	// synchronously cancels the pipeline's child context before the gate is
	// released, guaranteeing the walk observes the cancellation on its next step.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	libResult, err := app.LoadLibrary(ctx, result.DB, result.Writer,
		[]string{musicDir}, nil, stubExtractor{}, 100)
	if err != nil {
		t.Fatalf("LoadLibrary: %v", err)
	}

	// Wait until registration is blocked mid-walk, cancel the pipeline, then
	// release the gate so the walk can abort on its next step.
	select {
	case <-firstStep:
	case <-time.After(5 * time.Second):
		t.Fatal("watch registration never reached the gated step")
	}
	cancel()
	close(release)

	// StopAndWait must return promptly: the pipeline is tearing down rather than
	// finishing the full registration walk.
	stopped := make(chan struct{})
	go func() {
		libResult.StopAndWait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("StopAndWait blocked on watch registration instead of returning promptly")
	}

	if n := steps.Load(); int(n) >= numDirs {
		t.Fatalf("registration walked %d/%d directories after cancellation; "+
			"cancellation was not observed", n, numDirs)
	}

	assertChannelsDrained(t, libResult)
}
