package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
)

// TestSettingsSaveExcludedPathsChangedTriggersRescan verifies that changing
// excluded paths triggers a rescan command on save.
func TestSettingsSaveExcludedPathsChangedTriggersRescan(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults()
	cfg.MusicDirectories = []string{root}
	cfg.ExcludedPaths = nil
	m := New(context.Background(),
		WithConfig(cfg),
		WithLibraryRoots([]string{root}),
		WithExtractor(stubExtractor{}),
	)

	// Open settings and navigate to Library tab (default).
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)

	// Switch to the excluded paths sub-view and add a pattern.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	nm = next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm = next.(Model)
	for _, r := range "Podcasts" {
		next, _ = nm.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		nm = next.(Model)
	}
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm = next.(Model)

	// Save — must produce a rescan cmd in addition to the save cmd.
	_, cmd := nm.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+s after excluded paths change: expected non-nil cmd (rescan + save)")
	}
}

// TestSettingsSaveLibraryDirsUnchangedExcludedPathsChangedTriggersRescan
// verifies that changing only excluded paths (with unchanged library dirs)
// still triggers a rescan.
func TestSettingsSaveLibraryDirsUnchangedExcludedPathsChangedTriggersRescan(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults()
	cfg.MusicDirectories = []string{root}
	cfg.ExcludedPaths = []string{"Podcasts"}
	m := New(context.Background(),
		WithConfig(cfg),
		WithLibraryRoots([]string{root}),
		WithExtractor(stubExtractor{}),
	)

	// Open settings (Library tab, dirs sub-view).
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)

	// Navigate to excluded sub-view and delete the existing pattern.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	nm = next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm = next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm = next.(Model)

	// The overlay should have 0 excluded paths now.
	if len(nm.overlay.settings.excludedPaths) != 0 {
		t.Fatalf("precondition: expected 0 excludedPaths in overlay, got %d",
			len(nm.overlay.settings.excludedPaths))
	}

	_, cmd := nm.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+s after excluded paths change: expected non-nil cmd (rescan + save)")
	}
}
