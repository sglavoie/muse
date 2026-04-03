package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_NormalizeVolume_True(t *testing.T) {
	dir := t.TempDir()
	content := "normalize_volume = true\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !result.Config.NormalizeVolume {
		t.Errorf("NormalizeVolume = false, want true")
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

func TestLoad_NormalizeVolume_False(t *testing.T) {
	dir := t.TempDir()
	content := "normalize_volume = false\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.NormalizeVolume {
		t.Errorf("NormalizeVolume = true, want false")
	}
}

func TestLoad_NormalizeVolume_Omitted_UsesDefault(t *testing.T) {
	dir := t.TempDir()
	content := "default_volume = 80\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.NormalizeVolume != defaultNormalizeVolume {
		t.Errorf("NormalizeVolume = %v, want default %v", result.Config.NormalizeVolume, defaultNormalizeVolume)
	}
}

func TestLoad_GaplessAudio_False(t *testing.T) {
	dir := t.TempDir()
	content := "gapless_audio = false\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.GaplessAudio {
		t.Errorf("GaplessAudio = true, want false")
	}
}

func TestLoad_GaplessAudio_True(t *testing.T) {
	dir := t.TempDir()
	content := "gapless_audio = true\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !result.Config.GaplessAudio {
		t.Errorf("GaplessAudio = false, want true")
	}
}

func TestLoad_GaplessAudio_Omitted_UsesDefault(t *testing.T) {
	dir := t.TempDir()
	content := "default_volume = 80\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.GaplessAudio != defaultGaplessAudio {
		t.Errorf("GaplessAudio = %v, want default %v", result.Config.GaplessAudio, defaultGaplessAudio)
	}
}

func TestLoad_StereoWide_Defaults_WhenOmitted(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte("default_volume = 80\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg := result.Config
	if cfg.StereoWideEnabled != false {
		t.Errorf("StereoWideEnabled = %v, want false", cfg.StereoWideEnabled)
	}
	if cfg.StereoWideAmount != defaultStereoWideAmount {
		t.Errorf("StereoWideAmount = %d, want %d", cfg.StereoWideAmount, defaultStereoWideAmount)
	}
}

func TestLoad_StereoWide_FieldsLoaded(t *testing.T) {
	dir := t.TempDir()
	content := "stereo_wide_enabled = true\nstereo_wide_amount = 20\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !result.Config.StereoWideEnabled {
		t.Error("StereoWideEnabled = false, want true")
	}
	if result.Config.StereoWideAmount != 20 {
		t.Errorf("StereoWideAmount = %d, want 20", result.Config.StereoWideAmount)
	}
}

func TestLoad_StereoWide_AmountOutOfRange(t *testing.T) {
	dir := t.TempDir()
	content := "stereo_wide_amount = 99\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.StereoWideAmount != defaultStereoWideAmount {
		t.Errorf("StereoWideAmount = %d, want default %d", result.Config.StereoWideAmount, defaultStereoWideAmount)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected a warning for out-of-range stereo_wide_amount")
	}
}
