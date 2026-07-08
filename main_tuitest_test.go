package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui"
)

// runCLI is a small helper that drives runTUITestCLI with the given args and
// captured stdin, returning the exit code and the two output streams.
func runCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runTUITestCLI(args, strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writeScenario(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
	return path
}

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

// TestTUITestSession_NDJSONFlow drives the interactive session mode through an
// observe, action, reset, and quit request and validates each NDJSON response.
func TestTUITestSession_NDJSONFlow(t *testing.T) {
	input := strings.Join([]string{
		`{"type": "observe"}`,
		`{"type": "action", "action": {"key": "2"}}`,
		`{"type": "action", "action": {"assert": {"view": "Playlists"}}}`,
		`{"type": "reset"}`,
		`{"type": "quit"}`,
	}, "\n") + "\n"

	code, stdout, stderr := runCLI(t, input, "session")
	if code != tuiTestExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}

	lines := splitNonEmptyLines(stdout)
	if len(lines) != 5 {
		t.Fatalf("expected 5 responses, got %d:\n%s", len(lines), stdout)
	}

	decode := func(line string) sessionResponse {
		var resp sessionResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("decode response %q: %v", line, err)
		}
		return resp
	}

	// observe: on Library initially.
	if r := decode(lines[0]); !r.OK || r.Observation == nil || r.Observation.PrimaryView != "Library" {
		t.Errorf("observe response unexpected: %s", lines[0])
	}
	// action key "2": now on Playlists.
	if r := decode(lines[1]); !r.OK || r.Observation == nil || r.Observation.PrimaryView != "Playlists" {
		t.Errorf("action response unexpected: %s", lines[1])
	}
	// assert view Playlists: passes.
	if r := decode(lines[2]); !r.OK {
		t.Errorf("assert response should pass: %s", lines[2])
	}
	// reset: back to Library.
	if r := decode(lines[3]); !r.OK || r.Observation == nil || r.Observation.PrimaryView != "Library" {
		t.Errorf("reset response unexpected: %s", lines[3])
	}
	// quit: acknowledged.
	if r := decode(lines[4]); !r.OK || r.Type != "quit" {
		t.Errorf("quit response unexpected: %s", lines[4])
	}
}

// TestTUITestSession_ResetClearsPendingAsync opens the command palette in
// session mode (whose filter-activation blink command parks as a pending async
// future under the default drain timeout), then resets. The reset response and
// the following observation must reflect a clean, freshly-built driver rather
// than carrying the replaced driver's pending state or open overlay.
func TestTUITestSession_ResetClearsPendingAsync(t *testing.T) {
	input := strings.Join([]string{
		`{"type": "action", "action": {"key": ":"}}`,
		`{"type": "observe"}`,
		`{"type": "reset"}`,
		`{"type": "observe"}`,
		`{"type": "quit"}`,
	}, "\n") + "\n"

	code, stdout, stderr := runCLI(t, input, "session")
	if code != tuiTestExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}

	lines := splitNonEmptyLines(stdout)
	if len(lines) != 5 {
		t.Fatalf("expected 5 responses, got %d:\n%s", len(lines), stdout)
	}

	decode := func(line string) sessionResponse {
		var resp sessionResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("decode response %q: %v", line, err)
		}
		return resp
	}

	// Opening the palette succeeds and shows the command_palette overlay.
	if r := decode(lines[0]); !r.OK || r.Observation == nil || r.Observation.Overlay != "command_palette" {
		t.Fatalf("open-palette response unexpected: %s", lines[0])
	}
	// The parked blink future should register as pending async on the live
	// driver (nonzero), confirming there is stale state a reset must not carry.
	if r := decode(lines[1]); !r.OK || r.Observation == nil || r.Observation.PendingAsync == 0 {
		t.Fatalf("expected nonzero pending_async while palette is open: %s", lines[1])
	}

	// Reset must return a fresh Library driver with no overlay and no pending
	// async carried over from the replaced driver.
	reset := decode(lines[2])
	if !reset.OK || reset.Observation == nil {
		t.Fatalf("reset response unexpected: %s", lines[2])
	}
	if reset.Observation.PrimaryView != "Library" || reset.Observation.Overlay != "" {
		t.Fatalf("reset did not return a fresh driver: %s", lines[2])
	}
	if reset.Observation.PendingAsync != 0 {
		t.Fatalf("reset carried stale pending_async = %d: %s", reset.Observation.PendingAsync, lines[2])
	}

	// The post-reset observation confirms the clean state persists.
	if r := decode(lines[3]); !r.OK || r.Observation == nil ||
		r.Observation.Overlay != "" || r.Observation.PendingAsync != 0 {
		t.Fatalf("post-reset observe unexpected: %s", lines[3])
	}
	if r := decode(lines[4]); !r.OK || r.Type != "quit" {
		t.Errorf("quit response unexpected: %s", lines[4])
	}
}

func TestTUITestSession_ActionError(t *testing.T) {
	input := `{"type": "action", "action": {"assert": {"view": "Queue"}}}` + "\n"
	code, stdout, _ := runCLI(t, input, "session")
	if code != tuiTestExitOK {
		t.Fatalf("exit = %d", code)
	}
	lines := splitNonEmptyLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response, got %d", len(lines))
	}
	var resp sessionResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.OK || resp.Error == "" {
		t.Errorf("expected failing action response, got %s", lines[0])
	}
}

func TestTUITestSession_BadRequestLine(t *testing.T) {
	input := "not json at all\n" + `{"type": "quit"}` + "\n"
	code, stdout, _ := runCLI(t, input, "session")
	if code != tuiTestExitOK {
		t.Fatalf("exit = %d", code)
	}
	lines := splitNonEmptyLines(stdout)
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d:\n%s", len(lines), stdout)
	}
	var first sessionResponse
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if first.OK || first.Error == "" {
		t.Errorf("expected invalid-request error, got %s", lines[0])
	}
}

// TestTUITestSession_CommandsFlag verifies the opt-in --commands flag surfaces
// enabled palette command IDs in session observations.
func TestTUITestSession_CommandsFlag(t *testing.T) {
	input := strings.Join([]string{
		`{"type": "observe"}`,
		`{"type": "quit"}`,
	}, "\n") + "\n"

	code, stdout, stderr := runCLI(t, input, "session", "--commands")
	if code != tuiTestExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}

	lines := splitNonEmptyLines(stdout)
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d:\n%s", len(lines), stdout)
	}

	var resp sessionResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("decode observe: %v", err)
	}
	if !resp.OK || resp.Observation == nil {
		t.Fatalf("observe response unexpected: %s", lines[0])
	}
	if len(resp.Observation.AvailableCommands) == 0 {
		t.Fatalf("expected available_commands to be populated with --commands: %s", lines[0])
	}
	found := false
	for _, id := range resp.Observation.AvailableCommands {
		if id == "ui.toggle.footer_hints" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ui.toggle.footer_hints in available_commands, got %v", resp.Observation.AvailableCommands)
	}
}

// TestTUITestSession_CommandsOmittedByDefault verifies that without --commands
// the session observation leaves available_commands empty so default NDJSON
// payloads stay unchanged.
func TestTUITestSession_CommandsOmittedByDefault(t *testing.T) {
	input := strings.Join([]string{
		`{"type": "observe"}`,
		`{"type": "quit"}`,
	}, "\n") + "\n"

	code, stdout, stderr := runCLI(t, input, "session")
	if code != tuiTestExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}

	lines := splitNonEmptyLines(stdout)
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d:\n%s", len(lines), stdout)
	}

	var resp sessionResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("decode observe: %v", err)
	}
	if !resp.OK || resp.Observation == nil {
		t.Fatalf("observe response unexpected: %s", lines[0])
	}
	if len(resp.Observation.AvailableCommands) != 0 {
		t.Errorf("expected available_commands omitted by default, got %v", resp.Observation.AvailableCommands)
	}
	// The raw JSON line should not carry the field at all (omitempty).
	if strings.Contains(lines[0], "available_commands") {
		t.Errorf("default observe should omit available_commands key: %s", lines[0])
	}
}

// TestGlobalUsageOmitsTUITest guards the hidden-command contract: normal
// `muse --help` usage must not advertise the tui-test harness. configureUsage
// writes directly to os.Stderr, so the test captures it through a pipe.
func TestGlobalUsageOmitsTUITest(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	savedStderr := os.Stderr
	savedUsage := flag.Usage
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = savedStderr
		flag.Usage = savedUsage
	})

	configureUsage()
	flag.Usage()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read usage: %v", err)
	}
	usage := string(out)
	if strings.Contains(usage, "tui-test") {
		t.Errorf("normal usage must not mention tui-test:\n%s", usage)
	}
	// Sanity: the usage should still list a real command.
	if !strings.Contains(usage, "check") {
		t.Errorf("usage unexpectedly missing 'check':\n%s", usage)
	}
}

func splitNonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
