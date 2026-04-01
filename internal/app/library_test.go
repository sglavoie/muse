package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sglavoie/muse/internal/app"
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
	if len(libResult.DeletedPaths) != 0 {
		t.Errorf("expected 0 deleted paths, got %d", len(libResult.DeletedPaths))
	}
	if len(libResult.HashByPath) != 0 {
		t.Errorf("expected empty HashByPath, got %d entries", len(libResult.HashByPath))
	}
}

func TestLoadLibrary_WithExistingTracks(t *testing.T) {
	dirs := setupDirs(t)
	musicDir := t.TempDir()

	// Create a music file on disk so it doesn't appear in deletedPaths.
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

	if got, ok := libResult.HashByPath[trackPath]; !ok || got != "hash1" {
		t.Errorf("HashByPath[%q] = %q, want %q", trackPath, got, "hash1")
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
	defer drainChannels(cancel, libResult)

	// The track should appear in Tracks (it's from the DB cache).
	if len(libResult.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(libResult.Tracks))
	}

	// The missing file should be in DeletedPaths.
	found := false
	for _, p := range libResult.DeletedPaths {
		if p == missingPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in DeletedPaths, got %v", missingPath, libResult.DeletedPaths)
	}
}
