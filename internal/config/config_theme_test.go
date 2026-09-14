package config

import (
	"os"
	"testing"
)

func TestLoadTheme(t *testing.T) {
	for _, tc := range []struct {
		name, input, theme, primary string
		warnings                    int
	}{
		{"legacy", "default_volume = 75", "auto", "", 0},
		{"light", "theme = 'light'\nprimary_color = '#C0F'", "light", "#cc00ff", 0},
		{"dark", "theme = ' DARK '\nprimary_color = ' #23AB76 '", "dark", "#23ab76", 0},
		{"auto", "theme = 'auto'\nprimary_color = ''", "auto", "", 0},
		{"invalid", "theme = 'sepia'\nprimary_color = '#nope'", "auto", "", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(ConfigPath(dir), []byte(tc.input), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if result.Config.Theme != tc.theme || result.Config.PrimaryColor != tc.primary || len(result.Warnings) != tc.warnings {
				t.Fatalf("got theme=%q primary=%q warnings=%v", result.Config.Theme, result.Config.PrimaryColor, result.Warnings)
			}
			if err := Save(ConfigPath(dir), result.Config); err != nil {
				t.Fatal(err)
			}
			reloaded, err := Load(dir)
			if err != nil || reloaded.Config.Theme != tc.theme || reloaded.Config.PrimaryColor != tc.primary || len(reloaded.Warnings) != 0 {
				t.Fatalf("round trip failed: %+v, %v", reloaded, err)
			}
		})
	}
}

func TestNormalizePrimaryColorRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{"red", "123456", "#12", "#12345", "#12345678", "#gggggg", "#12 456", "#１２３", "#123\n456"} {
		if _, ok := NormalizePrimaryColor(value); ok {
			t.Errorf("accepted %q", value)
		}
	}
}
