package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/library"
)

// gatedExtractor blocks in Extract until release is closed or ctx is cancelled.
// It signals called (once) when Extract is first reached so a test can observe
// that the background walk actually invoked extraction.
type gatedExtractor struct {
	release chan struct{}
	called  chan struct{}
}

func newGatedExtractor() *gatedExtractor {
	return &gatedExtractor{
		release: make(chan struct{}),
		called:  make(chan struct{}, 1),
	}
}

func (g *gatedExtractor) Extract(ctx context.Context, files []library.HashedFile) ([]library.Track, error) {
	select {
	case g.called <- struct{}{}:
	default:
	}
	select {
	case <-g.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	out := make([]library.Track, 0, len(files))
	for _, f := range files {
		out = append(out, library.Track{Hash: f.Hash, Path: f.Path, Title: filepath.Base(f.Path)})
	}
	return out, nil
}

// TestLoadLibrary_ReturnsCachedDataBeforeSlowWalk verifies that LoadLibrary
// returns the cached tracks synchronously without waiting for the background
// diff walk / metadata extraction, which is gated open here.
func TestLoadLibrary_ReturnsCachedDataBeforeSlowWalk(t *testing.T) {
	dirs := setupDirs(t)
	musicDir := t.TempDir()

	// A new file on disk so the diff walk finds work and reaches the extractor.
	newPath := filepath.Join(musicDir, "new.mp3")
	if err := os.WriteFile(newPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write new track: %v", err)
	}

	result, err := app.Run(t.Context(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer result.DB.Close()

	// Seed a cached track so LoadLibrary has data to return immediately.
	insertTrack(t, result.DB, "cached", filepath.Join(musicDir, "cached.mp3"), "A", "T")

	gate := newGatedExtractor()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	returned := make(chan app.LibraryResult, 1)
	go func() {
		lr, loadErr := app.LoadLibrary(ctx, result.DB, result.Writer, []string{musicDir}, nil, gate, 100)
		if loadErr != nil {
			t.Errorf("LoadLibrary: %v", loadErr)
		}
		returned <- lr
	}()

	var libResult app.LibraryResult
	select {
	case libResult = <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("LoadLibrary did not return while the diff walk was blocked in extraction")
	}

	if len(libResult.Tracks) != 1 || libResult.Tracks[0].Hash != "cached" {
		t.Fatalf("expected cached track returned synchronously, got %+v", libResult.Tracks)
	}

	// The background walk must have reached the (still-blocked) extractor,
	// proving traversal runs asynchronously rather than before the return.
	select {
	case <-gate.called:
	case <-time.After(5 * time.Second):
		t.Fatal("background diff walk never reached the extractor")
	}

	// StopAndWait must cleanly stop the pipeline even while extraction is
	// blocked: cancellation unblocks the extractor via ctx.
	done := make(chan struct{})
	go func() {
		libResult.StopAndWait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("StopAndWait did not return after cancelling a blocked pipeline")
	}

	assertChannelsDrained(t, libResult)
}

// TestLoadLibrary_ReportsWatchStatus verifies the watcher availability is
// reported asynchronously on TuiWatchStatusCh exactly once.
func TestLoadLibrary_ReportsWatchStatus(t *testing.T) {
	dirs := setupDirs(t)
	musicDir := t.TempDir()

	result, err := app.Run(t.Context(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer result.DB.Close()

	ctx, cancel := context.WithCancel(t.Context())
	libResult, err := app.LoadLibrary(ctx, result.DB, result.Writer, []string{musicDir}, nil, stubExtractor{}, 100)
	if err != nil {
		cancel()
		t.Fatalf("LoadLibrary: %v", err)
	}
	defer func() {
		cancel()
		libResult.StopAndWait()
	}()

	select {
	case _, ok := <-libResult.TuiWatchStatusCh:
		if !ok {
			t.Fatal("watch status channel closed without a status message")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no watch status reported")
	}

	// The channel emits exactly one message and is then closed.
	select {
	case _, ok := <-libResult.TuiWatchStatusCh:
		if ok {
			t.Fatal("expected watch status channel to close after one message")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch status channel not closed after its single message")
	}
}

// TestLoadLibrary_StopAndWaitIdempotent verifies StopAndWait can be called
// repeatedly and concurrently without panicking or deadlocking.
func TestLoadLibrary_StopAndWaitIdempotent(t *testing.T) {
	dirs := setupDirs(t)
	musicDir := t.TempDir()

	result, err := app.Run(t.Context(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer result.DB.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	libResult, err := app.LoadLibrary(ctx, result.DB, result.Writer, []string{musicDir}, nil, stubExtractor{}, 100)
	if err != nil {
		t.Fatalf("LoadLibrary: %v", err)
	}

	const callers = 8
	done := make(chan struct{}, callers)
	for range callers {
		go func() {
			libResult.StopAndWait()
			done <- struct{}{}
		}()
	}
	for range callers {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent StopAndWait deadlocked")
		}
	}

	// A subsequent call is still a no-op.
	libResult.StopAndWait()

	assertChannelsDrained(t, libResult)
}

// assertChannelsDrained confirms every pipeline output channel is closed after
// the pipeline has stopped, proving no relay or bridge goroutine remains.
func assertChannelsDrained(t *testing.T, lr app.LibraryResult) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	scanClosed, watchClosed := false, false
	errClosed, statusClosed := false, false
	for !(scanClosed && watchClosed && errClosed && statusClosed) {
		select {
		case _, ok := <-lr.TuiScanCh:
			if !ok {
				scanClosed = true
				lr.TuiScanCh = nil
			}
		case _, ok := <-lr.TuiWatchCh:
			if !ok {
				watchClosed = true
				lr.TuiWatchCh = nil
			}
		case _, ok := <-lr.TuiScanErrCh:
			if !ok {
				errClosed = true
				lr.TuiScanErrCh = nil
			}
		case _, ok := <-lr.TuiWatchStatusCh:
			if !ok {
				statusClosed = true
				lr.TuiWatchStatusCh = nil
			}
		case <-deadline:
			t.Fatalf("channels not drained after stop: scan=%v watch=%v err=%v status=%v",
				scanClosed, watchClosed, errClosed, statusClosed)
		}
	}
}
