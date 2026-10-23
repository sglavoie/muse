package palette

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// PalettePin identifies footer commands kept out of the unfiltered task list.
// The legacy Top/Bottom roles identify Help and Quit respectively.
type PalettePin int

const (
	PinNone   PalettePin = iota
	PinTop               // Help (F1)
	PinBottom            // Quit (ctrl+c)
)

// CommandIntent orders an empty-query palette by the kind of task. Search
// favors inspection for broad topic queries; usage ranks commands within a tier.
type CommandIntent int

const (
	IntentOrdinary CommandIntent = iota
	IntentListening
	IntentRemoval
	IntentInspect // navigation and read-only information
)

// PaletteCommand describes a single entry in the command palette.
type PaletteCommand struct {
	ID             string // stable identifier for usage tracking; falls back to Name when empty
	Name           string
	Aliases        []string // search-only synonyms; display-name matches rank first
	Category       string   // short tag rendered dim before Name, from paletteCategories; empty for pins / uncategorized
	Shortcut       string   // optional keyboard shortcut shown right-aligned in the palette
	Pinned         PalettePin
	Exec           func() tea.Cmd
	Enabled        func() bool // nil means always enabled
	DisabledReason string      // recovery hint shown when the focused command is unavailable
	Detail         string      // explanatory text shown when the command is focused
	CurrentState   string      // setting shown in the detail line when focused
	Intent         CommandIntent
	Relevant       bool // belongs to the current view; boosts the empty-query list
}

// Key returns the stable key used to identify the command in the usage store.
func (c PaletteCommand) Key() string {
	if c.ID != "" {
		return c.ID
	}
	return c.Name
}

func (c PaletteCommand) IsEnabled() bool {
	return c.Enabled == nil || c.Enabled()
}

// CommandPalettePickedMsg is dispatched when the user confirms a command.
type CommandPalettePickedMsg struct {
	Command PaletteCommand
}

// CommandPaletteDismissedMsg is dispatched when the user dismisses the palette.
type CommandPaletteDismissedMsg struct{}

// CommandPalette is the overlay sub-model for the searchable command palette.
type CommandPalette struct {
	all       []PaletteCommand // full command list
	Filtered  []PaletteCommand // ranked matching commands
	cursor    int
	Filter    filter.State // always-active text input
	usage     *UsageStore
	lastQuery string // trimmed query from previous refilter; used to reset cursor on query change
	hasQuery  bool   // true once a refilter has run (distinguishes initial state from "" query change)
}

// NewCommandPalette creates a CommandPalette with the given commands. When
// usage is non-nil the initial body order (empty filter) is most-used first;
// otherwise the order is alphabetical within each intent tier. Help and Quit
// live in the footer and remain searchable. Text input is activated immediately.
func NewCommandPalette(commands []PaletteCommand, usage *UsageStore) (CommandPalette, tea.Cmd) {
	cp := CommandPalette{
		all:   commands,
		usage: usage,
	}
	var cmd tea.Cmd
	cp.Filter, cmd = filter.New().Activate()
	cp.Refilter()
	cp.cursor = 0
	return cp, cmd
}

// paletteCommandID returns the stable key for a command (Key() falls back to
// Name when ID is empty).
func paletteCommandID(c PaletteCommand) string { return c.Key() }

// paletteCommandName returns the display name used for fuzzy matching.
func paletteCommandName(c PaletteCommand) string { return c.Name }

// HandleKey processes a key press and returns the updated palette and optional Cmd.
func (cp CommandPalette) HandleKey(key string, msg tea.KeyPressMsg, heights ...int) (CommandPalette, tea.Cmd) {
	switch key {
	case "f1":
		return cp, cp.pickHelp()
	case "esc":
		return cp, func() tea.Msg { return CommandPaletteDismissedMsg{} }
	case "enter":
		return cp, cp.runFocused()
	case "up", "ctrl+k", "ctrl+p":
		return cp.prevCommand(), nil
	case "down", "ctrl+j", "ctrl+n":
		return cp.nextCommand(), nil
	case "pgup", "pgdown":
		return cp.page(key, heights), nil
	default:
		var cmd tea.Cmd
		cp.Filter, cmd = cp.Filter.Update(msg)
		cp.Refilter()
		return cp, cmd
	}
}

// runFocused records usage for the focused command and returns the Cmd that
// dispatches it. Returns nil when nothing runnable is focused — an out-of-range
// cursor or a command whose Enabled predicate is false.
func (cp CommandPalette) runFocused() tea.Cmd {
	if cp.cursor < 0 || cp.cursor >= len(cp.Filtered) {
		return nil
	}
	picked := cp.Filtered[cp.cursor]
	if !picked.IsEnabled() {
		return nil
	}
	if cp.usage != nil {
		cp.usage.Bump(picked.Key())
		cp.usage.SaveBestEffort()
	}
	return func() tea.Msg { return CommandPalettePickedMsg{Command: picked} }
}

// prevCommand moves the selection up one row, stopping at the first.
func (cp CommandPalette) prevCommand() CommandPalette {
	if cp.cursor > 0 {
		cp.cursor--
	}
	return cp
}

// nextCommand moves the selection down one row, stopping at the last.
func (cp CommandPalette) nextCommand() CommandPalette {
	if cp.cursor < len(cp.Filtered)-1 {
		cp.cursor++
	}
	return cp
}

// Refilter rebuilds the filtered list using subsequence-fuzzy matching with
// MRU+frequency weighting, then clamps the cursor. Search includes footer
// commands and ranks all commands by match.
func (cp *CommandPalette) Refilter() {
	query := strings.TrimSpace(cp.Filter.Text)

	body := cp.commandsForQuery(query)
	body = rank(body, query, paletteCommandID, paletteCommandName, func(c PaletteCommand) []string { return c.Aliases }, cp.usage)
	if query == "" {
		slices.SortStableFunc(body, func(a, b PaletteCommand) int {
			return commandPriority(b) - commandPriority(a)
		})
	} else if isTopicQuery(body, query) {
		slices.SortStableFunc(body, func(a, b PaletteCommand) int {
			return topicPriority(b, query) - topicPriority(a, query)
		})
	}

	cp.Filtered = body

	// When the query changes, reset the cursor to the first body match so the
	// user lands on the highest-scoring result rather than whatever entry
	// happens to share the old index after the result list changes.
	if cp.hasQuery && query != cp.lastQuery {
		cp.cursor = 0
	}
	cp.lastQuery = query
	cp.hasQuery = true

	if cp.cursor >= len(cp.Filtered) {
		cp.cursor = max(len(cp.Filtered)-1, 0)
	}
	if cp.cursor < 0 {
		cp.cursor = 0
	}
}

// Border characters.
const (
	cpBorderH  = "─"
	cpBorderV  = "│"
	cpCornerTL = "╭"
	cpCornerTR = "╮"
	cpCornerBL = "╰"
	cpCornerBR = "╯"
	cpCrossL   = "├"
	cpCrossR   = "┤"
)

// Render produces a horizontally centered, top-anchored overlay string for the
// palette. Only the height varies as the filter narrows the result list; the
// box keeps its width and its position at the top of the screen.
func (cp CommandPalette) Render(termWidth, termHeight int, s styles.Styles) string {
	return overlay.RenderTopLines(cp.buildBoxLines(termWidth, termHeight, s), "", termWidth)
}

const (
	cpModalTitle = " Command Palette "
	cpMinInner   = 30
)

// cpBoxMetrics holds the column widths one palette render is laid out against.
type cpBoxMetrics struct {
	innerWidth       int // space between the vertical borders
	contentWidth     int // space available to a row's text
	maxCategoryWidth int // widest category tag across the filtered commands
	maxShortcutWidth int // widest shortcut across the filtered commands
}

func (cp CommandPalette) buildBoxLines(termWidth, termHeight int, s styles.Styles) []string {
	return cp.compactBoxLines(termWidth, termHeight, cp.boxMetrics(termWidth), s)
}

func (c PaletteCommand) detailText() string {
	if !c.IsEnabled() {
		if c.DisabledReason != "" {
			return c.DisabledReason
		}
		return "Unavailable in this context"
	}
	if c.Detail != "" {
		return c.Detail
	}
	if c.CurrentState != "" {
		return "Current: " + c.CurrentState
	}
	return ""
}

// boxMetrics sizes the box from the title and every command's name, shortcut
// and category tag, then clamps it to the terminal width. The full command list
// — not the filtered one — fixes the columns so the box keeps one width (and
// one set of column positions) as the user types.
func (cp CommandPalette) boxMetrics(termWidth int) cpBoxMetrics {
	var m cpBoxMetrics
	for _, c := range cp.all {
		m.maxShortcutWidth = max(m.maxShortcutWidth, ansi.StringWidth(c.Shortcut))
		m.maxCategoryWidth = max(m.maxCategoryWidth, ansi.StringWidth(c.Category))
	}
	shortcutExtra := 0
	if m.maxShortcutWidth > 0 {
		shortcutExtra = m.maxShortcutWidth + 2 // 2-space gap between name and shortcut
	}
	categoryExtra := 0
	if m.maxCategoryWidth > 0 {
		categoryExtra = m.maxCategoryWidth + 3 // "[" + cat + "] "
	}

	// The +4 / +6 budgets = 2 borders + 2 cells inner padding (1 each side at
	// the wrap, plus 1 each side built into buildRowContent's leading and
	// trailing spaces) so rows sit two cells away from the vertical borders.
	inner := max(cpMinInner, ansi.StringWidth(cpModalTitle)+4)
	for _, c := range cp.all {
		inner = max(inner, ansi.StringWidth(c.Name)+6+shortcutExtra+categoryExtra)
	}
	if maxInner := termWidth - 4; termWidth > 0 && inner > maxInner {
		inner = maxInner
	}
	m.innerWidth = max(inner, 1)
	m.contentWidth = max(m.innerWidth-4, 0)
	return m
}

// Help and Quit stay available through search and the footer, leaving the
// unfiltered list to the user's task at every window size.
func (cp CommandPalette) commandsForQuery(query string) []PaletteCommand {
	var body []PaletteCommand
	for _, c := range cp.all {
		if query != "" || c.Pinned == PinNone {
			body = append(body, c)
		}
	}
	return body
}
