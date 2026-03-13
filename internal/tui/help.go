package tui

import (
	"fmt"
	"strings"
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
// logPath is displayed at the bottom of the overlay.
type HelpOverlay struct {
	logPath string
}

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
				{"R", "Rescan library"},
				{"r", "Cycle repeat mode"},
				{"z", "Toggle shuffle"},
				{"u", "Undo"},
				{"/", "Activate filter bar"},
				{"?", "Show this help"},
			},
		},
		{
			title: "Library",
			entries: []helpEntry{
				{"/", "Filter tracks"},
			},
		},
		{
			title: "Playlists",
			entries: []helpEntry{
				{"r", "Rename playlist (shadows global repeat)"},
				{"c", "Create playlist"},
				{"/", "Filter playlists"},
			},
		},
		{
			title: "Queue",
			entries: []helpEntry{
				{"up / down", "Move cursor"},
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
			},
		},
	}
}

// render builds the help overlay string: a title, context-grouped keybinding
// sections with section headers, and the log file path at the bottom.
func (h HelpOverlay) render() string {
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
	b.WriteString("Keybindings\n\n")

	for _, sec := range sections {
		b.WriteString(sec.title + "\n")
		for _, e := range sec.entries {
			fmt.Fprintf(&b, "  %-*s  %s\n", maxKeyLen, e.key, e.desc)
		}
		b.WriteString("\n")
	}

	b.WriteString("log: " + h.logPath)
	return b.String()
}
