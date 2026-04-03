package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	if err := Save(path, Config{MusicDirectory: "/old", DefaultVolume: 50, MPVExtraFlags: []string{}, VolumeIncrement: 5, SeekOffsetShort: 5.0, SeekOffsetLong: 30.0, TransientDurationSecs: 5, VolumeFeedbackDurationSecs: 2}); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}

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
	if strings.Contains(content, "music_directory =") {
		t.Error("Save: unexpected legacy 'music_directory' key in output")
	}
}
