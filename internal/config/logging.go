package config

import (
	"log/slog"
	"os"
	"path/filepath"
)

const logFileName = "muse.log"

var logPath string

// InitLogging opens (and truncates) the log file at $XDG_STATE_HOME/muse/muse.log,
// sets the global slog default to a JSON handler at the given level, and stores
// the resolved path for later display.
func InitLogging(stateDir string, level slog.Level) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(stateDir, logFileName)
	f, err := os.Create(path) // truncates on every startup
	if err != nil {
		return err
	}

	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
	logPath = path
	return nil
}

// LogPath returns the resolved log file path for display in startup output
// and the help overlay.
func LogPath() string {
	return logPath
}
