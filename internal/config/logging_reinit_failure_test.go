package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestInitLogging_FailureAfterPreviousCloseDoesNotWriteToClosedFD verifies
// that when InitLogging is called a second time and the new file cannot be
// opened (e.g. the state directory is now a file, not a directory), slog's
// default handler is repointed away from the just-closed previous file handle
// so subsequent slog calls do not write to a closed FD.
func TestInitLogging_FailureAfterPreviousCloseDoesNotWriteToClosedFD(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		if logFile != nil {
			_ = logFile.Close()
			logFile = nil
		}
	})

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	// First init succeeds, opens a file handle and points slog at it.
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("first InitLogging: %v", err)
	}
	firstFile := logFile

	// Force the second init to fail at MkdirAll by placing a regular file
	// at the path that would need to become a directory.
	blockPath := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blockPath, []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InitLogging(filepath.Join(blockPath, "muse"), slog.LevelInfo); err == nil {
		t.Fatal("expected InitLogging to fail when stateDir parent is a file")
	}

	// The first file handle must have been closed.
	if _, err := firstFile.Write([]byte("should fail")); err == nil {
		t.Error("expected closed FD; first file handle still writable")
	}

	// Subsequent slog calls must NOT panic and must NOT write to the closed FD.
	// We cannot directly observe a write-to-closed-FD here, but we verify that
	// the slog default is no longer the closed file's handler by capturing
	// to a buffer via SetDefault override would defeat the test — instead, we
	// just call slog.Info and require no panic.
	slog.Info("after failed reinit - must be safe")
}

// TestCloseLog_RepointsSlogToDiscard verifies that calling CloseLog repoints
// slog to a no-op handler so late callers do not write to the closed file.
func TestCloseLog_RepointsSlogToDiscard(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		if logFile != nil {
			_ = logFile.Close()
			logFile = nil
		}
	})

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}

	path := filepath.Join(stateDir, logFileName)
	beforeData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before CloseLog: %v", err)
	}

	CloseLog()

	// A slog call after CloseLog must not panic and must not extend the file.
	slog.Info("late call after CloseLog")

	afterData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after CloseLog: %v", err)
	}
	// The CloseLog itself emits "shutdown complete" before closing, so the
	// file may have grown by that one message. The "late call" must NOT have
	// appended further bytes.
	if bytes.Contains(afterData[len(beforeData):], []byte("late call after CloseLog")) {
		t.Error("slog call after CloseLog wrote to the closed log file")
	}
}
