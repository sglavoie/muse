package tui

// This file defines the fixture-independent JSON data contracts for the hidden
// `muse tui-test` layout harness. The contracts are deliberately decoupled from
// any model-driving or CLI behavior so later sessions (fixtures, driver/runner,
// and CLI) can import them without pulling in heavier dependencies. The JSON
// shapes are treated as stable enough for agents and CI scripts to consume.

// UITestActionKind identifies which operation a UITestAction performs. Every
// action object sets exactly one of the recognized operation fields.
type UITestActionKind string

const (
	// ActionKey dispatches a single parsed key press.
	ActionKey UITestActionKind = "key"
	// ActionText types a literal string into the active input.
	ActionText UITestActionKind = "text"
	// ActionResize dispatches a new terminal size.
	ActionResize UITestActionKind = "resize"
	// ActionWait drains pending async commands, optionally for a duration.
	ActionWait UITestActionKind = "wait"
	// ActionAssert evaluates a layout/state assertion.
	ActionAssert UITestActionKind = "assert"
	// ActionCommand invokes an allowlisted palette command directly.
	ActionCommand UITestActionKind = "command"
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

// UITestAssertionKind identifies which assertion a UITestAssertion evaluates.
type UITestAssertionKind string

const (
	// AssertContains requires the plain screen to contain a substring.
	AssertContains UITestAssertionKind = "contains"
	// AssertNotContains requires the plain screen to omit a substring.
	AssertNotContains UITestAssertionKind = "not_contains"
	// AssertView requires a specific active primary view.
	AssertView UITestAssertionKind = "view"
	// AssertOverlay requires a specific active overlay.
	AssertOverlay UITestAssertionKind = "overlay"
	// AssertStatusContains requires the status line to contain a substring.
	AssertStatusContains UITestAssertionKind = "status_contains"
	// AssertNoLayoutIssues requires the observation to report no layout issues.
	AssertNoLayoutIssues UITestAssertionKind = "no_layout_issues"
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
	// IssueInvalidUTF8 flags screen content that is not valid UTF-8.
	IssueInvalidUTF8 UITestIssueKind = "invalid_utf8"
	// IssueWidthOverflow flags a line wider than the terminal.
	IssueWidthOverflow UITestIssueKind = "width_overflow"
	// IssueHeightOverflow flags more rendered lines than the terminal height.
	IssueHeightOverflow UITestIssueKind = "height_overflow"
	// IssueTabCharacter flags a tab, which breaks column-width arithmetic.
	IssueTabCharacter UITestIssueKind = "tab_character"
	// IssueBlankScreen flags a screen that is empty after stripping ANSI.
	IssueBlankScreen UITestIssueKind = "blank_screen"
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

// UITestSnapshot is a compact, high-signal digest of model state attached to an
// observation. Session 2 (Fixtures And Snapshots) owns populating these fields
// and may extend this struct; the fields below are the stable core shared with
// observations and are all optional so partial snapshots serialize cleanly.
type UITestSnapshot struct {
	PrimaryView    string `json:"primary_view,omitempty"`
	Overlay        string `json:"overlay,omitempty"`
	Level          string `json:"level,omitempty"`
	Mode           string `json:"mode,omitempty"`
	Cursor         int    `json:"cursor,omitempty"`
	ScrollOffset   int    `json:"scroll_offset,omitempty"`
	VisibleCount   int    `json:"visible_count,omitempty"`
	FocusedLabel   string `json:"focused_label,omitempty"`
	FocusedHash    string `json:"focused_hash,omitempty"`
	QueueLength    int    `json:"queue_length,omitempty"`
	CurrentIndex   int    `json:"current_index,omitempty"`
	SelectionCount int    `json:"selection_count,omitempty"`
}

// UITestObservation is the normalized snapshot of the model after an action.
// The driver (session 3) populates it; the shape here is the stable contract.
type UITestObservation struct {
	CurrentView       string          `json:"current_view"`
	PrimaryView       string          `json:"primary_view"`
	Overlay           string          `json:"overlay"`
	Width             int             `json:"width"`
	Height            int             `json:"height"`
	PlainScreen       string          `json:"plain_screen"`
	AnsiScreen        string          `json:"ansi_screen,omitempty"`
	Status            string          `json:"status"`
	LayoutIssues      []UITestIssue   `json:"layout_issues"`
	PendingAsync      int             `json:"pending_async"`
	Snapshot          *UITestSnapshot `json:"snapshot,omitempty"`
	AvailableCommands []string        `json:"available_commands,omitempty"`
}
