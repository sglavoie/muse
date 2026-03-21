package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const logFileName = "muse.log"

var logPath string
var logFile *os.File
var logLevel slog.LevelVar

// InitLogging opens (and truncates) the log file at $XDG_STATE_HOME/muse/muse.log,
// sets the global slog default to a JSON handler at the given level, and stores
// the resolved path for later display.
func InitLogging(stateDir string, level slog.Level) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}

	path := filepath.Join(stateDir, logFileName)

	// Preserve the previous session log for debugging. Best-effort: if the
	// file doesn't exist yet (first run), Rename returns an error we ignore.
	_ = os.Rename(path, path+".prev")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
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
func CloseLog() {
	if logFile != nil {
		slog.Info("shutdown complete")
		_ = logFile.Close()
		logFile = nil
	}
}
