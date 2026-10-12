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
// match against section titles, entry keys, and descriptions. A matching title
// includes all entries in that section. Sections with zero matching
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
		if strings.Contains(strings.ToLower(sec.Title), q) {
			result = append(result, sec)
			continue
		}
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
