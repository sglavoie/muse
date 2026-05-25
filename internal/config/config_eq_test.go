package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestLoad_EQ_Defaults_WhenOmitted(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte("default_volume = 80\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
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
	dir := t.TempDir()
	content := `
eq_enabled = true
eq_preset = "rock"
eq_bands = [5, 4, 2, 0, -1, 0, 2, 3, 4, 5]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
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
	dir := t.TempDir()
	content := `eq_preset = "  rock  "
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
	if result.Config.EQPreset != "rock" {
		t.Errorf("EQPreset = %q, want %q", result.Config.EQPreset, "rock")
	}
}

func TestLoad_EQ_BandsClamped(t *testing.T) {
	dir := t.TempDir()
	content := "eq_bands = [100, -100, 0, 0, 0, 0, 0, 0, 0, 0]\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.EQBands[0] != 12 {
		t.Errorf("EQBands[0] = %d, want 12 (clamped)", result.Config.EQBands[0])
	}
	if result.Config.EQBands[1] != -12 {
		t.Errorf("EQBands[1] = %d, want -12 (clamped)", result.Config.EQBands[1])
	}
}

func TestLoad_EQ_InvalidBandCountUsesDefault(t *testing.T) {
	dir := t.TempDir()
	content := `
default_volume = 75
eq_bands = [1, 2, 3]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.DefaultVolume != 75 {
		t.Errorf("DefaultVolume = %d, want 75 (invalid eq_bands should not discard unrelated fields)", result.Config.DefaultVolume)
	}
	if result.Config.EQBands != ([10]int{}) {
		t.Errorf("EQBands = %v, want default zero bands", result.Config.EQBands)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "eq_bands") && strings.Contains(w, "expected 10") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about eq_bands band count, got: %v", result.Warnings)
	}
}

func TestLoad_EQ_InvalidPreset_FallsBackToFlat(t *testing.T) {
	dir := t.TempDir()
	content := "eq_preset = \"does_not_exist\"\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.EQPreset != defaultEQPreset {
		t.Errorf("EQPreset = %q, want %q on invalid key", result.Config.EQPreset, defaultEQPreset)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected a warning for unknown eq_preset")
	}
}

func TestLoad_EQ_CustomPreset_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	content := `
eq_preset = "my_sound"

[[eq_custom_presets]]
name = "My Sound"
key = "my_sound"
bands = [3, 1, 0, -2, 0, 0, 1, 2, 3, 4]

[[eq_custom_presets]]
name = "Late Night"
key = "late_night"
bands = [-2, -1, 0, 2, 3, 2, 0, -1, -3, -4]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
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
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = "Dup"
key = "dup"
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

[[eq_custom_presets]]
name = "Dup Again"
key = "dup"
bands = [1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.EQCustomPresets) != 1 {
		t.Errorf("EQCustomPresets len = %d, want 1 after dedup", len(result.Config.EQCustomPresets))
	}
}

func TestLoad_EQ_CustomPresetInvalidBandCountDropped(t *testing.T) {
	dir := t.TempDir()
	content := `
default_volume = 75

[[eq_custom_presets]]
name = "Too Short"
key = "too_short"
bands = [1, 2, 3]

[[eq_custom_presets]]
name = "Valid"
key = "valid"
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.DefaultVolume != 75 {
		t.Errorf("DefaultVolume = %d, want 75 (bad custom preset should not discard unrelated fields)", result.Config.DefaultVolume)
	}
	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(result.Config.EQCustomPresets))
	}
	if got := result.Config.EQCustomPresets[0].Key; got != "valid" {
		t.Errorf("kept custom preset key = %q, want valid", got)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "too_short") && strings.Contains(w, "expected 10") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about custom preset band count, got: %v", result.Warnings)
	}
}

func TestLoad_EQ_CustomPreset_BandsClamped(t *testing.T) {
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = "Over"
key = "over"
bands = [99, -99, 0, 0, 0, 0, 0, 0, 0, 0]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(result.Config.EQCustomPresets))
	}
	p := result.Config.EQCustomPresets[0]
	if p.Bands[0] != 12 {
		t.Errorf("custom preset band[0] = %d, want 12 (clamped)", p.Bands[0])
	}
	if p.Bands[1] != -12 {
		t.Errorf("custom preset band[1] = %d, want -12 (clamped)", p.Bands[1])
	}
}

func TestSave_EQ_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := defaults()
	cfg.EQEnabled = true
	cfg.EQPreset = "bass_boost"
	cfg.EQBands = [10]int{6, 5, 4, 2, 0, 0, 0, 0, 0, 0}
	cfg.EQCustomPresets = []EQCustomPreset{
		{Name: "My Preset", Key: "my_preset", Bands: [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var raw rawConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		t.Fatalf("DecodeFile() error = %v", err)
	}
	if raw.EQEnabled == nil || !*raw.EQEnabled {
		t.Error("EQEnabled not persisted")
	}
	if raw.EQPreset != "bass_boost" {
		t.Errorf("EQPreset = %q, want %q", raw.EQPreset, "bass_boost")
	}
	if raw.EQBands == nil {
		t.Fatal("EQBands not persisted")
	}
	if (*raw.EQBands)[0] != 6 {
		t.Errorf("EQBands[0] = %d, want 6", (*raw.EQBands)[0])
	}
	if len(raw.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(raw.EQCustomPresets))
	}
	if raw.EQCustomPresets[0].Key != "my_preset" {
		t.Errorf("custom preset key = %q, want %q", raw.EQCustomPresets[0].Key, "my_preset")
	}
}

func TestLoad_EQCustomPresets_KeyWhitespaceTrimmed(t *testing.T) {
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = "Padded"
key = "  padded_key  "
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(result.Config.EQCustomPresets))
	}
	if got := result.Config.EQCustomPresets[0].Key; got != "padded_key" {
		t.Errorf("Key = %q, want %q (whitespace should be trimmed)", got, "padded_key")
	}
}

func TestLoad_EQCustomPresets_EmptyKeyDroppedWithWarning(t *testing.T) {
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = "No Key"
key = ""
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

[[eq_custom_presets]]
name = "Whitespace Key"
key = "   "
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Config.EQCustomPresets) != 0 {
		t.Fatalf("EQCustomPresets len = %d, want 0", len(result.Config.EQCustomPresets))
	}
	emptyWarnings := 0
	for _, w := range result.Warnings {
		if strings.Contains(w, "empty") {
			emptyWarnings++
		}
	}
	if emptyWarnings != 2 {
		t.Errorf("expected 2 warnings about empty keys, got %d (%v)", emptyWarnings, result.Warnings)
	}
}

func TestLoad_EQCustomPresets_EmptyNameFallsBackToKey(t *testing.T) {
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = ""
key = "rock_remix"
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(result.Config.EQCustomPresets))
	}
	if got := result.Config.EQCustomPresets[0].Name; got != "rock_remix" {
		t.Errorf("Name = %q, want %q (should fall back to key)", got, "rock_remix")
	}
}

func TestLoad_EQCustomPresets_DuplicateKeyWarns(t *testing.T) {
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = "First"
key = "dup"
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

[[eq_custom_presets]]
name = "Second"
key = "dup"
bands = [1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1 after dedup", len(result.Config.EQCustomPresets))
	}
	if result.Config.EQCustomPresets[0].Name != "First" {
		t.Errorf("kept preset name = %q, want %q", result.Config.EQCustomPresets[0].Name, "First")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "duplicated") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about duplicated key, got: %v", result.Warnings)
	}
}

func TestLoad_EQCustomPresets_BuiltinKeyDropped(t *testing.T) {
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = "Rock Copy"
key = "rock"
bands = [1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Config.EQCustomPresets) != 0 {
		t.Fatalf("EQCustomPresets len = %d, want 0 for built-in key conflict", len(result.Config.EQCustomPresets))
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "built-in preset") && strings.Contains(w, "rock") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected built-in key conflict warning, got: %v", result.Warnings)
	}
}
