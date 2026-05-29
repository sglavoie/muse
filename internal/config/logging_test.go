package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

func TestInitLogging_AppendsAcrossSessions(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	// First init — write a log line.
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("first InitLogging: %v", err)
	}
	slog.Info("first run message")

	// Flush by closing the file handle directly so the second init can read it.
	CloseLog()

	// Second init — previous content must still be present (append mode).
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("second InitLogging: %v", err)
	}
	slog.Info("second run message")
	CloseLog()

	path := filepath.Join(stateDir, logFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	lines := countJSONLines(data)
	if lines < 2 {
		t.Errorf("expected at least 2 log lines across sessions, got %d\ncontent: %s", lines, data)
	}
}

func TestInitLogging_RotatesWhenOverSizeLimit(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(stateDir, logFileName)

	// Write a file that exceeds the rotation threshold.
	big := make([]byte, logRotateSize+1)
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatalf("writing oversized log: %v", err)
	}

	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}
	CloseLog()

	// The oversized file must have been rotated to muse.log.1.
	rotated := path + ".1"
	if _, err := os.Stat(rotated); err != nil {
		t.Errorf("expected rotated log at %s, stat error: %v", rotated, err)
	}

	// The current log file must be smaller than the threshold.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat current log: %v", err)
	}
	if fi.Size() >= logRotateSize {
		t.Errorf("current log is %d bytes, expected < %d after rotation", fi.Size(), logRotateSize)
	}
}

func TestInitLogging_RotationFailureReturnsError(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(stateDir, logFileName)
	big := make([]byte, logRotateSize+1)
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatalf("writing oversized log: %v", err)
	}

	blockingBackup := path + ".2"
	if err := os.Mkdir(blockingBackup, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blockingBackup, "block"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := InitLogging(stateDir, slog.LevelInfo)
	if err == nil {
		t.Fatal("InitLogging error = nil, want rotation failure")
	}
	if !strings.Contains(err.Error(), "rotate log") {
		t.Fatalf("InitLogging error = %v, want rotation context", err)
	}

	fi, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat current log after failed rotation: %v", statErr)
	}
	if fi.Size() != int64(len(big)) {
		t.Errorf("current log size = %d, want unchanged %d after failed rotation", fi.Size(), len(big))
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

func TestSetLogLevel(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		if logFile != nil {
			_ = logFile.Close()
			logFile = nil
		}
	})

	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}

	// Debug message emitted at Info level must not appear.
	slog.Debug("before upgrade: should not appear")

	path := filepath.Join(stateDir, logFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if n := countJSONLines(data); n != 0 {
		t.Errorf("expected 0 lines before SetLogLevel(Debug), got %d\ncontent: %s", n, data)
	}

	// After upgrading to Debug level the same kind of message must appear.
	SetLogLevel(slog.LevelDebug)
	slog.Debug("after upgrade: should appear")

	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file after SetLogLevel: %v", err)
	}
	if n := countJSONLines(data); n != 1 {
		t.Errorf("expected 1 line after SetLogLevel(Debug), got %d\ncontent: %s", n, data)
	}
}

func TestLogLevelFromConfig(t *testing.T) {
	cfgDir := t.TempDir()
	stateDir := filepath.Join(t.TempDir(), "muse")

	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		if logFile != nil {
			_ = logFile.Close()
			logFile = nil
		}
	})

	// Write a minimal config file with log_level = "debug".
	cfgContent := `log_level = "debug"` + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, configFileName), []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	loadResult, err := Load(cfgDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loadResult.Config.LogLevel != "debug" {
		t.Fatalf("expected LogLevel=debug, got %q", loadResult.Config.LogLevel)
	}

	// Init at Info so we start with a clean file, then apply the config level.
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}
	level, _ := ParseLogLevel(loadResult.Config.LogLevel)
	SetLogLevel(level)

	slog.Debug("end-to-end debug message")

	path := filepath.Join(stateDir, logFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if n := countJSONLines(data); n != 1 {
		t.Errorf("expected 1 debug line after config-driven SetLogLevel, got %d\ncontent: %s", n, data)
	}
}

// --- ParseLogLevel ---

func TestParseLogLevel_AllValues(t *testing.T) {
	cases := []struct {
		input  string
		want   slog.Level
		wantOK bool
	}{
		{"debug", slog.LevelDebug, true},
		{"warn", slog.LevelWarn, true},
		{"warning", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"info", slog.LevelInfo, true},
		{"", slog.LevelInfo, true},
		{"unknown", slog.LevelInfo, false},
		{"DEBUG", slog.LevelDebug, true}, // case-insensitive: uppercase is accepted
		{" WARN ", slog.LevelWarn, true},
	}
	for _, tc := range cases {
		got, ok := ParseLogLevel(tc.input)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("ParseLogLevel(%q) = (%v, %v), want (%v, %v)", tc.input, got, ok, tc.want, tc.wantOK)
		}
	}
}

// --- CloseLog ---

func TestCloseLog(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "closelog-muse")

	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		logFile = nil
	})

	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}

	CloseLog()
	if logFile != nil {
		t.Error("CloseLog: expected logFile=nil after close")
	}

	// Calling CloseLog again should not panic.
	CloseLog()
}

func TestCloseLog_HandlerSwappedBeforeFileClosed(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		logFile = nil
	})

	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}

	logFilePath := filepath.Join(stateDir, logFileName)
	preInfo, err := os.Stat(logFilePath)
	if err != nil {
		t.Fatalf("stat before close: %v", err)
	}
	preSize := preInfo.Size()

	CloseLog()
	slog.Info("post-close message should not write to file or panic")

	postInfo, err := os.Stat(logFilePath)
	if err != nil {
		t.Fatalf("stat after close: %v", err)
	}
	postClosePadding := int64(512)
	if delta := postInfo.Size() - preSize; delta > postClosePadding {
		t.Errorf("file grew by %d bytes after CloseLog; expected only the shutdown line", delta)
	}
}

// --- InitLogging closes previous file handle ---

func TestInitLogging_ClosesOldFileOnReinit(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		CloseLog()
	})

	// First init — opens a file handle.
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("first InitLogging: %v", err)
	}
	firstFile := logFile

	// Second init without an explicit CloseLog — the old handle must be
	// closed automatically to avoid leaking file descriptors.
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("second InitLogging: %v", err)
	}

	// Writing to the old handle should fail because it was closed.
	_, err := firstFile.Write([]byte("should fail"))
	if err == nil {
		t.Error("expected error writing to old log file handle after reinit, got nil (file descriptor leaked)")
	}
}

// --- SetLogLevel with nil logFile ---

func TestSetLogLevel_NilLogFile(t *testing.T) {
	// Must not panic when called before InitLogging.
	SetLogLevel(slog.LevelDebug)
}

// --- ConfigPath ---

func TestConfigPath(t *testing.T) {
	got := ConfigPath("/home/user/.config/muse")
	want := filepath.Join("/home/user/.config/muse", configFileName)
	if got != want {
		t.Errorf("ConfigPath = %q, want %q", got, want)
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
