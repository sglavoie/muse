package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui"
)

// A missing music directory must not end the session: checkMusicDirs reports it
// through the existing config-warning channel and hands back the overlay hint,
// so the TUI still starts.
func TestCheckMusicDirs_MissingDirBecomesWarningNotFailure(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	missing := filepath.Join(t.TempDir(), "no-such-music-dir")

	got, hint := checkMusicDirs(config.LoadResult{}, []string{missing}, configPath)

	if len(got.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", got.Warnings)
	}
	warning := got.Warnings[0]
	if !strings.Contains(warning, "invalid music directory") || !strings.Contains(warning, missing) {
		t.Errorf("warning = %q, want it to name the offending directory", warning)
	}
	if strings.Contains(warning, "\n") {
		t.Errorf("warning = %q, want a single line for the status bar", warning)
	}
	if hint != config.ConfigHint(configPath) {
		t.Errorf("hint = %q, want config.ConfigHint(%q)", hint, configPath)
	}
}

func TestCheckMusicDirs_NoDirsConfiguredStillWarns(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")

	got, hint := checkMusicDirs(config.LoadResult{}, nil, configPath)

	if len(got.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", got.Warnings)
	}
	if hint == "" {
		t.Error("hint is empty; the overlay would have nothing to show")
	}
}

// Pre-existing config warnings must survive; the music-directory warning is
// appended to them rather than replacing them.
func TestCheckMusicDirs_PreservesExistingWarnings(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	missing := filepath.Join(t.TempDir(), "no-such-music-dir")
	existing := "default_volume 150 is out of range (0-100), using 100"

	got, _ := checkMusicDirs(config.LoadResult{Warnings: []string{existing}}, []string{missing}, configPath)

	if len(got.Warnings) != 2 || got.Warnings[0] != existing {
		t.Fatalf("Warnings = %v, want the existing warning first", got.Warnings)
	}
}

func TestCheckMusicDirs_ValidDirAddsNothing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	musicDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(musicDir, "track.mp3"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, hint := checkMusicDirs(config.LoadResult{}, []string{musicDir}, configPath)

	if len(got.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none for a valid directory", got.Warnings)
	}
	if hint != "" {
		t.Errorf("hint = %q, want empty for a valid directory", hint)
	}
}

// The warning reaches the TUI through the channel that already exists, and the
// hint reaches the welcome overlay.
func TestCheckMusicDirs_WarningAndHintReachTheTUI(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	missing := filepath.Join(t.TempDir(), "no-such-music-dir")

	cfgResult, hint := checkMusicDirs(config.LoadResult{}, []string{missing}, configPath)
	m := tui.New(context.Background(),
		tui.WithConfigPath(configPath, false),
		tui.WithConfigWarnings(cfgResult.Warnings),
		tui.WithMusicDirHint(hint),
	)

	view := m.View().Content
	if !strings.Contains(view, "music_directories") {
		t.Errorf("welcome overlay did not surface the music_directories hint:\n%s", view)
	}
}
