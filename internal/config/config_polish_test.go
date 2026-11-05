package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadConfigText(t *testing.T, content string) LoadResult {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return result
}

func TestPolishDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Artwork != ArtworkAuto || !cfg.ArtworkAccent || !cfg.FadeOnPause || cfg.ReduceMotion {
		t.Fatalf("unexpected defaults: artwork %q tint %v fade %v reduce %v",
			cfg.Artwork, cfg.ArtworkAccent, cfg.FadeOnPause, cfg.ReduceMotion)
	}
}

func TestLoadPolishSettings(t *testing.T) {
	result := loadConfigText(t, "artwork = \"Blocks\"\nartwork_accent = false\nfade_on_pause = false\nreduce_motion = true\n")
	cfg := result.Config
	if cfg.Artwork != ArtworkBlocks || cfg.ArtworkAccent || cfg.FadeOnPause || !cfg.ReduceMotion {
		t.Fatalf("loaded artwork %q tint %v fade %v reduce %v", cfg.Artwork, cfg.ArtworkAccent, cfg.FadeOnPause, cfg.ReduceMotion)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", result.Warnings)
	}
}

func TestTrackChangeCardDefaultsOnAndLoads(t *testing.T) {
	if !Defaults().TrackChangeCard {
		t.Fatal("track_change_card should default to true")
	}
	result := loadConfigText(t, "track_change_card = false\n")
	if result.Config.TrackChangeCard {
		t.Fatal("track_change_card = false was not loaded")
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", result.Warnings)
	}
}

func TestLoadInvalidArtworkWarns(t *testing.T) {
	result := loadConfigText(t, "artwork = \"sixel\"\n")
	if result.Config.Artwork != ArtworkAuto {
		t.Fatalf("invalid artwork kept: %q", result.Config.Artwork)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "artwork") {
		t.Fatalf("want one artwork warning, got %v", result.Warnings)
	}
}

func TestLevelMeterDefaultsAutoAndLoads(t *testing.T) {
	if Defaults().LevelMeter != LevelMeterAuto {
		t.Fatalf("level_meter should default to auto, got %q", Defaults().LevelMeter)
	}
	result := loadConfigText(t, "level_meter = \"Off\"\n")
	if result.Config.LevelMeter != LevelMeterOff || len(result.Warnings) != 0 {
		t.Fatalf("level_meter = Off: got %q, warnings %v", result.Config.LevelMeter, result.Warnings)
	}
}

func TestLoadInvalidLevelMeterWarns(t *testing.T) {
	result := loadConfigText(t, "level_meter = \"loud\"\n")
	if result.Config.LevelMeter != LevelMeterAuto {
		t.Fatalf("invalid level_meter should fall back to auto, got %q", result.Config.LevelMeter)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "level_meter") {
		t.Fatalf("want one level_meter warning, got %v", result.Warnings)
	}
}
