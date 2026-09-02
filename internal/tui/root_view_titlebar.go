package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// renderTitleBar builds the centralized title bar showing view names with
// chevrons and numbered badges.
func (m Model) renderTitleBar() string {
	const chevronWidth = 4
	const badgeWidth = 2
	sep := "  "
	groupAfter := map[viewID]bool{viewQueue: true, viewFavorites: true}
	queueIndicator := m.titleBarQueueIndicator()
	filterBadge := m.titleBarFilterBadge()

	fullWidth := chevronWidth
	for i, id := range viewOrder {
		if i < 10 {
			fullWidth += badgeWidth
		}
		nameLen := len(m.viewDisplayName(id))
		if id == m.activeView {
			nameLen += 2
		}
		if id == viewQueue && queueIndicator != "" {
			nameLen += 2
		}
		fullWidth += nameLen
		if i < len(viewOrder)-1 {
			if groupAfter[id] {
				fullWidth += 3
			} else {
				fullWidth += len(sep)
			}
		}
	}

	if m.termWidth >= fullWidth {
		return m.renderWideTitleBar(sep, groupAfter, queueIndicator, filterBadge)
	}
	return m.renderNarrowTitleBar(chevronWidth, sep, filterBadge)
}

func (m Model) titleBarQueueIndicator() string {
	if m.activeView == viewQueue {
		return ""
	}
	switch {
	case m.playback.state == playback.StatePlaying && m.playback.currentIndex >= 0:
		return " ♪"
	case m.playback.state == playback.StatePaused:
		return " ⏸"
	default:
		return ""
	}
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

func (m Model) renderWideTitleBar(sep string, groupAfter map[viewID]bool, queueIndicator, filterBadge string) string {
	var parts []string
	for i, id := range viewOrder {
		name := m.viewDisplayName(id)
		var entry string
		switch {
		case i < 10 && id == m.activeView:
			badge := m.styles.Badge.Render(fmt.Sprintf("%d", (i+1)%10))
			entry = badge + " " + m.styles.ActiveTab.Render(" "+name+" ") + filterBadge
		case i < 10:
			badge := m.styles.Badge.Render(fmt.Sprintf("%d", (i+1)%10))
			entry = badge + " " + m.styles.TextTertiary.Render(name)
		case id == m.activeView:
			entry = m.styles.ActiveTab.Render(" "+name+" ") + filterBadge
		default:
			entry = m.styles.TextTertiary.Render(name)
		}
		if id == viewQueue && queueIndicator != "" {
			if m.playback.state == playback.StatePlaying {
				entry += m.styles.Success.Render(queueIndicator)
			} else {
				entry += m.styles.Warning.Render(queueIndicator)
			}
		}
		parts = append(parts, entry)
	}

	groupSep := m.styles.Dim.Render(" │ ")
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			if groupAfter[viewOrder[i-1]] {
				b.WriteString(groupSep)
			} else {
				b.WriteString(sep)
			}
		}
		b.WriteString(part)
	}
	return m.styles.TextTertiary.Render("‹ ") + b.String() + m.styles.TextTertiary.Render(" ›")
}

func (m Model) renderNarrowTitleBar(chevronWidth int, sep, filterBadge string) string {
	activeIdx := 0
	for i, id := range viewOrder {
		if id == m.activeView {
			activeIdx = i
			break
		}
	}

	available := m.termWidth - chevronWidth
	left := activeIdx
	right := activeIdx
	used := len(m.narrowTitleBarName(viewOrder[activeIdx])) + 2

	for {
		tryLeft := left > 0
		tryRight := right < len(viewOrder)-1
		if !tryLeft && !tryRight {
			break
		}
		expanded := false
		if tryRight {
			w := len(sep) + len(m.narrowTitleBarName(viewOrder[right+1]))
			if used+w <= available {
				right++
				used += w
				expanded = true
			}
		}
		if tryLeft {
			w := len(sep) + len(m.narrowTitleBarName(viewOrder[left-1]))
			if used+w <= available {
				left--
				used += w
				expanded = true
			}
		}
		if !expanded {
			break
		}
	}

	var parts []string
	for i := left; i <= right; i++ {
		id := viewOrder[i]
		name := m.narrowTitleBarName(id)
		if id == m.activeView {
			parts = append(parts, m.styles.ActiveTab.Render(" "+name+" ")+filterBadge)
		} else {
			parts = append(parts, m.styles.TextTertiary.Render(name))
		}
	}
	return m.styles.TextTertiary.Render("‹ ") + strings.Join(parts, sep) + m.styles.TextTertiary.Render(" ›")
}

func (m Model) narrowTitleBarName(id viewID) string {
	if id == viewLibrary {
		return libraryStripLabel(m.library.mode)
	}
	return viewName(id)
}
