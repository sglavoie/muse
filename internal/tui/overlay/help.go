package overlay

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// SearchHelpSections filters help sections by a case-insensitive substring
// match against both entry key and description. Sections with zero matching
// entries are omitted. An empty or whitespace-only query returns sections
// unchanged so a stray space typed into the filter does not blank out the
// overlay.
func SearchHelpSections(sections []HelpSection, query string) []HelpSection {
	query = strings.TrimSpace(query)
	if query == "" {
		return sections
	}
	q := strings.ToLower(query)
	var result []HelpSection
	for _, sec := range sections {
		var matched []HelpEntry
		for _, e := range sec.Entries {
			if strings.Contains(strings.ToLower(e.Key), q) || strings.Contains(strings.ToLower(e.Desc), q) {
				matched = append(matched, e)
			}
		}
		if len(matched) > 0 {
			result = append(result, HelpSection{Title: sec.Title, Context: sec.Context, Entries: matched})
		}
	}
	return result
}

// HelpEntry is a single row in the keybinding table.
type HelpEntry struct {
	Key  string
	Desc string
}

// HelpSection groups related keybinding entries under a named header.
type HelpSection struct {
	Title   string
	Context keys.Context
	Entries []HelpEntry
}

// HelpOverlay renders a static keybinding reference grouped into named sections.
type HelpOverlay struct{}

// StatusBarHelpSectionTitle names the legend section decoding the status bar's
// mode cluster. It is pinned first in every help mode, so both helpSections()
// and filteredHelpSections reference it by this constant.
const StatusBarHelpSectionTitle = "Status bar indicators"

// HelpSections returns the ordered list of keybinding sections shown in the
// help overlay. Each section corresponds to one bindingContext.
func HelpSections() []HelpSection {
	return []HelpSection{
		{
			// Pinned first: these are glyphs, not keybindings, and none of
			// them match the key that toggles them. The Global section below
			// lists the toggle keys themselves.
			Title:   StatusBarHelpSectionTitle,
			Context: keys.ContextGlobal,
			Entries: []HelpEntry{
				{"R / 1", "Repeat all / repeat one — toggle with r"},
				{"S", "Shuffle — toggle with z"},
				{"N", "Stop after current track — toggle with ctrl+n"},
				{"D", "Radio / Auto-DJ — toggle with A"},
			},
		},
		{
			Title:   "Global",
			Context: keys.ContextGlobal,
			Entries: []HelpEntry{
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
				{"8", "Switch to Insights"},
				{"F", "Choose favorite target (track rows) / toggle focused artist or album"},
				{"o", "Go To (artist/album/genre/source)"},
				{"tab / shift+tab", "Next / previous view"},
				{"[ / ]", "Previous / next view"},
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
				{"L", "Go to Insights"},
				{"q", "Queue summary"},
				{";", "Show playback mode legend"},
				{"f", "Focus now-playing track in queue"},
			},
		},
		{
			Title:   "Navigation (Library / Queue / Playlists / Browse / Favorites / Tags / Mixes)",
			Context: keys.ContextGlobal,
			Entries: []HelpEntry{
				{"j / k / down / up", "Move cursor down / up"},
				{"ctrl+d / ctrl+u", "Half-page down / up"},
				{"ctrl+f / ctrl+b", "Full page down / up"},
				{"g / G", "Go to top / bottom"},
			},
		},
		{
			Title:   "Library",
			Context: keys.ContextLibrary,
			Entries: []HelpEntry{
				{"` / ~", "Cycle sub-mode forward / backward (All ↔ Recently Added ↔ History)"},
				{"/", "Filter tracks (prefixes: a: t: l: g:)"},
				{"enter", "Enqueue focused / selected tracks"},
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
			Title:   "Playlists",
			Context: keys.ContextPlaylists,
			Entries: []HelpEntry{
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
			Title:   "Smart Rule Editor overlay",
			Context: keys.ContextPlaylists,
			Entries: []HelpEntry{
				{"enter", "Next step / confirm rule"},
				{"esc", "Back one step / close overlay"},
				{"tab", "Toggle AND / OR conjunction"},
				{"a", "Add rule"},
				{"d", "Remove focused rule"},
				{"left / right", "Cycle field or operator"},
			},
		},
		// Section order below mirrors the tab-bar order:
		// Library → Playlists → Queue → Browse → Favorites → Tags → Mixes.
		// Library and Playlists appear above; the rest follow here.
		{
			Title:   "Queue",
			Context: keys.ContextQueue,
			Entries: []HelpEntry{
				{"/", "Filter queue"},
				{"esc", "Clear committed filter"},
				{"enter", "Play selected track"},
				{"d", "Remove selected track"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"v", "Toggle visual line selection mode"},
				{"C", "Clear queue"},
				{"K / J", "Move track up / down"},
				{"S", "Save queue as playlist"},
				{"ctrl+y", "Yank (copy) focused / selected tracks"},
				{"ctrl+x", "Cut focused / selected tracks (move on paste)"},
				{"ctrl+p", "Paste yanked / cut tracks below cursor"},
			},
		},
		{
			Title:   "Browse (Artists)",
			Context: keys.ContextArtists,
			Entries: []HelpEntry{
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
			Title:   "Browse (Albums)",
			Context: keys.ContextAlbums,
			Entries: []HelpEntry{
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
			Title:   "Browse (Genres)",
			Context: keys.ContextGenres,
			Entries: []HelpEntry{
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
			Title:   "Favorites",
			Context: keys.ContextFavorites,
			Entries: []HelpEntry{
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
			Title:   "Tags",
			Context: keys.ContextTags,
			Entries: []HelpEntry{
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
				{"F", "Choose favorite target on track rows"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"v", "Toggle visual line selection mode"},
				{"a", "Add selected tracks to playlist"},
				{"/", "Filter"},
				{"u", "Undo last tag operation"},
			},
		},
		{
			Title:   "Mixes",
			Context: keys.ContextMixes,
			Entries: []HelpEntry{
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
			Title:   "Insights",
			Context: keys.ContextInsights,
			Entries: []HelpEntry{
				{"` / ~", "Next / previous panel"},
				{"w / W", "Next / previous window, or bucket size on Momentum and Discovery"},
				{"enter", "Drill into the focused row (Sessions first) / enqueue focused or selected tracks"},
				{"esc", "Back one level"},
				{"e", "Enqueue focused track"},
				{"E", "Enqueue everything behind the row, or replay a session in order"},
				{"!", "Break the rut: never-played tracks, or artists outside your top 20"},
				{"b", "Jump to the focused row's artist in Browse"},
				{"S", "Save the drilled-in tracks as a playlist"},
				{"A", "Start radio seeded from the focused row"},
				{"m", "Merge a Data Health genre collision into one spelling"},
				{"M", "Bulk-edit the tracks behind a Data Health finding"},
				{"x", "Toggle bulk selection"},
				{"X", "Clear bulk selection"},
				{"v", "Toggle visual line selection mode (drilled-in track lists only)"},
				{"a", "Add selected tracks to queue or playlist"},
				{"j / k / down / up", "Move cursor down / up"},
				{"ctrl+d / ctrl+u", "Half-page down / up"},
				{"g / G", "Go to top / bottom"},
			},
		},
		{
			// Note: "esc" here also dismisses the help overlay itself, which is
			// handled at the root level (outside the keybinding registry). The
			// Modals section covers any future modal dialogs as well.
			Title:   "Modals / Help overlay",
			Context: keys.ContextModals,
			Entries: []HelpEntry{
				{"esc", "Dismiss modal or close this help overlay"},
				{"space / left / right / shift+left / shift+right", "Preview pause / seek"},
				{"/", "Search keybindings"},
				{"j / k", "Move cursor down / up (destination picker)"},
				{"enter", "Confirm selection (destination picker)"},
			},
		},
	}
}

// FilteredHelpSections returns only sections whose context is contextGlobal or
// matches ctx. This powers the "view-specific" help mode toggled with "?".
// The status-bar legend stays pinned first here too, so it keeps the same
// position in both help modes.
func FilteredHelpSections(ctx keys.Context) []HelpSection {
	all := HelpSections()
	// Legend first, then the view-specific sections, then global sections.
	var pinned, viewSpecific, global []HelpSection
	for _, sec := range all {
		// Skip the context switch entirely: the legend is contextGlobal, so
		// when ctx is contextGlobal it would otherwise also land in
		// viewSpecific and be emitted twice.
		if sec.Title == StatusBarHelpSectionTitle {
			pinned = append(pinned, sec)
			continue
		}
		switch sec.Context {
		case ctx:
			viewSpecific = append(viewSpecific, sec)
		case keys.ContextGlobal:
			global = append(global, sec)
		}
	}
	out := make([]HelpSection, 0, len(pinned)+len(viewSpecific)+len(global))
	out = append(out, pinned...)
	out = append(out, viewSpecific...)
	out = append(out, global...)
	return out
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

// Render builds the help overlay string: a title, context-grouped keybinding
// sections with section headers, and the log file path at the bottom.
// When width > 0, long descriptions and section titles are word-wrapped.
func (h HelpOverlay) Render(s styles.Styles, sections []HelpSection, title string, width int) string {
	var b strings.Builder
	b.WriteString(s.Heading.Render(title) + "\n")
	// Doubled rule visually anchors the title as its own region (Common
	// Region + Uniform Connectedness) before the sections begin.
	ruleWidth := ansi.StringWidth(title)
	if width > 0 && width < ruleWidth {
		ruleWidth = width
	}
	if ruleWidth < 4 {
		ruleWidth = 4
	}
	b.WriteString(s.Dim.Render(strings.Repeat("═", ruleWidth)) + "\n\n")
	if len(sections) == 0 {
		b.WriteString("  (no matches)\n")
		return b.String()
	}

	for _, sec := range sections {
		if width > 0 {
			b.WriteString(s.Bold.Render(wrapText(sec.Title, width, 2)) + "\n")
		} else {
			b.WriteString(s.Bold.Render(sec.Title) + "\n")
		}

		// Compute maxKeyLen per section so each section aligns independently.
		maxKeyLen := 0
		for _, e := range sec.Entries {
			if len(e.Key) > maxKeyLen {
				maxKeyLen = len(e.Key)
			}
		}
		// descCol is the column where descriptions start: "  " + key + "  ".
		descCol := 2 + maxKeyLen + 2

		for _, e := range sec.Entries {
			styledKey := s.HelpKey.Render(fmt.Sprintf("%-*s", maxKeyLen, e.Key))
			if width > 0 && descCol+len(e.Desc) > width {
				descWidth := max(width-descCol, 1)
				wrapped := wrapText(e.Desc, descWidth, descCol)
				fmt.Fprintf(&b, "  %s  %s\n", styledKey, s.Dim.Render(wrapped))
			} else {
				fmt.Fprintf(&b, "  %s  %s\n", styledKey, s.Dim.Render(e.Desc))
			}
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}
