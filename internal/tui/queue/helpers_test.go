package queue

import "charm.land/lipgloss/v2"

// makeQueue returns a View pre-loaded with the given entries.
func makeQueue(width int, entries []Entry) View {
	return View{
		Entries: entries,
		Width:   width,
	}
}

// styledText returns text wrapped with the given lipgloss style, producing
// the same string that production code generates via Style.Render.
func styledText(style lipgloss.Style, text string) string {
	return style.Render(text)
}
