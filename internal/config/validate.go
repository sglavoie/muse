package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/audio"
)

// ConfigHint is the actionable instruction block appended to every
// music-directory validation error. It names the config file, the TOML key and
// a concrete example so a first-run user never has to open the file and guess
// which key to change. Mirrors the indented style of the missing-prerequisite
// errors in prereqs.go.
//
// It is exported so the TUI's welcome overlay can render exactly the same
// wording the error path uses; the two must never drift apart.
func ConfigHint(configPath string) string {
	return fmt.Sprintf(
		"\n  edit %s and set:\n      music_directories = [%q]",
		configPath, exampleMusicDirectory,
	)
}

// MusicDirsResult reports the outcome of validating the configured music
// directories. It exists so callers can tell "nothing to scan at all" apart
// from "some roots failed but the rest are being scanned" — the two cases need
// different user-facing copy, and Err alone cannot distinguish them.
type MusicDirsResult struct {
	// Usable holds the trimmed directories that validated, in config order.
	Usable []string
	// Err is the combined validation error, nil when every configured
	// directory validated.
	Err error
}

// Degraded reports a partially valid configuration: at least one directory is
// usable, but at least one other failed validation.
func (r MusicDirsResult) Degraded() bool { return r.Err != nil && len(r.Usable) > 0 }

// ValidateMusicDirs checks that each configured music directory exists and
// contains at least one audio file or non-hidden subdirectory at the top level.
// configPath is included in error messages so the user knows which file to edit.
// All directories are validated; errors from any are collected and returned together.
//
// Callers that need to distinguish zero usable directories from partial
// degradation should use ValidateMusicDirsDetailed instead.
func ValidateMusicDirs(dirs []string, configPath string) error {
	return ValidateMusicDirsDetailed(dirs, configPath).Err
}

// ValidateMusicDirsDetailed validates every configured music directory and
// reports both which ones are usable and the combined error for the rest.
func ValidateMusicDirsDetailed(dirs []string, configPath string) MusicDirsResult {
	var result MusicDirsResult
	var errs []string
	validated := 0
	for _, musicDir := range dirs {
		trimmed := strings.TrimSpace(musicDir)
		if trimmed == "" {
			continue
		}
		validated++
		if err := validateMusicDir(trimmed, configPath); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		result.Usable = append(result.Usable, trimmed)
	}
	if validated == 0 {
		result.Err = fmt.Errorf("no music directories configured%s", ConfigHint(configPath))
		return result
	}
	if len(errs) == 0 {
		return result
	}
	// Joined with a blank line rather than "; " because each message now spans
	// several lines; semicolon-joining multi-line strings reads as one blob.
	result.Err = fmt.Errorf("%s", strings.Join(errs, "\n\n"))
	return result
}

// validateMusicDir validates a single music directory.
func validateMusicDir(musicDir, configPath string) error {
	expanded, err := expandTilde(musicDir)
	if err != nil {
		return fmt.Errorf("music directory is not accessible: %w%s", err, ConfigHint(configPath))
	}

	entries, err := os.ReadDir(expanded)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("music directory %q does not exist%s", expanded, ConfigHint(configPath))
		}
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("music directory %q: permission denied%s", expanded, ConfigHint(configPath))
		}
		return fmt.Errorf("music directory %q cannot be read: %w%s", expanded, err, ConfigHint(configPath))
	}

	if len(entries) == 0 {
		return fmt.Errorf("music directory %q is empty; add music files, or point muse elsewhere%s", expanded, ConfigHint(configPath))
	}

	hasAudioOrSubdir := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			hasAudioOrSubdir = true
			break
		} else {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if audio.IsSupportedExtension(ext) {
				hasAudioOrSubdir = true
				break
			}
		}
	}
	if !hasAudioOrSubdir {
		return fmt.Errorf("music directory %q contains no supported audio files; add music files, or point muse elsewhere%s", expanded, ConfigHint(configPath))
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
