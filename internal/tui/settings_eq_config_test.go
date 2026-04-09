package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/config"
)

func TestEQToConfigRoundTrip(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQEnabled = true
	cfg.EQPreset = "rock"
	cfg.EQBands = [10]int{5, 4, 2, 0, -1, 0, 2, 3, 4, 5}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)

	got := o.ToConfig(cfg)

	if !got.EQEnabled {
		t.Error("ToConfig: expected EQEnabled=true")
	}
	if got.EQPreset != "rock" {
		t.Errorf("ToConfig: expected EQPreset=%q, got %q", "rock", got.EQPreset)
	}
	for i, v := range got.EQBands {
		if v != cfg.EQBands[i] {
			t.Errorf("ToConfig: band %d: expected %d, got %d", i, cfg.EQBands[i], v)
		}
	}
}

func TestEQToConfigCustomSentinelKeyIsCustom(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.fields[settingsTabEqualizer][1].Value = len(o.eqPresets) - 1
	got := o.ToConfig(config.Defaults())
	if got.EQPreset != "custom" {
		t.Errorf("ToConfig with sentinel: expected EQPreset=%q, got %q", "custom", got.EQPreset)
	}
}

func TestToConfigIncludesCustomPresets(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "My Sound", Key: "my_sound", Bands: [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	got := o.ToConfig(cfg)

	if len(got.EQCustomPresets) != 1 {
		t.Fatalf("ToConfig: expected 1 custom preset, got %d", len(got.EQCustomPresets))
	}
	if got.EQCustomPresets[0].Key != "my_sound" {
		t.Errorf("ToConfig: expected key %q, got %q", "my_sound", got.EQCustomPresets[0].Key)
	}
}

func TestToConfigExcludesBuiltinsAndSentinel(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	got := o.ToConfig(config.Defaults())

	if len(got.EQCustomPresets) != 0 {
		t.Errorf("ToConfig with no custom presets: expected 0, got %d", len(got.EQCustomPresets))
	}
}
