package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Defaults ---

func TestMixes_Defaults(t *testing.T) {
	cfg := defaults()
	mx := cfg.Mixes
	if mx.RediscoverMinPlays != defaultMixRediscoverMinPlays {
		t.Errorf("RediscoverMinPlays = %d, want %d", mx.RediscoverMinPlays, defaultMixRediscoverMinPlays)
	}
	if mx.RediscoverAgeDays != defaultMixRediscoverAgeDays {
		t.Errorf("RediscoverAgeDays = %d, want %d", mx.RediscoverAgeDays, defaultMixRediscoverAgeDays)
	}
	if mx.HiddenGemsMaxPlays != defaultMixHiddenGemsMaxPlays {
		t.Errorf("HiddenGemsMaxPlays = %d, want %d", mx.HiddenGemsMaxPlays, defaultMixHiddenGemsMaxPlays)
	}
	if mx.HiddenGemsTopArtists != defaultMixHiddenGemsTopArtists {
		t.Errorf("HiddenGemsTopArtists = %d, want %d", mx.HiddenGemsTopArtists, defaultMixHiddenGemsTopArtists)
	}
	if mx.HiddenGemsTopGenres != defaultMixHiddenGemsTopGenres {
		t.Errorf("HiddenGemsTopGenres = %d, want %d", mx.HiddenGemsTopGenres, defaultMixHiddenGemsTopGenres)
	}
	if mx.OnRepeatMinPlays != defaultMixOnRepeatMinPlays {
		t.Errorf("OnRepeatMinPlays = %d, want %d", mx.OnRepeatMinPlays, defaultMixOnRepeatMinPlays)
	}
	if mx.OnRepeatWindowDays != defaultMixOnRepeatWindowDays {
		t.Errorf("OnRepeatWindowDays = %d, want %d", mx.OnRepeatWindowDays, defaultMixOnRepeatWindowDays)
	}
	if mx.DeepCutsMinPlays != defaultMixDeepCutsMinPlays {
		t.Errorf("DeepCutsMinPlays = %d, want %d", mx.DeepCutsMinPlays, defaultMixDeepCutsMinPlays)
	}
	if mx.DeepCutsMaxPlays != defaultMixDeepCutsMaxPlays {
		t.Errorf("DeepCutsMaxPlays = %d, want %d", mx.DeepCutsMaxPlays, defaultMixDeepCutsMaxPlays)
	}
	if mx.DeepCutsMinCompletion != defaultMixDeepCutsMinCompletion {
		t.Errorf("DeepCutsMinCompletion = %f, want %f", mx.DeepCutsMinCompletion, defaultMixDeepCutsMinCompletion)
	}
	if mx.SufficientHistoryCount != defaultMixSufficientHistory {
		t.Errorf("SufficientHistoryCount = %d, want %d", mx.SufficientHistoryCount, defaultMixSufficientHistory)
	}
}

// --- No [mixes] section uses defaults ---

func TestMixes_OmittedSection_UsesDefaults(t *testing.T) {
	dir := t.TempDir()
	content := "default_volume = 80\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	mx := result.Config.Mixes
	if mx.RediscoverMinPlays != defaultMixRediscoverMinPlays {
		t.Errorf("RediscoverMinPlays = %d, want default %d", mx.RediscoverMinPlays, defaultMixRediscoverMinPlays)
	}
	if mx.SufficientHistoryCount != defaultMixSufficientHistory {
		t.Errorf("SufficientHistoryCount = %d, want default %d", mx.SufficientHistoryCount, defaultMixSufficientHistory)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

// --- TOML roundtrip ---

func TestMixes_TOMLRoundtrip(t *testing.T) {
	dir := t.TempDir()
	content := `
[mixes]
rediscover_min_plays = 15
rediscover_age_days = 60
hidden_gems_max_plays = 3
hidden_gems_top_artists = 30
hidden_gems_top_genres = 15
on_repeat_min_plays = 5
on_repeat_window_days = 14
deep_cuts_min_plays = 2
deep_cuts_max_plays = 8
deep_cuts_min_completion = 0.7
sufficient_history_count = 50
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	mx := result.Config.Mixes
	if mx.RediscoverMinPlays != 15 {
		t.Errorf("RediscoverMinPlays = %d, want 15", mx.RediscoverMinPlays)
	}
	if mx.RediscoverAgeDays != 60 {
		t.Errorf("RediscoverAgeDays = %d, want 60", mx.RediscoverAgeDays)
	}
	if mx.HiddenGemsMaxPlays != 3 {
		t.Errorf("HiddenGemsMaxPlays = %d, want 3", mx.HiddenGemsMaxPlays)
	}
	if mx.OnRepeatMinPlays != 5 {
		t.Errorf("OnRepeatMinPlays = %d, want 5", mx.OnRepeatMinPlays)
	}
	if mx.DeepCutsMinCompletion != 0.7 {
		t.Errorf("DeepCutsMinCompletion = %f, want 0.7", mx.DeepCutsMinCompletion)
	}
	if mx.SufficientHistoryCount != 50 {
		t.Errorf("SufficientHistoryCount = %d, want 50", mx.SufficientHistoryCount)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

// --- OOB values trigger warnings and use defaults ---

func TestMixes_OOB_RediscoverAgeDays_UsesDefault(t *testing.T) {
	dir := t.TempDir()
	content := "[mixes]\nrediscover_age_days = 999\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	logger, buf := captureLog()
	slog.SetDefault(logger)

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.Mixes.RediscoverAgeDays != defaultMixRediscoverAgeDays {
		t.Errorf("RediscoverAgeDays = %d, want default %d", result.Config.Mixes.RediscoverAgeDays, defaultMixRediscoverAgeDays)
	}
	if !strings.Contains(buf.String(), "rediscover_age_days") {
		t.Errorf("expected slog warning mentioning rediscover_age_days, got: %s", buf.String())
	}
	if len(result.Warnings) == 0 {
		t.Error("expected at least one warning for out-of-range rediscover_age_days")
	}
}

func TestMixes_OOB_DeepCutsMinCompletion_UsesDefault(t *testing.T) {
	dir := t.TempDir()
	content := "[mixes]\ndeep_cuts_min_completion = 1.5\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	logger, buf := captureLog()
	slog.SetDefault(logger)

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.Mixes.DeepCutsMinCompletion != defaultMixDeepCutsMinCompletion {
		t.Errorf("DeepCutsMinCompletion = %f, want default %f", result.Config.Mixes.DeepCutsMinCompletion, defaultMixDeepCutsMinCompletion)
	}
	if !strings.Contains(buf.String(), "deep_cuts_min_completion") {
		t.Errorf("expected slog warning mentioning deep_cuts_min_completion, got: %s", buf.String())
	}
}

func TestMixes_OOB_SufficientHistoryCount_UsesDefault(t *testing.T) {
	dir := t.TempDir()
	content := "[mixes]\nsufficient_history_count = 0\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.Mixes.SufficientHistoryCount != defaultMixSufficientHistory {
		t.Errorf("SufficientHistoryCount = %d, want default %d", result.Config.Mixes.SufficientHistoryCount, defaultMixSufficientHistory)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning for out-of-range sufficient_history_count = 0")
	}
}

func TestMixes_OOB_DeepCutsMinCompletion_WarningContainsFloatRange(t *testing.T) {
	dir := t.TempDir()
	content := "[mixes]\ndeep_cuts_min_completion = 1.5\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "deep_cuts_min_completion") && strings.Contains(w, "0-1") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning containing range '0-1' for deep_cuts_min_completion, got: %v", result.Warnings)
	}
}

func TestMixes_DeepCutsMinGreaterThanMaxUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	content := "[mixes]\ndeep_cuts_min_plays = 10\ndeep_cuts_max_plays = 5\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.Mixes.DeepCutsMinPlays != defaultMixDeepCutsMinPlays {
		t.Errorf("DeepCutsMinPlays = %d, want default %d",
			result.Config.Mixes.DeepCutsMinPlays, defaultMixDeepCutsMinPlays)
	}
	if result.Config.Mixes.DeepCutsMaxPlays != defaultMixDeepCutsMaxPlays {
		t.Errorf("DeepCutsMaxPlays = %d, want default %d",
			result.Config.Mixes.DeepCutsMaxPlays, defaultMixDeepCutsMaxPlays)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "deep_cuts_min_plays") && strings.Contains(w, "greater than") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning for inverted deep cuts play range, got: %v", result.Warnings)
	}
}
