package overlay

import (
	"fmt"
	"slices"
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
		var matched []helpEntry
		for _, e := range sec.entries {
			if strings.Contains(strings.ToLower(e.key), q) || strings.Contains(strings.ToLower(e.desc), q) {
				matched = append(matched, e)
			}
		}
		if len(matched) > 0 {
			result = append(result, HelpSection{Title: sec.Title, Context: sec.Context, entries: matched})
		}
	}
	return result
}

// helpEntry is a single row in the keybinding table.
type helpEntry struct {
	key  string
	desc string
}

// HelpSection groups related keybinding entries under a named header.
type HelpSection struct {
	Title   string
	Context keys.Context
	entries []helpEntry
}

// HelpOverlay renders a static keybinding reference grouped into named sections.
type HelpOverlay struct{}

// StatusBarHelpSectionTitle names the legend section decoding the status bar's
// mode cluster. It is pinned first in every help mode, so both
// helpSectionsTable and FilteredHelpSections reference it by this constant.
const StatusBarHelpSectionTitle = "Status bar indicators"

// HelpSections returns the ordered list of keybinding sections shown in the
// help overlay. Each section corresponds to one bindingContext. The returned
// slice is a fresh copy, so callers may sort or reslice it without disturbing
// helpSectionsTable.
func HelpSections() []HelpSection { return slices.Clone(helpSectionsTable) }

// helpSectionsTable is the canonical keybinding reference. It is data, not
// code: reach it through HelpSections so no caller can mutate it in place.
var helpSectionsTable = []HelpSection{
	{
		// Pinned first: these are glyphs, not keybindings, and none of
		// them match the key that toggles them. The Global section below
		// lists the toggle keys themselves.
		Title:   StatusBarHelpSectionTitle,
		Context: keys.ContextGlobal,
		entries: []helpEntry{
			{"R / 1", "Repeat all / repeat one — toggle with r"},
			{"S", "Shuffle — toggle with z"},
			{"N", "Stop after current track — toggle with ctrl+n"},
			{"D", "Radio / Auto-DJ — toggle with A"},
		},
	},
	{
		Title:   "Global",
		Context: keys.ContextGlobal,
		entries: []helpEntry{
			{"ctrl+c", "Quit"},
			{"space", "Play / Pause"},
			{"s", "Stop"},
			{"n", "Next track"},
			{"p", "Previous track"},
			{"left / right", "Seek backward / forward"},
			{"shift+left / shift+right", "Seek backward / forward (long)"},
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
			{"ctrl+a", "Tune Auto-DJ (presets, discovery, filters) and its Preview and Recipes tabs"},
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
			{":", "Command palette (offers Start Auto-DJ from selection…)"},
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
		entries: []helpEntry{
			{"j / k / down / up", "Move cursor down / up"},
			{"ctrl+d / ctrl+u", "Half-page down / up"},
			{"ctrl+f / ctrl+b", "Full page down / up"},
			{"g / G", "Go to top / bottom"},
		},
	},
	{
		Title:   "Library",
		Context: keys.ContextLibrary,
		entries: []helpEntry{
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
		Title:   "Smart Rule Editor overlay",
		Context: keys.ContextPlaylists,
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
		// The panel handles its own keys rather than going through the
		// keybinding registry, so this section is the only place they are
		// written down outside the panel's own footer hint — which is
		// truncated on a narrow terminal.
		Title:   "Auto-DJ panel overlay (ctrl+a)",
		Context: keys.ContextModals,
		entries: []helpEntry{
			{"tab / shift+tab", "Page between Tune, Preview, and Recipes"},
			{"ctrl+s", "Start the session, or apply the draft to the running one"},
			{"esc", "Close without changing anything"},
			{"j / k / down / up", "Move the cursor, or scroll Preview"},
			{"h / l / left / right", "Adjust the focused row (preset, discovery, anchor)"},
			{"enter", "Edit the focused filter, or press the Start/Apply button"},
			{"x", "Clear the focused filter"},
			{"r", "Replan the Preview draft, or drop filter values naming a deleted tag"},
			{"enter / s / u / n / d", "Recipes: load / save as / update / rename / delete"},
			{"tab (filter picker)", "Toggle the highlighted value in or out"},
			{"ctrl+u", "Clear the filter search, or the recipe name being typed"},
		},
	},
	// Section order below mirrors the tab-bar order:
	// Library → Playlists → Queue → Browse → Favorites → Tags → Mixes.
	// Library and Playlists appear above; the rest follow here.
	{
		Title:   "Queue",
		Context: keys.ContextQueue,
		entries: []helpEntry{
			{"/", "Filter queue"},
			{"esc", "Clear committed filter"},
			{"enter", "Play selected track (restarts the current one)"},
			{"d", "Remove selected track"},
			{"x", "Toggle bulk selection"},
			{"X", "Clear bulk selection"},
			{"v", "Toggle visual line selection mode"},
			{"C", "Clear queue"},
			{"K / J", "Move track up / down (queue order only)"},
			{"S", "Save queue as playlist"},
			{"ctrl+y", "Yank (copy) focused / selected tracks"},
			{"ctrl+x", "Cut focused / selected tracks (move on paste)"},
			{"ctrl+p", "Paste below cursor (queue order only)"},
		},
	},
	{
		Title:   "Browse (Artists)",
		Context: keys.ContextArtists,
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
		Title:   "Browse (Albums)",
		Context: keys.ContextAlbums,
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
		Title:   "Browse (Genres)",
		Context: keys.ContextGenres,
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
		Title:   "Favorites",
		Context: keys.ContextFavorites,
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
		Title:   "Tags",
		Context: keys.ContextTags,
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
		entries: []helpEntry{
			{"enter", "Open mix / enqueue track"},
			{"esc", "Back to mix list"},
			{"e", "Enqueue focused track"},
			{"E", "Enqueue all tracks in mix"},
			{"N", "Play next (insert after current track)"},
			{"x", "Dismiss focused track from future mixes"},
			{"X", "Undismiss focused track"},
			{"s", "Cycle sort field (track level); stop on the mix list"},
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
		entries: []helpEntry{
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
		entries: []helpEntry{
			{"esc", "Dismiss modal or close this help overlay"},
			{"space / left / right / shift+left / shift+right", "Preview pause / seek"},
			{"/", "Search keybindings"},
			{"j / k", "Move cursor down / up (destination picker)"},
			{"enter", "Confirm selection (destination picker)"},
		},
	},
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
	b.WriteString(s.Dim.Render(strings.Repeat("═", helpRuleWidth(title, width))) + "\n\n")
	if len(sections) == 0 {
		b.WriteString("  (no matches)\n")
		return b.String()
	}

	for _, sec := range sections {
		writeHelpSection(&b, s, sec, width)
	}

	return strings.TrimRight(b.String(), "\n")
}

// helpRuleWidth is the width of the rule under the help title: the title's
// display width, clamped to the terminal and floored so it stays visible.
func helpRuleWidth(title string, width int) int {
	ruleWidth := ansi.StringWidth(title)
	if width > 0 && width < ruleWidth {
		ruleWidth = width
	}
	return max(ruleWidth, 4)
}

// writeHelpSection writes one section header followed by its key/description
// rows, aligned in a key column sized for this section alone.
func writeHelpSection(b *strings.Builder, s styles.Styles, sec HelpSection, width int) {
	if width > 0 {
		b.WriteString(s.Bold.Render(wrapText(sec.Title, width, 2)) + "\n")
	} else {
		b.WriteString(s.Bold.Render(sec.Title) + "\n")
	}

	maxKeyLen := 0
	for _, e := range sec.entries {
		maxKeyLen = max(maxKeyLen, len(e.key))
	}
	// descCol is the column where descriptions start: "  " + key + "  ".
	descCol := 2 + maxKeyLen + 2

	for _, e := range sec.entries {
		styledKey := s.HelpKey.Render(fmt.Sprintf("%-*s", maxKeyLen, e.key))
		fmt.Fprintf(b, "  %s  %s\n", styledKey, s.Dim.Render(helpEntryDesc(e.desc, descCol, width)))
	}
	b.WriteString("\n")
}

// helpEntryDesc word-wraps a description into the space left of descCol when it
// would otherwise run past the terminal edge.
func helpEntryDesc(desc string, descCol, width int) string {
	if width > 0 && descCol+len(desc) > width {
		return wrapText(desc, max(width-descCol, 1), descCol)
	}
	return desc
}
