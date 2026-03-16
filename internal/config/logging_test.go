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

	cfg, _, err := Load(cfgDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected LogLevel=debug, got %q", cfg.LogLevel)
	}

	// Init at Info so we start with a clean file, then apply the config level.
	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}
	SetLogLevel(ParseLogLevel(cfg.LogLevel))

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
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
		{"DEBUG", slog.LevelInfo}, // case-sensitive: uppercase not recognized
	}
	for _, tc := range cases {
		got := ParseLogLevel(tc.input)
		if got != tc.want {
			t.Errorf("ParseLogLevel(%q) = %v, want %v", tc.input, got, tc.want)
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

// --- SetLogLevel with nil logFile ---

func TestSetLogLevel_NilLogFile(t *testing.T) {
	saved := logFile
	logFile = nil
	defer func() { logFile = saved }()

	// Must not panic.
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
