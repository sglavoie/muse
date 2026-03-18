package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// helpEntry is a single row in the keybinding table.
type helpEntry struct {
	key  string
	desc string
}

// helpSection groups related keybinding entries under a named header.
type helpSection struct {
	title   string
	entries []helpEntry
}

// HelpOverlay renders a static keybinding reference grouped into named sections.
type HelpOverlay struct{}

// helpSections returns the ordered list of keybinding sections shown in the
// help overlay. Each section corresponds to one bindingContext.
func helpSections() []helpSection {
	return []helpSection{
		{
			title: "Global",
			entries: []helpEntry{
				{"ctrl+c", "Quit"},
				{"space", "Play / Pause"},
				{"s", "Stop"},
				{"n", "Next track"},
				{"p", "Previous track"},
				{"left / right", "Seek ±5s"},
				{"shift+left / shift+right", "Seek ±30s"},
				{"= / -", "Volume up / down"},
				{"1", "Switch to Library"},
				{"2", "Switch to Playlists"},
				{"3", "Switch to Queue"},
				{"4", "Switch to Artists"},
				{"5", "Switch to Albums"},
				{"R", "Rescan library"},
				{"r", "Cycle repeat mode"},
				{"z", "Toggle shuffle"},
				{"u", "Undo"},
				{"/", "Activate filter bar"},
				{"?", "Show this help"},
			},
		},
		{
			title: "Navigation (Library / Queue / Playlists / Artists / Albums)",
			entries: []helpEntry{
				{"j / k / down / up", "Move cursor down / up"},
				{"ctrl+d / ctrl+u", "Half-page down / up"},
				{"ctrl+f / ctrl+b", "Full page down / up"},
				{"g / G", "Go to top / bottom"},
			},
		},
		{
			title: "Library",
			entries: []helpEntry{
				{"/", "Filter tracks"},
				{"enter", "Enqueue track"},
				{"x", "Toggle bulk selection"},
				{"a", "Add to queue or playlist"},
			},
		},
		{
			title: "Playlists",
			entries: []helpEntry{
				{"enter", "Open playlist / enqueue track"},
				{"esc", "Back to playlist list"},
				{"r", "Rename playlist (shadows global repeat)"},
				{"c", "Create playlist"},
				{"d", "Delete playlist / remove track"},
				{"x", "Toggle bulk selection"},
				{"a", "Remove selected tracks (bulk)"},
				{"E", "Enqueue all tracks from playlist"},
			},
		},
		{
			title: "Artists",
			entries: []helpEntry{
				{"/", "Filter current list"},
				{"enter", "Open albums / tracks / enqueue track"},
				{"esc", "Back one level"},
			},
		},
		{
			title: "Albums",
			entries: []helpEntry{
				{"/", "Filter current list"},
				{"enter", "Open tracks / enqueue track"},
				{"esc", "Back one level"},
			},
		},
		{
			title: "Queue",
			entries: []helpEntry{
				{"enter", "Play selected track"},
				{"d", "Remove selected track"},
				{"x", "Toggle bulk selection"},
				{"C", "Clear queue"},
				{"K / J", "Move track up / down"},
			},
		},
		{
			// Note: "esc" here also dismisses the help overlay itself, which is
			// handled at the root level (outside the keybinding registry). The
			// Modals section covers any future modal dialogs as well.
			title: "Modals / Help overlay",
			entries: []helpEntry{
				{"esc", "Dismiss modal or close this help overlay"},
				{"j / k", "Move cursor down / up (destination picker)"},
				{"enter", "Confirm selection (destination picker)"},
			},
		},
	}
}

// helpLineCount returns the total number of lines in the rendered help text.
// It uses a zero-value Styles since styling does not add newlines.
func helpLineCount() int {
	return strings.Count(HelpOverlay{}.render(styles.Styles{}), "\n") + 1
}

// render builds the help overlay string: a title, context-grouped keybinding
// sections with section headers, and the log file path at the bottom.
func (h HelpOverlay) render(s styles.Styles) string {
	sections := helpSections()

	// Find the longest key string across all sections for column alignment.
	maxKeyLen := 0
	for _, sec := range sections {
		for _, e := range sec.entries {
			if len(e.key) > maxKeyLen {
				maxKeyLen = len(e.key)
			}
		}
	}

	var b strings.Builder
	b.WriteString(s.Heading.Render("Keybindings") + "\n\n")

	for _, sec := range sections {
		b.WriteString(s.Bold.Render(sec.title) + "\n")
		for _, e := range sec.entries {
			fmt.Fprintf(&b, "  %-*s  %s\n", maxKeyLen, e.key, e.desc)
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}
