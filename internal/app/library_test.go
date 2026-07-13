package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// stubExtractor returns a Track per HashedFile with fixed metadata.
type stubExtractor struct{}

func (stubExtractor) Extract(_ context.Context, files []library.HashedFile) ([]library.Track, error) {
	var out []library.Track
	for _, f := range files {
		out = append(out, library.Track{
			Hash:   f.Hash,
			Path:   f.Path,
			Title:  filepath.Base(f.Path),
			Artist: "test-artist",
		})
	}
	return out, nil
}

type noopLibraryWriter struct{}

func (noopLibraryWriter) UpsertBatch([]library.Track) error { return nil }

func (noopLibraryWriter) MarkTrackMissing(string) error { return nil }

// drainChannels cancels the context and drains the scan/watch channels to
// avoid goroutine leaks from the watcher and bridge goroutines.
func drainChannels(cancel context.CancelFunc, lr app.LibraryResult) {
	cancel()
	for range lr.TuiScanCh {
	}
	for range lr.TuiWatchCh {
	}
}

func TestLoadLibrary_EmptyDB(t *testing.T) {
	dirs := setupDirs(t)
	musicDir := t.TempDir()

	deps := noopDeps()
	result, err := app.Run(t.Context(), dirs, deps)
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
	defer drainChannels(cancel, libResult)

	if len(libResult.Tracks) != 0 {
		t.Errorf("expected 0 tracks, got %d", len(libResult.Tracks))
	}
}

func TestLoadLibrary_ReadTracksErrorReturnsError(t *testing.T) {
	dirs := setupDirs(t)
	musicDir := t.TempDir()

	database := openTestDB(t, dirs)
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_, err := app.LoadLibrary(ctx, database, noopLibraryWriter{}, []string{musicDir}, nil, stubExtractor{}, 100)
	if err == nil {
		t.Fatal("LoadLibrary returned nil error after ReadTracks failed")
	}
	if !strings.Contains(err.Error(), "read tracks from DB") {
		t.Fatalf("LoadLibrary error = %v, want read tracks context", err)
	}
}

func TestLoadLibrary_NilWriterReturnsError(t *testing.T) {
	dirs := setupDirs(t)
	musicDir := t.TempDir()

	result, err := app.Run(t.Context(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer closeRunResult(t, result)

	var writer *db.Writer
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_, err = app.LoadLibrary(ctx, result.DB, writer, []string{musicDir}, nil, stubExtractor{}, 100)
	if err == nil {
		t.Fatal("LoadLibrary returned nil error with nil writer")
	}
	if !strings.Contains(err.Error(), "library writer is required") {
		t.Fatalf("LoadLibrary error = %v, want library writer validation", err)
	}
}

func TestLoadLibrary_WithExistingTracks(t *testing.T) {
	dirs := setupDirs(t)
	musicDir := t.TempDir()

	// Create a music file on disk so it is not reported as a deletion event.
	trackPath := filepath.Join(musicDir, "song.mp3")
	if err := os.WriteFile(trackPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write track: %v", err)
	}

	deps := noopDeps()
	result, err := app.Run(t.Context(), dirs, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer result.DB.Close()

	// Insert a track into the DB to simulate a cached library.
	insertTrack(t, result.DB, "hash1", trackPath, "Artist", "Title")

	ctx, cancel := context.WithCancel(t.Context())
	libResult, err := app.LoadLibrary(ctx, result.DB, result.Writer, []string{musicDir}, nil, stubExtractor{}, 100)
	if err != nil {
		cancel()
		t.Fatalf("LoadLibrary: %v", err)
	}
	defer drainChannels(cancel, libResult)

	if len(libResult.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(libResult.Tracks))
	}
	if libResult.Tracks[0].Hash != "hash1" {
		t.Errorf("track hash = %q, want %q", libResult.Tracks[0].Hash, "hash1")
	}
}

func TestLoadLibrary_DeletedPathsDetected(t *testing.T) {
	dirs := setupDirs(t)
	musicDir := t.TempDir()

	deps := noopDeps()
	result, err := app.Run(t.Context(), dirs, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer result.DB.Close()

	// Insert a track for a file that does NOT exist on disk.
	missingPath := filepath.Join(musicDir, "gone.mp3")
	insertTrack(t, result.DB, "hash-gone", missingPath, "A", "T")

	ctx, cancel := context.WithCancel(t.Context())
	libResult, err := app.LoadLibrary(ctx, result.DB, result.Writer, []string{musicDir}, nil, stubExtractor{}, 100)
	if err != nil {
		cancel()
		t.Fatalf("LoadLibrary: %v", err)
	}

	// The track should appear in Tracks (it's from the DB cache).
	if len(libResult.Tracks) != 1 {
		cancel()
		t.Fatalf("expected 1 track, got %d", len(libResult.Tracks))
	}

	// The missing file now flows asynchronously through the watch
	// reconciliation stream as a deletion event rather than a synchronous
	// DeletedPaths slice.
	found := false
	deadline := time.After(10 * time.Second)
watch:
	for {
		select {
		case ev, ok := <-libResult.TuiWatchCh:
			if !ok {
				break watch
			}
			if ev.Deleted && ev.Path == missingPath {
				found = true
				break watch
			}
		case <-deadline:
			break watch
		}
	}

	cancel()
	for range libResult.TuiScanCh {
	}
	for range libResult.TuiWatchCh {
	}

	if !found {
		t.Errorf("expected deletion event for %q on the watch channel", missingPath)
	}
}
