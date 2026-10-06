package tui

import (
	"context"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/config"
)

func themeKey(m Model, key string) Model {
	next, _ := m.Update(djPanelKey(key))
	return next.(Model)
}

func TestExplicitThemeSurvivesTerminalDetection(t *testing.T) {
	for _, preset := range []string{"dark", "light"} {
		cfg := config.Defaults()
		cfg.Theme, cfg.PrimaryColor = preset, "#ff4400"
		m := New(context.Background(), WithConfig(cfg))
		if m.styles.HasDark != (preset == "dark") {
			t.Fatal("configured theme was not applied at startup")
		}
		for _, c := range []color.Color{color.White, color.Black} {
			next, _ := m.Update(tea.BackgroundColorMsg{Color: c})
			m = next.(Model)
			if m.styles.HasDark != (preset == "dark") {
				t.Fatal("terminal response overrode explicit theme")
			}
			r, g, b, _ := m.styles.Primary.RGBA()
			if r != 0xffff || g != 0x4444 || b != 0 {
				t.Fatal("terminal response lost primary override")
			}
		}
	}
}

func TestThemePreviewCancelAndAutoDetection(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(tea.BackgroundColorMsg{Color: color.White})
	m = next.(Model)
	before := m.styles
	for _, key := range []string{"ctrl+,", "[", "j", "j", "j", "l"} {
		m = themeKey(m, key)
	}
	if !m.styles.HasDark || before.HasDark || m.cfg.Theme != "auto" {
		t.Fatal("preview did not switch independently of saved config and previous styles")
	}
	next, _ = m.Update(tea.BackgroundColorMsg{Color: color.White})
	m = next.(Model)
	if !m.styles.HasDark {
		t.Fatal("background detection overrode preview")
	}
	m = themeKey(m, "esc")
	if !m.styles.HasDark || m.overlay.settings == nil {
		t.Fatal("preview reverted before discard")
	}
	m = themeKey(m, "enter")
	if !m.styles.HasDark || m.overlay.settings == nil {
		t.Fatal("keep editing lost preview")
	}
	for _, key := range []string{"esc", "tab", "enter"} {
		m = themeKey(m, key)
	}
	if m.styles.HasDark || m.overlay.settings != nil {
		t.Fatal("cancel did not restore detected light theme")
	}
	// An explicit preset must retain detection so returning to Auto is correct.
	m.cfg.Theme = "dark"
	m = m.applyTheme()
	for _, key := range []string{"ctrl+,", "[", "j", "j", "j", "h"} {
		m = themeKey(m, key)
	}
	if m.styles.HasDark {
		t.Fatal("Auto preview did not use remembered terminal background")
	}
}

func TestThemeSavePersistsAndReopens(t *testing.T) {
	dir := t.TempDir()
	m := New(context.Background(), WithConfigPath(config.ConfigPath(dir), false))
	for _, key := range []string{"ctrl+,", "[", "j", "j", "j", "l", "l", "j", "enter"} {
		m = themeKey(m, key)
	}
	next, _ := m.Update(tea.PasteMsg{Content: "#C4A"})
	m = themeKey(next.(Model), "enter")
	if m.styles.HasDark || m.cfg.PrimaryColor != "" {
		t.Fatal("draft preview should be light without changing saved config")
	}
	m, cmd := m.handleSettingsSave()
	if cmd == nil {
		t.Fatal("save returned no persistence command")
	}
	if msg, ok := cmd().(settingsSavedMsg); !ok || msg.err != nil {
		t.Fatalf("save result: %+v", msg)
	}
	loaded, err := config.Load(dir)
	if err != nil || loaded.Config.Theme != "light" || loaded.Config.PrimaryColor != "#cc44aa" {
		t.Fatalf("saved theme did not round trip: %+v, %v", loaded, err)
	}
	m = themeKey(m, "ctrl+,")
	preset, primary := m.overlay.settings.ThemeState()
	if preset != "light" || primary != "#cc44aa" {
		t.Fatal("reopened settings lost the saved theme")
	}
}

func TestThemedViewsFillSupportedAndSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {60, 15}, {40, 10}} {
		for _, preset := range []string{"light", "dark"} {
			cfg := config.Defaults()
			cfg.Theme = preset
			m := New(context.Background(), WithConfig(cfg))
			next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = next.(Model)
			out := m.View().Content
			lines := strings.Split(ansi.Strip(out), "\n")
			if len(lines) != size[1] {
				t.Errorf("%s %v: got %d rows", preset, size, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != size[0] {
					t.Errorf("%s %v: row width %d", preset, size, ansi.StringWidth(line))
				}
			}
		}
	}
}

func TestThemeCanvasColorsAcrossViewsAndSettings(t *testing.T) {
	for _, preset := range []string{"light", "dark"} {
		for _, fixture := range []string{FixtureRich, FixtureNarrow, FixturePlayback} {
			t.Run(preset+"/"+fixture, func(t *testing.T) {
				m, err := BuildFixture(fixture)
				if err != nil {
					t.Fatal(err)
				}
				m.cfg.Theme = preset
				m = m.applyTheme()
				next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
				m = next.(Model)
				for _, view := range viewOrder {
					m.ActiveView = view
					assertThemeCellColors(t, m)
				}
				for _, key := range []string{"ctrl+,", "[", "j", "j", "j"} {
					m = themeKey(m, key)
				}
				assertThemeCellColors(t, m)
			})
		}
	}
}

func assertThemeCellColors(t *testing.T, m Model) {
	t.Helper()
	out := m.View().Content
	canvas := lipgloss.NewCanvas(lipgloss.Width(out), lipgloss.Height(out)).Compose(lipgloss.NewLayer(out))
	for y := range canvas.Height() {
		for x := range canvas.Width() {
			cell := canvas.CellAt(x, y)
			if cell.Width == 0 {
				continue // wide-character continuation has no independent style
			}
			if cell.Style.Fg == nil || cell.Style.Bg == nil {
				t.Fatalf("%s cell (%d,%d) %q falls back to terminal colors", ViewName(m.ActiveView), x, y, cell.Content)
			}
		}
	}
}

func TestPaletteToggleThemePersistsAndPreservesPrimary(t *testing.T) {
	for _, tc := range []struct {
		preset string
		dark   bool
		want   string
	}{
		{"dark", true, "light"},
		{"light", false, "dark"},
		{"auto", true, "light"},
		{"auto", false, "dark"},
	} {
		t.Run(tc.preset+"-to-"+tc.want, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.Theme, cfg.PrimaryColor = tc.preset, "#cc44aa"
			dir := t.TempDir()
			m := New(context.Background(), WithConfig(cfg), WithConfigPath(config.ConfigPath(dir), false))
			terminalColor := color.White
			if tc.dark {
				terminalColor = color.Black
			}
			next, _ := m.Update(tea.BackgroundColorMsg{Color: terminalColor})
			m = next.(Model)
			for _, want := range []string{tc.want, map[string]string{"light": "dark", "dark": "light"}[tc.want]} {
				var toggle tea.Cmd
				for _, command := range m.BuildPaletteCommands() {
					if command.ID == "ui.toggle.theme" {
						if command.Name != "Toggle theme" || (command.Enabled != nil && !command.Enabled()) {
							t.Fatal("theme command should be discoverable and enabled")
						}
						toggle = command.Exec()
					}
				}
				if toggle == nil {
					t.Fatal("theme command missing")
				}
				next, save := m.Update(toggle())
				m = next.(Model)
				if m.cfg.Theme != want || m.styles.HasDark != (want == "dark") || m.cfg.PrimaryColor != cfg.PrimaryColor {
					t.Fatalf("toggle got theme=%s dark=%v primary=%s", m.cfg.Theme, m.styles.HasDark, m.cfg.PrimaryColor)
				}
				if save == nil {
					t.Fatal("toggle did not persist")
				}
				if msg, ok := save().(settingsSavedMsg); !ok || msg.err != nil {
					t.Fatalf("save result: %+v", msg)
				}
				loaded, err := config.Load(dir)
				if err != nil || loaded.Config.Theme != want || loaded.Config.PrimaryColor != cfg.PrimaryColor || loaded.Config.DefaultVolume != cfg.DefaultVolume {
					t.Fatalf("toggle persistence failed: %+v, %v", loaded, err)
				}
			}
		})
	}
}
