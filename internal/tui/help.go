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
				{"4", "Switch to Browse"},
				{"5", "Switch to Favorites"},
				{"6", "Switch to Tags"},
				{"7", "Switch to Mixes"},
				{"F", "Toggle favorite"},
				{"o", "Go To (artist/album/genre/source)"},
				{"tab / shift+tab", "Next / previous view"},
				{"R", "Reset track position"},
				{"ctrl+r", "Rescan library"},
				{"r", "Cycle repeat mode"},
				{"z", "Toggle shuffle"},
				{"ctrl+n", "Stop after current track"},
				{"A", "Toggle radio mode (Auto-DJ)"},
				{"u", "Undo"},
				{"/", "Filter (Library / Queue / Browse / Favorites / Tags / Mixes)"},
				{"P", "Preview focused track"},
				{"I", "Show now-playing info"},
				{"i", "Show track info (exiftool)"},
				{"M", "Edit metadata of focused track"},
				{"ctrl+s", "Cycle sort field"},
				{"shift+up", "Sort ascending"},
				{"shift+down", "Sort descending"},
				{"?", "Show this help"},
				{"ctrl+,", "Open settings"},
				{":", "Command palette"},
				{"t", "Manage tags"},
				{"ctrl+t", "Filter views by tag"},
				{"ctrl+g", "Show debug panel"},
				{"L", "Listening stats"},
				{"q", "Queue summary"},
			},
		},
		{
			title:   "Navigation (Library / Queue / Playlists / Browse / Favorites / Tags / Mixes)",
			context: contextGlobal,
			entries: []helpEntry{
				{"j / k / down / up", "Move cursor down / up"},
				{"ctrl+d / ctrl+u", "Half-page down / up"},
				{"ctrl+f / ctrl+b", "Full page down / up"},
				{"g / G", "Go to top / bottom"},
			},
		},
		{
			title:   "Library",
			context: contextLibrary,
			entries: []helpEntry{
				{"` / ~", "Cycle sub-mode forward / backward (All ↔ Recently Added ↔ History)"},
				{"/", "Filter tracks (prefixes: a: t: l: g:)"},
				{"enter", "Enqueue track"},
				{"N", "Play next (insert after current track)"},
				{"shift+enter", "Replace queue with track"},
				{"esc", "Clear committed filter"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"v", "Toggle visual line selection mode"},
				{"a", "Add to queue or playlist"},
			},
		},
		{
			title:   "Playlists",
			context: contextPlaylists,
			entries: []helpEntry{
				{"enter", "Open playlist / enqueue track"},
				{"N", "Play next (insert after current track)"},
				{"shift+enter", "Replace queue with track"},
				{"esc", "Back to playlist list"},
				{"r", "Rename playlist / refresh smart playlist (track level)"},
				{"c", "Create playlist"},
				{"d", "Delete playlist / remove track"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"v", "Toggle visual line selection mode"},
				{"a", "Remove selected tracks (bulk)"},
				{"E", "Enqueue all tracks from playlist"},
				{"S", "Create smart playlist"},
				{"e", "Edit smart playlist rules"},
				{"K", "Move track up"},
				{"J", "Move track down"},
			},
		},
		{
			title:   "Smart Rule Editor overlay",
			context: contextPlaylists,
			entries: []helpEntry{
				{"enter", "Next step / confirm rule"},
				{"esc", "Back one step / close overlay"},
				{"tab", "Toggle AND / OR conjunction"},
				{"a", "Add rule"},
				{"d", "Remove focused rule"},
				{"left / right", "Cycle field or operator"},
			},
		},
		{
			title:   "Browse (Artists)",
			context: contextArtists,
			entries: []helpEntry{
				{"` / ~", "Cycle Browse mode forward / backward (Artists ↔ Albums ↔ Genres)"},
				{"/", "Filter current list"},
				{"enter", "Open albums / tracks / enqueue track"},
				{"N", "Play next (insert after current track)"},
				{"shift+enter", "Replace queue with track"},
				{"esc", "Back one level"},
				{"x", "Toggle bulk-selection on focused track"},
				{"X", "Clear all bulk-selected tracks"},
				{"v", "Toggle visual line selection mode (track level)"},
				{"a", "Add focused / selected tracks to queue or playlist"},
			},
		},
		{
			title:   "Browse (Albums)",
			context: contextAlbums,
			entries: []helpEntry{
				{"` / ~", "Cycle Browse mode forward / backward (Artists ↔ Albums ↔ Genres)"},
				{"/", "Filter current list"},
				{"enter", "Open tracks / enqueue track"},
				{"N", "Play next (insert after current track)"},
				{"shift+enter", "Replace queue with track"},
				{"esc", "Back one level"},
				{"x", "Toggle bulk-selection on focused track"},
				{"X", "Clear all bulk-selected tracks"},
				{"v", "Toggle visual line selection mode (track level)"},
				{"a", "Add focused / selected tracks to queue or playlist"},
			},
		},
		{
			title:   "Browse (Genres)",
			context: contextGenres,
			entries: []helpEntry{
				{"` / ~", "Cycle Browse mode forward / backward (Artists ↔ Albums ↔ Genres)"},
				{"/", "Filter current list"},
				{"enter", "Open artists / tracks / enqueue track"},
				{"N", "Play next (insert after current track)"},
				{"shift+enter", "Replace queue with track"},
				{"esc", "Back one level"},
				{"x", "Toggle bulk-selection on focused track"},
				{"X", "Clear all bulk-selected tracks"},
				{"v", "Toggle visual line selection mode (track level)"},
				{"a", "Add focused / selected tracks to queue or playlist"},
			},
		},
		{
			title:   "Queue",
			context: contextQueue,
			entries: []helpEntry{
				{"/", "Filter queue"},
				{"esc", "Clear committed filter"},
				{"enter", "Play selected track"},
				{"d", "Remove selected track"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"v", "Toggle visual line selection mode"},
				{"C", "Clear queue"},
				{"K / J", "Move track up / down"},
				{"f", "Focus now-playing track"},
				{"S", "Save queue as playlist"},
				{"ctrl+y", "Yank (copy) focused / selected tracks"},
				{"ctrl+x", "Cut focused / selected tracks (move on paste)"},
				{"ctrl+p", "Paste yanked / cut tracks below cursor"},
			},
		},
		{
			title:   "Favorites",
			context: contextFavorites,
			entries: []helpEntry{
				{"/", "Filter current list"},
				{"enter", "Open category / drill into tracks / enqueue"},
				{"N", "Play next (insert after current track)"},
				{"esc", "Back one level"},
				{"F", "Remove from favorites"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear selection"},
				{"v", "Toggle visual line selection mode"},
				{"a", "Bulk remove selected"},
				{"shift+enter", "Replace queue with track"},
			},
		},
		{
			title:   "Tags",
			context: contextTags,
			entries: []helpEntry{
				{"6", "Switch to Tags"},
				{"enter", "Drill into tag tracks / enqueue track"},
				{"backspace / h / esc", "Back to tag list"},
				{"c", "Create new tag (inline input)"},
				{"r", "Rename focused tag (inline input)"},
				{"d", "Delete focused tag (with undo)"},
				{"N", "Play next (insert after current track)"},
				{"E", "Enqueue all tracks for focused tag"},
				{"m", "Merge focused tag into another (irreversible)"},
				{"t", "Manage tags for focused track (tag picker)"},
				{"F", "Toggle favorite on focused track"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"v", "Toggle visual line selection mode"},
				{"a", "Add selected tracks to playlist"},
				{"/", "Filter"},
				{"u", "Undo last tag operation"},
			},
		},
		{
			title:   "Mixes",
			context: contextMixes,
			entries: []helpEntry{
				{"enter", "Open mix / enqueue track"},
				{"esc", "Back to mix list"},
				{"e", "Enqueue focused track"},
				{"E", "Enqueue all tracks in mix"},
				{"N", "Play next (insert after current track)"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"v", "Toggle visual line selection mode"},
				{"a", "Add selected tracks to queue or playlist"},
				{"s", "Cycle sort field (track level)"},
				{"S", "Save current mix as a static playlist"},
				{"A", "Start radio seeded from this mix"},
				{"/", "Filter tracks (track level)"},
				{"j / k / down / up", "Move cursor down / up"},
				{"ctrl+d / ctrl+u", "Half-page down / up"},
				{"g / G", "Go to top / bottom"},
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
				{"space / left / right / shift+left / shift+right", "Preview pause / seek"},
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

// wrapText word-wraps text to fit within maxWidth. Lines after the first are
// prefixed with indent spaces. If maxWidth is too small for even one word,
// words are placed one per line without breaking mid-word.
func wrapText(text string, maxWidth, indent int) string {
	raw := strings.Fields(text)
	if len(raw) == 0 {
		return ""
	}

	// Merge standalone "/" with the following word so separators like
	// "Library / Queue / Artists" never leave "/" alone on a line.
	words := make([]string, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == "/" && i+1 < len(raw) {
			words = append(words, "/ "+raw[i+1])
			i++
		} else {
			words = append(words, raw[i])
		}
	}

	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	lineLen := 0

	for i, w := range words {
		if i == 0 {
			b.WriteString(w)
			lineLen = len(w)
			continue
		}
		// Would adding this word (plus a space) exceed the limit?
		if lineLen+1+len(w) > maxWidth {
			b.WriteString("\n" + pad + w)
			lineLen = len(w)
		} else {
			b.WriteString(" " + w)
			lineLen += 1 + len(w)
		}
	}
	return b.String()
}

// render builds the help overlay string: a title, context-grouped keybinding
// sections with section headers, and the log file path at the bottom.
// When width > 0, long descriptions and section titles are word-wrapped.
func (h HelpOverlay) render(s styles.Styles, sections []helpSection, title string, width int) string {
	var b strings.Builder
	b.WriteString(s.Heading.Render(title) + "\n\n")
	if len(sections) == 0 {
		b.WriteString("  (no matches)\n")
		return b.String()
	}

	for _, sec := range sections {
		if width > 0 {
			b.WriteString(s.Bold.Render(wrapText(sec.title, width, 2)) + "\n")
		} else {
			b.WriteString(s.Bold.Render(sec.title) + "\n")
		}

		// Compute maxKeyLen per section so each section aligns independently.
		maxKeyLen := 0
		for _, e := range sec.entries {
			if len(e.key) > maxKeyLen {
				maxKeyLen = len(e.key)
			}
		}
		// descCol is the column where descriptions start: "  " + key + "  ".
		descCol := 2 + maxKeyLen + 2

		for _, e := range sec.entries {
			styledKey := s.HelpKey.Render(fmt.Sprintf("%-*s", maxKeyLen, e.key))
			if width > 0 && descCol+len(e.desc) > width {
				descWidth := max(width-descCol, 1)
				wrapped := wrapText(e.desc, descWidth, descCol)
				fmt.Fprintf(&b, "  %s  %s\n", styledKey, s.Dim.Render(wrapped))
			} else {
				fmt.Fprintf(&b, "  %s  %s\n", styledKey, s.Dim.Render(e.desc))
			}
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}
