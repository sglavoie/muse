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
	paths, err := filepath.Glob(filepath.Join(scenarioDir, "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("list scenarios: %v (%d files)", err, len(paths))
	}
	for _, path := range paths {
		name := filepath.Base(path)
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

func actionIndex(r uitest.UITestScenarioResult) int {
	if r.Error != nil {
		return r.Error.Index
	}
	return -1
}
