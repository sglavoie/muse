package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestInitLogging_CreatesLogFile(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}

	path := filepath.Join(stateDir, logFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("log file not created: %v", err)
	}
}

func TestInitLogging_LogPathGetter(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}

	want := filepath.Join(stateDir, logFileName)
	if got := LogPath(); got != want {
		t.Errorf("LogPath() = %q, want %q", got, want)
	}
}

func TestInitLogging_TruncatesOnStartup(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	// First init — write a log line.
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("first InitLogging: %v", err)
	}
	slog.Info("first run message")

	// Second init — file must be truncated (previous content gone).
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("second InitLogging: %v", err)
	}

	path := filepath.Join(stateDir, logFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty file after truncation, got %d bytes", len(data))
	}
}

func TestInitLogging_JSONLinesOutput(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}

	slog.Info("test message", "key", "value")

	path := filepath.Join(stateDir, logFileName)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening log file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("expected at least one log line")
	}
	line := scanner.Bytes()

	var entry map[string]any
	if err := json.Unmarshal(line, &entry); err != nil {
		t.Fatalf("log line is not valid JSON: %v\nline: %s", err, line)
	}
	if entry["msg"] != "test message" {
		t.Errorf("unexpected msg field: %v", entry["msg"])
	}
}

func TestInitLogging_LogLevelRespected(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	// Init at Info level — debug messages must not appear.
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}

	slog.Debug("should not appear")
	slog.Info("should appear")

	path := filepath.Join(stateDir, logFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}

	lines := countJSONLines(data)
	if lines != 1 {
		t.Errorf("expected 1 log line (Info only), got %d\ncontent: %s", lines, data)
	}
}

func TestInitLogging_DebugLevelEnabled(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	// Init at Debug level — both messages must appear.
	if err := InitLogging(stateDir, slog.LevelDebug); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}

	slog.Debug("debug line")
	slog.Info("info line")

	path := filepath.Join(stateDir, logFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}

	lines := countJSONLines(data)
	if lines != 2 {
		t.Errorf("expected 2 log lines (Debug+Info), got %d\ncontent: %s", lines, data)
	}
}

// countJSONLines returns the number of non-empty lines in data.
func countJSONLines(data []byte) int {
	count := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			count++
		}
	}
	return count
}
