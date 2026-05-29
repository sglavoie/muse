package styles

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// EmptyAction is one row in an empty-state card: a key hint and the action
// label that follows it.
type EmptyAction struct {
	Key   string
	Label string
}

// RenderEmptyCard returns a horizontally-centered (and, when height > 0,
// vertically-centered) rounded card containing a bold title, a blank, and
// one row per action formatted as `key  label`. width and height describe
// the viewport the card should sit inside; when either is non-positive the
// corresponding axis is rendered without centering.
func (s Styles) RenderEmptyCard(width, height int, title string, actions []EmptyAction) string {
	keyW := 0
	for _, a := range actions {
		if w := lipgloss.Width(a.Key); w > keyW {
			keyW = w
		}
	}

	inner := make([]string, 0, len(actions)+2)
	inner = append(inner, s.Bold.Render(title), "")
	for _, a := range actions {
		keyStr := s.EmptyKey.Render(padRightToWidth(a.Key, keyW))
		labelStr := s.EmptyHint.Render(a.Label)
		inner = append(inner, keyStr+"  "+labelStr)
	}

	contentW := 0
	for _, line := range inner {
		if w := lipgloss.Width(line); w > contentW {
			contentW = w
		}
	}
	for i, line := range inner {
		w := lipgloss.Width(line)
		if w < contentW {
			inner[i] = line + strings.Repeat(" ", contentW-w)
		}
	}

	card := s.EmptyCard.Render(strings.Join(inner, "\n"))
	cardLines := strings.Split(card, "\n")

	if width > 0 {
		for i, line := range cardLines {
			cardLines[i] = clampLineWidth(line, width)
		}
		cardLines = strings.Split(strings.Join(cardLines, "\n"), "\n")
	}

	boxW := 0
	for _, line := range cardLines {
		if w := lipgloss.Width(line); w > boxW {
			boxW = w
		}
	}

	if width > 0 {
		leftPad := max(0, (width-boxW)/2)
		if leftPad > 0 {
			pad := strings.Repeat(" ", leftPad)
			for i, line := range cardLines {
				cardLines[i] = pad + line
			}
		}
	}

	out := strings.Join(cardLines, "\n")

	if height > 0 {
		topPad := max(0, (height-len(cardLines))/2)
		if topPad > 0 {
			out = strings.Repeat("\n", topPad) + out
		}
	}
	return out
}

func padRightToWidth(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw >= w {
		return s
	}
	return s + strings.Repeat(" ", w-sw)
}

func clampLineWidth(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
