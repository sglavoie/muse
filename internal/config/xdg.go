package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const appName = "muse"

// Dirs holds the resolved XDG base directories for the application.
type Dirs struct {
	Config  string // $XDG_CONFIG_HOME/muse/
	Data    string // $XDG_DATA_HOME/muse/
	State   string // $XDG_STATE_HOME/muse/
	Runtime string // $XDG_RUNTIME_DIR/muse/
}

// ResolveDirs returns the XDG base directories for the application.
// On Linux the XDG Base Directory Specification defaults apply.
// On macOS, config falls back to ~/.config and data falls back to
// ~/Library/Application Support to match common CLI tool conventions.
func ResolveDirs() (Dirs, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Dirs{}, err
	}

	return Dirs{
		Config:  filepath.Join(resolveConfigHome(home), appName),
		Data:    filepath.Join(resolveDataHome(home), appName),
		State:   filepath.Join(resolveStateHome(home), appName),
		Runtime: filepath.Join(resolveRuntimeDir(), appName),
	}, nil
}

// resolveConfigHome returns $XDG_CONFIG_HOME, falling back to platform defaults.
func resolveConfigHome(home string) string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	// macOS and Linux both default to ~/.config for CLI tools.
	return filepath.Join(home, ".config")
}

// resolveDataHome returns $XDG_DATA_HOME, falling back to platform defaults.
func resolveDataHome(home string) string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support")
	}
	return filepath.Join(home, ".local", "share")
}

// resolveStateHome returns $XDG_STATE_HOME, falling back to platform defaults.
func resolveStateHome(home string) string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return v
	}
	return filepath.Join(home, ".local", "state")
}

// resolveRuntimeDir returns $XDG_RUNTIME_DIR, falling back to a temp-based path.
func resolveRuntimeDir() string {
	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		return v
	}
	return filepath.Join(os.TempDir(), appName+"-runtime")
}
