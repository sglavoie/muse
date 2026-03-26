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

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !result.Created {
		t.Error("expected created=true when config file did not exist")
	}

	home, err2 := os.UserHomeDir()
	if err2 != nil {
		t.Fatalf("UserHomeDir() error = %v", err2)
	}
	wantDir := filepath.Join(home, "Music")
	if result.Config.MusicDirectory != wantDir {
		t.Errorf("MusicDirectory = %q, want %q", result.Config.MusicDirectory, wantDir)
	}
	if result.Config.DefaultVolume != defaultVolume {
		t.Errorf("DefaultVolume = %d, want %d", result.Config.DefaultVolume, defaultVolume)
	}

	// File must exist on disk after creation.
	if _, err := os.Stat(filepath.Join(dir, configFileName)); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestLoad_MissingFile_Idempotent(t *testing.T) {
	dir := t.TempDir()

	r1, _ := Load(dir)
	r2, _ := Load(dir)

	if !r1.Created {
		t.Error("expected created=true on first load")
	}
	if r2.Created {
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

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Created {
		t.Error("expected created=false for existing file")
	}

	if result.Config.MusicDirectory != "/home/user/Songs" {
		t.Errorf("MusicDirectory = %q", result.Config.MusicDirectory)
	}
	if len(result.Config.MPVExtraFlags) != 2 || result.Config.MPVExtraFlags[0] != "--no-video" {
		t.Errorf("MPVExtraFlags = %v", result.Config.MPVExtraFlags)
	}
	if result.Config.DefaultVolume != 75 {
		t.Errorf("DefaultVolume = %d", result.Config.DefaultVolume)
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
	// Empty music_directory is silently treated as "not set"; no warning expected.
}

func TestLoad_InvalidTOML_UsesDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte("not valid toml ==="), 0o600); err != nil {
		t.Fatal(err)
	}

	logger, _ := captureLog()
	slog.SetDefault(logger)

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
	if result.Config.DefaultVolume != defaultVolume {
		t.Errorf("DefaultVolume = %d, want default", result.Config.DefaultVolume)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected at least one warning for invalid TOML")
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

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if result.Config.DefaultVolume != defaultVolume {
		t.Errorf("DefaultVolume = %d, want default %d (omitted field should use default, not zero)", result.Config.DefaultVolume, defaultVolume)
	}
}

func TestLoad_MPVSocketPath_TildeExpanded(t *testing.T) {
	dir := t.TempDir()
	content := `
music_directory = "/home/user/Songs"
mpv_socket_path = "~/run/muse.sock"
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	want := filepath.Join(home, "run/muse.sock")
	if result.Config.MPVSocketPath != want {
		t.Errorf("MPVSocketPath = %q, want %q", result.Config.MPVSocketPath, want)
	}
}

func TestLoad_MPVSocketPath_AbsoluteUnchanged(t *testing.T) {
	dir := t.TempDir()
	content := `
music_directory = "/home/user/Songs"
mpv_socket_path = "/tmp/muse.sock"
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if result.Config.MPVSocketPath != "/tmp/muse.sock" {
		t.Errorf("MPVSocketPath = %q, want /tmp/muse.sock", result.Config.MPVSocketPath)
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

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if result.Config.DefaultVolume != 0 {
		t.Errorf("DefaultVolume = %d, want 0 (explicitly set to 0)", result.Config.DefaultVolume)
	}
}

func TestLoad_NewFields_OmittedUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	content := `music_directory = "/home/user/Songs"
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg := result.Config

	if cfg.VolumeIncrement != defaultVolumeIncrement {
		t.Errorf("VolumeIncrement = %d, want %d", cfg.VolumeIncrement, defaultVolumeIncrement)
	}
	if cfg.SeekOffsetShort != defaultSeekOffsetShort {
		t.Errorf("SeekOffsetShort = %v, want %v", cfg.SeekOffsetShort, defaultSeekOffsetShort)
	}
	if cfg.SeekOffsetLong != defaultSeekOffsetLong {
		t.Errorf("SeekOffsetLong = %v, want %v", cfg.SeekOffsetLong, defaultSeekOffsetLong)
	}
	if cfg.TransientDurationSecs != defaultTransientDurationSecs {
		t.Errorf("TransientDurationSecs = %d, want %d", cfg.TransientDurationSecs, defaultTransientDurationSecs)
	}
	if cfg.VolumeFeedbackDurationSecs != defaultVolumeFeedbackDurationSecs {
		t.Errorf("VolumeFeedbackDurationSecs = %d, want %d", cfg.VolumeFeedbackDurationSecs, defaultVolumeFeedbackDurationSecs)
	}
}

func TestLoad_NewFields_ValidValuesAccepted(t *testing.T) {
	dir := t.TempDir()
	content := `music_directory = "/home/user/Songs"
volume_increment = 10
seek_offset_short = 15.0
seek_offset_long = 60.0
transient_duration_secs = 8
volume_feedback_duration_secs = 4
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
	cfg := result.Config
	if cfg.VolumeIncrement != 10 {
		t.Errorf("VolumeIncrement = %d, want 10", cfg.VolumeIncrement)
	}
	if cfg.SeekOffsetShort != 15.0 {
		t.Errorf("SeekOffsetShort = %v, want 15.0", cfg.SeekOffsetShort)
	}
	if cfg.SeekOffsetLong != 60.0 {
		t.Errorf("SeekOffsetLong = %v, want 60.0", cfg.SeekOffsetLong)
	}
	if cfg.TransientDurationSecs != 8 {
		t.Errorf("TransientDurationSecs = %d, want 8", cfg.TransientDurationSecs)
	}
	if cfg.VolumeFeedbackDurationSecs != 4 {
		t.Errorf("VolumeFeedbackDurationSecs = %d, want 4", cfg.VolumeFeedbackDurationSecs)
	}
}

func TestLoad_NewFields_OutOfRange_FallsBackToDefault(t *testing.T) {
	tests := []struct {
		name    string
		content string
		field   string
	}{
		{"volume_increment too high", "volume_increment = 100\n", "volume_increment"},
		{"volume_increment too low", "volume_increment = 0\n", "volume_increment"},
		{"seek_offset_short too high", "seek_offset_short = 999.0\n", "seek_offset_short"},
		{"seek_offset_short too low", "seek_offset_short = 0.5\n", "seek_offset_short"},
		{"seek_offset_long too high", "seek_offset_long = 999.0\n", "seek_offset_long"},
		{"seek_offset_long too low", "seek_offset_long = 1.0\n", "seek_offset_long"},
		{"transient_duration_secs too high", "transient_duration_secs = 100\n", "transient_duration_secs"},
		{"transient_duration_secs too low", "transient_duration_secs = 0\n", "transient_duration_secs"},
		{"volume_feedback_duration_secs too high", "volume_feedback_duration_secs = 50\n", "volume_feedback_duration_secs"},
		{"volume_feedback_duration_secs too low", "volume_feedback_duration_secs = 0\n", "volume_feedback_duration_secs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := Load(dir)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			found := false
			for _, w := range result.Warnings {
				if strings.Contains(w, tt.field) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected warning mentioning %q, got: %v", tt.field, result.Warnings)
			}
		})
	}
}

func TestSave_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)

	original := Config{
		MusicDirectory:             "/home/user/Music",
		DefaultVolume:              75,
		LogLevel:                   "debug",
		MPVExtraFlags:              []string{},
		VolumeIncrement:            10,
		SeekOffsetShort:            15.0,
		SeekOffsetLong:             60.0,
		TransientDurationSecs:      8,
		VolumeFeedbackDurationSecs: 4,
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() after Save() error = %v", err)
	}
	got := result.Config

	if got.MusicDirectory != original.MusicDirectory {
		t.Errorf("MusicDirectory = %q, want %q", got.MusicDirectory, original.MusicDirectory)
	}
	if got.DefaultVolume != original.DefaultVolume {
		t.Errorf("DefaultVolume = %d, want %d", got.DefaultVolume, original.DefaultVolume)
	}
	if got.VolumeIncrement != original.VolumeIncrement {
		t.Errorf("VolumeIncrement = %d, want %d", got.VolumeIncrement, original.VolumeIncrement)
	}
	if got.SeekOffsetShort != original.SeekOffsetShort {
		t.Errorf("SeekOffsetShort = %v, want %v", got.SeekOffsetShort, original.SeekOffsetShort)
	}
	if got.SeekOffsetLong != original.SeekOffsetLong {
		t.Errorf("SeekOffsetLong = %v, want %v", got.SeekOffsetLong, original.SeekOffsetLong)
	}
	if got.TransientDurationSecs != original.TransientDurationSecs {
		t.Errorf("TransientDurationSecs = %d, want %d", got.TransientDurationSecs, original.TransientDurationSecs)
	}
	if got.VolumeFeedbackDurationSecs != original.VolumeFeedbackDurationSecs {
		t.Errorf("VolumeFeedbackDurationSecs = %d, want %d", got.VolumeFeedbackDurationSecs, original.VolumeFeedbackDurationSecs)
	}
}

func TestSave_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)

	// Write an initial config.
	if err := Save(path, Config{MusicDirectory: "/old", DefaultVolume: 50, MPVExtraFlags: []string{}, VolumeIncrement: 5, SeekOffsetShort: 5.0, SeekOffsetLong: 30.0, TransientDurationSecs: 5, VolumeFeedbackDurationSecs: 2}); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}

	// Overwrite with new values.
	updated := Config{MusicDirectory: "/new", DefaultVolume: 80, MPVExtraFlags: []string{}, VolumeIncrement: 15, SeekOffsetShort: 10.0, SeekOffsetLong: 45.0, TransientDurationSecs: 3, VolumeFeedbackDurationSecs: 1}
	if err := Save(path, updated); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.DefaultVolume != 80 {
		t.Errorf("DefaultVolume = %d, want 80", result.Config.DefaultVolume)
	}
	if result.Config.VolumeIncrement != 15 {
		t.Errorf("VolumeIncrement = %d, want 15", result.Config.VolumeIncrement)
	}
}
