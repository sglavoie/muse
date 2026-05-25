package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	dirs := result.Config.MusicDirectories
	if len(dirs) != 1 {
		t.Fatalf("MusicDirectories: expected 1 after dedup, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != "/home/user/Music" {
		t.Errorf("MusicDirectories[0] = %q, want %q", dirs[0], "/home/user/Music")
	}
}

func TestLoad_BlankMusicDirectoriesFallsBackToLegacyMusicDirectory(t *testing.T) {
	dir := t.TempDir()
	content := `music_directories = ["", "   "]
music_directory = "  /home/user/Songs/  "
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(result.Config.MusicDirectories) != 1 {
		t.Fatalf("MusicDirectories: expected 1 entry, got %d: %v", len(result.Config.MusicDirectories), result.Config.MusicDirectories)
	}
	if result.Config.MusicDirectories[0] != "/home/user/Songs" {
		t.Errorf("MusicDirectories[0] = %q, want %q", result.Config.MusicDirectories[0], "/home/user/Songs")
	}
	if result.Config.MusicDirectory != "/home/user/Songs" {
		t.Errorf("MusicDirectory = %q, want %q", result.Config.MusicDirectory, "/home/user/Songs")
	}
}

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

func TestLoad_MusicDirectories_TrailingSlashCleaned(t *testing.T) {
	dir := t.TempDir()
	content := `music_directories = ["/home/user/Music/"]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.MusicDirectories) != 1 {
		t.Fatalf("MusicDirectories: expected 1 entry, got %v", result.Config.MusicDirectories)
	}
	if result.Config.MusicDirectories[0] != "/home/user/Music" {
		t.Errorf("MusicDirectories[0] = %q, want %q (trailing slash should be cleaned)",
			result.Config.MusicDirectories[0], "/home/user/Music")
	}
	if result.Config.MusicDirectory != "/home/user/Music" {
		t.Errorf("MusicDirectory = %q, want %q (legacy mirror should also be cleaned)",
			result.Config.MusicDirectory, "/home/user/Music")
	}
}

func TestLoad_MusicDirectories_WhitespaceTrimmed(t *testing.T) {
	dir := t.TempDir()
	content := `music_directories = ["  /home/user/Music  "]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.MusicDirectories) != 1 {
		t.Fatalf("MusicDirectories: expected 1 entry, got %v", result.Config.MusicDirectories)
	}
	got := result.Config.MusicDirectories[0]
	if strings.ContainsAny(got, " \t") {
		t.Errorf("MusicDirectories[0] = %q contains whitespace; expected trimmed", got)
	}
	if got != "/home/user/Music" {
		t.Errorf("MusicDirectories[0] = %q, want %q", got, "/home/user/Music")
	}
}

func TestLoad_MusicDirectories_EmptyEntryDropped(t *testing.T) {
	dir := t.TempDir()
	content := `music_directories = ["/home/user/Music", "   ", ""]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.MusicDirectories) != 1 {
		t.Fatalf("MusicDirectories: expected 1 entry after dropping empties, got %v", result.Config.MusicDirectories)
	}
	if result.Config.MusicDirectories[0] != "/home/user/Music" {
		t.Errorf("MusicDirectories[0] = %q, want /home/user/Music", result.Config.MusicDirectories[0])
	}
}
