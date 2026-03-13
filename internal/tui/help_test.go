package tui

import (
	"strings"
	"testing"
)

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
