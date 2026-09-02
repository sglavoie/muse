package uitest

// This file defines the fixture-independent JSON data contracts for the hidden
// `muse tui-test` layout harness. The contracts are deliberately decoupled from
// any model-driving or CLI behavior so later sessions (fixtures, driver/runner,
// and CLI) can import them without pulling in heavier dependencies. The JSON
// shapes are treated as stable enough for agents and CI scripts to consume.

import "github.com/sglavoie/muse/internal/tui"

// actionKind identifies which operation a UITestAction performs. Every
// action object sets exactly one of the recognized operation fields.
type actionKind string

const (
	// actionKey dispatches a single parsed key press.
	actionKey actionKind = "key"
	// actionText types a literal string into the active input.
	actionText actionKind = "text"
	// actionResize dispatches a new terminal size.
	actionResize actionKind = "resize"
	// actionWait drains pending async commands, optionally for a duration.
	actionWait actionKind = "wait"
	// actionAssert evaluates a layout/state assertion.
	actionAssert actionKind = "assert"
	// actionCommand invokes an allowlisted palette command directly.
	actionCommand actionKind = "command"
)

// UITestAction is a single scripted step in a scenario. Exactly one operation
// field is populated; pointer fields distinguish "absent" from "present but
// empty" during strict decoding. Later sessions execute these against a
// fixture-backed model.
type UITestAction struct {
	// Label is an optional human-readable tag surfaced in action errors.
	Label string `json:"label,omitempty"`

	Key     *string          `json:"key,omitempty"`
	Text    *string          `json:"text,omitempty"`
	Resize  *string          `json:"resize,omitempty"`
	Wait    *string          `json:"wait,omitempty"`
	Assert  *UITestAssertion `json:"assert,omitempty"`
	Command *string          `json:"command,omitempty"`
}

// assertionKind identifies which assertion a UITestAssertion evaluates.
type assertionKind string

const (
	// assertContains requires the plain screen to contain a substring.
	assertContains assertionKind = "contains"
	// assertNotContains requires the plain screen to omit a substring.
	assertNotContains assertionKind = "not_contains"
	// assertView requires a specific active primary view.
	assertView assertionKind = "view"
	// assertOverlay requires a specific active overlay.
	assertOverlay assertionKind = "overlay"
	// assertStatusContains requires the status line to contain a substring.
	assertStatusContains assertionKind = "status_contains"
	// assertNoLayoutIssues requires the observation to report no layout issues.
	assertNoLayoutIssues assertionKind = "no_layout_issues"
)

// UITestAssertion is a single expectation evaluated against an observation.
// Exactly one field is populated.
type UITestAssertion struct {
	Contains       *string `json:"contains,omitempty"`
	NotContains    *string `json:"not_contains,omitempty"`
	View           *string `json:"view,omitempty"`
	Overlay        *string `json:"overlay,omitempty"`
	StatusContains *string `json:"status_contains,omitempty"`
	NoLayoutIssues *bool   `json:"no_layout_issues,omitempty"`
}

// UITestScenario is a named, ordered list of actions run against one fixture at
// one initial terminal size.
type UITestScenario struct {
	Name    string         `json:"name"`
	Fixture string         `json:"fixture,omitempty"`
	Size    string         `json:"size,omitempty"`
	Actions []UITestAction `json:"actions"`
}

// UITestIssueKind enumerates the layout problems the harness can detect.
type UITestIssueKind string

const (
	// issueInvalidUTF8 flags screen content that is not valid UTF-8.
	issueInvalidUTF8 UITestIssueKind = "invalid_utf8"
	// issueWidthOverflow flags a line wider than the terminal.
	issueWidthOverflow UITestIssueKind = "width_overflow"
	// issueHeightOverflow flags more rendered lines than the terminal height.
	issueHeightOverflow UITestIssueKind = "height_overflow"
	// issueTabCharacter flags a tab, which breaks column-width arithmetic.
	issueTabCharacter UITestIssueKind = "tab_character"
	// issueBlankScreen flags a screen that is empty after stripping ANSI.
	issueBlankScreen UITestIssueKind = "blank_screen"
)

// UITestIssue is one detected layout problem. Line is 1-based and set only for
// line-scoped issues. Actual/Limit carry the observed and permitted magnitudes
// (width columns for width_overflow, line counts for height_overflow).
type UITestIssue struct {
	Kind   UITestIssueKind `json:"kind"`
	Line   int             `json:"line,omitempty"`
	Actual int             `json:"actual,omitempty"`
	Limit  int             `json:"limit,omitempty"`
	Detail string          `json:"detail,omitempty"`
}

// UITestObservation is the normalized snapshot of the model after an action.
// The driver (session 3) populates it; the shape here is the stable contract.
type UITestObservation struct {
	CurrentView       string              `json:"current_view"`
	PrimaryView       string              `json:"primary_view"`
	Overlay           string              `json:"overlay"`
	Width             int                 `json:"width"`
	Height            int                 `json:"height"`
	PlainScreen       string              `json:"plain_screen"`
	AnsiScreen        string              `json:"ansi_screen,omitempty"`
	Status            string              `json:"status"`
	LayoutIssues      []UITestIssue       `json:"layout_issues"`
	PendingAsync      int                 `json:"pending_async"`
	Snapshot          *tui.UITestSnapshot `json:"snapshot,omitempty"`
	AvailableCommands []string            `json:"available_commands,omitempty"`
}
