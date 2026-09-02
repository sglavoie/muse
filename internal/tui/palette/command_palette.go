package palette

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/filter"
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
	ID       string // stable identifier for usage tracking; falls back to Name when empty
	Name     string
	Category string // short tag rendered dim before Name, from paletteCategories; empty for pins / uncategorized
	Shortcut string // optional keyboard shortcut shown right-aligned in the palette
	Pinned   PalettePin
	Exec     func() tea.Cmd
	Enabled  func() bool // nil means always enabled
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
		if cp.cursor >= 0 && cp.cursor < len(cp.Filtered) {
			picked := cp.Filtered[cp.cursor]
			if !picked.IsEnabled() {
				return cp, nil
			}
			if cp.usage != nil {
				cp.usage.Bump(picked.Key())
				cp.usage.SaveBestEffort()
			}
			return cp, func() tea.Msg { return CommandPalettePickedMsg{Command: picked} }
		}
	case "up", "ctrl+k", "ctrl+p":
		if cp.cursor > 0 {
			cp.cursor--
		}
	case "down", "ctrl+j", "ctrl+n":
		if cp.cursor < len(cp.Filtered)-1 {
			cp.cursor++
		}
	default:
		var cmd tea.Cmd
		cp.Filter, cmd = cp.Filter.Update(msg)
		cp.Refilter()
		return cp, cmd
	}
	return cp, nil
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

// Render produces a centered overlay string for the palette.
func (cp CommandPalette) Render(termWidth, termHeight int, s styles.Styles) string {
	boxLines := cp.buildBoxLines(termWidth, termHeight, s)

	boxW := ansi.StringWidth(boxLines[0])
	boxH := len(boxLines)

	leftPad := 0
	if termWidth > boxW {
		leftPad = (termWidth - boxW) / 2
	}
	topPad := 0
	if termHeight > boxH {
		topPad = (termHeight - boxH) / 2
	}

	prefix := strings.Repeat(" ", leftPad)
	var b strings.Builder
	for range topPad {
		b.WriteByte('\n')
	}
	for _, line := range boxLines {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

func (cp CommandPalette) buildBoxLines(termWidth, termHeight int, s styles.Styles) []string {
	const modalTitle = " Command Palette "
	const minInner = 30
	const recentHeader = "Recent & frequent"

	// Compute max shortcut width across filtered commands.
	maxShortcutWidth := 0
	for _, c := range cp.Filtered {
		if w := ansi.StringWidth(c.Shortcut); w > maxShortcutWidth {
			maxShortcutWidth = w
		}
	}
	shortcutExtra := 0
	if maxShortcutWidth > 0 {
		shortcutExtra = maxShortcutWidth + 2 // 2-space gap between name and shortcut
	}

	// Compute max category width (rendered as "[cat] " before the name).
	maxCategoryWidth := 0
	for _, c := range cp.Filtered {
		if w := ansi.StringWidth(c.Category); w > maxCategoryWidth {
			maxCategoryWidth = w
		}
	}
	categoryExtra := 0
	if maxCategoryWidth > 0 {
		categoryExtra = maxCategoryWidth + 3 // "[" + cat + "] "
	}

	// Compute inner width from title, filter, and command names + shortcuts + category prefix.
	// +4 / +6 budget below = 2 borders + 2 cells inner padding (1 each side
	// at the wrap, plus 1 each side built into buildRowContent's leading and
	// trailing spaces) so rows sit two cells away from the vertical borders.
	innerWidth := minInner
	if ansi.StringWidth(modalTitle)+4 > innerWidth {
		innerWidth = ansi.StringWidth(modalTitle) + 4
	}
	for _, c := range cp.Filtered {
		if ansi.StringWidth(c.Name)+6+shortcutExtra+categoryExtra > innerWidth {
			innerWidth = ansi.StringWidth(c.Name) + 6 + shortcutExtra + categoryExtra
		}
	}
	if maxInner := termWidth - 4; termWidth > 0 && innerWidth > maxInner {
		innerWidth = maxInner
	}
	if innerWidth < 1 {
		innerWidth = 1
	}
	contentWidth := max(innerWidth-4, 0)

	var lines []string

	// Top border.
	lines = append(lines, cpCornerTL+strings.Repeat(cpBorderH, innerWidth)+cpCornerTR)

	// Title row (centered).
	displayTitle := text.Truncate(modalTitle, innerWidth)
	titlePad := max(0, innerWidth-ansi.StringWidth(displayTitle))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, cpBorderV+strings.Repeat(" ", leftTP)+s.Heading.Render(displayTitle)+strings.Repeat(" ", rightTP)+cpBorderV)

	// Doubled rule below the title (Common Region + Uniform Connectedness).
	lines = append(lines, cpCrossDoubleL+strings.Repeat(cpBorderDoubleH, innerWidth)+cpCrossDoubleR)

	// Inner top padding row.
	lines = append(lines, cpBorderV+strings.Repeat(" ", innerWidth)+cpBorderV)

	// Filter input row.
	filterText := cp.Filter.View()
	if !cp.Filter.Active {
		filterText = cp.Filter.Text
	}
	filterDisplay := " / " + filterText
	filterPadded := text.Pad(filterDisplay, contentWidth)
	lines = append(lines, cpBorderV+"  "+filterPadded+"  "+cpBorderV)

	// Separator before commands.
	lines = append(lines, cpCrossL+strings.Repeat(cpBorderH, innerWidth)+cpCrossR)

	commandBudget := cpCommandRowBudget(termHeight)

	// Command rows.
	if len(cp.Filtered) == 0 {
		noMatch := text.Pad("(no matches)", contentWidth)
		lines = append(lines, cpBorderV+"  "+noMatch+"  "+cpBorderV)
	} else {
		dimItalic := s.Dim.Italic(true)
		accentDim := s.Accent.Faint(true).Italic(true)
		emptyQuery := strings.TrimSpace(cp.Filter.Text) == ""

		visible := cp.visibleRows(commandBudget, emptyQuery)
		for _, row := range visible {
			// Insert separator + (optional) "Recent & frequent" header between
			// top pins and body.
			if row.kind == cpPaletteSeparator {
				lines = append(lines, cpCrossL+strings.Repeat(cpBorderH, innerWidth)+cpCrossR)
				continue
			}
			if row.kind == cpPaletteHeader {
				header := text.Pad(recentHeader, contentWidth)
				lines = append(lines, cpBorderV+"  "+s.Dim.Render(header)+"  "+cpBorderV)
				continue
			}

			i := row.index
			cmd := cp.Filtered[i]
			rowContent := cp.buildRowContent(cmd, contentWidth, maxCategoryWidth, maxShortcutWidth, s)
			switch {
			case i == cp.cursor && !cmd.IsEnabled():
				lines = append(lines, cpBorderV+" "+accentDim.Render(rowContent)+" "+cpBorderV)
			case i == cp.cursor:
				lines = append(lines, cpBorderV+" "+s.Accent.Render(rowContent)+" "+cpBorderV)
			case !cmd.IsEnabled():
				lines = append(lines, cpBorderV+" "+dimItalic.Render(rowContent)+" "+cpBorderV)
			default:
				lines = append(lines, cpBorderV+" "+rowContent+" "+cpBorderV)
			}
		}
	}

	// Inner bottom padding row.
	lines = append(lines, cpBorderV+strings.Repeat(" ", innerWidth)+cpBorderV)

	// Bottom border.
	lines = append(lines, cpCornerBL+strings.Repeat(cpBorderH, innerWidth)+cpCornerBR)

	// Hint line.
	lines = append(lines, "  "+s.Dim.Render(text.Truncate("↑/↓ navigate · enter run · esc cancel", max(termWidth-2, 0))))

	return lines
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

func (cp CommandPalette) visibleRows(budget int, emptyQuery bool) []cpPaletteRow {
	if budget <= 0 || len(cp.Filtered) == 0 {
		return nil
	}

	bodyStart := cp.topPins
	bodyEnd := len(cp.Filtered) - cp.bottomPin
	topCount := cp.topPins
	bodyCount := max(bodyEnd-bodyStart, 0)
	bottomCount := cp.bottomPin

	includeTopBodySep := topCount > 0 && bodyCount > 0
	includeHeader := includeTopBodySep && emptyQuery
	includeBodyBottomSep := bottomCount > 0 && bodyCount > 0

	fixed := topCount + bottomCount
	if includeTopBodySep {
		fixed++
	}
	if includeHeader {
		fixed++
	}
	if includeBodyBottomSep {
		fixed++
	}

	bodyBudget := bodyCount
	if fixed+bodyCount > budget {
		bodyBudget = budget - fixed
		if bodyBudget < 0 {
			includeTopBodySep = false
			includeHeader = false
			includeBodyBottomSep = false
			fixed = topCount + bottomCount
			bodyBudget = budget - fixed
		}
	}
	bodyBudget = clamp(bodyBudget, 0, bodyCount)

	topVisible := min(topCount, budget)
	remaining := budget - topVisible
	bottomVisible := min(bottomCount, max(remaining, 0))
	remaining -= bottomVisible
	if remaining < bodyBudget {
		bodyBudget = max(remaining, 0)
	}

	bodyOffset := 0
	if bodyBudget > 0 && bodyBudget < bodyCount {
		cursor := cp.cursor - bodyStart
		if cursor < 0 || cursor >= bodyCount {
			cursor = 0
		}
		bodyOffset = clamp(cursor-bodyBudget/2, 0, bodyCount-bodyBudget)
	}

	var rows []cpPaletteRow
	for i := 0; i < topVisible; i++ {
		rows = append(rows, cpPaletteRow{kind: cpPaletteCommand, index: i})
	}
	if bodyBudget > 0 {
		if includeTopBodySep && len(rows) < budget {
			rows = append(rows, cpPaletteRow{kind: cpPaletteSeparator})
		}
		if includeHeader && len(rows) < budget {
			rows = append(rows, cpPaletteRow{kind: cpPaletteHeader})
		}
		for i := 0; i < bodyBudget && len(rows) < budget; i++ {
			rows = append(rows, cpPaletteRow{kind: cpPaletteCommand, index: bodyStart + bodyOffset + i})
		}
	}
	if bottomVisible > 0 {
		if includeBodyBottomSep && bodyBudget > 0 && len(rows) < budget {
			rows = append(rows, cpPaletteRow{kind: cpPaletteSeparator})
		}
		start := len(cp.Filtered) - bottomVisible
		for i := start; i < len(cp.Filtered) && len(rows) < budget; i++ {
			rows = append(rows, cpPaletteRow{kind: cpPaletteCommand, index: i})
		}
	}
	return rows
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
