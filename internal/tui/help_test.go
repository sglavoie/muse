package tui

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestHelpSectionsMatchRegistry verifies that every key registered in
// newRegistry() appears in the corresponding helpSections() entry, preventing
// the static help overlay from drifting as new actions are added.
func TestHelpSectionsMatchRegistry(t *testing.T) {
	reg := newRegistry()
	sections := helpSections()

	// Build a map of section title → set of individual key strings.
	// Composite display entries like "left / right" are split on " / ".
	sectionKeys := make(map[string]map[string]bool)
	for _, sec := range sections {
		sectionKeys[sec.title] = make(map[string]bool)
		for _, e := range sec.entries {
			for k := range strings.SplitSeq(e.key, " / ") {
				sectionKeys[sec.title][strings.TrimSpace(k)] = true
			}
		}
	}

	// Map the binding contexts that have a direct corresponding section.
	// contextModals dismiss is handled in root.Update directly and is
	// intentionally absent from the registry.
	ctxSection := map[bindingContext]string{
		contextGlobal:        "Global",
		contextLibrary:       "Library",
		contextRecentlyAdded: "Recently Added",
		contextHistory:       "History",
		contextQueue:         "Queue",
		contextPlaylists:     "Playlists",
		contextArtists:       "Artists",
		contextAlbums:        "Albums",
		contextFavorites:     "Favorites",
		contextGenres:        "Genres",
		contextTags:          "Tags",
	}

	// The "Navigation" section covers keys shared across all row-based views.
	// Merge its keys into each of those sections for the check.
	if navKeys, ok := sectionKeys["Navigation (Library / Queue / Playlists / Artists / Albums / Favorites / Genres / Recently Added / History)"]; ok {
		for _, sec := range []string{"Library", "Recently Added", "History", "Queue", "Playlists", "Artists", "Albums", "Favorites", "Genres", "Tags"} {
			if sectionKeys[sec] == nil {
				sectionKeys[sec] = make(map[string]bool)
			}
			for k := range navKeys {
				sectionKeys[sec][k] = true
			}
		}
	}

	for rk := range reg.bindings {
		secTitle, ok := ctxSection[rk.ctx]
		if !ok {
			continue
		}
		if !sectionKeys[secTitle][rk.key] {
			t.Errorf("registry key (context=%s, key=%q) is not listed in helpSections() %q section",
				secTitle, rk.key, secTitle)
		}
	}

	// Reverse check: every individual key in helpSections() (except composite
	// display-only entries like "left / right" and "= / -") must have a
	// corresponding registry entry so that stale help text cannot accumulate
	// after a keybinding is removed.
	//
	// Composite display entries where BOTH halves are registered are fine; we
	// only flag a key when neither half appears in any binding for that context.
	// The Modals section is intentionally excluded here because esc is handled
	// in root.Update directly. The Navigation section covers keys shared
	// across Library, Queue, and Playlists — each key must exist in at least
	// one of those contexts.
	reverseCtx := map[string]bindingContext{
		"Global":         contextGlobal,
		"Library":        contextLibrary,
		"Recently Added": contextRecentlyAdded,
		"History":        contextHistory,
		"Queue":          contextQueue,
		"Playlists":      contextPlaylists,
		"Artists":        contextArtists,
		"Albums":         contextAlbums,
		"Favorites":      contextFavorites,
		"Genres":         contextGenres,
		"Tags":           contextTags,
	}
	// Navigation keys are shared; check that each appears in at least one
	// of the view contexts.
	navContexts := []bindingContext{contextLibrary, contextRecentlyAdded, contextHistory, contextQueue, contextPlaylists, contextArtists, contextAlbums, contextFavorites, contextGenres, contextTags}
	for _, sec := range sections {
		ctx, directMatch := reverseCtx[sec.title]
		isNav := sec.title == "Navigation (Library / Queue / Playlists / Artists / Albums / Favorites / Genres / Recently Added / History)"
		if !directMatch && !isNav {
			continue
		}
		for _, e := range sec.entries {
			parts := strings.Split(e.key, " / ")
			anyFound := false
			for _, part := range parts {
				k := strings.TrimSpace(part)
				if directMatch {
					if reg.lookup(ctx, k) != actionNone || reg.lookup(contextGlobal, k) != actionNone {
						anyFound = true
						break
					}
				} else {
					// Navigation section: check all three view contexts.
					for _, nc := range navContexts {
						if reg.lookup(nc, k) != actionNone {
							anyFound = true
							break
						}
					}
					if anyFound {
						break
					}
				}
			}
			if !anyFound {
				t.Errorf("helpSections() %q section has entry %q with no corresponding registry binding (stale help text?)",
					sec.title, e.key)
			}
		}
	}
}

func TestHelpOverlayRendersAllSectionHeaders(t *testing.T) {
	h := HelpOverlay{}
	out := h.render(styles.Styles{}, helpSections(), "Keybindings", 0)

	for _, header := range []string{"Global", "Library", "Playlists", "Artists", "Albums", "Queue", "Modals"} {
		if !strings.Contains(out, header) {
			t.Errorf("help overlay missing section header %q", header)
		}
	}
}

func TestHelpOverlayRendersRepresentativeEntries(t *testing.T) {
	h := HelpOverlay{}
	out := h.render(styles.Styles{}, helpSections(), "Keybindings", 0)

	checks := []struct {
		section string
		text    string
	}{
		// Global entries
		{"Global", "ctrl+c"},
		{"Global", "space"},
		{"Global", "Quit"},
		{"Global", "Play / Pause"},
		// Library entries
		{"Library", "/"},
		// Playlists entries
		{"Playlists", "Rename playlist"},
		{"Playlists", "c"},
		// Artists entries
		{"Artists", "enter"},
		{"Artists", "esc"},
		// Albums entries
		{"Albums", "enter"},
		{"Albums", "esc"},
		// Queue entries
		{"Queue", "enter"},
		{"Queue", "Remove selected track"},
		// Modals entries
		{"Modals", "esc"},
		{"Modals", "Dismiss"},
		{"Modals", "Preview pause / seek"},
	}

	for _, c := range checks {
		if !strings.Contains(out, c.text) {
			t.Errorf("help overlay (%s section): expected entry containing %q", c.section, c.text)
		}
	}
}

func TestFilteredHelpSections(t *testing.T) {
	sections := filteredHelpSections(contextLibrary)

	// Should contain Global, Navigation, and Library — but not Queue, Playlists, etc.
	allowed := map[string]bool{
		"Global": true,
		"Navigation (Library / Queue / Playlists / Artists / Albums / Favorites / Genres / Recently Added / History)": true,
		"Library": true,
	}
	for _, sec := range sections {
		if !allowed[sec.title] {
			t.Errorf("filteredHelpSections(contextLibrary) included unexpected section %q", sec.title)
		}
	}
	if len(sections) != len(allowed) {
		t.Errorf("filteredHelpSections(contextLibrary) returned %d sections, want %d", len(sections), len(allowed))
	}
}

func TestSearchHelpSections(t *testing.T) {
	sections := helpSections()
	result := searchHelpSections(sections, "shuffle")

	// "shuffle" should match the Global entry "Toggle shuffle".
	if len(result) == 0 {
		t.Fatal("searchHelpSections(\"shuffle\") returned no sections")
	}
	for _, sec := range result {
		for _, e := range sec.entries {
			lower := strings.ToLower(e.key) + " " + strings.ToLower(e.desc)
			if !strings.Contains(lower, "shuffle") {
				t.Errorf("section %q: entry (%q, %q) does not match query \"shuffle\"", sec.title, e.key, e.desc)
			}
		}
	}

	// Only Global should match; view-specific sections have no "shuffle" entry.
	if len(result) != 1 || result[0].title != "Global" {
		titles := make([]string, len(result))
		for i, s := range result {
			titles[i] = s.title
		}
		t.Errorf("expected only Global section, got %v", titles)
	}
}

func TestSearchHelpSectionsCaseInsensitive(t *testing.T) {
	sections := helpSections()
	for _, query := range []string{"QUIT", "quit", "Quit"} {
		result := searchHelpSections(sections, query)
		if len(result) == 0 {
			t.Fatalf("searchHelpSections(%q) returned no sections", query)
		}
		found := false
		for _, sec := range result {
			for _, e := range sec.entries {
				if strings.Contains(strings.ToLower(e.desc), "quit") {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("searchHelpSections(%q) did not return the Quit entry", query)
		}
	}
}

func TestSearchHelpSectionsEmptyQuery(t *testing.T) {
	sections := helpSections()
	result := searchHelpSections(sections, "")
	if len(result) != len(sections) {
		t.Fatalf("empty query: expected %d sections, got %d", len(sections), len(result))
	}
	for i := range sections {
		if result[i].title != sections[i].title {
			t.Errorf("empty query: section %d title mismatch: %q vs %q", i, result[i].title, sections[i].title)
		}
		if len(result[i].entries) != len(sections[i].entries) {
			t.Errorf("empty query: section %q entry count mismatch: %d vs %d", result[i].title, len(result[i].entries), len(sections[i].entries))
		}
	}
}

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

func TestHelpOverlayWrapsLongDescriptions(t *testing.T) {
	sections := []helpSection{
		{
			title:   "Test Section With A Very Long Title That Should Wrap",
			context: contextGlobal,
			entries: []helpEntry{
				{"key", "Short desc"},
				{"/", "Filter (Library / Queue / Artists / Albums / Favorites / Genres)"},
			},
		},
	}
	// Width 50 should force both the title and the long description to wrap.
	out := HelpOverlay{}.render(styles.Styles{}, sections, "Help", 50)

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
	sections := filteredHelpSections(contextQueue)
	out := HelpOverlay{}.render(styles.Styles{}, sections, "Keybindings — Queue", 0)

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
