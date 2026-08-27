package app_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui"
)

func closeRunResult(t *testing.T, result *app.RunResult) {
	t.Helper()
	result.CancelWriter()
	result.Writer.Wait()
	result.DB.Close()
	result.Lockfile.Release()
}

// TestRunRestorePlayerCalled verifies that deps.RestorePlayer is invoked with
// the correct path and position when a session with a current_hash is present.
func TestRunRestorePlayerCalled(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	database := openTestDB(t, dirs)
	audioPath := filepath.Join(dirs.Runtime, "session.mp3")
	if err := os.WriteFile(audioPath, []byte("fake audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-restore", audioPath, "Restore", "Artist")
	insertQueueEntry(t, database, "hash-restore", 1000)
	setSessionRow(t, database, "hash-restore", 42000, false, "none")
	database.Close()

	var (
		mu                  sync.Mutex
		restorePlayerCalled bool
		restorePath         string
		restorePositionMs   int64
	)

	deps := app.Deps{
		CheckPrereqs: func(context.Context) ([]string, error) { return nil, nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
		RestorePlayer: func(path string, positionMs int64) error {
			mu.Lock()
			restorePlayerCalled = true
			restorePath = path
			restorePositionMs = positionMs
			mu.Unlock()
			return nil
		},
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	closeRunResult(t, result)

	mu.Lock()
	called := restorePlayerCalled
	gotPath := restorePath
	gotPos := restorePositionMs
	mu.Unlock()

	if !called {
		t.Error("RestorePlayer was not called")
	}
	if gotPath != audioPath {
		t.Errorf("RestorePlayer path: want %q, got %q", audioPath, gotPath)
	}
	if gotPos != 42000 {
		t.Errorf("RestorePlayer positionMs: want 42000, got %d", gotPos)
	}
}

// TestRunRestorePlayerNotCalledWithoutCurrentHash verifies that RestorePlayer
// is not called when the session has no current_hash.
func TestRunRestorePlayerNotCalledWithoutCurrentHash(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	database := openTestDB(t, dirs)
	audioPath := filepath.Join(dirs.Runtime, "nopos.mp3")
	if err := os.WriteFile(audioPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-nopos", audioPath, "", "")
	insertQueueEntry(t, database, "hash-nopos", 1000)
	database.Close()

	restorePlayerCalled := false
	deps := app.Deps{
		CheckPrereqs: func(context.Context) ([]string, error) { return nil, nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
		RestorePlayer: func(_ string, _ int64) error {
			restorePlayerCalled = true
			return nil
		},
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	closeRunResult(t, result)

	if restorePlayerCalled {
		t.Error("RestorePlayer must not be called when session has no current_hash")
	}
}

// TestRunRestorePlayerNilDepSafe verifies that Run succeeds even when
// RestorePlayer is nil and a session with a current_hash is present.
func TestRunRestorePlayerNilDepSafe(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	database := openTestDB(t, dirs)
	audioPath := filepath.Join(dirs.Runtime, "safe.mp3")
	if err := os.WriteFile(audioPath, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-safe", audioPath, "", "")
	insertQueueEntry(t, database, "hash-safe", 1000)
	setSessionRow(t, database, "hash-safe", 1000, false, "none")
	database.Close()

	deps := app.Deps{
		CheckPrereqs:  func(context.Context) ([]string, error) { return nil, nil },
		LaunchMPV:     func(_ context.Context, _ string) error { return nil },
		RunScan:       func(_ context.Context, _ *db.DB) error { return nil },
		RestorePlayer: nil,
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run with nil RestorePlayer: %v", err)
	}
	closeRunResult(t, result)
}

// TestRunRestoredSessionInResult verifies that RunResult.Restored contains
// the queue entries loaded from the database.
func TestRunRestoredSessionInResult(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	database := openTestDB(t, dirs)
	audioPath := filepath.Join(dirs.Runtime, "result.mp3")
	if err := os.WriteFile(audioPath, []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-result", audioPath, "Res", "Ult")
	insertQueueEntry(t, database, "hash-result", 1000)
	setSessionRow(t, database, "hash-result", 5000, true, "all")
	database.Close()

	result, err := app.Run(context.Background(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	closeRunResult(t, result)

	r := result.Restored
	if r == nil {
		t.Fatal("RunResult.Restored is nil")
	}
	if len(r.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(r.QueueEntries))
	}
	if r.QueueEntries[0].Missing {
		t.Error("QueueEntry.Missing: want false for readable file")
	}
	if r.RepeatMode != tui.RepeatAll {
		t.Errorf("RepeatMode: want RepeatAll, got %v", r.RepeatMode)
	}
	if !r.Shuffle {
		t.Error("Shuffle: want true")
	}
	if r.CurrentHash != "hash-result" {
		t.Errorf("CurrentHash: want %q, got %q", "hash-result", r.CurrentHash)
	}
	if r.PositionMs != 5000 {
		t.Errorf("PositionMs: want 5000, got %d", r.PositionMs)
	}
}
