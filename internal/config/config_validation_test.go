package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestLoad_InvalidVolume_FallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	content := `
music_directory = "/home/user/Songs"
default_volume = 999
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	logger, buf := captureLog()
	slog.SetDefault(logger)

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if result.Config.DefaultVolume != defaultVolume {
		t.Errorf("DefaultVolume = %d, want default %d", result.Config.DefaultVolume, defaultVolume)
	}

	if !strings.Contains(buf.String(), "default_volume") {
		t.Errorf("expected slog warning mentioning default_volume, got: %s", buf.String())
	}
}

func TestLoad_EmptyMusicDirectory_FallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	content := `
music_directory = ""
default_volume = 50
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	home, err2 := os.UserHomeDir()
	if err2 != nil {
		t.Fatalf("UserHomeDir() error = %v", err2)
	}
	wantDir := filepath.Join(home, "Music")
	if result.Config.MusicDirectory != wantDir {
		t.Errorf("MusicDirectory = %q, want %q", result.Config.MusicDirectory, wantDir)
	}
}

func TestWriteDefaults_ContainsCommentsAndValidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := defaults()

	if err := writeDefaults(path, cfg); err != nil {
		t.Fatalf("writeDefaults: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	content := string(data)

	for _, comment := range []string{"# Paths to your music library roots", "# Default playback volume"} {
		if !strings.Contains(content, comment) {
			t.Errorf("expected comment %q in output:\n%s", comment, content)
		}
	}

	var parsed Config
	if _, err := toml.DecodeFile(path, &parsed); err != nil {
		t.Fatalf("output is not valid TOML: %v\n%s", err, content)
	}
	if parsed.DefaultVolume != cfg.DefaultVolume {
		t.Errorf("DefaultVolume = %d, want %d", parsed.DefaultVolume, cfg.DefaultVolume)
	}
	if parsed.LogLevel != cfg.LogLevel {
		t.Errorf("LogLevel = %q, want %q", parsed.LogLevel, cfg.LogLevel)
	}
}

func TestLoad_InvalidVolume_ReturnsWarning(t *testing.T) {
	dir := t.TempDir()
	content := `
music_directory = "/home/user/Songs"
default_volume = 150
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	logger, _ := captureLog()
	slog.SetDefault(logger)

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings for out-of-range default_volume, got none")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "default_volume") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning mentioning default_volume, got: %v", result.Warnings)
	}
}

func TestLoad_InvalidLogLevel_ReturnsWarning(t *testing.T) {
	dir := t.TempDir()
	content := `
log_level = "banana"
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	logger, _ := captureLog()
	slog.SetDefault(logger)

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings for unrecognized log_level, got none")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "log_level") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning mentioning log_level, got: %v", result.Warnings)
	}
}

func TestLoad_CaseInsensitiveLogLevel(t *testing.T) {
	dir := t.TempDir()
	content := `
log_level = "DEBUG"
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings for case-variant log level, got: %v", result.Warnings)
	}
	if result.Config.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", result.Config.LogLevel, "debug")
	}
}
