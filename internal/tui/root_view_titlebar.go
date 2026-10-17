package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// renderTitleBar keeps numbered destinations in fixed positions as focus and
// filters change. At compact widths every view still has a mnemonic label.
func (m Model) renderTitleBar() string {
	names := make([]string, len(viewOrder))
	for i, id := range viewOrder {
		names[i] = m.viewDisplayName(id)
	}
	if m.titleBarWidth(names) > m.termWidth {
		names = []string{"Lib", "Lists", "Queue", "Browse", "Fav", "Tags", "Mix", "Insights"}
	}
	if m.titleBarWidth(names) > m.termWidth {
		names = []string{"Lib", "PL", "Q", "Brw", "Fav", "Tag", "Mix", "Ins"}
	}
	var parts []string
	for i, id := range viewOrder {
		label := fmt.Sprintf("%d %s", i+1, names[i])
		if id == m.ActiveView {
			parts = append(parts, m.styles.ActiveTab.Render("["+label+"]"))
		} else {
			parts = append(parts, m.styles.TextTertiary.Render(" "+label+" "))
		}
	}
	return strings.Join(parts, "")
}

func (m Model) titleBarWidth(names []string) int {
	width := 0
	for _, name := range names {
		width += 4 + ansi.StringWidth(name)
	}
	return width
}

// Filters live on the existing divider, so enabling one never moves a tab.
func (m Model) renderTitleDivider() string {
	badge := m.titleBarFilterBadge()
	if badge == "" {
		return m.styles.TextTertiary.Render(strings.Repeat("─", m.termWidth))
	}
	badge = ansi.Truncate(badge, max(0, m.termWidth-4), "…")
	return m.styles.TextTertiary.Render("──") + badge + " " +
		m.styles.TextTertiary.Render(strings.Repeat("─", max(0, m.termWidth-3-ansi.StringWidth(badge))))
}

func (m Model) titleBarFilterBadge() string {
	badge := ""
	if m.hasCommittedFilter() {
		badge += m.styles.Dim.Render(" [filtered]")
	}
	if m.activeTagFilter != nil {
		badge += m.styles.Dim.Render(" [tag: " + m.tagNames[*m.activeTagFilter] + "]")
	}
	return badge
}
