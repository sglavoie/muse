package tui

// This file shapes scenario execution and results for the hidden
// `muse tui-test` layout harness. RunScenario builds a fixture-backed driver,
// runs a scenario's actions in order, stops at the first failure, and returns a
// JSON-friendly result describing what ran and — on failure — a structured
// action error with the failure-point observation.

import (
	"fmt"
	"time"
)

// Default terminal size used when neither the scenario nor the run options
// specify one.
const (
	defaultScenarioWidth  = 100
	defaultScenarioHeight = 30
)

// UITestActionError describes a single failed action within a scenario run.
type UITestActionError struct {
	Index       int                `json:"index"`
	Label       string             `json:"label,omitempty"`
	Action      UITestAction       `json:"action"`
	Reason      string             `json:"reason"`
	Observation *UITestObservation `json:"observation,omitempty"`
}

// UITestScenarioResult is the JSON-friendly outcome of running a scenario.
type UITestScenarioResult struct {
	OK               bool                `json:"ok"`
	Scenario         string              `json:"scenario"`
	Fixture          string              `json:"fixture"`
	Size             string              `json:"size"`
	ActionsRun       int                 `json:"actions_run"`
	FinalObservation *UITestObservation  `json:"final_observation,omitempty"`
	Observations     []UITestObservation `json:"observations,omitempty"`
	Error            *UITestActionError  `json:"error,omitempty"`
}

// RunOptions parameterizes a scenario run. Fixture and Size override the
// scenario's own values when non-empty.
type RunOptions struct {
	Fixture             string
	Size                string
	IncludeANSI         bool
	IncludeCommands     bool
	CollectObservations bool
	DrainTimeout        time.Duration
}

// RunScenario builds a fixture-backed driver and runs scenario's actions in
// order, stopping at the first failure. A returned error indicates a setup
// problem (unknown fixture, malformed size) rather than an action failure;
// action failures are reported inside the result with OK set to false.
func RunScenario(scenario UITestScenario, opts RunOptions) (UITestScenarioResult, error) {
	fixture := scenario.Fixture
	if opts.Fixture != "" {
		fixture = opts.Fixture
	}
	if fixture == "" {
		fixture = FixtureRich
	}

	sizeStr := scenario.Size
	if opts.Size != "" {
		sizeStr = opts.Size
	}
	width, height := defaultScenarioWidth, defaultScenarioHeight
	if sizeStr != "" {
		w, h, err := ParseSize(sizeStr)
		if err != nil {
			return UITestScenarioResult{}, fmt.Errorf("scenario %q: %w", scenario.Name, err)
		}
		width, height = w, h
	}

	model, err := BuildFixture(fixture)
	if err != nil {
		return UITestScenarioResult{}, err
	}

	driver := NewUITestDriver(model, width, height, opts.DrainTimeout)
	obsOpts := ObserveOptions{IncludeANSI: opts.IncludeANSI, IncludeCommands: opts.IncludeCommands}

	result := UITestScenarioResult{
		OK:       true,
		Scenario: scenario.Name,
		Fixture:  fixture,
		Size:     fmt.Sprintf("%dx%d", width, height),
	}

	for i, action := range scenario.Actions {
		obs, aerr := driver.ApplyAction(action, obsOpts)
		result.ActionsRun++

		observation := obs
		result.FinalObservation = &observation
		if opts.CollectObservations {
			result.Observations = append(result.Observations, obs)
		}

		if aerr != nil {
			failure := observation
			result.OK = false
			result.Error = &UITestActionError{
				Index:       i,
				Label:       action.Label,
				Action:      action,
				Reason:      aerr.Error(),
				Observation: &failure,
			}
			return result, nil
		}
	}

	return result, nil
}
