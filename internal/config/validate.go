package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateMusicDir checks that the configured music directory exists and
// contains at least one entry. configPath is included in error messages so
// the user knows which file to edit.
func ValidateMusicDir(musicDir, configPath string) error {
	expanded, err := expandTilde(musicDir)
	if err != nil {
		return fmt.Errorf("music directory is not accessible; edit %s to set a valid path: %w", configPath, err)
	}

	entries, err := os.ReadDir(expanded)
	if err != nil {
		return fmt.Errorf("music directory %q does not exist or cannot be read; edit %s to set a valid path", expanded, configPath)
	}

	if len(entries) == 0 {
		return fmt.Errorf("music directory %q is empty; add music files or edit %s to set a different path", expanded, configPath)
	}

	return nil
}

// expandTilde replaces a leading "~" with the current user's home directory.
func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}
