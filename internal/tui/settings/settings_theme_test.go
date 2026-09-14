package settings

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestThemeFieldsPreviewAndMerge(t *testing.T) {
	cfg := config.Defaults()
	o := NewSettingsOverlay(cfg, 80, cfg.MusicDirectories)
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
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	o := NewSettingsOverlay(cfg, 80, cfg.MusicDirectories)
	o.Tab, o.cursor = TabDisplay, displayPrimary
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
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
		if !strings.Contains(out, "Primary color") || !strings.Contains(out, "Use #RGB") {
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
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	o.PrimaryInput.SetValue("")
	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != ActionThemeLive || o.primaryColor != "" {
		t.Fatal("blank color should use the preset default")
	}
}
