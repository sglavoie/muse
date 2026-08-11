package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui"
)

func TestTUITestFixtures_Text(t *testing.T) {
	code, stdout, stderr := runCLI(t, "", "fixtures")
	if code != tuiTestExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	for _, name := range tui.FixtureNames() {
		if !strings.Contains(stdout, name) {
			t.Errorf("fixtures output missing %q:\n%s", name, stdout)
		}
	}
	// One fixture per line.
	lines := strings.Fields(strings.TrimSpace(stdout))
	if len(lines) != len(tui.FixtureNames()) {
		t.Errorf("expected %d fixtures, got %d: %q", len(tui.FixtureNames()), len(lines), stdout)
	}
}

func TestTUITestFixtures_JSON(t *testing.T) {
	code, stdout, stderr := runCLI(t, "", "fixtures", "--json")
	if code != tuiTestExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	var payload struct {
		Fixtures []string `json:"fixtures"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal fixtures json: %v (out=%q)", err, stdout)
	}
	if got, want := len(payload.Fixtures), len(tui.FixtureNames()); got != want {
		t.Fatalf("fixtures count = %d, want %d", got, want)
	}
	if payload.Fixtures[0] != tui.FixtureNames()[0] {
		t.Errorf("first fixture = %q, want %q", payload.Fixtures[0], tui.FixtureNames()[0])
	}
}

func TestTUITestRun_JSONSuccess(t *testing.T) {
	path := writeScenario(t, `{
		"name": "ok",
		"actions": [
			{"assert": {"view": "Library"}},
			{"key": "2"},
			{"assert": {"view": "Playlists"}},
			{"assert": {"no_layout_issues": true}}
		]
	}`)

	code, stdout, stderr := runCLI(t, "", "run", "--scenario", path, "--json")
	if code != tuiTestExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	var result tui.UITestScenarioResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("unmarshal result: %v (out=%q)", err, stdout)
	}
	if !result.OK {
		t.Errorf("expected ok result, got %+v", result)
	}
	if result.Scenario != "ok" {
		t.Errorf("scenario = %q", result.Scenario)
	}
	if result.ActionsRun != 4 {
		t.Errorf("actions_run = %d, want 4", result.ActionsRun)
	}
	if result.Fixture != tui.FixtureRich {
		t.Errorf("fixture = %q, want %q", result.Fixture, tui.FixtureRich)
	}
}

func TestTUITestRun_FailureExitCode(t *testing.T) {
	path := writeScenario(t, `{
		"name": "fails",
		"actions": [
			{"assert": {"view": "Queue"}, "label": "wrong-view"}
		]
	}`)

	code, stdout, stderr := runCLI(t, "", "run", "--scenario", path, "--json")
	if code != tuiTestExitFailure {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, tuiTestExitFailure, stderr)
	}
	var result tui.UITestScenarioResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("unmarshal result: %v (out=%q)", err, stdout)
	}
	if result.OK {
		t.Fatal("expected failing result")
	}
	if result.Error == nil {
		t.Fatal("expected structured action error")
	}
	if result.Error.Label != "wrong-view" {
		t.Errorf("error label = %q, want wrong-view", result.Error.Label)
	}
	if result.Error.Observation == nil {
		t.Error("expected failure-point observation")
	}
}

func TestTUITestRun_SummaryFailureToStdout(t *testing.T) {
	path := writeScenario(t, `{
		"name": "fails",
		"actions": [{"assert": {"contains": "definitely-not-on-screen"}}]
	}`)

	code, stdout, _ := runCLI(t, "", "run", "--scenario", path)
	if code != tuiTestExitFailure {
		t.Fatalf("exit = %d, want %d", code, tuiTestExitFailure)
	}
	if !strings.HasPrefix(stdout, "FAIL") {
		t.Errorf("expected FAIL summary, got %q", stdout)
	}
}

func TestTUITestRun_InvalidScenarioJSON(t *testing.T) {
	path := writeScenario(t, `{ this is not valid json `)
	code, _, stderr := runCLI(t, "", "run", "--scenario", path)
	if code != tuiTestExitUsage {
		t.Fatalf("exit = %d, want %d", code, tuiTestExitUsage)
	}
	if stderr == "" {
		t.Error("expected error on stderr for invalid scenario")
	}
}

func TestTUITestRun_UnknownFixture(t *testing.T) {
	path := writeScenario(t, `{"name": "x", "actions": [{"assert": {"view": "Library"}}]}`)
	code, _, stderr := runCLI(t, "", "run", "--scenario", path, "--fixture", "bogus")
	if code != tuiTestExitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, tuiTestExitUsage, stderr)
	}
	if !strings.Contains(stderr, "bogus") {
		t.Errorf("expected unknown-fixture error mentioning fixture, got %q", stderr)
	}
}

func TestTUITestRun_MissingScenarioFlag(t *testing.T) {
	code, _, stderr := runCLI(t, "", "run")
	if code != tuiTestExitUsage {
		t.Fatalf("exit = %d, want %d", code, tuiTestExitUsage)
	}
	if !strings.Contains(stderr, "--scenario") {
		t.Errorf("expected --scenario required error, got %q", stderr)
	}
}

func TestTUITestRun_MissingScenarioFile(t *testing.T) {
	code, _, stderr := runCLI(t, "", "run", "--scenario", filepath.Join(t.TempDir(), "nope.json"))
	if code != tuiTestExitUsage {
		t.Fatalf("exit = %d, want %d", code, tuiTestExitUsage)
	}
	if stderr == "" {
		t.Error("expected read error on stderr")
	}
}

func TestTUITest_NoSubcommand(t *testing.T) {
	code, _, stderr := runCLI(t, "")
	if code != tuiTestExitUsage {
		t.Fatalf("exit = %d, want %d", code, tuiTestExitUsage)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected usage on stderr, got %q", stderr)
	}
}

func TestTUITest_UnknownSubcommand(t *testing.T) {
	code, _, stderr := runCLI(t, "", "bogus")
	if code != tuiTestExitUsage {
		t.Fatalf("exit = %d, want %d", code, tuiTestExitUsage)
	}
	if !strings.Contains(stderr, "unknown subcommand") {
		t.Errorf("expected unknown-subcommand error, got %q", stderr)
	}
}

func TestTUITest_Help(t *testing.T) {
	code, stdout, _ := runCLI(t, "", "--help")
	if code != tuiTestExitOK {
		t.Fatalf("exit = %d, want %d", code, tuiTestExitOK)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("expected usage on stdout, got %q", stdout)
	}
}

func TestTUITest_InvalidFlag(t *testing.T) {
	code, _, stderr := runCLI(t, "", "fixtures", "--nope")
	if code != tuiTestExitUsage {
		t.Fatalf("exit = %d, want %d", code, tuiTestExitUsage)
	}
	if stderr == "" {
		t.Error("expected flag error on stderr")
	}
}
