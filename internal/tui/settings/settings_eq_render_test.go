package settings

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestEQRenderEqualizerTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	o.Tab = TabEqualizer
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	for _, label := range []string{"Enabled", "Preset", "31 Hz", "16 kHz"} {
		if !strings.Contains(out, label) {
			t.Errorf("Render EQ tab: expected field label %q in output", label)
		}
	}
}

func TestEQRenderShowsPresetName(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "bass_boost"
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab = TabEqualizer
	o.cursor = 1
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "Bass Boost") {
		t.Errorf("Render EQ tab: expected preset name 'Bass Boost' in output, got:\n%s", out)
	}
}

func TestEQRenderShowsNameInputPrompt(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	o.Tab = TabEqualizer
	input := NewSettingsInput()
	input.SetValue("My")
	o.EQNameInput = input
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "Save as:") {
		t.Errorf("Render with name input: expected 'Save as:' in output, got:\n%s", out)
	}
}

func TestEQRenderShowsDeleteConfirmation(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "Night Mode", Key: "night_mode", Bands: [10]int{}},
	}
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab = TabEqualizer
	o.cursor = 1
	o.Fields[TabEqualizer][1].Value = len(config.BuiltinEQPresets)
	o.EQConfirmDel = true
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "Delete") {
		t.Errorf("Render with delete confirm: expected 'Delete' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Night Mode") {
		t.Errorf("Render with delete confirm: expected 'Night Mode' in output, got:\n%s", out)
	}
}

func TestEQHintLineShowsOnEQTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	o.Tab = TabEqualizer
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "s save") {
		t.Errorf("EQ tab hint: expected 's save' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "d delete") {
		t.Errorf("EQ tab hint: expected 'd delete' in output, got:\n%s", out)
	}
}

func TestEQHintLineIncludesJKFields(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	o.Tab = TabEqualizer
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "j/k navigate") {
		t.Errorf("EQ tab hint: expected 'j/k navigate' in output, got:\n%s", out)
	}
}
