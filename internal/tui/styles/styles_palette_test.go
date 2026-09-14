package styles_test

import (
	"encoding/hex"
	"image/color"
	"math"
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// relativeLuminance returns the WCAG 2.x relative luminance of an sRGB color
// (channels in [0,255]). Result is in [0,1].
func relativeLuminance(c color.RGBA) float64 {
	conv := func(b uint8) float64 {
		v := float64(b) / 255.0
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*conv(c.R) + 0.7152*conv(c.G) + 0.0722*conv(c.B)
}

// contrastRatio returns the WCAG contrast ratio between two sRGB colors.
func contrastRatio(fg, bg color.RGBA) float64 {
	l1 := relativeLuminance(fg)
	l2 := relativeLuminance(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// parseHex parses a "#rrggbb" string into a color.RGBA with alpha 0xff.
func parseHex(t *testing.T, hexColor string) color.RGBA {
	t.Helper()
	if len(hexColor) != 7 || hexColor[0] != '#' {
		t.Fatalf("parseHex: bad hex %q (want #rrggbb)", hexColor)
	}
	rgb, err := hex.DecodeString(hexColor[1:])
	if err != nil {
		t.Fatalf("parseHex(%q): %v", hexColor, err)
	}
	if len(rgb) != 3 {
		t.Fatalf("parseHex(%q): got %d bytes, want 3", hexColor, len(rgb))
	}
	return color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 0xff}
}

func colorRGBA(t *testing.T, c color.Color) color.RGBA {
	t.Helper()
	if c == nil {
		t.Fatal("color is nil")
	}
	rgba, ok := color.RGBAModel.Convert(c).(color.RGBA)
	if !ok {
		t.Fatalf("color model conversion returned %T, want color.RGBA", color.RGBAModel.Convert(c))
	}
	return rgba
}

func TestEmptyHintUsesExplicitDimForeground(t *testing.T) {
	cases := []struct {
		name    string
		hasDark bool
		want    string
	}{
		{"dark", true, "#9aa6bd"},
		{"light", false, "#566177"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := colorRGBA(t, styles.NewStyles(c.hasDark).EmptyHint.GetForeground())
			want := parseHex(t, c.want)
			if got != want {
				t.Fatalf("EmptyHint foreground = %#v, want %#v", got, want)
			}
		})
	}
}

func TestProgressEmptyUsesExplicitDimForeground(t *testing.T) {
	cases := []struct {
		name    string
		hasDark bool
		want    string
	}{
		{"dark", true, "#9aa6bd"},
		{"light", false, "#566177"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := colorRGBA(t, styles.NewStyles(c.hasDark).ProgressEmpty.GetForeground())
			want := parseHex(t, c.want)
			if got != want {
				t.Fatalf("ProgressEmpty foreground = %#v, want %#v", got, want)
			}
		})
	}
}

func TestProgressEmptyUsesExplicitTrackBackground(t *testing.T) {
	cases := []struct {
		name    string
		hasDark bool
		want    string
	}{
		{"dark", true, "#3b4252"},
		{"light", false, "#d0d7e3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := colorRGBA(t, styles.NewStyles(c.hasDark).ProgressEmpty.GetBackground())
			want := parseHex(t, c.want)
			if got != want {
				t.Fatalf("ProgressEmpty background = %#v, want %#v", got, want)
			}
		})
	}
}

// Check the actual styles on every ordinary row surface in both presets.
func TestPresetTextContrast(t *testing.T) {
	for _, dark := range []bool{false, true} {
		s := styles.NewStyles(dark)
		for name, fg := range map[string]color.Color{
			"primary": s.TextPrimary.GetForeground(), "secondary": s.TextSecondary.GetForeground(),
			"tertiary": s.TextTertiary.GetForeground(), "dim": s.Dim.GetForeground(),
		} {
			for _, bg := range []color.Color{s.Background, s.ZebraBg, s.SelectionBg, s.Accent.GetBackground()} {
				if ratio := contrastRatio(colorRGBA(t, fg), colorRGBA(t, bg)); ratio < 4.5 {
					t.Errorf("dark=%v %s contrast %.2f < 4.5", dark, name, ratio)
				}
			}
		}
	}
}

func TestActiveTabContrast(t *testing.T) {
	for _, c := range []struct {
		name    string
		hasDark bool
	}{
		{"dark", true},
		{"light", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			activeTab := styles.NewStyles(c.hasDark).ActiveTab
			fg := colorRGBA(t, activeTab.GetForeground())
			bg := colorRGBA(t, activeTab.GetBackground())
			ratio := contrastRatio(fg, bg)
			if ratio < 4.5 {
				t.Fatalf("ActiveTab contrast %.2f:1 < 4.5:1 (fg %#v on bg %#v)", ratio, fg, bg)
			}
		})
	}
}
