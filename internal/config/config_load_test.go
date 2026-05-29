package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	if _, err := os.Stat(filepath.Join(dir, configFileName)); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestLoad_EmptyConfigDirCreatesDefaultConfigInCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	result, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}
	if !result.Created {
		t.Error("expected created=true when config file did not exist")
	}
	if _, err := os.Stat(configFileName); err != nil {
		t.Errorf("config file not created in current directory: %v", err)
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

func TestLoad_InvalidTOML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte("not valid toml ==="), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatal("Load() error = nil, want parse error")
	} else if !strings.Contains(err.Error(), "decode config") {
		t.Fatalf("Load() error = %v, want decode config context", err)
	}
}

func TestLoad_UnknownKey_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	content := `
music_directory = "/home/user/Songs"
typo_key = true
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatal("Load() error = nil, want unknown key error")
	} else if !strings.Contains(err.Error(), "typo_key") {
		t.Fatalf("Load() error = %v, want unknown key name", err)
	}
}

func TestLoad_DecodeIOError_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatal("Load() error = nil, want read/decode error")
	} else if !strings.Contains(err.Error(), "decode config") {
		t.Fatalf("Load() error = %v, want decode config context", err)
	}
}

func TestConfig_StructFields(t *testing.T) {
	cfg := defaults()
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
