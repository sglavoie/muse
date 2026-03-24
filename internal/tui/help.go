package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// searchHelpSections filters help sections by a case-insensitive substring
// match against both entry key and description. Sections with zero matching
// entries are omitted. An empty query returns sections unchanged.
func searchHelpSections(sections []helpSection, query string) []helpSection {
	if query == "" {
		return sections
	}
	q := strings.ToLower(query)
	var result []helpSection
	for _, sec := range sections {
		var matched []helpEntry
		for _, e := range sec.entries {
			if strings.Contains(strings.ToLower(e.key), q) || strings.Contains(strings.ToLower(e.desc), q) {
				matched = append(matched, e)
			}
		}
		if len(matched) > 0 {
			result = append(result, helpSection{title: sec.title, context: sec.context, entries: matched})
		}
	}
	return result
}

// helpEntry is a single row in the keybinding table.
type helpEntry struct {
	key  string
	desc string
}

// helpSection groups related keybinding entries under a named header.
type helpSection struct {
	title   string
	context bindingContext
	entries []helpEntry
}

// HelpOverlay renders a static keybinding reference grouped into named sections.
type HelpOverlay struct{}

// helpSections returns the ordered list of keybinding sections shown in the
// help overlay. Each section corresponds to one bindingContext.
func helpSections() []helpSection {
	return []helpSection{
		{
			title:   "Global",
			context: contextGlobal,
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
				{"6", "Switch to Favorites"},
				{"F", "Toggle favorite"},
				{"A", "Jump to artist"},
				{"B", "Jump to album"},
				{"tab / shift+tab", "Next / previous view"},
				{"R", "Rescan library"},
				{"r", "Cycle repeat mode"},
				{"z", "Toggle shuffle"},
				{"u", "Undo"},
				{"/", "Filter (Library / Queue / Artists / Albums / Favorites)"},
				{"P", "Preview focused track"},
				{"I", "Show now-playing info"},
				{"i", "Show track info (exiftool)"},
				{"M", "Edit metadata of focused track"},
				{"ctrl+s", "Cycle sort field"},
				{"ctrl+a", "Sort ascending"},
				{"ctrl+d", "Sort descending (when sorting) / half-page down"},
				{"?", "Show this help"},
			},
		},
		{
			title:   "Navigation (Library / Queue / Playlists / Artists / Albums)",
			context: contextGlobal,
			entries: []helpEntry{
				{"j / k / down / up", "Move cursor down / up"},
				{"ctrl+u", "Half-page up"},
				{"ctrl+f / ctrl+b", "Full page down / up"},
				{"g / G", "Go to top / bottom"},
			},
		},
		{
			title:   "Library",
			context: contextLibrary,
			entries: []helpEntry{
				{"/", "Filter tracks"},
				{"enter", "Enqueue track"},
				{"shift+enter", "Replace queue with track"},
				{"esc", "Clear committed filter"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"a", "Add to queue or playlist"},
			},
		},
		{
			title:   "Playlists",
			context: contextPlaylists,
			entries: []helpEntry{
				{"enter", "Open playlist / enqueue track"},
				{"shift+enter", "Replace queue with track"},
				{"esc", "Back to playlist list"},
				{"r", "Rename playlist"},
				{"c", "Create playlist"},
				{"d", "Delete playlist / remove track"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"a", "Remove selected tracks (bulk)"},
				{"E", "Enqueue all tracks from playlist"},
			},
		},
		{
			title:   "Artists",
			context: contextArtists,
			entries: []helpEntry{
				{"/", "Filter current list"},
				{"enter", "Open albums / tracks / enqueue track"},
				{"shift+enter", "Replace queue with track"},
				{"esc", "Back one level"},
				{"x", "Toggle bulk-selection on focused track"},
				{"X", "Clear all bulk-selected tracks"},
				{"a", "Add focused / selected tracks to queue or playlist"},
			},
		},
		{
			title:   "Albums",
			context: contextAlbums,
			entries: []helpEntry{
				{"/", "Filter current list"},
				{"enter", "Open tracks / enqueue track"},
				{"shift+enter", "Replace queue with track"},
				{"esc", "Back one level"},
				{"x", "Toggle bulk-selection on focused track"},
				{"X", "Clear all bulk-selected tracks"},
				{"a", "Add focused / selected tracks to queue or playlist"},
			},
		},
		{
			title:   "Queue",
			context: contextQueue,
			entries: []helpEntry{
				{"/", "Filter queue"},
				{"enter", "Play selected track"},
				{"d", "Remove selected track"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"C", "Clear queue"},
				{"K / J", "Move track up / down"},
				{"f", "Focus now-playing track"},
				{"S", "Save queue as playlist"},
			},
		},
		{
			title:   "Favorites",
			context: contextFavorites,
			entries: []helpEntry{
				{"/", "Filter current list"},
				{"enter", "Open category / drill into tracks / enqueue"},
				{"esc", "Back one level"},
				{"F", "Remove from favorites"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear selection"},
				{"a", "Bulk remove selected"},
				{"shift+enter", "Replace queue with track"},
			},
		},
		{
			// Note: "esc" here also dismisses the help overlay itself, which is
			// handled at the root level (outside the keybinding registry). The
			// Modals section covers any future modal dialogs as well.
			title:   "Modals / Help overlay",
			context: contextModals,
			entries: []helpEntry{
				{"esc", "Dismiss modal or close this help overlay"},
				{"/", "Search keybindings"},
				{"j / k", "Move cursor down / up (destination picker)"},
				{"enter", "Confirm selection (destination picker)"},
			},
		},
	}
}

// filteredHelpSections returns only sections whose context is contextGlobal or
// matches ctx. This powers the "view-specific" help mode toggled with "?".
func filteredHelpSections(ctx bindingContext) []helpSection {
	all := helpSections()
	// View-specific section first, then global sections.
	var viewSpecific, global []helpSection
	for _, sec := range all {
		switch sec.context {
		case ctx:
			viewSpecific = append(viewSpecific, sec)
		case contextGlobal:
			global = append(global, sec)
		}
	}
	return append(viewSpecific, global...)
}

// render builds the help overlay string: a title, context-grouped keybinding
// sections with section headers, and the log file path at the bottom.
func (h HelpOverlay) render(s styles.Styles, sections []helpSection, title string) string {
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
	b.WriteString(s.Heading.Render(title) + "\n\n")
	if len(sections) == 0 {
		b.WriteString("  (no matches)\n")
		return b.String()
	}

	for _, sec := range sections {
		b.WriteString(s.Bold.Render(sec.title) + "\n")
		for _, e := range sec.entries {
			fmt.Fprintf(&b, "  %-*s  %s\n", maxKeyLen, e.key, e.desc)
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}
