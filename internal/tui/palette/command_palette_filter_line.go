package palette

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

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
