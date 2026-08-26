package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_NotifyOnTrackChange_True(t *testing.T) {
	dir := t.TempDir()
	content := "notify_on_track_change = true\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !result.Config.NotifyOnTrackChange {
		t.Errorf("NotifyOnTrackChange = false, want true")
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

func TestLoad_NotifyOnTrackChange_False(t *testing.T) {
	dir := t.TempDir()
	content := "notify_on_track_change = false\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.NotifyOnTrackChange {
		t.Errorf("NotifyOnTrackChange = true, want false")
	}
}

func TestLoad_NotifyOnTrackChange_Omitted_UsesDefault(t *testing.T) {
	dir := t.TempDir()
	content := "default_volume = 80\n"
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.NotifyOnTrackChange != defaultNotifyOnTrackChange {
		t.Errorf("NotifyOnTrackChange = %v, want default %v",
			result.Config.NotifyOnTrackChange, defaultNotifyOnTrackChange)
	}
}
