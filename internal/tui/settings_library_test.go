package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestLibraryTabInitializesFromMusicDirs(t *testing.T) {
	dirs := []string{"/music/rock", "/music/jazz"}
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, dirs)
	if len(o.libraryDirs) != 2 {
		t.Fatalf("expected 2 libraryDirs, got %d", len(o.libraryDirs))
	}
	if o.libraryDirs[0] != dirs[0] || o.libraryDirs[1] != dirs[1] {
		t.Errorf("libraryDirs = %v, want %v", o.libraryDirs, dirs)
	}
}

func TestLibraryTabDeleteDirRemovesEntry(t *testing.T) {
	dirs := []string{"/music/rock", "/music/jazz"}
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, dirs)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !o.libConfirmDel {
		t.Fatal("expected libConfirmDel=true after 'd'")
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if len(o.libraryDirs) != 1 {
		t.Fatalf("expected 1 dir after delete, got %d: %v", len(o.libraryDirs), o.libraryDirs)
	}
	if o.libraryDirs[0] != "/music/jazz" {
		t.Errorf("remaining dir = %q, want %q", o.libraryDirs[0], "/music/jazz")
	}
}

func TestLibraryTabDeleteRefusesIfOnlyOneDir(t *testing.T) {
	dirs := []string{"/music/rock"}
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, dirs)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if o.libConfirmDel {
		t.Error("expected libConfirmDel=false when only one directory remains")
	}
	if len(o.libraryDirs) != 1 {
		t.Errorf("expected 1 dir, got %d", len(o.libraryDirs))
	}
}

func TestLibraryTabDeleteCancelRestoresList(t *testing.T) {
	dirs := []string{"/music/rock", "/music/jazz"}
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, dirs)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if o.libConfirmDel {
		t.Error("expected libConfirmDel=false after esc")
	}
	if len(o.libraryDirs) != 2 {
		t.Errorf("expected 2 dirs after cancel, got %d", len(o.libraryDirs))
	}
}

func TestLibraryTabToConfigRoundTrip(t *testing.T) {
	dirs := []string{"/music/rock", "/music/jazz"}
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, dirs)
	got := o.ToConfig(config.Defaults())

	if len(got.MusicDirectories) != 2 {
		t.Fatalf("MusicDirectories: expected 2, got %d: %v", len(got.MusicDirectories), got.MusicDirectories)
	}
	if got.MusicDirectories[0] != dirs[0] || got.MusicDirectories[1] != dirs[1] {
		t.Errorf("MusicDirectories = %v, want %v", got.MusicDirectories, dirs)
	}
	if got.MusicDirectory != dirs[0] {
		t.Errorf("MusicDirectory = %q, want %q", got.MusicDirectory, dirs[0])
	}
}

func TestExcludedSubViewToggle(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	if o.libSubView != libSubViewDirs {
		t.Fatalf("initial libSubView: expected libSubViewDirs, got %v", o.libSubView)
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if o.libSubView != libSubViewExcluded {
		t.Errorf("'e': expected libSubViewExcluded, got %v", o.libSubView)
	}
}

func TestExcludedSubViewEscReturnsToDir(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if o.libSubView != libSubViewExcluded {
		t.Fatalf("precondition: expected libSubViewExcluded")
	}
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if o.libSubView != libSubViewDirs {
		t.Errorf("esc: expected libSubViewDirs, got %v", o.libSubView)
	}
	if action != settingsNone {
		t.Errorf("esc from excluded sub-view: expected settingsNone, got %v", action)
	}
}

func TestExcludedPathAddViaInput(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if o.exclPathInput == nil {
		t.Fatal("'a': expected exclPathInput to be non-nil")
	}

	for _, r := range "Podcasts" {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if o.exclPathInput != nil {
		t.Error("Enter: expected exclPathInput to be nil after confirm")
	}
	if len(o.excludedPaths) != 1 || o.excludedPaths[0] != "Podcasts" {
		t.Errorf("excludedPaths = %v, want [Podcasts]", o.excludedPaths)
	}
	if !o.dirty {
		t.Error("expected dirty=true after adding a pattern")
	}
}

func TestExcludedPathAddDuplicateIsNoop(t *testing.T) {
	cfg := config.Defaults()
	cfg.ExcludedPaths = []string{"Podcasts"}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	for _, r := range "Podcasts" {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(o.excludedPaths) != 1 {
		t.Errorf("duplicate add: expected 1 entry, got %d: %v", len(o.excludedPaths), o.excludedPaths)
	}
}

func TestExcludedPathAddInputEscCancels(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	for _, r := range "Podcasts" {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if o.exclPathInput != nil {
		t.Error("esc in input: expected exclPathInput to be nil")
	}
	if len(o.excludedPaths) != 0 {
		t.Errorf("esc in input: expected 0 paths, got %d", len(o.excludedPaths))
	}
}

func TestExcludedPathDeleteFlow(t *testing.T) {
	cfg := config.Defaults()
	cfg.ExcludedPaths = []string{"Podcasts", "Audiobooks"}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !o.exclConfirmDel {
		t.Fatal("'d': expected exclConfirmDel=true")
	}

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if o.exclConfirmDel {
		t.Error("confirm: expected exclConfirmDel=false")
	}
	if len(o.excludedPaths) != 1 || o.excludedPaths[0] != "Audiobooks" {
		t.Errorf("after delete: excludedPaths = %v, want [Audiobooks]", o.excludedPaths)
	}
}

func TestExcludedPathDeleteEscCancels(t *testing.T) {
	cfg := config.Defaults()
	cfg.ExcludedPaths = []string{"Podcasts"}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if o.exclConfirmDel {
		t.Error("esc: expected exclConfirmDel=false")
	}
	if len(o.excludedPaths) != 1 {
		t.Errorf("esc: expected 1 path, got %d", len(o.excludedPaths))
	}
}

func TestExcludedPathsToConfigRoundTrip(t *testing.T) {
	cfg := config.Defaults()
	cfg.ExcludedPaths = []string{"Podcasts", "Audiobooks"}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	got := o.ToConfig(cfg)
	if len(got.ExcludedPaths) != 2 {
		t.Fatalf("ExcludedPaths: expected 2, got %d: %v", len(got.ExcludedPaths), got.ExcludedPaths)
	}
	if got.ExcludedPaths[0] != "Podcasts" || got.ExcludedPaths[1] != "Audiobooks" {
		t.Errorf("ExcludedPaths = %v, want [Podcasts Audiobooks]", got.ExcludedPaths)
	}
}

func TestTabSwitchResetsToLibSubViewDirs(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if o.libSubView != libSubViewExcluded {
		t.Fatalf("precondition: expected libSubViewExcluded")
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.libSubView != libSubViewDirs {
		t.Errorf("tab switch: expected libSubView reset to libSubViewDirs, got %v", o.libSubView)
	}
}

func TestExcludedPathsRenderNone(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.libSubView = libSubViewExcluded
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "(none)") {
		t.Errorf("Render excluded empty: expected '(none)' in output, got:\n%s", out)
	}
}

func TestExcludedPathsRenderShowsPatterns(t *testing.T) {
	cfg := config.Defaults()
	cfg.ExcludedPaths = []string{"Podcasts", "Audiobooks"}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o.libSubView = libSubViewExcluded
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "Podcasts") {
		t.Errorf("Render excluded: expected 'Podcasts' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Audiobooks") {
		t.Errorf("Render excluded: expected 'Audiobooks' in output, got:\n%s", out)
	}
}
