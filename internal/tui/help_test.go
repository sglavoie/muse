package tui

import (
	"strings"
	"testing"
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
	// contextLibrary has no registry entries currently; contextModals dismiss
	// is handled in root.Update directly and is intentionally absent from the
	// registry.
	ctxSection := map[bindingContext]string{
		contextGlobal:    "Global",
		contextQueue:     "Queue",
		contextPlaylists: "Playlists",
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
	// The Modals and Library sections are intentionally excluded here because
	// their keys are handled outside the registry (esc in root.Update, / is a
	// global binding listed in both Global and Library for readability).
	reverseCtx := map[string]bindingContext{
		"Global":    contextGlobal,
		"Queue":     contextQueue,
		"Playlists": contextPlaylists,
	}
	for _, sec := range sections {
		ctx, ok := reverseCtx[sec.title]
		if !ok {
			continue
		}
		for _, e := range sec.entries {
			parts := strings.Split(e.key, " / ")
			anyFound := false
			for _, part := range parts {
				k := strings.TrimSpace(part)
				if reg.lookup(ctx, k) != actionNone || reg.lookup(contextGlobal, k) != actionNone {
					anyFound = true
					break
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
	h := HelpOverlay{logPath: "/var/log/muse/muse.log"}
	out := h.render()

	for _, header := range []string{"Global", "Library", "Playlists", "Queue", "Modals"} {
		if !strings.Contains(out, header) {
			t.Errorf("help overlay missing section header %q", header)
		}
	}
}

func TestHelpOverlayRendersRepresentativeEntries(t *testing.T) {
	h := HelpOverlay{logPath: "/var/log/muse/muse.log"}
	out := h.render()

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

func TestHelpOverlayRendersLogPath(t *testing.T) {
	logPath := "/state/muse/muse.log"
	h := HelpOverlay{logPath: logPath}
	out := h.render()

	if !strings.Contains(out, logPath) {
		t.Errorf("help overlay missing log path %q, got:\n%s", logPath, out)
	}
}

func TestHelpOverlayLogPathPrefixed(t *testing.T) {
	logPath := "/tmp/test.log"
	h := HelpOverlay{logPath: logPath}
	out := h.render()

	want := "log: " + logPath
	if !strings.Contains(out, want) {
		t.Errorf("expected %q in help overlay output, got:\n%s", want, out)
	}
}
