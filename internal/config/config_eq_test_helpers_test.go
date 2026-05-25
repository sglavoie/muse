package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func loadEQConfigFromContent(t *testing.T, content string) LoadResult {
	t.Helper()

	dir := writeEQConfigFile(t, content)
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return result
}

func writeEQConfigFile(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func saveEQConfig(t *testing.T, cfg Config) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return path
}

func decodeRawEQConfig(t *testing.T, path string) rawConfig {
	t.Helper()

	var raw rawConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		t.Fatalf("DecodeFile() error = %v", err)
	}
	return raw
}

func requireWarningContaining(t *testing.T, warnings []string, parts ...string) {
	t.Helper()

	for _, warning := range warnings {
		matches := true
		for _, part := range parts {
			if !strings.Contains(warning, part) {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}

	t.Fatalf("expected warning containing %v, got %v", parts, warnings)
}

func requireWarningCountContaining(t *testing.T, warnings []string, part string, want int) {
	t.Helper()

	got := 0
	for _, warning := range warnings {
		if strings.Contains(warning, part) {
			got++
		}
	}
	if got != want {
		t.Fatalf("expected %d warnings containing %q, got %d (%v)", want, part, got, warnings)
	}
}
