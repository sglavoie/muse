package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/audio"
)

// ValidateMusicDirs checks that each configured music directory exists and
// contains at least one audio file or non-hidden subdirectory at the top level.
// configPath is included in error messages so the user knows which file to edit.
// All directories are validated; errors from any are collected and returned together.
func ValidateMusicDirs(dirs []string, configPath string) error {
	var errs []string
	for _, musicDir := range dirs {
		if err := validateMusicDir(musicDir, configPath); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}

// validateMusicDir validates a single music directory.
func validateMusicDir(musicDir, configPath string) error {
	expanded, err := expandTilde(musicDir)
	if err != nil {
		return fmt.Errorf("music directory is not accessible; edit %s to set a valid path: %w", configPath, err)
	}

	entries, err := os.ReadDir(expanded)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("music directory %q does not exist; edit %s to set a valid path", expanded, configPath)
		}
		if errors.Is(err, os.ErrPermission) {
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
			if audio.IsSupportedExtension(ext) {
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

// expandTilde replaces a leading "~/" or a standalone "~" with the current
// user's home directory. Paths like "~user/foo" (tilde followed by a username)
// are returned unchanged because Go's os.UserHomeDir only resolves the current
// user's home.
func expandTilde(path string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[2:]), nil
}
