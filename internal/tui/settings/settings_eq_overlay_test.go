package settings

import (
	"testing"

	"github.com/sglavoie/muse/internal/config"
)

func TestEQTabFieldCount(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	eq := o.Fields[TabEqualizer]
	if len(eq) != 12 {
		t.Fatalf("EQ tab: expected 12 fields, got %d", len(eq))
	}
}

func TestEQTabPresetsPopulated(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	want := len(config.BuiltinEQPresets) + 1
	if len(o.EQPresets) != want {
		t.Fatalf("eqPresets: expected %d entries, got %d", want, len(o.EQPresets))
	}
	last := o.EQPresets[len(o.EQPresets)-1]
	if last.Name != eqCustomSentinelName {
		t.Errorf("last eqPreset: expected %q, got %q", eqCustomSentinelName, last.Name)
	}
}

func TestEQPresetFieldMaxMatchesSentinelIndex(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	presetField := o.Fields[TabEqualizer][1]
	wantMax := len(o.EQPresets) - 1
	if presetField.max != wantMax {
		t.Errorf("Preset field Max: expected %d, got %d", wantMax, presetField.max)
	}
}

func TestEQDefaultPresetIsFlatWhenConfigIsFlat(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "flat"
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	presetIdx := o.Fields[TabEqualizer][1].Value
	if o.EQPresets[presetIdx].Key != "flat" {
		t.Errorf("default preset: expected key %q, got %q", "flat", o.EQPresets[presetIdx].Key)
	}
}

func TestEQUnknownPresetFallsBackToCustomSentinel(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "nonexistent_preset"
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	presetIdx := o.Fields[TabEqualizer][1].Value
	sentinelIdx := len(o.EQPresets) - 1
	if presetIdx != sentinelIdx {
		t.Errorf("unknown preset: expected sentinel index %d, got %d", sentinelIdx, presetIdx)
	}
}

func TestCustomPresetsFromConfigPopulateOverlay(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "My Preset", Key: "my_preset", Bands: [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)

	want := len(config.BuiltinEQPresets) + 1 + 1
	if len(o.EQPresets) != want {
		t.Fatalf("eqPresets with custom: expected %d, got %d", want, len(o.EQPresets))
	}
	sentinel := len(o.EQPresets) - 1
	custom := o.EQPresets[sentinel-1]
	if custom.Key != "my_preset" {
		t.Errorf("custom preset key: expected %q, got %q", "my_preset", custom.Key)
	}
	if custom.Builtin {
		t.Error("custom preset should have Builtin=false")
	}
}

func TestEQStateHelper(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQEnabled = true
	cfg.EQBands = [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)

	enabled, bands := o.EQState()
	if !enabled {
		t.Error("EQState: expected enabled=true")
	}
	for i, v := range bands {
		if v != cfg.EQBands[i] {
			t.Errorf("EQState: band %d: expected %d, got %d", i, cfg.EQBands[i], v)
		}
	}
}
