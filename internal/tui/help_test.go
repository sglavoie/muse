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
		contextGlobal:    "Global",
		contextLibrary:   "Library",
		contextQueue:     "Queue",
		contextPlaylists: "Playlists",
		contextArtists:   "Artists",
		contextAlbums:    "Albums",
	}

	// The "Navigation" section covers keys shared across Library, Queue, and
	// Playlists. Merge its keys into each of those sections for the check.
	if navKeys, ok := sectionKeys["Navigation (Library / Queue / Playlists / Artists / Albums)"]; ok {
		for _, sec := range []string{"Library", "Queue", "Playlists", "Artists", "Albums"} {
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
		"Global":    contextGlobal,
		"Library":   contextLibrary,
		"Queue":     contextQueue,
		"Playlists": contextPlaylists,
	}
	// Navigation keys are shared; check that each appears in at least one
	// of the three view contexts.
	navContexts := []bindingContext{contextLibrary, contextQueue, contextPlaylists, contextArtists, contextAlbums}
	for _, sec := range sections {
		ctx, directMatch := reverseCtx[sec.title]
		isNav := sec.title == "Navigation (Library / Queue / Playlists / Artists / Albums)"
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

func TestHelpLineCountPositive(t *testing.T) {
	n := helpLineCount()
	if n <= 0 {
		t.Errorf("helpLineCount() = %d, want > 0", n)
	}
	// Should include at least the heading, 6 section headers, and their entries
	if n < 20 {
		t.Errorf("helpLineCount() = %d, expected at least 20 lines for all sections", n)
	}
}

func TestHelpOverlayRendersAllSectionHeaders(t *testing.T) {
	h := HelpOverlay{}
	out := h.render(styles.Styles{})

	for _, header := range []string{"Global", "Library", "Playlists", "Artists", "Albums", "Queue", "Modals"} {
		if !strings.Contains(out, header) {
			t.Errorf("help overlay missing section header %q", header)
		}
	}
}

func TestHelpOverlayRendersRepresentativeEntries(t *testing.T) {
	h := HelpOverlay{}
	out := h.render(styles.Styles{})

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
	}

	for _, c := range checks {
		if !strings.Contains(out, c.text) {
			t.Errorf("help overlay (%s section): expected entry containing %q", c.section, c.text)
		}
	}
}
