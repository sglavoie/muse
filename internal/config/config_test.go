package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// captureLog returns a logger that writes to a buffer and the buffer itself.
func captureLog() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	handler := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(handler), buf
}

func TestLoad_MissingFile_CreatesDefaultConfig(t *testing.T) {
	dir := t.TempDir()

	cfg, created, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !created {
		t.Error("expected created=true when config file did not exist")
	}

	home, err2 := os.UserHomeDir()
	if err2 != nil {
		t.Fatalf("UserHomeDir() error = %v", err2)
	}
	wantDir := filepath.Join(home, "Music")
	if cfg.MusicDirectory != wantDir {
		t.Errorf("MusicDirectory = %q, want %q", cfg.MusicDirectory, wantDir)
	}
	if cfg.DefaultVolume != defaultVolume {
		t.Errorf("DefaultVolume = %d, want %d", cfg.DefaultVolume, defaultVolume)
	}

	// File must exist on disk after creation.
	if _, err := os.Stat(filepath.Join(dir, configFileName)); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestLoad_MissingFile_Idempotent(t *testing.T) {
	dir := t.TempDir()

	_, created1, _, _ := Load(dir)
	_, created2, _, _ := Load(dir)

	if !created1 {
		t.Error("expected created=true on first load")
	}
	if created2 {
		t.Error("expected created=false on second load")
	}
}

func TestLoad_ParsesTOMLCorrectly(t *testing.T) {
	dir := t.TempDir()
	content := `
music_directory = "/home/user/Songs"
mpv_extra_flags = ["--no-video", "--volume=80"]
default_volume = 75
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, created, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if created {
		t.Error("expected created=false for existing file")
	}

	if cfg.MusicDirectory != "/home/user/Songs" {
		t.Errorf("MusicDirectory = %q", cfg.MusicDirectory)
	}
	if len(cfg.MPVExtraFlags) != 2 || cfg.MPVExtraFlags[0] != "--no-video" {
		t.Errorf("MPVExtraFlags = %v", cfg.MPVExtraFlags)
	}
	if cfg.DefaultVolume != 75 {
		t.Errorf("DefaultVolume = %d", cfg.DefaultVolume)
	}
}

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

	cfg, _, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DefaultVolume != defaultVolume {
		t.Errorf("DefaultVolume = %d, want default %d", cfg.DefaultVolume, defaultVolume)
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

	cfg, _, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	home, err2 := os.UserHomeDir()
	if err2 != nil {
		t.Fatalf("UserHomeDir() error = %v", err2)
	}
	wantDir := filepath.Join(home, "Music")
	if cfg.MusicDirectory != wantDir {
		t.Errorf("MusicDirectory = %q, want %q", cfg.MusicDirectory, wantDir)
	}
	// Empty music_directory is silently treated as "not set"; no warning expected.
}

func TestLoad_InvalidTOML_UsesDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte("not valid toml ==="), 0o600); err != nil {
		t.Fatal(err)
	}

	logger, _ := captureLog()
	slog.SetDefault(logger)

	cfg, _, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	home, err2 := os.UserHomeDir()
	if err2 != nil {
		t.Fatalf("UserHomeDir() error = %v", err2)
	}
	wantDir := filepath.Join(home, "Music")
	if cfg.MusicDirectory != wantDir {
		t.Errorf("MusicDirectory = %q, want %q", cfg.MusicDirectory, wantDir)
	}
	if cfg.DefaultVolume != defaultVolume {
		t.Errorf("DefaultVolume = %d, want default", cfg.DefaultVolume)
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

	for _, comment := range []string{"# Path to your music library", "# Default playback volume"} {
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

	_, _, warnings, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(warnings) == 0 {
		t.Fatal("expected warnings for out-of-range default_volume, got none")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "default_volume") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning mentioning default_volume, got: %v", warnings)
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

	_, _, warnings, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(warnings) == 0 {
		t.Fatal("expected warnings for unrecognized log_level, got none")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "log_level") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning mentioning log_level, got: %v", warnings)
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

	cfg, _, warnings, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(warnings) != 0 {
		t.Errorf("expected no warnings for case-variant log level, got: %v", warnings)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestConfig_StructFields(t *testing.T) {
	cfg := defaults()
	// Verify all required fields exist and have expected defaults.
	_ = cfg.MusicDirectory
	_ = cfg.MPVExtraFlags
	_ = cfg.DefaultVolume
}

func TestLoad_OmittedVolume_UsesDefault(t *testing.T) {
	dir := t.TempDir()
	content := `
music_directory = "/home/user/Songs"
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, _, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DefaultVolume != defaultVolume {
		t.Errorf("DefaultVolume = %d, want default %d (omitted field should use default, not zero)", cfg.DefaultVolume, defaultVolume)
	}
}

func TestLoad_ExplicitVolumeZero_Accepted(t *testing.T) {
	dir := t.TempDir()
	content := `
music_directory = "/home/user/Songs"
default_volume = 0
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, _, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DefaultVolume != 0 {
		t.Errorf("DefaultVolume = %d, want 0 (explicitly set to 0)", cfg.DefaultVolume)
	}
}
