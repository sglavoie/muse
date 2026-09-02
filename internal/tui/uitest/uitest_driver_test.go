package uitest

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// sp / bp build pointers for constructing scenario actions and assertions in
// tests, mirroring the pointer-optional contract fields.
func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }

// newTestDriver builds a fixture-backed driver at the given size for tests.
func newTestDriver(t *testing.T, fixture string, w, h int) *UITestDriver {
	t.Helper()
	model, err := tui.BuildFixture(fixture)
	if err != nil {
		t.Fatalf("BuildFixture(%q): %v", fixture, err)
	}
	return NewUITestDriver(model, w, h, 20*time.Millisecond)
}

func TestDriverResizeAndRender(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)

	obs := d.Observe(ObserveOptions{})
	if obs.Width != 100 || obs.Height != 30 {
		t.Fatalf("size = %dx%d, want 100x30", obs.Width, obs.Height)
	}
	if strings.TrimSpace(obs.PlainScreen) == "" {
		t.Fatal("expected non-blank screen after initial resize")
	}
	if obs.PrimaryView != "Library" {
		t.Fatalf("primary view = %q, want Library", obs.PrimaryView)
	}
	if obs.Snapshot == nil {
		t.Fatal("expected snapshot to be attached")
	}

	d.Resize(80, 24)
	obs = d.Observe(ObserveOptions{})
	if obs.Width != 80 || obs.Height != 24 {
		t.Fatalf("resized size = %dx%d, want 80x24", obs.Width, obs.Height)
	}
}

func TestDriverKeyNavigation(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)

	if err := d.SendKey("3"); err != nil {
		t.Fatalf("SendKey(3): %v", err)
	}
	if got := d.Observe(ObserveOptions{}).PrimaryView; got != "Queue" {
		t.Fatalf("after '3', primary view = %q, want Queue", got)
	}

	if err := d.SendKey("6"); err != nil {
		t.Fatalf("SendKey(6): %v", err)
	}
	if got := d.Observe(ObserveOptions{}).PrimaryView; got != "Tags" {
		t.Fatalf("after '6', primary view = %q, want Tags", got)
	}
}

func TestDriverTextInputThroughCommandPalette(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)

	if err := d.SendKey(":"); err != nil {
		t.Fatalf("SendKey(:): %v", err)
	}
	obs := d.Observe(ObserveOptions{})
	if obs.Overlay != "command_palette" {
		t.Fatalf("overlay = %q, want command_palette", obs.Overlay)
	}

	d.SendText("footer")
	obs = d.Observe(ObserveOptions{})
	if !strings.Contains(obs.PlainScreen, "footer") {
		t.Fatalf("expected typed query 'footer' on screen:\n%s", obs.PlainScreen)
	}
}

func TestDriverCommandDrainAndPendingAsync(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)
	d.drainTimeout = 5 * time.Millisecond

	release := make(chan tea.Msg)
	slow := func() tea.Msg { return <-release }

	d.drain(slow)
	if len(d.pending) != 1 {
		t.Fatalf("pending = %d, want 1 after slow command times out", len(d.pending))
	}
	if got := d.Observe(ObserveOptions{}).PendingAsync; got != 1 {
		t.Fatalf("PendingAsync = %d, want 1", got)
	}

	// Resolve the parked future and confirm the message is fed back through the
	// model (a notification lands in the status bar) and pending clears.
	go func() { release <- statusbar.TransientNotifyMsg("late-arrival") }()
	d.waitPending(200 * time.Millisecond)
	if len(d.pending) != 0 {
		t.Fatalf("pending = %d, want 0 after wait resolves it", len(d.pending))
	}
	obs := d.Observe(ObserveOptions{})
	if obs.PendingAsync != 0 {
		t.Fatalf("PendingAsync = %d, want 0 after wait", obs.PendingAsync)
	}
	if !strings.Contains(obs.Status, "late-arrival") {
		t.Fatalf("expected resolved notification in status, got %q", obs.Status)
	}
}

func TestDriverWaitActionResolvesPending(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)
	d.drainTimeout = 5 * time.Millisecond

	d.drain(func() tea.Msg {
		time.Sleep(25 * time.Millisecond)
		return statusbar.TransientNotifyMsg("ready")
	})
	if len(d.pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(d.pending))
	}

	obs, err := d.ApplyAction(UITestAction{Wait: sp("100ms")}, ObserveOptions{})
	if err != nil {
		t.Fatalf("wait action: %v", err)
	}
	if obs.PendingAsync != 0 {
		t.Fatalf("PendingAsync = %d, want 0 after wait action", obs.PendingAsync)
	}
}

func TestDriverReleaseClearsPending(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)
	d.drainTimeout = 5 * time.Millisecond

	// Park a future that never resolves so the driver holds pending accounting.
	release := make(chan tea.Msg)
	d.drain(func() tea.Msg { return <-release })
	if len(d.pending) != 1 {
		t.Fatalf("pending = %d, want 1 before release", len(d.pending))
	}
	if got := d.Observe(ObserveOptions{}).PendingAsync; got != 1 {
		t.Fatalf("PendingAsync = %d, want 1 before release", got)
	}

	// Release detaches the parked future so the driver reports a clean slate.
	d.Release()
	if len(d.pending) != 0 {
		t.Fatalf("pending = %d, want 0 after release", len(d.pending))
	}
	if got := d.Observe(ObserveOptions{}).PendingAsync; got != 0 {
		t.Fatalf("PendingAsync = %d, want 0 after release", got)
	}

	// The detached goroutine can still complete without blocking or leaking.
	close(release)
}

func TestDriverAssertionPasses(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)

	cases := []UITestAssertion{
		{View: sp("Library")},
		{Contains: sp("Track A1")},
		{NoLayoutIssues: bp(true)},
	}
	for i, a := range cases {
		assertion := a
		obs := d.Observe(ObserveOptions{})
		if err := d.evalAssertion(&assertion, obs); err != nil {
			t.Fatalf("case %d: unexpected assertion failure: %v", i, err)
		}
	}
}

func TestDriverAssertionFailures(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)
	obs := d.Observe(ObserveOptions{})

	cases := []UITestAssertion{
		{Contains: sp("this-text-should-not-appear-anywhere")},
		{View: sp("Mixes")},
		{Overlay: sp("help")},
		{StatusContains: sp("zzz-not-in-status-zzz")},
	}
	for i, a := range cases {
		assertion := a
		if err := d.evalAssertion(&assertion, obs); err == nil {
			t.Fatalf("case %d: expected assertion failure, got nil", i)
		}
	}
}

func TestDriverNoLayoutIssuesReportsDetectedIssue(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)
	obs := UITestObservation{
		LayoutIssues: []UITestIssue{{
			Kind:   IssueWidthOverflow,
			Line:   1,
			Actual: 61,
			Limit:  60,
		}},
	}

	err := d.evalAssertion(&UITestAssertion{NoLayoutIssues: bp(true)}, obs)
	if err == nil {
		t.Fatal("expected no_layout_issues assertion to fail when issues are present")
	}
	if !strings.Contains(err.Error(), "width_overflow") {
		t.Fatalf("expected failure to summarize layout issue, got %v", err)
	}
}

func TestDriverSafeCommandAllowlist(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)

	// Accepted: opening the debug panel is a safe internal command.
	if _, err := d.ApplyAction(UITestAction{Command: sp("Show debug panel")}, ObserveOptions{}); err != nil {
		t.Fatalf("safe command by name: %v", err)
	}
	if got := d.Observe(ObserveOptions{}).Overlay; got != "debug" {
		t.Fatalf("overlay = %q, want debug", got)
	}

	// Accepted by ID: toggling footer hints notifies the status bar.
	if _, err := d.ApplyAction(UITestAction{Command: sp("ui.toggle.footer_hints")}, ObserveOptions{}); err != nil {
		t.Fatalf("safe command by id: %v", err)
	}
	if got := d.Observe(ObserveOptions{}).Status; !strings.Contains(got, "Footer hints:") {
		t.Fatalf("status = %q, want it to contain 'Footer hints:'", got)
	}
}

func TestDriverRejectsUnsafeAndUnknownCommands(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)

	// A Finder-revealing command is present (a track is focused) but must be
	// rejected as side-effectful.
	_, err := d.ApplyAction(UITestAction{Command: sp("Reveal in Finder")}, ObserveOptions{})
	if err == nil {
		t.Fatal("expected 'Reveal in Finder' to be rejected")
	}
	if !strings.Contains(err.Error(), "not permitted") {
		t.Fatalf("rejection error = %v, want 'not permitted'", err)
	}

	// An unknown command name is a structured failure, distinct from rejection.
	_, err = d.ApplyAction(UITestAction{Command: sp("no-such-command")}, ObserveOptions{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown command error = %v, want 'not found'", err)
	}
}

func TestDriverKeyDrivenPaletteRejectsUnsafeCommand(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)

	// Open the command palette, filter to the side-effectful "Reveal in Finder"
	// command, and select it with enter — the exact path a scenario could use to
	// bypass the direct-command allowlist.
	if err := d.SendKey(":"); err != nil {
		t.Fatalf("SendKey(:): %v", err)
	}
	if got := d.Observe(ObserveOptions{}).Overlay; got != "command_palette" {
		t.Fatalf("overlay = %q, want command_palette", got)
	}

	d.SendText("Reveal in Finder")
	if err := d.SendKey("enter"); err != nil {
		t.Fatalf("SendKey(enter): %v", err)
	}

	obs := d.Observe(ObserveOptions{})
	// Fixture safety must close the palette and surface a clear notification
	// rather than executing revealFile against the real Finder.
	if obs.Overlay == "command_palette" {
		t.Fatalf("expected palette to close after rejected selection, overlay = %q", obs.Overlay)
	}
	if !strings.Contains(obs.Status, "blocked in fixture mode") {
		t.Fatalf("expected fixture-safety notification in status, got %q", obs.Status)
	}
}

func TestDriverKeyDrivenPaletteRunsSafeCommand(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)

	// The same key-driven path must still run a fixture-safe command so normal
	// palette selection is unaffected by the safety gate.
	if err := d.SendKey(":"); err != nil {
		t.Fatalf("SendKey(:): %v", err)
	}
	d.SendText("Toggle footer hints")
	if err := d.SendKey("enter"); err != nil {
		t.Fatalf("SendKey(enter): %v", err)
	}

	obs := d.Observe(ObserveOptions{})
	if obs.Overlay == "command_palette" {
		t.Fatalf("expected palette to close after selection, overlay = %q", obs.Overlay)
	}
	if !strings.Contains(obs.Status, "Footer hints:") {
		t.Fatalf("expected safe command to run, status = %q", obs.Status)
	}
}

func TestDriverObserveAvailableCommands(t *testing.T) {
	d := newTestDriver(t, tui.FixtureRich, 100, 30)
	obs := d.Observe(ObserveOptions{IncludeCommands: true})
	if len(obs.AvailableCommands) == 0 {
		t.Fatal("expected available commands to be populated when requested")
	}
	found := false
	for _, id := range obs.AvailableCommands {
		if id == "ui.toggle.footer_hints" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ui.toggle.footer_hints in available commands: %v", obs.AvailableCommands)
	}
}
