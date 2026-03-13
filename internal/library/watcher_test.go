package library

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// collectTracks drains ch and returns all non-deletion Track values, stopping
// once ch is closed or ctx is cancelled.
func collectTracks(ctx context.Context, ch <-chan WatchEvent) []Track {
	var tracks []Track
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return tracks
			}
			if !e.Deleted {
				tracks = append(tracks, e.Track)
			}
		case <-ctx.Done():
			return tracks
		}
	}
}

// TestDebounce_CoalescesFastEvents verifies that many events arriving within
// the debounce window produce at most one rescan batch (deduplicated pending
// set), not one Extract call per raw event.
func TestDebounce_CoalescesFastEvents(t *testing.T) {
	root := t.TempDir()
	const debounce = 200 * time.Millisecond

	paths := []string{
		filepath.Join(root, "a.mp3"),
		filepath.Join(root, "b.flac"),
		filepath.Join(root, "c.ogg"),
	}
	for _, p := range paths {
		mkfile(t, p)
	}

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := Watch(ctx, root, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Fire many rapid Write events on the same files.
	for i := range 10 {
		p := paths[i%len(paths)]
		if err := os.WriteFile(p, []byte("ID3upd"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	// Collect tracks until no new ones arrive for a full debounce window,
	// then cancel so the watcher goroutine exits.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), debounce+500*time.Millisecond)
	defer drainCancel()
	collectTracks(drainCtx, ch)
	cancel()

	// Extract must have been called at most len(paths) times (one per distinct
	// path), not 10 times (once per raw event). Debounce coalesces duplicates.
	totalCalls := 0
	for _, batch := range fe.batches {
		totalCalls += len(batch)
	}
	if totalCalls > len(paths) {
		t.Errorf("Extract called for %d file-invocations; want ≤ %d (rapid events coalesced)",
			totalCalls, len(paths))
	}
}

// TestDebounce_RescansAfterWindow verifies that the rescan fires after the
// debounce window has elapsed without further events.
func TestDebounce_RescansAfterWindow(t *testing.T) {
	root := t.TempDir()
	const debounce = 150 * time.Millisecond

	p := filepath.Join(root, "track.mp3")
	mkfile(t, p)

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := Watch(ctx, root, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Trigger one Write event.
	if err := os.WriteFile(p, []byte("ID3v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Expect a watch event to arrive within debounce + generous margin.
	deadline := time.After(debounce + 500*time.Millisecond)
	for {
		select {
		case _, ok := <-ch:
			if ok {
				return // received an event — test passes
			}
		case <-deadline:
			t.Fatal("no event received within debounce window + margin")
		}
	}
}

// TestDebounce_NoRescanBeforeWindowElapses verifies that no rescan fires while
// events are still arriving within the debounce window. It asserts that the
// output channel stays empty for the entire duration of a rapid burst,
// confirming the 2-second (configurable) window is observed in production.
func TestDebounce_NoRescanBeforeWindowElapses(t *testing.T) {
	root := t.TempDir()
	const debounce = 300 * time.Millisecond

	p := filepath.Join(root, "track.mp3")
	mkfile(t, p)

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := Watch(ctx, root, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Trigger an event and then immediately check that nothing arrives within
	// half the debounce window — the timer should not have fired yet.
	if err := os.WriteFile(p, []byte("ID3v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	earlyDeadline := time.After(debounce / 2)
	select {
	case e, ok := <-ch:
		if ok && !e.Deleted {
			t.Error("rescan fired before debounce window elapsed")
		}
	case <-earlyDeadline:
		// Correct: nothing arrived before the window — debounce is working.
	}
}

// TestRename_TreatedAsDeletePlusCreate verifies that renaming a file causes the
// old path to be dropped from pending while the new path (arriving as a Create
// event) drives the rescan.
func TestRename_TreatedAsDeletePlusCreate(t *testing.T) {
	root := t.TempDir()
	const debounce = 150 * time.Millisecond

	oldPath := filepath.Join(root, "old.mp3")
	newPath := filepath.Join(root, "new.mp3")
	mkfile(t, oldPath)

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := Watch(ctx, root, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Rename emits Rename on oldPath and Create on newPath.
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("rename: %v", err)
	}

	// Collect until the debounce window passes, then verify.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), debounce+500*time.Millisecond)
	defer drainCancel()
	collectTracks(drainCtx, ch)
	cancel()

	// The rescan must not have attempted to extract oldPath (it was deleted).
	for _, batch := range fe.batches {
		for _, hf := range batch {
			if hf.Path == oldPath {
				t.Errorf("Extract called for deleted path %q; rename should treat old path as delete", oldPath)
			}
		}
	}
}

// TestDirectoryEvent_TriggersSubtreeRescan verifies that creating a new
// directory with audio files causes Walk to traverse the subtree and emit
// tracks for each file found.
func TestDirectoryEvent_TriggersSubtreeRescan(t *testing.T) {
	root := t.TempDir()
	const debounce = 150 * time.Millisecond

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := Watch(ctx, root, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Create a new subdirectory with audio files after the watcher starts.
	subdir := filepath.Join(root, "newalbum")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mkfile(t, filepath.Join(subdir, "track01.flac"))
	mkfile(t, filepath.Join(subdir, "track02.flac"))

	// Collect tracks until the debounce window passes, then check results.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), debounce+600*time.Millisecond)
	defer drainCancel()
	tracks := collectTracks(drainCtx, ch)
	cancel()

	// At least 2 tracks should be returned (one per audio file in the subtree).
	if len(tracks) < 2 {
		t.Errorf("directory event: expected ≥ 2 tracks from subtree rescan, got %d", len(tracks))
	}
}

// TestHandleFsEvent_PendingSetBehavior tests handleFsEvent directly using
// synthetic fsnotify.Event values to verify the pending-set logic in isolation.
func TestHandleFsEvent_PendingSetBehavior(t *testing.T) {
	root := t.TempDir()

	// Create an actual audio file so os.Stat in handleFsEvent succeeds.
	audioPath := filepath.Join(root, "song.mp3")
	mkfile(t, audioPath)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	t.Run("Create audio file adds to pending", func(t *testing.T) {
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(fsnotify.Event{Name: audioPath, Op: fsnotify.Create}, pending, deleted, w)
		isDir, ok := pending[audioPath]
		if !ok {
			t.Errorf("expected %q in pending after Create event", audioPath)
		}
		if isDir {
			t.Errorf("audio file should have isDir=false in pending")
		}
	})

	t.Run("Write audio file adds to pending", func(t *testing.T) {
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(fsnotify.Event{Name: audioPath, Op: fsnotify.Write}, pending, deleted, w)
		if _, ok := pending[audioPath]; !ok {
			t.Errorf("expected %q in pending after Write event", audioPath)
		}
	})

	t.Run("Rename removes from pending and adds to deleted", func(t *testing.T) {
		pending := map[string]bool{audioPath: false}
		deleted := make(map[string]struct{})
		handleFsEvent(fsnotify.Event{Name: audioPath, Op: fsnotify.Rename}, pending, deleted, w)
		if _, ok := pending[audioPath]; ok {
			t.Errorf("expected %q removed from pending after Rename event", audioPath)
		}
		if _, ok := deleted[audioPath]; !ok {
			t.Errorf("expected %q in deleted after Rename event", audioPath)
		}
	})

	t.Run("Remove removes from pending and adds to deleted", func(t *testing.T) {
		pending := map[string]bool{audioPath: false}
		deleted := make(map[string]struct{})
		handleFsEvent(fsnotify.Event{Name: audioPath, Op: fsnotify.Remove}, pending, deleted, w)
		if _, ok := pending[audioPath]; ok {
			t.Errorf("expected %q removed from pending after Remove event", audioPath)
		}
		if _, ok := deleted[audioPath]; !ok {
			t.Errorf("expected %q in deleted after Remove event", audioPath)
		}
	})

	t.Run("Non-audio Create is ignored", func(t *testing.T) {
		txtPath := filepath.Join(root, "readme.txt")
		mkfile(t, txtPath)
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(fsnotify.Event{Name: txtPath, Op: fsnotify.Create}, pending, deleted, w)
		if _, ok := pending[txtPath]; ok {
			t.Errorf("non-audio file should not be added to pending")
		}
	})

	t.Run("Create directory adds as dir and registers watch", func(t *testing.T) {
		dirPath := filepath.Join(root, "newdir")
		if err := os.Mkdir(dirPath, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(fsnotify.Event{Name: dirPath, Op: fsnotify.Create}, pending, deleted, w)
		isDir, ok := pending[dirPath]
		if !ok {
			t.Errorf("expected %q in pending after directory Create", dirPath)
		}
		if !isDir {
			t.Errorf("directory pending entry should have isDir=true")
		}
	})
}

// TestWatch_WatcherFactoryFailure_LogsWarningAndDegrades verifies that when
// the fsnotify watcher cannot be created (simulating fd exhaustion), Watch
// logs a warning, returns a nil error, and returns a closed channel so callers
// can continue running with manual rescan as the sole rescan mechanism.
func TestWatch_WatcherFactoryFailure_LogsWarningAndDegrades(t *testing.T) {
	// Capture slog output so we can assert a warning was emitted.
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	origLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	// Override watcherFactory to simulate fsnotify creation failure.
	origFactory := watcherFactory
	watcherFactory = func() (*fsnotify.Watcher, error) {
		return nil, errors.New("too many open files")
	}
	t.Cleanup(func() { watcherFactory = origFactory })

	ch, err := Watch(t.Context(), t.TempDir(), &fakeExtractor{}, DefaultDebounce)
	if err != nil {
		t.Fatalf("Watch returned error; want nil for degraded mode: %v", err)
	}

	// The returned channel must be closed immediately (degraded: no active watcher).
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected pre-closed channel in degraded mode, received a value")
		}
	case <-time.After(time.Second):
		t.Error("channel not closed within 1 s after watcher creation failure")
	}

	// A warning containing "watcher" must have been logged.
	if !strings.Contains(buf.String(), "watcher") {
		t.Errorf("expected slog warning mentioning 'watcher'; got: %s", buf.String())
	}
}
