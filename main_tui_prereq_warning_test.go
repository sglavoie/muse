package main

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui"
)

// fallbackWarning is the shape config.checkMetadataBackend produces when
// ExifTool is unusable and ffprobe takes over.
const fallbackWarning = "ExifTool is unavailable, falling back to ffprobe for track metadata " +
	"(scans are roughly 2.4x slower): exiftool not found on $PATH"

// TestPrereqFallbackWarningReachesTUI pins the whole plumbing path for a
// degraded metadata backend: app.Run hands the warning back on RunResult,
// runTUI merges it into the config warnings, and the model renders it. Before
// this the warning existed only in the log file, so the user saw a slower scan
// with no on-screen explanation.
func TestPrereqFallbackWarningReachesTUI(t *testing.T) {
	cfgResult := config.LoadResult{Warnings: []string{"default_volume 150 is out of range (0-100), using 100"}}

	merged := mergeStartupWarnings(cfgResult, []string{fallbackWarning})

	m := tui.New(context.Background(), tui.WithConfigWarnings(merged.Warnings))
	view := m.View().Content
	if !strings.Contains(view, "falling back to ffprobe") {
		t.Errorf("the ffprobe fallback warning never reached the TUI:\n%s", view)
	}
	if !strings.Contains(view, "default_volume") {
		t.Errorf("merging dropped the pre-existing config warning:\n%s", view)
	}
}

// The merge happens at exactly one place, but a warning that reaches both the
// config-load channel and the prerequisite channel must still be shown once.
func TestPrereqWarningsMergedWithoutDuplication(t *testing.T) {
	cfgResult := config.LoadResult{Warnings: []string{fallbackWarning}}

	merged := mergeStartupWarnings(cfgResult, []string{fallbackWarning, fallbackWarning})

	if len(merged.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", merged.Warnings)
	}

	m := tui.New(context.Background(), tui.WithConfigWarnings(merged.Warnings))
	if n := strings.Count(m.View().Content, "falling back to ffprobe"); n != 1 {
		t.Errorf("warning rendered %d times, want 1", n)
	}
}

// Merging must not write through the caller's backing array, and a healthy
// startup must add nothing.
func TestMergeStartupWarningsNoPrereqWarnings(t *testing.T) {
	// backing's second element is the spare capacity an in-place append would
	// scribble on; slicing it down is the only way to observe that happening.
	backing := []string{"existing", "untouched"}
	cfgResult := config.LoadResult{Warnings: backing[:1]}

	merged := mergeStartupWarnings(cfgResult, nil)
	if len(merged.Warnings) != 1 || merged.Warnings[0] != "existing" {
		t.Fatalf("Warnings = %v, want the existing warning unchanged", merged.Warnings)
	}

	withPrereq := mergeStartupWarnings(cfgResult, []string{fallbackWarning})
	if len(withPrereq.Warnings) != 2 {
		t.Fatalf("Warnings = %v, want two", withPrereq.Warnings)
	}
	if backing[1] != "untouched" {
		t.Fatalf("merge wrote through the caller's backing array: %v", backing)
	}
}
