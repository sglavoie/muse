package settings

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestThemeFieldsPreviewAndMerge(t *testing.T) {
	cfg := config.Defaults()
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab, o.cursor = TabDisplay, displayTheme
	for _, want := range []string{"dark", "light"} {
		var action Action
		o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
		preset, _ := o.ThemeState()
		if action != ActionThemeLive || preset != want {
			t.Fatalf("got preset=%q action=%v", preset, action)
		}
	}
	o.cursor = displayPrimary
	o = openHexEditor(t, o)
	var consumed bool
	o, _, consumed = o.ForwardMsg(tea.PasteMsg{Content: "#C4A"})
	if !consumed {
		t.Fatal("primary editor did not consume paste")
	}
	var action Action
	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != ActionThemeLive || o.PrimaryInput != nil {
		t.Fatal("valid color did not commit preview")
	}
	saved := o.ToConfig(cfg)
	if saved.Theme != "light" || saved.PrimaryColor != "#cc44aa" || saved.LogLevel != cfg.LogLevel {
		t.Fatalf("unexpected merge: theme=%q primary=%q", saved.Theme, saved.PrimaryColor)
	}
	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if action != ActionThemeLive || o.primaryColor != "" {
		t.Fatal("reset did not restore preset color")
	}
}

func TestPrimaryValidationAndEscape(t *testing.T) {
	cfg := config.Defaults()
	cfg.PrimaryColor = "#123456"
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab, o.cursor = TabDisplay, displayPrimary
	o = openHexEditor(t, o)
	o.PrimaryInput.SetValue("not a color")
	var action Action
	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != actionNone || o.PrimaryInput == nil || o.primaryError == "" || o.primaryColor != cfg.PrimaryColor {
		t.Fatal("invalid color was not retained with validation feedback")
	}
	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if action == ActionSave {
		t.Fatal("saved while invalid color was being edited")
	}
	for _, dark := range []bool{false, true} {
		out := ansi.Strip(o.Render(60, 12, styles.NewStyles(dark)))
		if strings.Contains(out, "…") {
			t.Fatalf("short input and controls should fit without truncation:\n%s", out)
		}
		if !strings.Contains(out, "Accent") || !strings.Contains(out, "Use #RGB") {
			t.Fatalf("small editor lost field or error:\n%s", out)
		}
		if len(strings.Split(strings.TrimSuffix(out, "\n"), "\n")) > 12 {
			t.Fatalf("small editor exceeds height:\n%s", out)
		}
		for _, line := range strings.Split(out, "\n") {
			if ansi.StringWidth(line) > 60 {
				t.Fatal("editor exceeds terminal width")
			}
		}
	}
	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != actionNone || o.PrimaryInput != nil || o.primaryColor != cfg.PrimaryColor {
		t.Fatal("escape should discard only the color edit")
	}
	o = openHexEditor(t, o)
	o.PrimaryInput.SetValue("")
	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != ActionThemeLive || o.primaryColor != "" {
		t.Fatal("blank color should use the preset default")
	}
}

func TestAccentCyclesNamedPresetsLive(t *testing.T) {
	cfg := config.Defaults()
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab, o.cursor = TabDisplay, displayPrimary
	if got := ansi.Strip(o.fieldSpinner(o.Fields[TabDisplay][displayPrimary])); got != "< Blue >" {
		t.Fatalf("default accent label = %q", got)
	}
	for _, want := range slices.Concat(config.AccentPresets[1:], []string{""}) {
		var action Action
		o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
		if _, primary := o.ThemeState(); action != ActionThemeLive || primary != want {
			t.Fatalf("got primary=%q action=%v, want %q", primary, action, want)
		}
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	last := config.AccentPresets[len(config.AccentPresets)-1]
	if o.primaryColor != last || o.ToConfig(cfg).PrimaryColor != last {
		t.Fatalf("h from blue should wrap to %s, got %q", last, o.primaryColor)
	}
	o.primaryColor = "#cc44aa"
	if got := o.fieldSpinner(o.Fields[TabDisplay][displayPrimary]); got != "< Custom #cc44aa >" {
		t.Fatalf("custom accent label = %q", got)
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if o.primaryColor != "" {
		t.Fatalf("l from custom should start at blue, got %q", o.primaryColor)
	}
}

// openHexEditor reaches the hex input through the picker's Custom cell.
func openHexEditor(t *testing.T, o SettingsOverlay) SettingsOverlay {
	t.Helper()
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'G', Text: "G"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if o.PrimaryInput == nil || o.accentPicker != nil {
		t.Fatal("Custom cell did not open the hex editor")
	}
	return o
}

func TestAccentPickerShowsEveryAccentAndPreviews(t *testing.T) {
	cfg := config.Defaults()
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab, o.cursor = TabDisplay, displayPrimary
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if o.accentPicker == nil {
		t.Fatal("enter did not open the accent picker")
	}
	for _, size := range [][2]int{{100, 30}, {60, 15}} {
		out := ansi.Strip(o.Render(size[0], size[1], styles.NewStyles(true)))
		for _, name := range config.AccentPresets {
			if !strings.Contains(out, capitalize(name)) {
				t.Fatalf("%dx%d picker hides %s:\n%s", size[0], size[1], name, out)
			}
		}
		for _, line := range strings.Split(out, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("picker exceeds width %d: %q", size[0], line)
			}
		}
	}
	var action Action
	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if want := config.AccentPresets[accentCols]; action != ActionThemeLive || o.primaryColor != want {
		t.Fatalf("j should preview %s, got %q (%v)", want, o.primaryColor, action)
	}
	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != ActionThemeLive || o.primaryColor != "" || o.accentPicker != nil {
		t.Fatal("esc should close the picker and restore the original accent")
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != actionNone || o.accentPicker != nil || o.ToConfig(cfg).PrimaryColor != config.AccentPresets[1] {
		t.Fatalf("enter should keep %s, got %q", config.AccentPresets[1], o.primaryColor)
	}
}

func TestAccentPickerCustomCellKeepsCustomColor(t *testing.T) {
	cfg := config.Defaults()
	cfg.PrimaryColor = "#cc44aa"
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab, o.cursor = TabDisplay, displayPrimary
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if o.accentPicker.cursor != accentCustomCell() {
		t.Fatal("custom color should open with the Custom cell focused")
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if action != ActionThemeLive || o.primaryColor != "#cc44aa" {
		t.Fatalf("returning to Custom should preview the original hex, got %q", o.primaryColor)
	}
}
