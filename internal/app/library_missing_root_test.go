package app_test

import (
	"context"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/library"
)

// recordingWriter counts MarkTrackMissing calls so a test can prove that a
// music directory which cannot be read never causes the cached library to be
// marked missing wholesale.
type recordingWriter struct {
	mu      sync.Mutex
	missing []string
}

func (w *recordingWriter) UpsertBatch([]library.Track) error { return nil }

func (w *recordingWriter) MarkTrackMissing(hash string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.missing = append(w.missing, hash)
	return nil
}

func (w *recordingWriter) missingCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.missing)
}

func (w *recordingWriter) missingHashes() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.missing)
}

// waitForWatchStatus reads the single watcher-availability message, failing the
// test if the pipeline never gets that far.
func waitForWatchStatus(t *testing.T, lr app.LibraryResult) library.WatchStatusMsg {
	t.Helper()
	select {
	case msg := <-lr.TuiWatchStatusCh:
		return msg
	case <-time.After(10 * time.Second):
		t.Fatal("watcher status never reported")
		return library.WatchStatusMsg{}
	}
}

// A configured music directory that does not exist must not stall or fail the
// library pipeline: the watcher degrades, the diff walk yields nothing, and the
// cached tracks stay untouched.
func TestLoadLibrary_NonexistentMusicDirIsNonFatal(t *testing.T) {
	dirs := setupDirs(t)
	missing := filepath.Join(t.TempDir(), "no-such-music-dir")

	result, err := app.Run(t.Context(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer result.DB.Close()

	// Seed a cached track that lives under the missing root; it must not be
	// marked missing just because the root could not be read.
	if err := result.Writer.UpsertBatch([]library.Track{{
		Hash: "cached-hash", Path: filepath.Join(missing, "a.mp3"), Title: "a", Artist: "x",
	}}); err != nil {
		t.Fatalf("seed UpsertBatch: %v", err)
	}

	writer := &recordingWriter{}
	ctx, cancel := context.WithCancel(t.Context())
	libResult, err := app.LoadLibrary(ctx, result.DB, writer, []string{missing}, nil, stubExtractor{}, 100)
	if err != nil {
		cancel()
		t.Fatalf("LoadLibrary: %v", err)
	}

	if len(libResult.Tracks) != 1 {
		t.Errorf("cached tracks = %d, want 1", len(libResult.Tracks))
	}
	if status := waitForWatchStatus(t, libResult); status.Available {
		t.Error("watcher reported available for a nonexistent root")
	}
	// Scan channel closes when the walk finishes; it must not hang or panic.
	for range libResult.TuiScanCh {
		t.Error("unexpected scanned track from a nonexistent root")
	}
	libResult.StopAndWait()
	cancel()
	for range libResult.TuiWatchCh {
	}

	if n := writer.missingCount(); n != 0 {
		t.Errorf("MarkTrackMissing called %d times, want 0 for an unreadable root", n)
	}
}

// With one unreadable and one readable music directory, deletion
// reconciliation must keep running for the readable one: only the track that
// genuinely vanished from the root that was walked is marked missing, while the
// track under the unmounted root is left alone.
func TestLoadLibrary_MixedRootsMarksOnlyReadableDeletion(t *testing.T) {
	dirs := setupDirs(t)
	unreadable := filepath.Join(t.TempDir(), "unmounted-music-dir")
	readable := t.TempDir()

	result, err := app.Run(t.Context(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer result.DB.Close()

	// Both tracks are cached and neither exists on disk. Only the one under the
	// readable root was actually observed to be absent.
	protectedPath := filepath.Join(unreadable, "protected.mp3")
	deletedPath := filepath.Join(readable, "deleted.mp3")
	if err := result.Writer.UpsertBatch([]library.Track{
		{Hash: "protected-hash", Path: protectedPath, Title: "protected", Artist: "x"},
		{Hash: "deleted-hash", Path: deletedPath, Title: "deleted", Artist: "x"},
	}); err != nil {
		t.Fatalf("seed UpsertBatch: %v", err)
	}

	writer := &recordingWriter{}
	ctx, cancel := context.WithCancel(t.Context())
	libResult, err := app.LoadLibrary(ctx, result.DB, writer,
		[]string{unreadable, readable}, nil, stubExtractor{}, 100)
	if err != nil {
		cancel()
		t.Fatalf("LoadLibrary: %v", err)
	}

	if len(libResult.Tracks) != 2 {
		t.Errorf("cached tracks = %d, want 2", len(libResult.Tracks))
	}
	waitForWatchStatus(t, libResult)

	select {
	case ev := <-libResult.TuiWatchCh:
		if !ev.Deleted || ev.Path != deletedPath {
			t.Errorf("watch event = %+v, want a deletion of %s", ev, deletedPath)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("deletion under the readable root was never reported")
	}

	for range libResult.TuiScanCh {
		t.Error("unexpected scanned track: neither root holds an audio file")
	}
	drainChannels(cancel, libResult)
	libResult.StopAndWait()

	if got := writer.missingHashes(); !slices.Equal(got, []string{"deleted-hash"}) {
		t.Errorf("MarkTrackMissing hashes = %v, want exactly [deleted-hash]", got)
	}
}

// Zero configured roots is the shape a fresh config takes once the music
// directory list is emptied; the pipeline must still start and stop cleanly.
func TestLoadLibrary_NoMusicDirsIsNonFatal(t *testing.T) {
	dirs := setupDirs(t)

	result, err := app.Run(t.Context(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer result.DB.Close()

	ctx, cancel := context.WithCancel(t.Context())
	libResult, err := app.LoadLibrary(ctx, result.DB, &recordingWriter{}, nil, nil, stubExtractor{}, 100)
	if err != nil {
		cancel()
		t.Fatalf("LoadLibrary: %v", err)
	}
	waitForWatchStatus(t, libResult)
	for range libResult.TuiScanCh {
		t.Error("unexpected scanned track with no roots")
	}
	drainChannels(cancel, libResult)
}
