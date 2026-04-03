package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
