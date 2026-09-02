package tui

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
	model        Model
	width        int
	height       int
	drainTimeout time.Duration

	// pending holds in-flight command futures that did not resolve within the
	// drain timeout. A wait action revisits them.
	pending []<-chan tea.Msg
}

// NewUITestDriver builds a driver around model and applies the initial size.
// A non-positive drainTimeout falls back to defaultDrainTimeout.
func NewUITestDriver(model Model, width, height int, drainTimeout time.Duration) *UITestDriver {
	if drainTimeout <= 0 {
		drainTimeout = defaultDrainTimeout
	}
	d := &UITestDriver{model: model, drainTimeout: drainTimeout}
	d.Resize(width, height)
	return d
}

// Resize dispatches a new terminal size and records it for layout checks.
func (d *UITestDriver) Resize(width, height int) {
	d.width = width
	d.height = height
	d.dispatch(tea.WindowSizeMsg{Width: width, Height: height})
}

// SendKey parses name and dispatches the equivalent key press.
func (d *UITestDriver) SendKey(name string) error {
	msg, err := ParseKey(name)
	if err != nil {
		return err
	}
	d.dispatch(msg)
	return nil
}

// SendText types a literal string, one printable key press per rune.
func (d *UITestDriver) SendText(text string) {
	for _, r := range text {
		d.dispatch(printableKey(r, 0))
	}
}

// dispatch feeds one message through Model.Update and drains the returned cmd.
func (d *UITestDriver) dispatch(msg tea.Msg) {
	next, cmd := d.model.Update(msg)
	d.model = next.(Model)
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
	d.model = next.(Model)
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
	plain := PlainScreen(raw)

	obs := UITestObservation{
		CurrentView:  d.currentViewName(),
		PrimaryView:  viewName(d.model.activeView),
		Overlay:      d.model.activeOverlayName(),
		Width:        d.width,
		Height:       d.height,
		PlainScreen:  plain,
		Status:       PlainScreen(d.model.renderStatusBar()),
		LayoutIssues: DetectLayoutIssues(raw, plain, d.width, d.height),
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
	if overlay := d.model.activeOverlayName(); overlay != "" {
		return overlay
	}
	return viewName(d.model.activeView)
}

// availableCommands returns the identifiers of every currently-enabled palette
// command in build order.
func (d *UITestDriver) availableCommands() []string {
	cmds := d.model.buildPaletteCommands()
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
	kind, err := a.Validate()
	if err != nil {
		return d.Observe(opts), err
	}

	switch kind {
	case ActionKey:
		if err := d.SendKey(*a.Key); err != nil {
			return d.Observe(opts), fmt.Errorf("key: %w", err)
		}
	case ActionText:
		d.SendText(*a.Text)
	case ActionResize:
		w, h, err := ParseSize(*a.Resize)
		if err != nil {
			return d.Observe(opts), fmt.Errorf("resize: %w", err)
		}
		d.Resize(w, h)
	case ActionWait:
		d.waitPending(waitDuration(*a.Wait, d.drainTimeout))
	case ActionCommand:
		if err := d.runCommand(*a.Command); err != nil {
			return d.Observe(opts), err
		}
	case ActionAssert:
		obs := d.Observe(opts)
		return obs, d.evalAssertion(a.Assert, obs)
	}

	return d.Observe(opts), nil
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
	kind, err := a.Kind()
	if err != nil {
		return err
	}
	switch kind {
	case AssertContains:
		if !strings.Contains(obs.PlainScreen, *a.Contains) {
			return fmt.Errorf("expected screen to contain %q", *a.Contains)
		}
	case AssertNotContains:
		if strings.Contains(obs.PlainScreen, *a.NotContains) {
			return fmt.Errorf("expected screen not to contain %q", *a.NotContains)
		}
	case AssertView:
		if !strings.EqualFold(obs.PrimaryView, *a.View) {
			return fmt.Errorf("expected primary view %q, got %q", *a.View, obs.PrimaryView)
		}
	case AssertOverlay:
		if !strings.EqualFold(obs.Overlay, *a.Overlay) {
			return fmt.Errorf("expected overlay %q, got %q", *a.Overlay, obs.Overlay)
		}
	case AssertStatusContains:
		if !strings.Contains(obs.Status, *a.StatusContains) {
			return fmt.Errorf("expected status to contain %q", *a.StatusContains)
		}
	case AssertNoLayoutIssues:
		if len(obs.LayoutIssues) > 0 {
			return fmt.Errorf("expected no layout issues, found %d: %s", len(obs.LayoutIssues), summarizeIssues(obs.LayoutIssues))
		}
	}
	return nil
}

// summarizeIssues renders a short, stable description of detected issues for
// assertion error messages.
func summarizeIssues(issues []UITestIssue) string {
	parts := make([]string, 0, len(issues))
	for _, iss := range issues {
		switch iss.Kind {
		case IssueWidthOverflow:
			parts = append(parts, fmt.Sprintf("%s(line %d: %d>%d)", iss.Kind, iss.Line, iss.Actual, iss.Limit))
		case IssueHeightOverflow:
			parts = append(parts, fmt.Sprintf("%s(%d>%d)", iss.Kind, iss.Actual, iss.Limit))
		case IssueTabCharacter:
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
	cmds := d.model.buildPaletteCommands()
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
	if !isSafeDirectCommand(match.Key()) {
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

// safeDirectCommandIDs is the allowlist of palette command identifiers that are
// safe to invoke directly against a fixture: internal navigation and UI/state
// toggles whose handlers never touch real config, databases, music files, mpv,
// Finder, or the clipboard. Every ID here must exist in the built command set;
// TestSafeDirectCommandIDsExist catches a renamed catalog ID that would
// otherwise silently unlist a fixture-safe command.
func safeDirectCommandIDs() map[string]struct{} {
	return map[string]struct{}{
		"help.show":              {},
		"help.modes_legend":      {},
		"debug.show":             {},
		"radio.toggle":           {},
		"nav.playlists":          {},
		"nav.queue":              {},
		"nav.favorites":          {},
		"nav.tags":               {},
		"nav.mixes":              {},
		"nav.library.all":        {},
		"nav.library.recent":     {},
		"nav.library.history":    {},
		"nav.browse.artists":     {},
		"nav.browse.albums":      {},
		"nav.browse.genres":      {},
		"ui.toggle.footer_hints": {},
		"ui.toggle.track_notify": {},
		"queue.toggle.follow":    {},
	}
}

// isSafeDirectCommand reports whether the palette command identifier is on the
// fixture-safe direct-command allowlist.
func isSafeDirectCommand(id string) bool {
	_, ok := safeDirectCommandIDs()[id]
	return ok
}
