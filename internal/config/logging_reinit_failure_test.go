package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestInitLogging_FailureKeepsPreviousLogger verifies that a failed reinit does
// not drop the already-working logger.
func TestInitLogging_FailureKeepsPreviousLogger(t *testing.T) {
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
	path := filepath.Join(stateDir, logFileName)

	// First init succeeds, opens a file handle and points slog at it.
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("first InitLogging: %v", err)
	}
	firstFile := logFile
	slog.Info("before failed reinit")
	if err := logFile.Sync(); err != nil {
		t.Fatalf("sync initial log file: %v", err)
	}

	// Force the second init to fail at MkdirAll by placing a regular file
	// at the path that would need to become a directory.
	blockPath := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blockPath, []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InitLogging(filepath.Join(blockPath, "muse"), slog.LevelInfo); err == nil {
		t.Fatal("expected InitLogging to fail when stateDir parent is a file")
	}

	if logFile != firstFile {
		t.Fatal("failed reinit should keep the existing log file handle active")
	}

	slog.Info("after failed reinit - must be preserved")
	if err := logFile.Sync(); err != nil {
		t.Fatalf("sync preserved log file: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file after failed reinit: %v", err)
	}
	if !bytes.Contains(data, []byte("after failed reinit - must be preserved")) {
		t.Fatalf("expected preserved logger to keep writing to %s, got:\n%s", path, data)
	}
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
