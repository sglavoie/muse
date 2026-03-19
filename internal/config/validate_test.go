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
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should mention 'does not exist', got: %s", err.Error())
	}
}

func TestValidateMusicDir_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test as root")
	}
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	restrictedDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(restrictedDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(restrictedDir, 0o755) })

	err := ValidateMusicDir(restrictedDir, configPath)
	if err == nil {
		t.Fatal("expected error for permission-denied directory, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should mention 'permission denied', got: %s", err.Error())
	}
}

func TestValidateMusicDir_OnlyNonAudioFiles(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	musicDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(musicDir, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(musicDir, "cover.jpg"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	err := ValidateMusicDir(musicDir, configPath)
	if err == nil {
		t.Fatal("expected error for directory with only non-audio files, got nil")
	}
	if !strings.Contains(err.Error(), "no supported audio files") {
		t.Errorf("error should mention 'no supported audio files', got: %s", err.Error())
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

func TestValidateMusicDir_ValidDirectoryWithSubdirs(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	musicDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create subdirectories that would contain audio files (typical library layout).
	if err := os.Mkdir(filepath.Join(musicDir, "Artist"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ValidateMusicDir(musicDir, configPath); err != nil {
		t.Errorf("expected no error for directory with subdirectories, got: %s", err.Error())
	}
}

func TestValidateMusicDir_OnlyHiddenDirs(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	musicDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(musicDir, ".cache"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := ValidateMusicDir(musicDir, configPath)
	if err == nil {
		t.Fatal("expected error for directory with only hidden subdirectories, got nil")
	}
	if !strings.Contains(err.Error(), "no supported audio files") {
		t.Errorf("error should mention 'no supported audio files', got: %s", err.Error())
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
