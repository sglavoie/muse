package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sglavoie/muse/internal/tui/uitest"
)

// scenarioDir is the committed example scenario directory, relative to the
// repository root (the working directory for main-package tests).
const scenarioDir = "docs/tui-testing/scenarios"

// gatedScenarios are stable baseline examples that must pass in CI. They run
// against their declared fixtures and are expected to report ok:true.
var gatedScenarios = []string{
	"playlist-fields-and-favorites-selection.json",
	"seek-to-time.json",
	"playlist-durations.json",
	"selection-summaries.json",
	"context-polish.json",
	"playlist-filtering.json",
	"usability-polish.json",
	"filter-and-queue-clarity.json",
	"baseline-layout.json",
	"usability-followup.json",
	"all-primary-views.json",
	"overlays.json",
	"themes.json",
	"command-palette-search.json",
	"layout-regressions-rich.json",
	"layout-regressions-empty.json",
	"narrow-long-text.json",
	"metadata-editor-narrow.json",
	"metadata-editor-mixed.json",
	"autodj-panel.json",
}

// ungatedScenarios are aggressive supported-size stress examples kept for manual bug
// discovery. They must parse and run without a setup error, but their pass/fail
// outcome is only logged.
var ungatedScenarios = []string{
	"resize-stress.json",
}

func loadScenario(t *testing.T, name string) uitest.UITestScenario {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scenarioDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	scenario, err := uitest.ParseScenario(data)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return scenario
}

func TestCommittedScenarios_Gated(t *testing.T) {
	for _, name := range gatedScenarios {
		t.Run(name, func(t *testing.T) {
			scenario := loadScenario(t, name)
			result, err := uitest.RunScenario(scenario, uitest.RunOptions{})
			if err != nil {
				t.Fatalf("setup error: %v", err)
			}
			if !result.OK {
				reason := "unknown"
				if result.Error != nil {
					reason = result.Error.Reason
				}
				t.Fatalf("scenario %q failed at action %d: %s", name,
					actionIndex(result), reason)
			}
		})
	}
}

func TestCommittedScenarios_UngatedRunClean(t *testing.T) {
	for _, name := range ungatedScenarios {
		t.Run(name, func(t *testing.T) {
			scenario := loadScenario(t, name)
			result, err := uitest.RunScenario(scenario, uitest.RunOptions{})
			if err != nil {
				// A setup error (unknown fixture, malformed size) is a real
				// defect even for ungated scenarios.
				t.Fatalf("setup error: %v", err)
			}
			if result.OK {
				t.Logf("ungated scenario %q currently passes", name)
			} else {
				reason := "unknown"
				if result.Error != nil {
					reason = result.Error.Reason
				}
				t.Logf("ungated scenario %q surfaced a layout issue at action %d: %s",
					name, actionIndex(result), reason)
			}
		})
	}
}

func actionIndex(r uitest.UITestScenarioResult) int {
	if r.Error != nil {
		return r.Error.Index
	}
	return -1
}
