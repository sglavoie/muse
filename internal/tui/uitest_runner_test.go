package tui

import (
	"strings"
	"testing"
	"time"
)

func TestRunScenarioSuccess(t *testing.T) {
	scenario := UITestScenario{
		Name:    "navigate",
		Fixture: FixtureRich,
		Size:    "100x30",
		Actions: []UITestAction{
			{Assert: &UITestAssertion{View: sp("Library")}},
			{Key: sp("3")},
			{Assert: &UITestAssertion{View: sp("Queue")}},
			{Assert: &UITestAssertion{NoLayoutIssues: bp(true)}},
		},
	}

	result, err := RunScenario(scenario, RunOptions{DrainTimeout: 20 * time.Millisecond, CollectObservations: true})
	if err != nil {
		t.Fatalf("RunScenario: %v", err)
	}
	if !result.OK {
		t.Fatalf("scenario failed unexpectedly: %+v", result.Error)
	}
	if result.ActionsRun != len(scenario.Actions) {
		t.Fatalf("actions_run = %d, want %d", result.ActionsRun, len(scenario.Actions))
	}
	if len(result.Observations) != len(scenario.Actions) {
		t.Fatalf("collected %d observations, want %d", len(result.Observations), len(scenario.Actions))
	}
	if result.FinalObservation == nil || result.FinalObservation.PrimaryView != "Queue" {
		t.Fatalf("final observation not Queue: %+v", result.FinalObservation)
	}
	if result.Fixture != FixtureRich || result.Size != "100x30" {
		t.Fatalf("result fixture/size = %q/%q", result.Fixture, result.Size)
	}
}

func TestRunScenarioStopsAtFirstFailure(t *testing.T) {
	scenario := UITestScenario{
		Name:    "failing",
		Fixture: FixtureRich,
		Actions: []UITestAction{
			{Key: sp("3")},
			{Label: "wrong-view", Assert: &UITestAssertion{View: sp("Mixes")}},
			{Key: sp("6")}, // should never run
		},
	}

	result, err := RunScenario(scenario, RunOptions{DrainTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("RunScenario: %v", err)
	}
	if result.OK {
		t.Fatal("expected scenario to fail")
	}
	if result.ActionsRun != 2 {
		t.Fatalf("actions_run = %d, want 2 (stop at first failure)", result.ActionsRun)
	}
	if result.Error == nil {
		t.Fatal("expected structured action error")
	}
	if result.Error.Index != 1 || result.Error.Label != "wrong-view" {
		t.Fatalf("action error index/label = %d/%q, want 1/wrong-view", result.Error.Index, result.Error.Label)
	}
	if result.Error.Observation == nil {
		t.Fatal("expected failure-point observation on action error")
	}
	if !strings.Contains(result.Error.Reason, "Mixes") {
		t.Fatalf("reason = %q, want it to mention Mixes", result.Error.Reason)
	}
}

func TestRunScenarioFixtureAndSizeOverride(t *testing.T) {
	scenario := UITestScenario{
		Name:    "override",
		Fixture: FixtureRich,
		Size:    "100x30",
		Actions: []UITestAction{{Assert: &UITestAssertion{View: sp("Library")}}},
	}

	result, err := RunScenario(scenario, RunOptions{
		Fixture:      FixtureEmpty,
		Size:         "60x20",
		DrainTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunScenario: %v", err)
	}
	if result.Fixture != FixtureEmpty {
		t.Fatalf("fixture = %q, want override to empty", result.Fixture)
	}
	if result.Size != "60x20" {
		t.Fatalf("size = %q, want override to 60x20", result.Size)
	}
}

func TestRunScenarioUnknownFixtureErrors(t *testing.T) {
	scenario := UITestScenario{
		Name:    "bad-fixture",
		Actions: []UITestAction{{Assert: &UITestAssertion{View: sp("Library")}}},
	}
	if _, err := RunScenario(scenario, RunOptions{Fixture: "does-not-exist"}); err == nil {
		t.Fatal("expected error for unknown fixture")
	}
}

func TestRunScenarioDefaultsToRichFixture(t *testing.T) {
	scenario := UITestScenario{
		Name:    "no-fixture",
		Actions: []UITestAction{{Assert: &UITestAssertion{View: sp("Library")}}},
	}
	result, err := RunScenario(scenario, RunOptions{DrainTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("RunScenario: %v", err)
	}
	if result.Fixture != FixtureRich {
		t.Fatalf("fixture = %q, want default rich", result.Fixture)
	}
	if result.Size != "100x30" {
		t.Fatalf("size = %q, want default 100x30", result.Size)
	}
}
