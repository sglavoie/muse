package styles

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/config"
)

// Every named accent resolves to its own shade in each preset, and that shade
// is legible as text on the preset canvas without contrast adjustment.
func TestAccentPresetsResolvePerTheme(t *testing.T) {
	for _, name := range config.AccentPresets {
		shades, ok := accentPalette[name]
		if !ok {
			t.Fatalf("accent %q has no palette entry", name)
		}
		for _, dark := range []bool{false, true} {
			want := shades.light
			if dark {
				want = shades.dark
			}
			th := NewTheme(dark, name)
			if got := Hex(th.Primary); got != want {
				t.Errorf("%s dark=%v: primary %s, want %s", name, dark, got, want)
			}
			if c := Contrast(lipgloss.Color(want), th.Background); c < 4.5 {
				t.Errorf("%s dark=%v: contrast %.2f on canvas, want >= 4.5", name, dark, c)
			}
		}
	}
	if len(accentPalette) != len(config.AccentPresets) {
		t.Errorf("palette has %d entries for %d presets", len(accentPalette), len(config.AccentPresets))
	}
}

func TestAccentPresetNamesAreCaseInsensitive(t *testing.T) {
	if Hex(NewTheme(true, " Teal ").Primary) != accentPalette["teal"].dark {
		t.Fatal("accent names should ignore case and surrounding space")
	}
}
