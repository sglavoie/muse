package config

import "testing"

func TestLoad_EQ_Defaults_WhenOmitted(t *testing.T) {
	result := loadEQConfigFromContent(t, `default_volume = 80
`)
	cfg := result.Config

	if cfg.EQEnabled {
		t.Error("EQEnabled should default to false")
	}
	if cfg.EQPreset != defaultEQPreset {
		t.Errorf("EQPreset = %q, want %q", cfg.EQPreset, defaultEQPreset)
	}
	for i, v := range cfg.EQBands {
		if v != 0 {
			t.Errorf("EQBands[%d] = %d, want 0", i, v)
		}
	}
	if len(cfg.EQCustomPresets) != 0 {
		t.Errorf("EQCustomPresets len = %d, want 0", len(cfg.EQCustomPresets))
	}
}

func TestLoad_EQ_FieldsLoaded(t *testing.T) {
	result := loadEQConfigFromContent(t, `
eq_enabled = true
eq_preset = "rock"
eq_bands = [5, 4, 2, 0, -1, 0, 2, 3, 4, 5]
`)
	cfg := result.Config

	if !cfg.EQEnabled {
		t.Error("EQEnabled should be true")
	}
	if cfg.EQPreset != "rock" {
		t.Errorf("EQPreset = %q, want %q", cfg.EQPreset, "rock")
	}
	want := [10]int{5, 4, 2, 0, -1, 0, 2, 3, 4, 5}
	if cfg.EQBands != want {
		t.Errorf("EQBands = %v, want %v", cfg.EQBands, want)
	}
}

func TestLoad_EQ_PresetWhitespaceTrimmed(t *testing.T) {
	result := loadEQConfigFromContent(t, `eq_preset = "  rock  "
`)

	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
	if result.Config.EQPreset != "rock" {
		t.Errorf("EQPreset = %q, want %q", result.Config.EQPreset, "rock")
	}
}

func TestLoad_EQ_CustomSentinelPresetAccepted(t *testing.T) {
	result := loadEQConfigFromContent(t, `eq_preset = "custom"
eq_bands = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
`)

	if result.Config.EQPreset != "custom" {
		t.Errorf("EQPreset = %q, want %q", result.Config.EQPreset, "custom")
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none for custom sentinel preset", result.Warnings)
	}
}

func TestLoad_EQ_BandsClamped(t *testing.T) {
	result := loadEQConfigFromContent(t, `eq_bands = [100, -100, 0, 0, 0, 0, 0, 0, 0, 0]
`)

	if result.Config.EQBands[0] != 12 {
		t.Errorf("EQBands[0] = %d, want 12 (clamped)", result.Config.EQBands[0])
	}
	if result.Config.EQBands[1] != -12 {
		t.Errorf("EQBands[1] = %d, want -12 (clamped)", result.Config.EQBands[1])
	}
}

func TestLoad_EQ_InvalidBandCountUsesDefault(t *testing.T) {
	result := loadEQConfigFromContent(t, `
default_volume = 75
eq_bands = [1, 2, 3]
`)

	if result.Config.DefaultVolume != 75 {
		t.Errorf("DefaultVolume = %d, want 75 (invalid eq_bands should not discard unrelated fields)", result.Config.DefaultVolume)
	}
	if result.Config.EQBands != ([10]int{}) {
		t.Errorf("EQBands = %v, want default zero bands", result.Config.EQBands)
	}
	requireWarningContaining(t, result.Warnings, "eq_bands", "expected 10")
}

func TestLoad_EQ_InvalidPreset_FallsBackToFlat(t *testing.T) {
	result := loadEQConfigFromContent(t, `eq_preset = "does_not_exist"
`)

	if result.Config.EQPreset != defaultEQPreset {
		t.Errorf("EQPreset = %q, want %q on invalid key", result.Config.EQPreset, defaultEQPreset)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected a warning for unknown eq_preset")
	}
}
