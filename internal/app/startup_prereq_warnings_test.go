package app_test

import (
	"context"
	"slices"
	"testing"

	"github.com/sglavoie/muse/internal/app"
)

// TestRunReturnsPrereqWarnings verifies that the advisory warnings from step 5
// survive on RunResult instead of being logged and discarded. They are how the
// TUI learns that the metadata backend is degraded.
func TestRunReturnsPrereqWarnings(t *testing.T) {
	dirs := setupDirs(t)
	dirs.Runtime = shortTestDir(t)
	installLogger(t)

	warnings := []string{
		"ExifTool is unavailable, falling back to ffprobe for track metadata (scans are roughly 2.4x slower): exiftool not found on $PATH",
		"desktop notifications are unavailable: no backend",
	}

	deps := app.Deps{
		CheckPrereqs: func(context.Context) ([]string, error) { return warnings, nil },
		LaunchMPV:    func(context.Context, string) error { return nil },
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if !slices.Equal(result.PrereqWarnings, warnings) {
		t.Errorf("PrereqWarnings = %v, want %v", result.PrereqWarnings, warnings)
	}
}

// A healthy startup must not invent a warning: an empty slice here is what
// keeps the TUI's status bar silent.
func TestRunReturnsNoPrereqWarningsWhenHealthy(t *testing.T) {
	dirs := setupDirs(t)
	dirs.Runtime = shortTestDir(t)
	installLogger(t)

	deps := app.Deps{
		CheckPrereqs: func(context.Context) ([]string, error) { return nil, nil },
		LaunchMPV:    func(context.Context, string) error { return nil },
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if len(result.PrereqWarnings) != 0 {
		t.Errorf("PrereqWarnings = %v, want none", result.PrereqWarnings)
	}
}
