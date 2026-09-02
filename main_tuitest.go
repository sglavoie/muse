package main

// This file wires the hidden `muse tui-test` layout-testing harness into the
// CLI. It is intentionally omitted from normal `muse --help` output: the harness
// is an internal tool for agents and CI scripts that drive the real TUI model
// in process against deterministic, fixture-backed data. It performs no real
// I/O — no config, database, music files, mpv, Finder, or clipboard — so it is
// safe to run anywhere.
//
// runTUITestCLI is written so main-package tests can exercise every subcommand
// directly (passing explicit args and I/O streams) without forking the binary.

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sglavoie/muse/internal/tui"
	"github.com/sglavoie/muse/internal/tui/uitest"
)

// Exit codes for the tui-test harness.
const (
	tuiTestExitOK      = 0 // pass
	tuiTestExitFailure = 1 // scenario/action/assertion failure
	tuiTestExitUsage   = 2 // invalid CLI input or invalid scenario input
)

// runTUITestCLI dispatches the hidden `muse tui-test` subcommands. args are the
// tokens after `tui-test` (e.g. os.Args[2:]). It returns a process exit code and
// never calls os.Exit itself so tests can invoke it directly.
func runTUITestCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_ = printTUITestUsage(stderr)
		return tuiTestExitUsage
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "-h", "--help", "help":
		if err := printTUITestUsage(stdout); err != nil {
			return tuiTestExitUsage
		}
		return tuiTestExitOK
	case "fixtures":
		return runTUITestFixtures(rest, stdout, stderr)
	case "run":
		return runTUITestRun(rest, stdout, stderr)
	case "session":
		return runTUITestSession(rest, stdin, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "muse tui-test: unknown subcommand %q\n", sub)
		_ = printTUITestUsage(stderr)
		return tuiTestExitUsage
	}
}

// printTUITestUsage documents the harness subcommands. It is only reachable via
// `muse tui-test` itself; the top-level `muse --help` never lists it.
func printTUITestUsage(w io.Writer) error {
	_, err := fmt.Fprint(w, `Usage: muse tui-test <command> [flags]

Hidden layout-testing harness that drives the TUI in process against
fixture-backed data. No real config, database, music files, mpv, Finder, or
clipboard are touched.

Commands:
  fixtures [--json]
        List the available fixtures.
  run --scenario PATH [--fixture NAME] [--size WxH] [--json] [--observations] [--ansi] [--drain-timeout DUR]
        Run a scenario file and report the result. Exit 0 pass, 1 failure, 2 invalid input.
  session [--fixture NAME] [--size WxH] [--ansi] [--commands]
        Interactive NDJSON session: one JSON request per stdin line, one JSON
        response per stdout line. Request types: observe, action, reset, quit.

Fixtures: `+strings.Join(tui.FixtureNames(), ", ")+`
`)
	return err
}

// newTUITestFlagSet builds a subcommand flag set that reports errors to stderr
// and returns them (rather than exiting) so callers map failures to exit code 2.
func newTUITestFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("muse tui-test "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// runTUITestFixtures lists the built-in fixtures in text or JSON form.
func runTUITestFixtures(args []string, stdout, stderr io.Writer) int {
	fs := newTUITestFlagSet("fixtures", stderr)
	asJSON := fs.Bool("json", false, "emit the fixture list as JSON")
	if err := fs.Parse(args); err != nil {
		return tuiTestExitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "muse tui-test fixtures: unexpected argument %q\n", fs.Arg(0))
		return tuiTestExitUsage
	}

	names := tui.FixtureNames()
	if *asJSON {
		if err := writeJSONLine(stdout, map[string][]string{"fixtures": names}); err != nil {
			_, _ = fmt.Fprintf(stderr, "muse tui-test fixtures: %v\n", err)
			return tuiTestExitUsage
		}
		return tuiTestExitOK
	}
	for _, n := range names {
		if _, err := fmt.Fprintln(stdout, n); err != nil {
			_, _ = fmt.Fprintf(stderr, "muse tui-test fixtures: %v\n", err)
			return tuiTestExitUsage
		}
	}
	return tuiTestExitOK
}

// runTUITestRun executes a scenario file and reports its result.
func runTUITestRun(args []string, stdout, stderr io.Writer) int {
	fs := newTUITestFlagSet("run", stderr)
	scenarioPath := fs.String("scenario", "", "path to the scenario JSON file (required)")
	fixture := fs.String("fixture", "", "override the scenario fixture")
	size := fs.String("size", "", "override the terminal size (WxH)")
	asJSON := fs.Bool("json", false, "emit the full result as JSON")
	observations := fs.Bool("observations", false, "collect a per-action observation trail")
	ansi := fs.Bool("ansi", false, "include the raw ANSI screen in observations")
	drainTimeout := fs.Duration("drain-timeout", 0, "per-command drain timeout (e.g. 50ms)")
	if err := fs.Parse(args); err != nil {
		return tuiTestExitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "muse tui-test run: unexpected argument %q\n", fs.Arg(0))
		return tuiTestExitUsage
	}
	if strings.TrimSpace(*scenarioPath) == "" {
		_, _ = fmt.Fprintln(stderr, "muse tui-test run: --scenario is required")
		return tuiTestExitUsage
	}

	data, err := os.ReadFile(*scenarioPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "muse tui-test run: read scenario: %v\n", err)
		return tuiTestExitUsage
	}
	scenario, err := uitest.ParseScenario(data)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "muse tui-test run: %v\n", err)
		return tuiTestExitUsage
	}

	opts := uitest.RunOptions{
		Fixture:             *fixture,
		Size:                *size,
		IncludeANSI:         *ansi,
		CollectObservations: *observations,
		DrainTimeout:        *drainTimeout,
	}
	result, err := uitest.RunScenario(scenario, opts)
	if err != nil {
		// Setup problem (unknown fixture, malformed size): invalid input.
		_, _ = fmt.Fprintf(stderr, "muse tui-test run: %v\n", err)
		return tuiTestExitUsage
	}

	if *asJSON {
		if err := writeJSONIndented(stdout, result); err != nil {
			_, _ = fmt.Fprintf(stderr, "muse tui-test run: %v\n", err)
			return tuiTestExitUsage
		}
	} else {
		if err := writeRunSummary(stdout, result); err != nil {
			_, _ = fmt.Fprintf(stderr, "muse tui-test run: %v\n", err)
			return tuiTestExitUsage
		}
	}

	if !result.OK {
		return tuiTestExitFailure
	}
	return tuiTestExitOK
}

// writeRunSummary prints a compact human-readable pass/fail line to stdout.
func writeRunSummary(w io.Writer, result uitest.UITestScenarioResult) error {
	if result.OK {
		_, err := fmt.Fprintf(w, "PASS %q (fixture %s, %s, %d actions)\n",
			result.Scenario, result.Fixture, result.Size, result.ActionsRun)
		return err
	}
	if _, err := fmt.Fprintf(w, "FAIL %q (fixture %s, %s, %d actions run)\n",
		result.Scenario, result.Fixture, result.Size, result.ActionsRun); err != nil {
		return err
	}
	if result.Error != nil {
		label := ""
		if result.Error.Label != "" {
			label = " [" + result.Error.Label + "]"
		}
		if _, err := fmt.Fprintf(w, "  action %d%s: %s\n", result.Error.Index, label, result.Error.Reason); err != nil {
			return err
		}
	}
	return nil
}

// sessionRequest is one NDJSON request line for the interactive session mode.
type sessionRequest struct {
	Type   string               `json:"type"`
	Action *uitest.UITestAction `json:"action,omitempty"`
}

// sessionResponse is one NDJSON response line for the interactive session mode.
type sessionResponse struct {
	OK          bool                      `json:"ok"`
	Type        string                    `json:"type,omitempty"`
	Observation *uitest.UITestObservation `json:"observation,omitempty"`
	Error       string                    `json:"error,omitempty"`
}

// runTUITestSession runs an interactive NDJSON loop over stdin/stdout. Stdout
// stays pure NDJSON (one response object per line); human diagnostics go to
// stderr. Recognized request types: observe, action, reset, quit.
func runTUITestSession(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newTUITestFlagSet("session", stderr)
	fixture := fs.String("fixture", tui.FixtureRich, "fixture to drive")
	size := fs.String("size", "100x30", "terminal size (WxH)")
	ansi := fs.Bool("ansi", false, "include the raw ANSI screen in observations")
	commands := fs.Bool("commands", false, "include enabled palette command IDs in observations")
	if err := fs.Parse(args); err != nil {
		return tuiTestExitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "muse tui-test session: unexpected argument %q\n", fs.Arg(0))
		return tuiTestExitUsage
	}

	width, height, err := uitest.ParseSize(*size)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "muse tui-test session: %v\n", err)
		return tuiTestExitUsage
	}

	newDriver := func() (*uitest.UITestDriver, error) {
		model, err := tui.BuildFixture(*fixture)
		if err != nil {
			return nil, err
		}
		return uitest.NewUITestDriver(model, width, height, 0), nil
	}

	driver, err := newDriver()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "muse tui-test session: %v\n", err)
		return tuiTestExitUsage
	}

	obsOpts := uitest.ObserveOptions{IncludeANSI: *ansi, IncludeCommands: *commands}
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req sessionRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeSessionResponse(stdout, sessionResponse{OK: false, Error: fmt.Sprintf("invalid request: %v", err)})
			continue
		}

		switch req.Type {
		case "observe":
			obs := driver.Observe(obsOpts)
			writeSessionResponse(stdout, sessionResponse{OK: true, Type: "observe", Observation: &obs})
		case "action":
			if req.Action == nil {
				writeSessionResponse(stdout, sessionResponse{OK: false, Type: "action", Error: "action request missing \"action\" field"})
				continue
			}
			obs, aerr := driver.ApplyAction(*req.Action, obsOpts)
			resp := sessionResponse{OK: aerr == nil, Type: "action", Observation: &obs}
			if aerr != nil {
				resp.Error = aerr.Error()
			}
			writeSessionResponse(stdout, resp)
		case "reset":
			d, err := newDriver()
			if err != nil {
				// The fixture already built once, so this is unexpected; report
				// it but keep the existing driver so the session stays usable.
				writeSessionResponse(stdout, sessionResponse{OK: false, Type: "reset", Error: err.Error()})
				continue
			}
			// Detach the outgoing driver's pending async futures before swapping
			// so stale accounting from the replaced driver cannot bleed into the
			// fresh driver's observations.
			driver.Release()
			driver = d
			obs := driver.Observe(obsOpts)
			writeSessionResponse(stdout, sessionResponse{OK: true, Type: "reset", Observation: &obs})
		case "quit":
			writeSessionResponse(stdout, sessionResponse{OK: true, Type: "quit"})
			return tuiTestExitOK
		default:
			writeSessionResponse(stdout, sessionResponse{OK: false, Error: fmt.Sprintf("unknown request type %q", req.Type)})
		}
	}
	if err := scanner.Err(); err != nil {
		_, _ = fmt.Fprintf(stderr, "muse tui-test session: read: %v\n", err)
		return tuiTestExitUsage
	}
	return tuiTestExitOK
}

// writeSessionResponse marshals resp as a single NDJSON line on stdout.
func writeSessionResponse(w io.Writer, resp sessionResponse) {
	if err := writeJSONLine(w, resp); err != nil {
		// Fall back to a minimal error line so the peer still sees a response.
		_, _ = fmt.Fprintf(w, "{\"ok\":false,\"error\":%q}\n", err.Error())
	}
}

// writeJSONLine writes v as one compact JSON line (NDJSON).
func writeJSONLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// writeJSONIndented writes v as pretty-printed JSON followed by a newline.
func writeJSONIndented(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}
