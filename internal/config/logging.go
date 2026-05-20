package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const logFileName = "muse.log"

var logPath string
var logFile *os.File
var logLevel slog.LevelVar

const logRotateSize = 5 * 1024 * 1024 // 5 MiB

// InitLogging opens the log file at $XDG_STATE_HOME/muse/muse.log in append
// mode so that log entries from multiple sessions accumulate in one file.
// If the file exceeds logRotateSize it is renamed to muse.log.1 before
// opening, keeping the total log data bounded.
// Sets the global slog default to a JSON handler at the given level, and
// stores the resolved path for later display.
func InitLogging(stateDir string, level slog.Level) error {
	// Close any previously opened log file to avoid leaking file handles
	// when InitLogging is called more than once (e.g. after a config reload).
	// Repoint slog to io.Discard immediately so subsequent slog calls during
	// the reinit do not race against (or write to) the just-closed FD even if
	// any of the remaining steps fail.
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: &logLevel})))
	}

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fmt.Errorf("init logging: create state dir: %w", err)
	}

	path := filepath.Join(stateDir, logFileName)

	// Rotate if the current log exceeds the size threshold.
	if fi, err := os.Stat(path); err == nil && fi.Size() >= logRotateSize {
		_ = os.Remove(path + ".2")
		_ = os.Rename(path+".1", path+".2")
		_ = os.Rename(path, path+".1")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("init logging: open log file: %w", err)
	}

	logLevel.Set(level)
	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: &logLevel})
	slog.SetDefault(slog.New(h))
	logPath = path
	logFile = f
	return nil
}

// LogPath returns the resolved log file path for display in startup output
// and the help overlay.
func LogPath() string {
	return logPath
}

// SetLogLevel atomically updates the active log level. Safe to call at any
// time, including before InitLogging.
func SetLogLevel(level slog.Level) {
	logLevel.Set(level)
}

// ParseLogLevel converts a log-level string (as used in the config file) to
// its corresponding slog.Level. The comparison is case-insensitive.
// Unrecognised values default to Info.
func ParseLogLevel(s string) (slog.Level, bool) {
	s = strings.ToLower(s)
	switch s {
	case "debug":
		return slog.LevelDebug, true
	case "info", "":
		return slog.LevelInfo, true
	case "warn":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

// CloseLog flushes and closes the log file handle. It should be called as the
// last step of the shutdown sequence, after all log writes have completed.
// Subsequent slog calls are routed to io.Discard so that late callers do not
// write to the closed file descriptor.
func CloseLog() {
	if logFile != nil {
		slog.Info("shutdown complete")
		_ = logFile.Close() // deferred close in error path, file will be GC'd
		logFile = nil
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: &logLevel})))
	}
}
