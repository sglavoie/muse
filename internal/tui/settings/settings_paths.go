package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validateLibraryDir keeps stored roots absolute and rejects aliases of roots
// already in the library, including symlinks and case variants on macOS.
func validateLibraryDir(raw string, dirs []string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("enter a directory path")
	}
	path, err := filepath.Abs(expandTildeSettings(raw))
	if err != nil {
		return "", fmt.Errorf("cannot resolve directory: %w", err)
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("directory does not exist")
	}
	if err != nil {
		return "", fmt.Errorf("cannot access directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	for _, dir := range dirs {
		existing, statErr := os.Stat(expandTildeSettings(dir))
		if statErr == nil && os.SameFile(info, existing) {
			return "", fmt.Errorf("directory is already in the library")
		}
	}
	return path, nil
}

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
