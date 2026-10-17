package palette

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// PalettePin identifies whether a command is pinned to the top or bottom of
// the empty-query palette result list. Search results are always ranked by match.
type PalettePin int

const (
	PinNone   PalettePin = iota
	PinTop               // anchor at the top of the list (e.g. Help)
	PinBottom            // anchor at the bottom of the list (e.g. Quit)
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
	Relevant       bool        // belongs to the current view; boosts the empty-query list
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
	Filtered  []PaletteCommand // [topPins..., body..., bottomPins...]
	topPins   int              // number of leading pinned rows in filtered
	bottomPin int              // number of trailing pinned rows in filtered
	cursor    int
	Filter    filter.State // always-active text input
	usage     *UsageStore
	lastQuery string // trimmed query from previous refilter; used to reset cursor on query change
	hasQuery  bool   // true once a refilter has run (distinguishes initial state from "" query change)
	compact   bool
	keepPin   string // preserve a focused Help/Quit row when shrinking a tall palette
}

// WithHeight adapts the palette without moving focus to a different command.
func (cp CommandPalette) WithHeight(height int) CommandPalette {
	compact := compactPalette(height)
	if cp.compact == compact {
		return cp
	}
	focused := ""
	if cp.cursor < len(cp.Filtered) {
		focused = cp.Filtered[cp.cursor].Key()
	}
	cp.compact = compact
	cp.keepPin = ""
	if compact && cp.cursor < len(cp.Filtered) && cp.Filtered[cp.cursor].Pinned != PinNone {
		cp.keepPin = focused
	}
	cp.Refilter()
	cp.cursor = cp.topPins
	for i, command := range cp.Filtered {
		if command.Key() == focused {
			cp.cursor = i
			break
		}
	}
	return cp
}

// NewCommandPalette creates a CommandPalette with the given commands. When
// usage is non-nil the initial body order (empty filter) is most-used first;
// otherwise the order is alphabetical. Pinned commands always anchor at the
// top/bottom regardless of score. The filter text input is activated
// immediately.
func NewCommandPalette(commands []PaletteCommand, usage *UsageStore) (CommandPalette, tea.Cmd) {
	cp := CommandPalette{
		all:   commands,
		usage: usage,
	}
	var cmd tea.Cmd
	cp.Filter, cmd = filter.New().Activate()
	cp.Refilter()
	cp.cursor = cp.topPins
	return cp, cmd
}

// paletteCommandID returns the stable key for a command (Key() falls back to
// Name when ID is empty).
func paletteCommandID(c PaletteCommand) string { return c.Key() }

// paletteCommandName returns the display name used for fuzzy matching.
func paletteCommandName(c PaletteCommand) string { return c.Name }

// HandleKey processes a key press and returns the updated palette and optional Cmd.
func (cp CommandPalette) HandleKey(key string, msg tea.KeyPressMsg, heights ...int) (CommandPalette, tea.Cmd) {
	if len(heights) > 0 {
		cp = cp.WithHeight(heights[0])
	}
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
// MRU+frequency weighting, then clamps the cursor. Pins anchor the empty-query list; during a search every command competes
// on its match so unrelated pins cannot crowd out useful results.
func (cp *CommandPalette) Refilter() {
	query := strings.TrimSpace(cp.Filter.Text)

	topPins, body, bottomPins := cp.commandGroups(query)
	body = rank(body, query, paletteCommandID, paletteCommandName, func(c PaletteCommand) []string { return c.Aliases }, cp.usage)
	if query == "" {
		slices.SortStableFunc(body, func(a, b PaletteCommand) int {
			return commandPriority(b) - commandPriority(a)
		})
	}

	cp.topPins = len(topPins)
	cp.bottomPin = len(bottomPins)
	combined := make([]PaletteCommand, 0, len(topPins)+len(body)+len(bottomPins))
	combined = append(combined, topPins...)
	combined = append(combined, body...)
	combined = append(combined, bottomPins...)
	cp.Filtered = combined

	// When the query changes, reset the cursor to the first body match so the
	// user lands on the highest-scoring result rather than whatever entry
	// happens to share the old index (which can be a bottom pin after the body
	// shrinks, accidentally triggering Quit on enter).
	if cp.hasQuery && query != cp.lastQuery {
		cp.cursor = cp.topPins
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
	// Doubled rule below the title (Common Region + Uniform Connectedness).
	cpBorderDoubleH = "═"
	cpCrossDoubleL  = "╞"
	cpCrossDoubleR  = "╡"
)

// Render produces a horizontally centered, top-anchored overlay string for the
// palette. Only the height varies as the filter narrows the result list; the
// box keeps its width and its position at the top of the screen.
func (cp CommandPalette) Render(termWidth, termHeight int, s styles.Styles) string {
	cp = cp.WithHeight(termHeight)
	return overlay.RenderTopLines(cp.buildBoxLines(termWidth, termHeight, s), "", termWidth)
}

const (
	cpModalTitle   = " Command Palette "
	cpMinInner     = 30
	cpRecentHeader = "Recent & frequent"
)

// cpBoxMetrics holds the column widths one palette render is laid out against.
type cpBoxMetrics struct {
	innerWidth       int // space between the vertical borders
	contentWidth     int // space available to a row's text
	maxCategoryWidth int // widest category tag across the filtered commands
	maxShortcutWidth int // widest shortcut across the filtered commands
}

func (cp CommandPalette) buildBoxLines(termWidth, termHeight int, s styles.Styles) []string {
	m := cp.boxMetrics(termWidth)
	if compactPalette(termHeight) {
		return cp.compactBoxLines(termWidth, termHeight, m, s)
	}
	padRow := cpBorderV + strings.Repeat(" ", m.innerWidth) + cpBorderV

	lines := []string{
		// Top border.
		cpCornerTL + strings.Repeat(cpBorderH, m.innerWidth) + cpCornerTR,
		cp.titleLine(m, s),
		// Doubled rule below the title (Common Region + Uniform Connectedness).
		cpCrossDoubleL + strings.Repeat(cpBorderDoubleH, m.innerWidth) + cpCrossDoubleR,

		cp.filterLine(m),
		// Separator before commands.
		cpCrossL + strings.Repeat(cpBorderH, m.innerWidth) + cpCrossR,
	}
	if !compactPalette(termHeight) {
		lines = append(lines[:3], append([]string{padRow}, lines[3:]...)...)
	}
	lines = append(lines, cp.commandLines(cpCommandRowBudget(termHeight), m, s)...)
	lines = append(lines,
		// Reuse the bottom padding row for the focused command’s details.
		cp.reasonLine(m, s),
		// Bottom border.
		cpCornerBL+strings.Repeat(cpBorderH, m.innerWidth)+cpCornerBR,
		// Hint line.
		"  "+s.Dim.Render(text.Truncate("↑/↓ nav · PgUp/Dn page · enter run · esc close", max(min(termWidth-2, m.innerWidth), 0))),
	)
	return lines
}

// reasonLine keeps the dialog's height stable as focus moves between commands.
func (cp CommandPalette) reasonLine(m cpBoxMetrics, s styles.Styles) string {
	reason := ""
	if cp.cursor >= 0 && cp.cursor < len(cp.Filtered) {
		reason = cp.Filtered[cp.cursor].detailText()
	}
	return cpBorderV + "  " + s.Dim.Render(text.Pad(reason, m.contentWidth)) + "  " + cpBorderV
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

// titleLine renders the centered title row.
func (cp CommandPalette) titleLine(m cpBoxMetrics, s styles.Styles) string {
	title := fmt.Sprintf("Command Palette · %d results", len(cp.Filtered))
	if len(cp.Filtered) > 0 {
		title = fmt.Sprintf("Command Palette · %d/%d", cp.cursor+1, len(cp.Filtered))
	}
	displayTitle := text.Truncate(title, m.innerWidth)
	titlePad := max(0, m.innerWidth-ansi.StringWidth(displayTitle))
	leftTP := titlePad / 2
	return cpBorderV + strings.Repeat(" ", leftTP) + s.Heading.Render(displayTitle) +
		strings.Repeat(" ", titlePad-leftTP) + cpBorderV
}

func (cp CommandPalette) commandGroups(query string) ([]PaletteCommand, []PaletteCommand, []PaletteCommand) {
	var topPins, body, bottomPins []PaletteCommand
	for _, c := range cp.all {
		if query != "" {
			body = append(body, c)
			continue
		}
		if cp.compact && c.Pinned != PinNone {
			if c.Key() == cp.keepPin {
				body = append(body, c)
			}
			continue
		}
		switch c.Pinned {
		case PinTop:
			topPins = append(topPins, c)
		case PinBottom:
			bottomPins = append(bottomPins, c)
		default:
			body = append(body, c)
		}
	}
	return topPins, body, bottomPins
}
