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
		MusicDirectories:           []string{"/home/user/Music"},
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

	if len(got.MusicDirectories) == 0 || got.MusicDirectories[0] != original.MusicDirectories[0] {
		t.Errorf("MusicDirectories[0] = %q, want %q", got.MusicDirectories, original.MusicDirectories[0])
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

// TestLoad_LegacyMusicDirectoryMigratestoMusicDirectories verifies that a
// config with only the legacy music_directory field is loaded into
// MusicDirectories correctly, enabling backward compatibility.
func TestLoad_LegacyMusicDirectoryMigratestoMusicDirectories(t *testing.T) {
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

	if len(result.Config.MusicDirectories) != 1 {
		t.Fatalf("MusicDirectories: expected 1 entry, got %d", len(result.Config.MusicDirectories))
	}
	if result.Config.MusicDirectories[0] != "/home/user/Songs" {
		t.Errorf("MusicDirectories[0] = %q, want %q", result.Config.MusicDirectories[0], "/home/user/Songs")
	}
	if result.Config.MusicDirectory != "/home/user/Songs" {
		t.Errorf("MusicDirectory = %q, want %q", result.Config.MusicDirectory, "/home/user/Songs")
	}
}

// TestLoad_BothFieldsPresentMergesAndDeduplicates verifies that when both
// music_directories and music_directory are present, music_directories takes
// precedence and duplicates are removed after filepath.Clean.
func TestLoad_BothFieldsPresentMergesAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	content := `music_directories = ["/home/user/Music", "/home/user/Music/"]
music_directory = "/home/user/Songs"
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// music_directories takes precedence; duplicate path should be deduplicated.
	dirs := result.Config.MusicDirectories
	if len(dirs) != 1 {
		t.Fatalf("MusicDirectories: expected 1 after dedup, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != "/home/user/Music" {
		t.Errorf("MusicDirectories[0] = %q, want %q", dirs[0], "/home/user/Music")
	}
}

// TestSave_WritesMusicDirectoriesNotMusicDirectory verifies that Save writes
// music_directories and not the legacy music_directory key.
func TestSave_WritesMusicDirectoriesNotMusicDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)

	cfg := defaults()
	cfg.MusicDirectories = []string{"/my/library"}
	cfg.MusicDirectory = "/my/library"

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "music_directories") {
		t.Error("Save: expected 'music_directories' key in output")
	}
	// The legacy key must not appear in saved output (it's tagged toml:"-").
	if strings.Contains(content, "music_directory =") {
		t.Error("Save: unexpected legacy 'music_directory' key in output")
	}
}

// TestLoad_TildeExpansionInMusicDirectories verifies that tilde-prefixed paths
// in music_directories are expanded to the user's home directory.
func TestLoad_TildeExpansionInMusicDirectories(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	dir := t.TempDir()
	content := `music_directories = ["~/Music"]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	expected := filepath.Join(home, "Music")
	if len(result.Config.MusicDirectories) == 0 || result.Config.MusicDirectories[0] != expected {
		t.Errorf("MusicDirectories[0] = %v, want %q", result.Config.MusicDirectories, expected)
	}
}

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

// --- EQ config tests ---

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
	if raw.EQBands[0] != 6 {
		t.Errorf("EQBands[0] = %d, want 6", raw.EQBands[0])
	}
	if len(raw.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(raw.EQCustomPresets))
	}
	if raw.EQCustomPresets[0].Key != "my_preset" {
		t.Errorf("custom preset key = %q, want %q", raw.EQCustomPresets[0].Key, "my_preset")
	}
}

// --- Stereo wide config tests ---

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

// --- ExcludedPaths config tests ---

func TestLoad_ExcludedPaths_Loaded(t *testing.T) {
	dir := t.TempDir()
	content := `excluded_paths = ["Podcasts", "Audiobooks"]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.ExcludedPaths) != 2 {
		t.Fatalf("ExcludedPaths: expected 2, got %d: %v", len(result.Config.ExcludedPaths), result.Config.ExcludedPaths)
	}
	if result.Config.ExcludedPaths[0] != "Podcasts" {
		t.Errorf("ExcludedPaths[0] = %q, want %q", result.Config.ExcludedPaths[0], "Podcasts")
	}
	if result.Config.ExcludedPaths[1] != "Audiobooks" {
		t.Errorf("ExcludedPaths[1] = %q, want %q", result.Config.ExcludedPaths[1], "Audiobooks")
	}
}

func TestLoad_ExcludedPaths_Deduplicated(t *testing.T) {
	dir := t.TempDir()
	content := `excluded_paths = ["Podcasts", "Podcasts", "Audiobooks"]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.ExcludedPaths) != 2 {
		t.Errorf("ExcludedPaths after dedup: expected 2, got %d: %v", len(result.Config.ExcludedPaths), result.Config.ExcludedPaths)
	}
}

func TestLoad_ExcludedPaths_WhitespaceTrimmed(t *testing.T) {
	dir := t.TempDir()
	content := `excluded_paths = ["  Podcasts  ", "Audiobooks"]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.ExcludedPaths) == 0 {
		t.Fatal("ExcludedPaths: expected at least one entry")
	}
	if result.Config.ExcludedPaths[0] != "Podcasts" {
		t.Errorf("ExcludedPaths[0] = %q, want %q (whitespace trimmed)", result.Config.ExcludedPaths[0], "Podcasts")
	}
}

func TestLoad_ExcludedPaths_EmptyEntriesRemoved(t *testing.T) {
	dir := t.TempDir()
	content := `excluded_paths = ["Podcasts", "", "   ", "Audiobooks"]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.ExcludedPaths) != 2 {
		t.Errorf("ExcludedPaths: expected 2 after removing empty entries, got %d: %v",
			len(result.Config.ExcludedPaths), result.Config.ExcludedPaths)
	}
}

func TestLoad_ExcludedPaths_TrailingSlashCleaned(t *testing.T) {
	dir := t.TempDir()
	content := `excluded_paths = ["/Users/me/Music/ai/"]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.ExcludedPaths) != 1 {
		t.Fatalf("ExcludedPaths: expected 1, got %d", len(result.Config.ExcludedPaths))
	}
	if result.Config.ExcludedPaths[0] != "/Users/me/Music/ai" {
		t.Errorf("ExcludedPaths[0] = %q, want trailing slash cleaned", result.Config.ExcludedPaths[0])
	}
}

func TestLoad_ExcludedPaths_ComponentPreserved(t *testing.T) {
	dir := t.TempDir()
	content := `excluded_paths = ["Podcasts"]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.ExcludedPaths) != 1 || result.Config.ExcludedPaths[0] != "Podcasts" {
		t.Errorf("ExcludedPaths = %v, want [Podcasts] unchanged", result.Config.ExcludedPaths)
	}
}

func TestLoad_ExcludedPaths_Omitted_IsNil(t *testing.T) {
	dir := t.TempDir()
	content := `default_volume = 80
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.ExcludedPaths != nil {
		t.Errorf("ExcludedPaths: expected nil when omitted, got %v", result.Config.ExcludedPaths)
	}
}
