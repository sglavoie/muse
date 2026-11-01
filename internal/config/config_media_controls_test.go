package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MediaControls(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"omitted uses the default", "default_volume = 80\n", defaultMediaControls},
		{"disabled", "media_controls = false\n", false},
		{"enabled", "media_controls = true\n", true},
	}
	if !defaultMediaControls {
		t.Fatal("media controls must default to on")
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
			if result.Config.MediaControls != tt.want {
				t.Errorf("MediaControls = %v, want %v", result.Config.MediaControls, tt.want)
			}
		})
	}
}
