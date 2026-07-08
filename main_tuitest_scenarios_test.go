package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sglavoie/muse/internal/tui"
)

// scenarioDir is the committed example scenario directory, relative to the
// repository root (the working directory for main-package tests).
const scenarioDir = "docs/tui-testing/scenarios"

// gatedScenarios are stable baseline examples that must pass in CI. They run
// against their declared fixtures and are expected to report ok:true.
var gatedScenarios = []string{
	"baseline-layout.json",
	"all-primary-views.json",
	"overlays.json",
	"command-palette-search.json",
}

// ungatedScenarios are aggressive narrow/stress examples kept for manual bug
// discovery. They must parse and run without a setup error, but their pass/fail
// outcome is only logged — current renderers may not satisfy them at the small
// sizes they exercise.
var ungatedScenarios = []string{
	"resize-stress.json",
	"narrow-long-text.json",
}

func loadScenario(t *testing.T, name string) tui.UITestScenario {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scenarioDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	scenario, err := tui.ParseScenario(data)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return scenario
}

func TestCommittedScenarios_Gated(t *testing.T) {
	for _, name := range gatedScenarios {
		t.Run(name, func(t *testing.T) {
			scenario := loadScenario(t, name)
			result, err := tui.RunScenario(scenario, tui.RunOptions{})
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
			result, err := tui.RunScenario(scenario, tui.RunOptions{})
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

func actionIndex(r tui.UITestScenarioResult) int {
	if r.Error != nil {
		return r.Error.Index
	}
	return -1
}
