package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMusicDir_MissingDirectory(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	missingDir := filepath.Join(tmp, "no-such-dir")

	err := ValidateMusicDir(missingDir, configPath)
	if err == nil {
		t.Fatal("expected error for missing directory, got nil")
	}
	if !strings.Contains(err.Error(), configPath) {
		t.Errorf("error should contain config path %q, got: %s", configPath, err.Error())
	}
	if !strings.Contains(err.Error(), "does not exist") && !strings.Contains(err.Error(), "cannot be read") {
		t.Errorf("error should mention missing/unreadable directory, got: %s", err.Error())
	}
}

func TestValidateMusicDir_EmptyDirectory(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	emptyDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := ValidateMusicDir(emptyDir, configPath)
	if err == nil {
		t.Fatal("expected error for empty directory, got nil")
	}
	if !strings.Contains(err.Error(), configPath) {
		t.Errorf("error should contain config path %q, got: %s", configPath, err.Error())
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty directory, got: %s", err.Error())
	}
}

func TestValidateMusicDir_ValidDirectory(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	musicDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(musicDir, "song.mp3"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateMusicDir(musicDir, configPath); err != nil {
		t.Errorf("expected no error for valid directory, got: %s", err.Error())
	}
}

func TestValidateMusicDir_TildeExpansion(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")

	// Using a tilde path that resolves to a non-existent directory ensures
	// tilde expansion runs without depending on real home dir contents.
	err := ValidateMusicDir("~/no-such-music-dir-xyz", configPath)
	if err == nil {
		t.Fatal("expected error for missing tilde-expanded directory, got nil")
	}
	if !strings.Contains(err.Error(), configPath) {
		t.Errorf("error should contain config path %q, got: %s", configPath, err.Error())
	}
}
