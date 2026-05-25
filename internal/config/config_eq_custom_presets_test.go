package config

import "testing"

const duplicateEQCustomPresetConfig = `
[[eq_custom_presets]]
name = "First"
key = "dup"
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

[[eq_custom_presets]]
name = "Second"
key = "dup"
bands = [1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
`

func TestLoad_EQ_CustomPreset_RoundTrip(t *testing.T) {
	result := loadEQConfigFromContent(t, `
eq_preset = "my_sound"

[[eq_custom_presets]]
name = "My Sound"
key = "my_sound"
bands = [3, 1, 0, -2, 0, 0, 1, 2, 3, 4]

[[eq_custom_presets]]
name = "Late Night"
key = "late_night"
bands = [-2, -1, 0, 2, 3, 2, 0, -1, -3, -4]
`)
	cfg := result.Config

	if len(cfg.EQCustomPresets) != 2 {
		t.Fatalf("EQCustomPresets len = %d, want 2", len(cfg.EQCustomPresets))
	}
	if cfg.EQCustomPresets[0].Key != "my_sound" {
		t.Errorf("EQCustomPresets[0].Key = %q, want %q", cfg.EQCustomPresets[0].Key, "my_sound")
	}
	if cfg.EQCustomPresets[1].Key != "late_night" {
		t.Errorf("EQCustomPresets[1].Key = %q, want %q", cfg.EQCustomPresets[1].Key, "late_night")
	}
	if cfg.EQPreset != "my_sound" {
		t.Errorf("EQPreset = %q, want %q", cfg.EQPreset, "my_sound")
	}
}

func TestLoad_EQ_CustomPresets_Deduplicated(t *testing.T) {
	result := loadEQConfigFromContent(t, duplicateEQCustomPresetConfig)

	if len(result.Config.EQCustomPresets) != 1 {
		t.Errorf("EQCustomPresets len = %d, want 1 after dedup", len(result.Config.EQCustomPresets))
	}
}

func TestLoad_EQ_CustomPresetInvalidBandCountDropped(t *testing.T) {
	result := loadEQConfigFromContent(t, `
default_volume = 75

[[eq_custom_presets]]
name = "Too Short"
key = "too_short"
bands = [1, 2, 3]

[[eq_custom_presets]]
name = "Valid"
key = "valid"
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`)

	if result.Config.DefaultVolume != 75 {
		t.Errorf("DefaultVolume = %d, want 75 (bad custom preset should not discard unrelated fields)", result.Config.DefaultVolume)
	}
	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(result.Config.EQCustomPresets))
	}
	if got := result.Config.EQCustomPresets[0].Key; got != "valid" {
		t.Errorf("kept custom preset key = %q, want valid", got)
	}
	requireWarningContaining(t, result.Warnings, "too_short", "expected 10")
}

func TestLoad_EQ_CustomPreset_BandsClamped(t *testing.T) {
	result := loadEQConfigFromContent(t, `
[[eq_custom_presets]]
name = "Over"
key = "over"
bands = [99, -99, 0, 0, 0, 0, 0, 0, 0, 0]
`)

	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(result.Config.EQCustomPresets))
	}
	preset := result.Config.EQCustomPresets[0]
	if preset.Bands[0] != 12 {
		t.Errorf("custom preset band[0] = %d, want 12 (clamped)", preset.Bands[0])
	}
	if preset.Bands[1] != -12 {
		t.Errorf("custom preset band[1] = %d, want -12 (clamped)", preset.Bands[1])
	}
}

func TestLoad_EQCustomPresets_KeyWhitespaceTrimmed(t *testing.T) {
	result := loadEQConfigFromContent(t, `
[[eq_custom_presets]]
name = "Padded"
key = "  padded_key  "
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`)

	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(result.Config.EQCustomPresets))
	}
	if got := result.Config.EQCustomPresets[0].Key; got != "padded_key" {
		t.Errorf("Key = %q, want %q (whitespace should be trimmed)", got, "padded_key")
	}
}

func TestLoad_EQCustomPresets_EmptyKeyDroppedWithWarning(t *testing.T) {
	result := loadEQConfigFromContent(t, `
[[eq_custom_presets]]
name = "No Key"
key = ""
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

[[eq_custom_presets]]
name = "Whitespace Key"
key = "   "
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`)

	if len(result.Config.EQCustomPresets) != 0 {
		t.Fatalf("EQCustomPresets len = %d, want 0", len(result.Config.EQCustomPresets))
	}
	requireWarningCountContaining(t, result.Warnings, "empty", 2)
}

func TestLoad_EQCustomPresets_EmptyNameFallsBackToKey(t *testing.T) {
	result := loadEQConfigFromContent(t, `
[[eq_custom_presets]]
name = ""
key = "rock_remix"
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`)

	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(result.Config.EQCustomPresets))
	}
	if got := result.Config.EQCustomPresets[0].Name; got != "rock_remix" {
		t.Errorf("Name = %q, want %q (should fall back to key)", got, "rock_remix")
	}
}

func TestLoad_EQCustomPresets_DuplicateKeyWarns(t *testing.T) {
	result := loadEQConfigFromContent(t, duplicateEQCustomPresetConfig)

	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1 after dedup", len(result.Config.EQCustomPresets))
	}
	if result.Config.EQCustomPresets[0].Name != "First" {
		t.Errorf("kept preset name = %q, want %q", result.Config.EQCustomPresets[0].Name, "First")
	}
	requireWarningContaining(t, result.Warnings, "duplicated")
}

func TestLoad_EQCustomPresets_BuiltinKeyDropped(t *testing.T) {
	result := loadEQConfigFromContent(t, `
[[eq_custom_presets]]
name = "Rock Copy"
key = "rock"
bands = [1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
`)

	if len(result.Config.EQCustomPresets) != 0 {
		t.Fatalf("EQCustomPresets len = %d, want 0 for built-in key conflict", len(result.Config.EQCustomPresets))
	}
	requireWarningContaining(t, result.Warnings, "built-in preset", "rock")
}
