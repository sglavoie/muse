package tui

import (
	"os"
	"strings"
)

// collapseHomeDir replaces the home directory prefix with "~" for display.
func collapseHomeDir(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}

// expandTildeSettings expands a leading "~" to the user's home directory.
func expandTildeSettings(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if len(path) == 1 {
		return home
	}
	if path[1] == '/' {
		return home + path[1:]
	}
	return path
}
