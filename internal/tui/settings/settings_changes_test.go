package settings

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestSettingsChangedFieldsAndTabsFitCompactWindow(t *testing.T) {
	cfg := config.Defaults()
	cfg.MusicDirectories = []string{"/music", "/other"}
	cfg.ExcludedPaths = []string{"*/skip/*"}
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab = TabPlayback
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	o.Fields[TabDisplay][displayTheme].Value = 1
	o.Fields[TabEqualizer][2].Value++
	o.LibraryDirs = o.LibraryDirs[:1]
	for tab := tabLibrary; tab < tabCount; tab++ {
		o.Tab = tab
		screen := ansi.Strip(o.Render(60, 12, styles.NewStyles(true)))
		for _, want := range []string{"4 unsaved changes", "Library*", "Playback*", "Equalizer*", "Display*", "ctrl+s save", "esc"} {
			if !strings.Contains(screen, want) {
				t.Fatalf("tab %d missing %q:\n%s", tab, want, screen)
			}
		}
		if len(strings.Split(strings.TrimRight(screen, "\n"), "\n")) > 12 {
			t.Fatalf("too tall:\n%s", screen)
		}
		for _, line := range strings.Split(screen, "\n") {
			if ansi.StringWidth(line) > 60 {
				t.Fatalf("too wide: %s", line)
			}
		}
		if tab == TabPlayback && !strings.Contains(screen, "Volume step *") {
			t.Fatal("changed field is unmarked")
		}
	}
	o.Tab = TabPlayback
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	screen := ansi.Strip(o.Render(60, 12, styles.NewStyles(true)))
	if !strings.Contains(screen, "3 unsaved changes") || strings.Contains(screen, "Playback*") || strings.Contains(screen, "Volume step *") {
		t.Fatalf("restored field remains marked:\n%s", screen)
	}
}

func TestSettingsChangeCountCoversCollectionsAndRestoration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tab    Tab
		change func(*SettingsOverlay)
	}{
		{"directories", tabLibrary, func(o *SettingsOverlay) { o.LibraryDirs = append(o.LibraryDirs, "/another") }},
		{"exclusions", tabLibrary, func(o *SettingsOverlay) { o.ExcludedPaths = append(o.ExcludedPaths, "*/skip/*") }},
		{"presets", TabEqualizer, func(o *SettingsOverlay) {
			o.EQPresets = append(o.EQPresets, config.EQPresetDef{Name: "Mine", Key: "mine"})
		}},
		{"color", TabDisplay, func(o *SettingsOverlay) { o.primaryColor = "#abcdef" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Defaults()
			o := NewSettingsOverlay(cfg, []string{"/music"})
			for _, fields := range o.changedFields() {
				if changedCount(fields) != 0 {
					t.Fatal("opening state marked as changed")
				}
			}
			tc.change(&o)
			changes := o.changedFields()
			if changedCount(changes[tc.tab]) != 1 || !o.hasChanges() {
				t.Fatalf("change not reflected: %v", changes)
			}
		})
	}
}
