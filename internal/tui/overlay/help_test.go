package overlay

import (
	"slices"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestHelpSectionsMatchRegistry verifies that every key registered in
// newRegistry() appears in the corresponding helpSections() entry, preventing
// the static help overlay from drifting as new actions are added.
func TestHelpSectionsMatchRegistry(t *testing.T) {
	reg := keys.NewRegistry()
	sections := HelpSections()

	// Build a map of section title → set of individual key strings.
	// Composite display entries like "left / right" are split on " / ".
	sectionKeys := make(map[string]map[string]bool)
	for _, sec := range sections {
		sectionKeys[sec.Title] = make(map[string]bool)
		for _, e := range sec.entries {
			for k := range strings.SplitSeq(e.key, " / ") {
				sectionKeys[sec.Title][strings.TrimSpace(k)] = true
			}
		}
	}

	// Map the binding contexts that have a direct corresponding section.
	// contextModals dismiss is handled in root.Update directly and is
	// intentionally absent from the registry.
	ctxSection := map[keys.Context]string{
		keys.ContextGlobal:    "Global",
		keys.ContextLibrary:   "Library",
		keys.ContextQueue:     "Queue",
		keys.ContextPlaylists: "Playlists",
		keys.ContextArtists:   "Browse (Artists)",
		keys.ContextAlbums:    "Browse (Albums)",
		keys.ContextFavorites: "Favorites",
		keys.ContextGenres:    "Browse (Genres)",
		keys.ContextTags:      "Tags",
	}

	// The "Navigation" section covers keys shared across all row-based views.
	// Merge its keys into each of those sections for the check.
	if navKeys, ok := sectionKeys["Navigation (Library / Queue / Playlists / Browse / Favorites / Tags / Mixes)"]; ok {
		for _, sec := range []string{"Library", "Queue", "Playlists", "Browse (Artists)", "Browse (Albums)", "Favorites", "Browse (Genres)", "Tags"} {
			if sectionKeys[sec] == nil {
				sectionKeys[sec] = make(map[string]bool)
			}
			for k := range navKeys {
				sectionKeys[sec][k] = true
			}
		}
	}

	for rk := range reg.Bindings {
		secTitle, ok := ctxSection[rk.Ctx]
		if !ok {
			continue
		}
		if !sectionKeys[secTitle][rk.Key] {
			t.Errorf("registry key (context=%s, key=%q) is not listed in helpSections() %q section",
				secTitle, rk.Key, secTitle)
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
	reverseCtx := map[string]keys.Context{
		"Global":           keys.ContextGlobal,
		"Library":          keys.ContextLibrary,
		"Queue":            keys.ContextQueue,
		"Playlists":        keys.ContextPlaylists,
		"Browse — Artists": keys.ContextArtists,
		"Browse — Albums":  keys.ContextAlbums,
		"Favorites":        keys.ContextFavorites,
		"Browse — Genres":  keys.ContextGenres,
		"Tags":             keys.ContextTags,
	}
	// Navigation keys are shared; check that each appears in at least one
	// of the view contexts.
	navContexts := []keys.Context{keys.ContextLibrary, keys.ContextQueue, keys.ContextPlaylists, keys.ContextArtists, keys.ContextAlbums, keys.ContextFavorites, keys.ContextGenres, keys.ContextTags}
	for _, sec := range sections {
		ctx, directMatch := reverseCtx[sec.Title]
		isNav := sec.Title == "Navigation (Library / Queue / Playlists / Browse / Favorites / Tags / Mixes)"
		if !directMatch && !isNav {
			continue
		}
		for _, e := range sec.entries {
			parts := strings.Split(e.key, " / ")
			anyFound := false
			for _, part := range parts {
				k := strings.TrimSpace(part)
				if directMatch {
					if reg.Lookup(ctx, k) != keys.ActionNone || reg.Lookup(keys.ContextGlobal, k) != keys.ActionNone {
						anyFound = true
						break
					}
				} else {
					// Navigation section: check all three view contexts.
					for _, nc := range navContexts {
						if reg.Lookup(nc, k) != keys.ActionNone {
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
					sec.Title, e.key)
			}
		}
	}
}

func TestHelpOverlayRendersAllSectionHeaders(t *testing.T) {
	h := HelpOverlay{}
	out := h.Render(styles.Styles{}, HelpSections(), "Keybindings", 0)

	for _, header := range []string{"Global", "Library", "Playlists", "Artists", "Albums", "Queue", "Modals"} {
		if !strings.Contains(out, header) {
			t.Errorf("help overlay missing section header %q", header)
		}
	}
}

func TestHelpOverlayRendersRepresentativeEntries(t *testing.T) {
	h := HelpOverlay{}
	out := h.Render(styles.Styles{}, HelpSections(), "Keybindings", 0)

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
	sections := FilteredHelpSections(keys.ContextLibrary)

	// Should contain the pinned status-bar legend, Global, Navigation, and
	// Library — but not Queue, Playlists, etc.
	allowed := map[string]bool{
		"Status bar indicators": true,
		"Global":                true,
		"Navigation (Library / Queue / Playlists / Browse / Favorites / Tags / Mixes)": true,
		"Library": true,
	}
	for _, sec := range sections {
		if !allowed[sec.Title] {
			t.Errorf("filteredHelpSections(contextLibrary) included unexpected section %q", sec.Title)
		}
	}
	if len(sections) != len(allowed) {
		t.Errorf("filteredHelpSections(contextLibrary) returned %d sections, want %d", len(sections), len(allowed))
	}
}

// helpContexts is every bindingContext a help overlay can be filtered by.
var helpContexts = []keys.Context{
	keys.ContextGlobal, keys.ContextLibrary, keys.ContextPlaylists, keys.ContextQueue,
	keys.ContextArtists, keys.ContextAlbums, keys.ContextFavorites, keys.ContextGenres,
	keys.ContextTags, keys.ContextModals, keys.ContextMixes, keys.ContextBrowse, keys.ContextInsights,
}

// The status-bar legend explains glyphs that match no keybinding, so it is
// pinned first in both the unfiltered list and every view-filtered one — the
// filtered mode is the default (m.helpFiltered starts true).
func TestStatusBarHelpSectionIsFirst(t *testing.T) {
	if got := HelpSections()[0].Title; got != StatusBarHelpSectionTitle {
		t.Errorf("helpSections()[0].title = %q, want %q", got, StatusBarHelpSectionTitle)
	}
	for _, ctx := range helpContexts {
		sections := FilteredHelpSections(ctx)
		if len(sections) == 0 {
			t.Errorf("filteredHelpSections(%d) returned no sections", ctx)
			continue
		}
		if ctx != keys.ContextBrowse && sections[0].Context != ctx {
			t.Errorf("scoped help should begin with context %d, got %s", ctx, sections[0].Title)
		}
	}
}

// Regression: the legend carries contextGlobal, so pinning it must skip the
// context switch entirely — otherwise a contextGlobal view emits it once as
// the pinned copy and again as a global section.
func TestStatusBarHelpSectionNotDuplicated(t *testing.T) {
	for _, ctx := range helpContexts {
		count := 0
		for _, sec := range FilteredHelpSections(ctx) {
			if sec.Title == StatusBarHelpSectionTitle {
				count++
			}
		}
		if count != 1 {
			t.Errorf("filteredHelpSections(%d) contains %d %q sections, want 1", ctx, count, StatusBarHelpSectionTitle)
		}
	}
}

// The legend must name, for every glyph in the status-bar mode cluster, the
// key that toggles it — and that key must still be bound to the matching
// action, so the descriptions cannot drift from the registry.
func TestStatusBarHelpSectionDescribesToggleKeys(t *testing.T) {
	want := []struct {
		glyph  string
		toggle string
		act    keys.Action
	}{
		{"Repeat all/one", "r", keys.ActionRepeatCycle},
		{"Shuffle", "z", keys.ActionShuffleToggle},
		{"Stop after track", "ctrl+n", keys.ActionStopAfterCurrent},
		{"Auto-DJ", "A", keys.ActionRadioToggle},
	}

	var sec HelpSection
	for _, s := range HelpSections() {
		if s.Title == StatusBarHelpSectionTitle {
			sec = s
			break
		}
	}
	if len(sec.entries) != len(want)+1 {
		t.Fatalf("%q section has %d entries, want %d", StatusBarHelpSectionTitle, len(sec.entries), len(want))
	}

	reg := keys.NewRegistry()
	for i, w := range want {
		e := sec.entries[i]
		if e.key != w.glyph {
			t.Errorf("entry %d: key = %q, want %q", i, e.key, w.glyph)
		}
		if !strings.Contains(e.desc, "toggle with "+w.toggle) {
			t.Errorf("entry %q: desc %q does not name toggle key %q", e.key, e.desc, w.toggle)
		}
		if got := reg.Lookup(keys.ContextGlobal, w.toggle); got != w.act {
			t.Errorf("entry %q: key %q resolves to action %d, want %d", e.key, w.toggle, got, w.act)
		}
	}
}

func TestSearchHelpSections(t *testing.T) {
	sections := HelpSections()
	result := SearchHelpSections(sections, "shuffle")

	// "shuffle" should match the Global entry "Toggle shuffle".
	if len(result) == 0 {
		t.Fatal("searchHelpSections(\"shuffle\") returned no sections")
	}
	for _, sec := range result {
		for _, e := range sec.entries {
			lower := strings.ToLower(e.key) + " " + strings.ToLower(e.desc)
			if !strings.Contains(lower, "shuffle") {
				t.Errorf("section %q: entry (%q, %q) does not match query \"shuffle\"", sec.Title, e.key, e.desc)
			}
		}
	}

	// Only the pinned status-bar legend (its "S" row) and Global should match;
	// view-specific sections have no "shuffle" entry.
	want := []string{"Status bar indicators", "Global"}
	got := make([]string, len(result))
	for i, s := range result {
		got[i] = s.Title
	}
	if !slices.Equal(got, want) {
		t.Errorf("expected sections %v, got %v", want, got)
	}
}

func TestSearchHelpSectionsCaseInsensitive(t *testing.T) {
	sections := HelpSections()
	for _, query := range []string{"QUIT", "quit", "Quit"} {
		result := SearchHelpSections(sections, query)
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

// Regression: a stray space typed into the help search filter should be
// treated as an empty query rather than searching for a literal " " (which
// matches almost nothing and blanks the overlay).
func TestSearchHelpSectionsWhitespaceQuery(t *testing.T) {
	sections := HelpSections()
	for _, query := range []string{" ", "   ", "\t", " \n "} {
		result := SearchHelpSections(sections, query)
		if len(result) != len(sections) {
			t.Errorf("whitespace query %q: expected %d sections, got %d", query, len(sections), len(result))
		}
	}
}

func TestSearchHelpSectionsEmptyQuery(t *testing.T) {
	sections := HelpSections()
	result := SearchHelpSections(sections, "")
	if len(result) != len(sections) {
		t.Fatalf("empty query: expected %d sections, got %d", len(sections), len(result))
	}
	for i := range sections {
		if result[i].Title != sections[i].Title {
			t.Errorf("empty query: section %d title mismatch: %q vs %q", i, result[i].Title, sections[i].Title)
		}
		if len(result[i].entries) != len(sections[i].entries) {
			t.Errorf("empty query: section %q entry count mismatch: %d vs %d", result[i].Title, len(result[i].entries), len(sections[i].entries))
		}
	}
}
