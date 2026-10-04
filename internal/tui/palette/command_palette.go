package palette

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// PalettePin identifies whether a command is pinned to the top or bottom of
// the palette result list (regardless of filter or score order).
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
	Category       string // short tag rendered dim before Name, from paletteCategories; empty for pins / uncategorized
	Shortcut       string // optional keyboard shortcut shown right-aligned in the palette
	Pinned         PalettePin
	Exec           func() tea.Cmd
	Enabled        func() bool // nil means always enabled
	DisabledReason string      // recovery hint shown when the focused command is unavailable
	Detail         string      // explanatory text shown when the command is focused
	CurrentState   string      // setting shown in the detail line when focused
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
	return cp, cmd
}

// paletteCommandID returns the stable key for a command (Key() falls back to
// Name when ID is empty).
func paletteCommandID(c PaletteCommand) string { return c.Key() }

// paletteCommandName returns the display name used for fuzzy matching.
func paletteCommandName(c PaletteCommand) string { return c.Name }

// HandleKey processes a key press and returns the updated palette and optional Cmd.
func (cp CommandPalette) HandleKey(key string, msg tea.KeyPressMsg) (CommandPalette, tea.Cmd) {
	switch key {
	case "esc":
		return cp, func() tea.Msg { return CommandPaletteDismissedMsg{} }
	case "enter":
		return cp, cp.runFocused()
	case "up", "ctrl+k", "ctrl+p":
		return cp.prevCommand(), nil
	case "down", "ctrl+j", "ctrl+n":
		return cp.nextCommand(), nil
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
// MRU+frequency weighting, then clamps the cursor. Top-pinned and
// bottom-pinned commands always appear at the anchors regardless of the
// current filter; only the body section participates in scoring.
func (cp *CommandPalette) Refilter() {
	query := strings.TrimSpace(cp.Filter.Text)

	var topPins, body, bottomPins []PaletteCommand
	for _, c := range cp.all {
		switch c.Pinned {
		case PinTop:
			topPins = append(topPins, c)
		case PinBottom:
			bottomPins = append(bottomPins, c)
		default:
			body = append(body, c)
		}
	}
	body = Rank(body, query, paletteCommandID, paletteCommandName, cp.usage)

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
		if len(body) > 0 {
			cp.cursor = cp.topPins
		} else {
			cp.cursor = 0
		}
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
	padRow := cpBorderV + strings.Repeat(" ", m.innerWidth) + cpBorderV

	lines := []string{
		// Top border.
		cpCornerTL + strings.Repeat(cpBorderH, m.innerWidth) + cpCornerTR,
		cp.titleLine(m, s),
		// Doubled rule below the title (Common Region + Uniform Connectedness).
		cpCrossDoubleL + strings.Repeat(cpBorderDoubleH, m.innerWidth) + cpCrossDoubleR,
		// Inner top padding row.
		padRow,
		cp.filterLine(m),
		// Separator before commands.
		cpCrossL + strings.Repeat(cpBorderH, m.innerWidth) + cpCrossR,
	}
	lines = append(lines, cp.commandLines(cpCommandRowBudget(termHeight), m, s)...)
	lines = append(lines,
		// Reuse the bottom padding row for the focused command’s details.
		cp.reasonLine(m, s),
		// Bottom border.
		cpCornerBL+strings.Repeat(cpBorderH, m.innerWidth)+cpCornerBR,
		// Hint line.
		"  "+s.Dim.Render(text.Truncate("↑/↓ navigate · enter run · esc cancel", max(termWidth-2, 0))),
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
	displayTitle := text.Truncate(cpModalTitle, m.innerWidth)
	titlePad := max(0, m.innerWidth-ansi.StringWidth(displayTitle))
	leftTP := titlePad / 2
	return cpBorderV + strings.Repeat(" ", leftTP) + s.Heading.Render(displayTitle) +
		strings.Repeat(" ", titlePad-leftTP) + cpBorderV
}

// filterLine renders the always-active "/ query" input row.
func (cp CommandPalette) filterLine(m cpBoxMetrics) string {
	filterText := cp.Filter.View()
	if !cp.Filter.Active {
		filterText = cp.Filter.Text
	}
	return cpBorderV + "  " + text.Pad(" / "+filterText, m.contentWidth) + "  " + cpBorderV
}

// commandLines renders the command area: either the "(no matches)" row or the
// rows the height budget leaves room for.
func (cp CommandPalette) commandLines(budget int, m cpBoxMetrics, s styles.Styles) []string {
	if len(cp.Filtered) == 0 {
		return []string{cpBorderV + "  " + text.Pad("(no matches)", m.contentWidth) + "  " + cpBorderV}
	}
	emptyQuery := strings.TrimSpace(cp.Filter.Text) == ""
	var lines []string
	for _, row := range cp.visibleRows(budget, emptyQuery) {
		lines = append(lines, cp.paletteRowLine(row, m, s))
	}
	return lines
}

// paletteRowLine renders one visible row: the separator between the pinned and
// body sections, the "Recent & frequent" header, or a command styled by whether
// it is focused and whether it is enabled.
func (cp CommandPalette) paletteRowLine(row cpPaletteRow, m cpBoxMetrics, s styles.Styles) string {
	switch row.kind {
	case cpPaletteSeparator:
		return cpCrossL + strings.Repeat(cpBorderH, m.innerWidth) + cpCrossR
	case cpPaletteHeader:
		return cpBorderV + "  " + s.Dim.Render(text.Pad(cpRecentHeader, m.contentWidth)) + "  " + cpBorderV
	}

	cmd := cp.Filtered[row.index]
	body := cp.buildRowContent(cmd, m.contentWidth, m.maxCategoryWidth, m.maxShortcutWidth, s)
	switch {
	case row.index == cp.cursor && !cmd.IsEnabled():
		body = s.Accent.Faint(true).Italic(true).Render(body)
	case row.index == cp.cursor:
		body = s.Accent.Render(body)
	case !cmd.IsEnabled():
		body = s.Dim.Italic(true).Render(body)
	}
	return cpBorderV + " " + body + " " + cpBorderV
}

type cpPaletteRowKind int

const (
	cpPaletteCommand cpPaletteRowKind = iota
	cpPaletteSeparator
	cpPaletteHeader
)

type cpPaletteRow struct {
	kind  cpPaletteRowKind
	index int
}

func cpCommandRowBudget(termHeight int) int {
	if termHeight <= 0 {
		return 1 << 30
	}
	const nonCommandLines = 9 // borders/title/filter/padding/hint around the command area.
	return max(termHeight-nonCommandLines, 1)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// cpRowPlan is the resolved layout of the command area: how many pinned rows
// fit at each anchor, which slice of the body is shown, and which chrome rows
// survived the height budget.
type cpRowPlan struct {
	topVisible    int  // leading pinned rows that fit
	bottomVisible int  // trailing pinned rows that fit
	bodyStart     int  // index in Filtered where the body section begins
	bodyOffset    int  // scroll offset of the body window within the body
	bodyBudget    int  // body rows that fit
	topBodySep    bool // separator between the top pins and the body
	header        bool // "Recent & frequent" header above the body
	bodyBottomSep bool // separator between the body and the bottom pins
}

func (cp CommandPalette) visibleRows(budget int, emptyQuery bool) []cpPaletteRow {
	if budget <= 0 || len(cp.Filtered) == 0 {
		return nil
	}
	return cp.emitRows(cp.planRows(budget, emptyQuery), budget)
}

// planRows resolves how the height budget is shared between the pinned rows,
// the body and the chrome rows separating them. Pinned rows win outright; the
// body takes whatever is left and scrolls to keep the cursor in view.
func (cp CommandPalette) planRows(budget int, emptyQuery bool) cpRowPlan {
	plan := cpRowPlan{bodyStart: cp.topPins}
	bodyCount := max(len(cp.Filtered)-cp.bottomPin-plan.bodyStart, 0)

	plan.topBodySep = cp.topPins > 0 && bodyCount > 0
	// The body is only ordered by use once something has been used; before
	// that it is alphabetical and a "Recent & frequent" header would be false.
	plan.header = plan.topBodySep && emptyQuery && cp.usage.hasRecords()
	plan.bodyBottomSep = cp.bottomPin > 0 && bodyCount > 0
	plan.bodyBudget = plan.fitBody(budget, cp.topPins+cp.bottomPin, bodyCount)

	plan.topVisible = min(cp.topPins, budget)
	remaining := budget - plan.topVisible
	plan.bottomVisible = min(cp.bottomPin, max(remaining, 0))
	remaining -= plan.bottomVisible
	if remaining < plan.bodyBudget {
		plan.bodyBudget = max(remaining, 0)
	}

	plan.bodyOffset = cp.bodyWindowOffset(plan.bodyStart, plan.bodyBudget, bodyCount)
	return plan
}

// fitBody shrinks the body to what the budget leaves after the pinned and
// chrome rows. When even that is negative, the chrome rows are dropped so the
// pinned rows keep their space.
func (p *cpRowPlan) fitBody(budget, pinned, bodyCount int) int {
	fixed := pinned + countTrue(p.topBodySep, p.header, p.bodyBottomSep)
	if fixed+bodyCount <= budget {
		return bodyCount
	}
	bodyBudget := budget - fixed
	if bodyBudget < 0 {
		p.topBodySep = false
		p.header = false
		p.bodyBottomSep = false
		bodyBudget = budget - pinned
	}
	return clamp(bodyBudget, 0, bodyCount)
}

// bodyWindowOffset centres the body window on the cursor when the body does not
// fit whole.
func (cp CommandPalette) bodyWindowOffset(bodyStart, bodyBudget, bodyCount int) int {
	if bodyBudget <= 0 || bodyBudget >= bodyCount {
		return 0
	}
	cursor := cp.cursor - bodyStart
	if cursor < 0 || cursor >= bodyCount {
		cursor = 0
	}
	return clamp(cursor-bodyBudget/2, 0, bodyCount-bodyBudget)
}

// emitRows expands a resolved plan into the ordered row list.
func (cp CommandPalette) emitRows(plan cpRowPlan, budget int) []cpPaletteRow {
	var rows []cpPaletteRow
	for i := range plan.topVisible {
		rows = append(rows, cpPaletteRow{kind: cpPaletteCommand, index: i})
	}
	rows = appendBodyRows(rows, plan, budget)
	return cp.appendBottomPinRows(rows, plan, budget)
}

// appendBodyRows appends the top/body separator, the recency header and the
// body window, stopping at the height budget.
func appendBodyRows(rows []cpPaletteRow, plan cpRowPlan, budget int) []cpPaletteRow {
	if plan.bodyBudget <= 0 {
		return rows
	}
	if plan.topBodySep && len(rows) < budget {
		rows = append(rows, cpPaletteRow{kind: cpPaletteSeparator})
	}
	if plan.header && len(rows) < budget {
		rows = append(rows, cpPaletteRow{kind: cpPaletteHeader})
	}
	for i := 0; i < plan.bodyBudget && len(rows) < budget; i++ {
		rows = append(rows, cpPaletteRow{kind: cpPaletteCommand, index: plan.bodyStart + plan.bodyOffset + i})
	}
	return rows
}

// appendBottomPinRows appends the body/bottom separator and the bottom-pinned
// rows, stopping at the height budget.
func (cp CommandPalette) appendBottomPinRows(rows []cpPaletteRow, plan cpRowPlan, budget int) []cpPaletteRow {
	if plan.bottomVisible <= 0 {
		return rows
	}
	if plan.bodyBottomSep && plan.bodyBudget > 0 && len(rows) < budget {
		rows = append(rows, cpPaletteRow{kind: cpPaletteSeparator})
	}
	for i := len(cp.Filtered) - plan.bottomVisible; i < len(cp.Filtered) && len(rows) < budget; i++ {
		rows = append(rows, cpPaletteRow{kind: cpPaletteCommand, index: i})
	}
	return rows
}

// countTrue reports how many of the flags are set, sizing the chrome rows that
// share the height budget with the commands.
func countTrue(flags ...bool) int {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}

// buildRowContent renders a single command row's inner text: leading space,
// optional "[category] " prefix, padded name, and right-aligned shortcut.
// The category prefix is styled dim and the shortcut dim.
func (cp CommandPalette) buildRowContent(cmd PaletteCommand, contentWidth, maxCategoryWidth, maxShortcutWidth int, s styles.Styles) string {
	// Category column: fixed-width "[cat] " or all-spaces when no categories
	// are present anywhere in the list.
	categoryPart := ""
	categoryWidth := 0
	if maxCategoryWidth > 0 {
		categoryWidth = maxCategoryWidth + 3 // "[" + cat + "] "
		if cmd.Category != "" {
			padded := text.PadLeft(cmd.Category, maxCategoryWidth)
			categoryPart = s.PaletteCategory.Render("["+padded+"]") + " "
		} else {
			categoryPart = strings.Repeat(" ", categoryWidth)
		}
	}

	if maxShortcutWidth > 0 {
		nameWidth := max(0, contentWidth-maxShortcutWidth-2-categoryWidth)
		name := text.Pad(cmd.Name, nameWidth)
		sc := text.PadLeft(cmd.Shortcut, maxShortcutWidth)
		return " " + clampPaletteRowBody(categoryPart+name+"  "+s.Dim.Render(sc), contentWidth) + " "
	}
	nameWidth := max(0, contentWidth-categoryWidth)
	return " " + clampPaletteRowBody(categoryPart+text.Pad(cmd.Name, nameWidth), contentWidth) + " "
}

func clampPaletteRowBody(body string, width int) string {
	body = ansi.Truncate(body, max(width, 0), "")
	if w := ansi.StringWidth(body); w < width {
		body += strings.Repeat(" ", width-w)
	}
	return body
}
