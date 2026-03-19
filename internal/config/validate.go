package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// audioExtensions is the set of supported audio file extensions for validation.
// This intentionally duplicates library.audioExtensions to avoid a cross-package
// dependency — only the top-level directory is checked.
var audioExtensions = map[string]struct{}{
	".mp3":  {},
	".flac": {},
	".ogg":  {},
	".opus": {},
	".m4a":  {},
	".wav":  {},
	".wma":  {},
	".aac":  {},
	".aiff": {},
	".ape":  {},
	".wv":   {},
}

// ValidateMusicDir checks that the configured music directory exists and
// contains at least one audio file or non-hidden subdirectory at the top level.
// configPath is included in error messages so the user knows which file to edit.
func ValidateMusicDir(musicDir, configPath string) error {
	expanded, err := expandTilde(musicDir)
	if err != nil {
		return fmt.Errorf("music directory is not accessible; edit %s to set a valid path: %w", configPath, err)
	}

	entries, err := os.ReadDir(expanded)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("music directory %q does not exist; edit %s to set a valid path", expanded, configPath)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("music directory %q: permission denied; edit %s to set a valid path", expanded, configPath)
		}
		return fmt.Errorf("music directory %q cannot be read; edit %s to set a valid path: %w", expanded, configPath, err)
	}

	if len(entries) == 0 {
		return fmt.Errorf("music directory %q is empty; add music files or edit %s to set a different path", expanded, configPath)
	}

	hasAudioOrSubdir := false
	for _, e := range entries {
		if e.IsDir() {
			if !strings.HasPrefix(e.Name(), ".") {
				hasAudioOrSubdir = true
				break
			}
		} else {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if _, ok := audioExtensions[ext]; ok {
				hasAudioOrSubdir = true
				break
			}
		}
	}
	if !hasAudioOrSubdir {
		return fmt.Errorf("music directory %q contains no supported audio files; add music files or edit %s to set a different path", expanded, configPath)
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
