package uitest

// This file implements the in-process driver for the hidden `muse tui-test`
// layout harness. A UITestDriver owns a single fixture-backed tui.Model and
// drives it directly: it applies resizes, key presses, and text, drains the
// tea.Cmd values the model returns under bounded timeouts, evaluates
// assertions, and renders normalized observations. It performs no real I/O
// beyond what the fixture's in-memory fakes allow.

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui"
	"github.com/sglavoie/muse/internal/tui/palette"
)

// defaultDrainTimeout bounds how long a single dispatched command is allowed to
// run before it is parked as a pending future.
const defaultDrainTimeout = 50 * time.Millisecond

// maxDrainSteps caps how many commands one dispatch may drain, preventing an
// infinite command chain from hanging the harness.
const maxDrainSteps = 256

// ObserveOptions controls the optional, heavier fields of an observation.
type ObserveOptions struct {
	// IncludeANSI attaches the raw ANSI-styled screen to the observation.
	IncludeANSI bool
	// IncludeCommands attaches the list of currently-available palette command
	// identifiers to the observation.
	IncludeCommands bool
}

// UITestDriver drives one fixture-backed Model in process. It is not safe for
// concurrent use; a scenario runs its actions sequentially on a single driver.
type UITestDriver struct {
	model        tui.Model
	width        int
	height       int
	drainTimeout time.Duration

	// pending holds in-flight command futures that did not resolve within the
	// drain timeout. A wait action revisits them.
	pending []<-chan tea.Msg
}

// NewUITestDriver builds a driver around model and applies the initial size.
// A non-positive drainTimeout falls back to defaultDrainTimeout.
func NewUITestDriver(model tui.Model, width, height int, drainTimeout time.Duration) *UITestDriver {
	if drainTimeout <= 0 {
		drainTimeout = defaultDrainTimeout
	}
	d := &UITestDriver{model: model, drainTimeout: drainTimeout}
	d.resize(width, height)
	return d
}

// resize dispatches a new terminal size and records it for layout checks.
func (d *UITestDriver) resize(width, height int) {
	d.width = width
	d.height = height
	d.dispatch(tea.WindowSizeMsg{Width: width, Height: height})
}

// sendKey parses name and dispatches the equivalent key press.
func (d *UITestDriver) sendKey(name string) error {
	msg, err := parseKey(name)
	if err != nil {
		return err
	}
	d.dispatch(msg)
	return nil
}

// sendText types a literal string, one printable key press per rune.
func (d *UITestDriver) sendText(text string) {
	for _, r := range text {
		d.dispatch(printableKey(r, 0))
	}
}

// dispatch feeds one message through Model.Update and drains the returned cmd.
func (d *UITestDriver) dispatch(msg tea.Msg) {
	next, cmd := d.model.Update(msg)
	d.model = next.(tui.Model)
	d.drain(cmd)
}

// drain runs cmd and any commands it produces (including tea.BatchMsg
// expansions and follow-on cmds from fed-back messages) under a per-command
// timeout, capped at maxDrainSteps. Commands that time out are parked in
// pending for a later wait to resolve.
func (d *UITestDriver) drain(cmd tea.Cmd) {
	var queue []tea.Cmd
	if cmd != nil {
		queue = append(queue, cmd)
	}
	d.drainQueue(queue)
}

func (d *UITestDriver) drainQueue(queue []tea.Cmd) {
	steps := 0
	for len(queue) > 0 {
		if steps >= maxDrainSteps {
			// Safety cap reached: park whatever remains as pending futures
			// rather than looping forever.
			for _, c := range queue {
				if c != nil {
					d.pending = append(d.pending, startCmd(c))
				}
			}
			return
		}
		steps++

		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}

		ch := startCmd(c)
		select {
		case msg := <-ch:
			queue = d.consume(msg, queue)
		case <-time.After(d.drainTimeout):
			d.pending = append(d.pending, ch)
		}
	}
}

// consume handles a resolved message: batch messages expand into more queued
// commands, everything else is fed back through Update and any resulting cmd
// is queued.
func (d *UITestDriver) consume(msg tea.Msg, queue []tea.Cmd) []tea.Cmd {
	if msg == nil {
		return queue
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		return append(queue, batch...)
	}
	next, cmd := d.model.Update(msg)
	d.model = next.(tui.Model)
	if cmd != nil {
		queue = append(queue, cmd)
	}
	return queue
}

// waitPending revisits parked futures, giving each up to timeout to resolve.
// Resolved messages are fed back through the model and their follow-on commands
// drained. Futures that stay unresolved remain pending.
func (d *UITestDriver) waitPending(timeout time.Duration) {
	if timeout <= 0 {
		timeout = d.drainTimeout
	}
	if len(d.pending) == 0 {
		return
	}

	var still []<-chan tea.Msg
	var resolved []tea.Msg
	for _, ch := range d.pending {
		select {
		case msg := <-ch:
			resolved = append(resolved, msg)
		case <-time.After(timeout):
			still = append(still, ch)
		}
	}
	d.pending = still

	for _, msg := range resolved {
		d.drainQueue(d.consume(msg, nil))
	}
}

// startCmd runs c in a goroutine and returns a buffered channel that receives
// its message. The buffer lets the goroutine finish even if the future is never
// read, so a resolved command never leaks.
func startCmd(c tea.Cmd) <-chan tea.Msg {
	ch := make(chan tea.Msg, 1)
	go func() { ch <- c() }()
	return ch
}

// Release detaches this driver's pending async accounting so it reports no
// in-flight futures. Each parked future is a goroutine sending to its own
// buffered channel (see startCmd), so dropping the references lets those
// goroutines finish and be garbage-collected without leaking. Call Release when
// discarding a driver — e.g. a `muse tui-test session` reset that swaps in a
// fresh driver — so a replaced driver's stale pending state cannot be mistaken
// for the live driver's.
func (d *UITestDriver) Release() {
	d.pending = nil
}

// Observe renders the current screen and returns a normalized observation.
func (d *UITestDriver) Observe(opts ObserveOptions) UITestObservation {
	raw := d.model.View().Content
	plain := plainScreen(raw)

	obs := UITestObservation{
		CurrentView:  d.currentViewName(),
		PrimaryView:  tui.ViewName(d.model.ActiveView),
		Overlay:      d.model.ActiveOverlayName(),
		Width:        d.width,
		Height:       d.height,
		PlainScreen:  plain,
		Status:       plainScreen(d.model.RenderStatusBar()),
		LayoutIssues: detectLayoutIssues(raw, plain, d.width, d.height),
		PendingAsync: len(d.pending),
		Snapshot:     d.model.Snapshot(),
	}
	if obs.LayoutIssues == nil {
		obs.LayoutIssues = []UITestIssue{}
	}
	if opts.IncludeANSI {
		obs.AnsiScreen = raw
	}
	if opts.IncludeCommands {
		obs.AvailableCommands = d.availableCommands()
	}
	return obs
}

// currentViewName returns the active overlay name when an overlay is showing,
// otherwise the active primary view name.
func (d *UITestDriver) currentViewName() string {
	if overlay := d.model.ActiveOverlayName(); overlay != "" {
		return overlay
	}
	return tui.ViewName(d.model.ActiveView)
}

// availableCommands returns the identifiers of every currently-enabled palette
// command in build order.
func (d *UITestDriver) availableCommands() []string {
	cmds := d.model.BuildPaletteCommands()
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		if c.IsEnabled() {
			out = append(out, c.Key())
		}
	}
	return out
}

// ApplyAction runs a single validated action and returns the resulting
// observation. A non-nil error indicates the action failed (bad key, rejected
// command, failed assertion); the returned observation is the failure-point
// observation.
func (d *UITestDriver) ApplyAction(a UITestAction, opts ObserveOptions) (UITestObservation, error) {
	kind, err := a.validate()
	if err != nil {
		return d.Observe(opts), err
	}
	// An assertion is the one action that reads the model rather than driving
	// it, so it reports the very observation it judged.
	if kind == actionAssert {
		obs := d.Observe(opts)
		return obs, d.evalAssertion(a.Assert, obs)
	}
	if err := d.applyOperation(kind, a); err != nil {
		return d.Observe(opts), err
	}
	return d.Observe(opts), nil
}

// applyOperation drives the model with the one operation a validated non-assert
// action carries.
func (d *UITestDriver) applyOperation(kind actionKind, a UITestAction) error {
	switch kind {
	case actionKey:
		if err := d.sendKey(*a.Key); err != nil {
			return fmt.Errorf("key: %w", err)
		}
	case actionText:
		d.sendText(*a.Text)
	case actionResize:
		w, h, err := ParseSize(*a.Resize)
		if err != nil {
			return fmt.Errorf("resize: %w", err)
		}
		d.resize(w, h)
	case actionWait:
		d.waitPending(waitDuration(*a.Wait, d.drainTimeout))
	case actionCommand:
		return d.runCommand(*a.Command)
	}
	return nil
}

// waitDuration resolves a wait action's optional duration string, falling back
// to the driver's drain timeout when empty. The string is already validated.
func waitDuration(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if dur, err := time.ParseDuration(raw); err == nil {
		return dur
	}
	return fallback
}

// evalAssertion checks a single assertion against an observation.
func (d *UITestDriver) evalAssertion(a *UITestAssertion, obs UITestObservation) error {
	kind, err := a.kind()
	if err != nil {
		return err
	}
	switch kind {
	case assertContains:
		return wantContains("screen", obs.PlainScreen, *a.Contains)
	case assertNotContains:
		return wantNotContains("screen", obs.PlainScreen, *a.NotContains)
	case assertView:
		return wantEqualFold("primary view", obs.PrimaryView, *a.View)
	case assertOverlay:
		return wantEqualFold("overlay", obs.Overlay, *a.Overlay)
	case assertStatusContains:
		return wantContains("status", obs.Status, *a.StatusContains)
	case assertNoLayoutIssues:
		return wantNoLayoutIssues(obs.LayoutIssues)
	}
	return nil
}

// wantContains checks that the named part of the observation contains want.
func wantContains(subject, got, want string) error {
	if !strings.Contains(got, want) {
		return fmt.Errorf("expected %s to contain %q", subject, want)
	}
	return nil
}

// wantNotContains checks that the named part of the observation does not
// contain unwanted.
func wantNotContains(subject, got, unwanted string) error {
	if strings.Contains(got, unwanted) {
		return fmt.Errorf("expected %s not to contain %q", subject, unwanted)
	}
	return nil
}

// wantEqualFold checks a named observation field against want, case-insensitively.
func wantEqualFold(subject, got, want string) error {
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("expected %s %q, got %q", subject, want, got)
	}
	return nil
}

// wantNoLayoutIssues checks that the observation detected no layout problems.
func wantNoLayoutIssues(issues []UITestIssue) error {
	if len(issues) > 0 {
		return fmt.Errorf("expected no layout issues, found %d: %s", len(issues), summarizeIssues(issues))
	}
	return nil
}

// summarizeIssues renders a short, stable description of detected issues for
// assertion error messages.
func summarizeIssues(issues []UITestIssue) string {
	parts := make([]string, 0, len(issues))
	for _, iss := range issues {
		switch iss.Kind {
		case issueWidthOverflow:
			parts = append(parts, fmt.Sprintf("%s(line %d: %d>%d)", iss.Kind, iss.Line, iss.Actual, iss.Limit))
		case issueHeightOverflow:
			parts = append(parts, fmt.Sprintf("%s(%d>%d)", iss.Kind, iss.Actual, iss.Limit))
		case issueTabCharacter:
			parts = append(parts, fmt.Sprintf("%s(line %d)", iss.Kind, iss.Line))
		default:
			parts = append(parts, string(iss.Kind))
		}
	}
	return strings.Join(parts, ", ")
}

// runCommand resolves a direct command action against the palette command list
// and runs it only when it is on the fixture-safe allowlist. It rejects
// unknown, disabled, or side-effectful commands with a structured error.
func (d *UITestDriver) runCommand(name string) error {
	cmds := d.model.BuildPaletteCommands()
	var match *palette.PaletteCommand
	for i := range cmds {
		if cmds[i].Key() == name || strings.EqualFold(cmds[i].Name, name) {
			match = &cmds[i]
			break
		}
	}
	if match == nil {
		return fmt.Errorf("command %q not found", name)
	}
	if !tui.IsSafeDirectCommand(match.Key()) {
		return fmt.Errorf("command %q (%s) is not permitted in fixture mode", match.Name, match.Key())
	}
	if !match.IsEnabled() {
		return fmt.Errorf("command %q is not currently enabled", match.Name)
	}
	if match.Exec != nil {
		d.drain(match.Exec())
	}
	return nil
}
