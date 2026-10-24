package overlay

import (
	"slices"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxWidth int
		indent   int
		want     string
	}{
		{
			name:     "no wrap needed",
			text:     "short text",
			maxWidth: 20,
			indent:   4,
			want:     "short text",
		},
		{
			name:     "wraps at word boundary",
			text:     "Filter (Library / Queue / Artists)",
			maxWidth: 25,
			indent:   4,
			want:     "Filter (Library / Queue\n    / Artists)",
		},
		{
			name:     "empty text",
			text:     "",
			maxWidth: 20,
			indent:   4,
			want:     "",
		},
		{
			name:     "single long word",
			text:     "superlongword",
			maxWidth: 5,
			indent:   2,
			want:     "superlongword",
		},
		{
			name:     "multiple wraps",
			text:     "one two three four five",
			maxWidth: 10,
			indent:   2,
			want:     "one two\n  three four\n  five",
		},
		{
			name:     "measures display width, not bytes",
			text:     "Play — next · then",
			maxWidth: 18,
			indent:   2,
			want:     "Play — next · then",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.text, tt.maxWidth, tt.indent)
			if got != tt.want {
				t.Errorf("wrapText(%q, %d, %d)\n got: %q\nwant: %q", tt.text, tt.maxWidth, tt.indent, got, tt.want)
			}
		})
	}
}

// TestHelpEntryDescUsesDisplayWidth verifies that a description whose byte
// length exceeds the room but whose display width fits is left unwrapped.
func TestHelpEntryDescUsesDisplayWidth(t *testing.T) {
	desc := "Toggle — repeat · shuffle"
	if got := helpEntryDesc(desc, 4, 4+len([]rune(desc))); got != desc {
		t.Errorf("helpEntryDesc wrapped a fitting description: %q", got)
	}
}

func TestHelpOverlayWrapsLongDescriptions(t *testing.T) {
	sections := []HelpSection{
		{
			Title:   "Test Section With A Very Long Title That Should Wrap",
			Context: keys.ContextGlobal,
			entries: []helpEntry{
				{"key", "Short desc"},
				{"/", "Filter (Library / Queue / Artists / Albums / Favorites / Genres)"},
			},
		},
	}
	// Width 50 should force both the title and the long description to wrap.
	out := HelpOverlay{}.Render(styles.Styles{}, sections, "Help", 50)

	// The long description should be split across lines.
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if len(line) > 50 {
			t.Errorf("line exceeds width 50 (%d chars): %q", len(line), line)
		}
	}
	// The short description should remain intact on one line.
	if !strings.Contains(out, "Short desc") {
		t.Error("short description missing from output")
	}
}

func TestFilteredHelpRender(t *testing.T) {
	sections := FilteredHelpSections(keys.ContextQueue)
	out := HelpOverlay{}.Render(styles.Styles{}, sections, "Keybindings — Queue", 0)

	// Check that Queue section is present as a standalone section header.
	if !strings.Contains(out, "\nQueue\n") {
		t.Error("filtered help render missing Queue section header")
	}
	// Verify view-specific sections that should NOT be present.
	// Use exact section title matches to avoid false positives from
	// the Navigation section title which mentions all view names.
	for _, absent := range []string{"\nLibrary\n", "\nPlaylists\n", "\nArtists\n", "\nAlbums\n"} {
		if strings.Contains(out, absent) {
			t.Errorf("filtered help render should not contain section header %q", strings.TrimSpace(absent))
		}
	}
}

// TestHelpSeekEntriesNameNoFixedOffset guards that the seek entries do not
// promise a fixed number of seconds: seek_offset_short and seek_offset_long
// are configurable, so any number printed here can be wrong.
func TestHelpSeekEntriesNameNoFixedOffset(t *testing.T) {
	for _, sec := range HelpSections() {
		for _, e := range sec.entries {
			if !strings.HasPrefix(e.desc, "Seek") {
				continue
			}
			if strings.ContainsAny(e.desc, "0123456789±") {
				t.Errorf("%s: %q hardcodes a seek offset: %q", sec.Title, e.key, e.desc)
			}
		}
	}
}

// TestMixesHelpListsOnlyBoundKeys guards that every key the Mixes help
// section lists does something in Mixes: v and a used to be listed (and
// bound) though Mixes has no visual or bulk selection, and x / X were
// described as selection keys though they dismiss and undismiss.
func TestMixesHelpListsOnlyBoundKeys(t *testing.T) {
	reg := keys.NewRegistry()
	for _, sec := range HelpSections() {
		if sec.Context != keys.ContextMixes {
			continue
		}
		for _, e := range sec.entries {
			for k := range strings.SplitSeq(e.key, " / ") {
				k = strings.TrimSpace(k)
				if reg.Lookup(keys.ContextMixes, k) == keys.ActionNone && reg.Lookup(keys.ContextGlobal, k) == keys.ActionNone {
					t.Errorf("Mixes help lists %q (%s), which is unbound in Mixes", k, e.desc)
				}
			}
			if (e.key == "x" || e.key == "X") && strings.Contains(e.desc, "selection") {
				t.Errorf("Mixes help describes %q as %q, but it dismisses", e.key, e.desc)
			}
		}
	}
}

func TestSearchHelpSectionsMatchesTitleAndKeepsEntryMatches(t *testing.T) {
	sections := []HelpSection{
		{Title: "Auto-DJ panel", entries: []helpEntry{{"tab", "Next tab"}, {"ctrl+s", "Apply settings"}}},
		{Title: "Global", entries: []helpEntry{{"A", "Toggle Auto-DJ"}, {"s", "Stop"}}},
		{Title: "Queue", entries: []helpEntry{{"d", "Remove track"}}},
	}
	for _, query := range []string{"Auto-DJ", "  AUTO-DJ  "} {
		got := SearchHelpSections(sections, query)
		if len(got) != 2 || got[0].Title != "Auto-DJ panel" || !slices.Equal(got[0].entries, sections[0].entries) {
			t.Fatalf("%q did not include full matching section: %+v", query, got)
		}
		if got[1].Title != "Global" || len(got[1].entries) != 1 || got[1].entries[0].key != "A" {
			t.Fatalf("%q lost or widened entry matches: %+v", query, got)
		}
	}
	if len(sections[1].entries) != 2 {
		t.Fatal("search changed source entries")
	}
}
