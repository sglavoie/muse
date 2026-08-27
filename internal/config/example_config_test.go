package config

import (
	"os"
	"strings"
	"testing"
)

// exampleConfigPath is the repo-root documentation copy of the embedded
// configTemplate. The embedded template stays the source of truth; this file
// exists so someone browsing the repository can read every available option
// before installing.
const exampleConfigPath = "../../config.example.toml"

// TestExampleConfigMatchesTemplate guards against drift. configTemplate is a
// printf format string, so the checked-in copy cannot be a verbatim copy — it
// is the template *rendered* with placeholder values. Regenerate after editing
// the template with:
//
//	MUSE_UPDATE_EXAMPLE_CONFIG=1 go test ./internal/config/ -run TestExampleConfigMatchesTemplate
func TestExampleConfigMatchesTemplate(t *testing.T) {
	want := exampleConfig()

	if os.Getenv("MUSE_UPDATE_EXAMPLE_CONFIG") != "" {
		if err := os.WriteFile(exampleConfigPath, []byte(want), 0o644); err != nil { //nolint:gosec // G306: documentation file, world-readable on purpose
			t.Fatalf("rewriting %s: %v", exampleConfigPath, err)
		}
		t.Logf("regenerated %s", exampleConfigPath)
		return
	}

	got, err := os.ReadFile(exampleConfigPath)
	if err != nil {
		t.Fatalf("reading %s: %v", exampleConfigPath, err)
	}
	if string(got) != want {
		t.Errorf("config.example.toml has drifted from the embedded configTemplate; "+
			"regenerate with MUSE_UPDATE_EXAMPLE_CONFIG=1 go test ./internal/config/ "+
			"-run TestExampleConfigMatchesTemplate\n"+
			"--- got (%d bytes) ---\n%s\n--- want (%d bytes) ---\n%s",
			len(got), got, len(want), want)
	}
}

// TestExampleConfigUsesPlaceholderPath makes sure no real machine path leaks
// into the published example.
func TestExampleConfigUsesPlaceholderPath(t *testing.T) {
	data, err := os.ReadFile(exampleConfigPath)
	if err != nil {
		t.Fatalf("reading %s: %v", exampleConfigPath, err)
	}
	want := `music_directories = ["` + exampleMusicDirectory + `"]`
	if !strings.Contains(string(data), want) {
		t.Errorf("config.example.toml should contain %q", want)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" && strings.Contains(string(data), home) {
		t.Errorf("config.example.toml leaks the current home directory %q", home)
	}
}
