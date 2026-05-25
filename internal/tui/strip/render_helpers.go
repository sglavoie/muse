package strip

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func renderStrip(layout *stripLayout, s styles.Styles) string {
	left := renderLeftZone(layout, s)
	leftWidth := ansi.StringWidth(left)
	rightWidth := layout.rightWidth()
	if rightWidth == 0 {
		pad := layout.width - leftWidth
		if pad < 0 {
			pad = 0
		}
		return left + strings.Repeat(" ", pad)
	}

	right := renderRightZone(layout, s)
	pad := layout.width - leftWidth - rightWidth
	if pad < 0 {
		pad = 0
	}
	return left + strings.Repeat(" ", pad) + right
}

func renderLeftZone(layout *stripLayout, s styles.Styles) string {
	divider := s.Dim.Render(Separator)

	var b strings.Builder
	b.WriteString(renderBreadcrumb(layout.breadcrumb, s))
	if layout.includeSelection {
		b.WriteString(divider)
		b.WriteString(renderSelectionChip(layout.selection.Count, s))
	}
	if layout.includeSort {
		b.WriteString(divider)
		b.WriteString(s.TextTertiary.Render("sort "))
		b.WriteString(s.TextPrimary.Render(layout.sort.Label))
		if layout.sort.Indicator != "" {
			b.WriteString(s.AccentMark.Render(layout.sort.Indicator))
		}
	}
	if layout.includeFilter {
		b.WriteString(divider)
		b.WriteString(s.TextTertiary.Render(filterPrefix))
		switch {
		case layout.filter.Active && layout.filter.Text != "":
			b.WriteString(layout.filterValue)
		case layout.filter.Active && layout.filter.Text == "":
			b.WriteString(layout.filterValue)
		default:
			b.WriteString(s.TextPrimary.Render(layout.filterValue))
		}
		b.WriteString(s.TextTertiary.Render(filterSuffix))
	}
	return b.String()
}

func renderBreadcrumb(breadcrumb []string, s styles.Styles) string {
	chevronStyled := s.Dim.Render(Chevron)

	var b strings.Builder
	for i, crumb := range breadcrumb {
		if i > 0 {
			b.WriteString(chevronStyled)
		}

		style := s.TextSecondary
		if len(breadcrumb) > 1 && i == len(breadcrumb)-1 {
			style = s.TextPrimary
		}
		b.WriteString(style.Render(crumb))
	}
	return b.String()
}

func renderSelectionChip(count int, s styles.Styles) string {
	chipStyle := s.Accent.Foreground(s.TextPrimary.GetForeground())
	return chipStyle.Render(fmt.Sprintf("◉ %d selected", count))
}

func renderRightZone(layout *stripLayout, s styles.Styles) string {
	var b strings.Builder
	if layout.includeCounts {
		countsStyle := s.Dim
		if layout.counts.Filtering {
			countsStyle = s.TextSecondary
		}
		b.WriteString(countsStyle.Render(layout.countsPlain))
	}
	if layout.includeHint {
		if layout.includeCounts {
			b.WriteString(" ")
		}
		b.WriteString(renderKeyLabelPair(layout.hint, s))
	}
	return b.String()
}
