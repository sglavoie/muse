package main

import (
	"encoding/json"
	"strings"
	"testing"
)

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
