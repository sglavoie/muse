package styles

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// accentPalette holds the shades behind each named accent (see
// config.AccentPresets). Dark shades are soft pastels that sit calmly on the
// dark canvas; light shades are deeper so text accents stay readable on the
// pale canvas. Each is tuned to be about as prominent as the default blue.
// The neutrals keep their character per preset: white is near-white on Dark
// and near-black ink on Light, since white on a pale canvas would vanish.
var accentPalette = map[string]struct{ light, dark string }{
	"blue":   {colorAccentPrimaryLight, colorAccentPrimaryDark},
	"teal":   {"#16766f", "#7fd1c7"},
	"green":  {"#3b7a35", "#a3d48c"},
	"olive":  {"#5c6a26", "#c5cc8c"},
	"yellow": {"#856600", "#e9cb7d"},
	"sand":   {"#7a6640", "#dcc8a2"},
	"orange": {"#ad5418", "#f2ae84"},
	"brown":  {"#875632", "#d2a77e"},
	"red":    {"#b3383e", "#f0918b"},
	"pink":   {"#a83a75", "#f2a7c8"},
	"purple": {"#6a4cbc", "#c3aaf2"},
	"grey":   {"#5b626d", "#aab0bb"},
	"white":  {"#1d222b", "#eceef2"},
}

type themeColors struct {
	background color.Color
	primary    color.Color
	onPrimary  color.Color
	accentText color.Color
	focus      color.Color
	selection  color.Color
	nowPlaying color.Color
}

func newThemeColors(hasDark bool, primary string) themeColors {
	ld := lipgloss.LightDark(hasDark)
	bg := ld(lipgloss.Color("#f8f9fc"), lipgloss.Color("#191e28"))
	base := ld(lipgloss.Color(colorAccentPrimaryLight), lipgloss.Color(colorAccentPrimaryDark))
	primary = strings.ToLower(strings.TrimSpace(primary))
	if shades, ok := accentPalette[primary]; ok {
		base = ld(lipgloss.Color(shades.light), lipgloss.Color(shades.dark))
	} else if isHexColor(primary) {
		base = lipgloss.Color(primary)
	}
	// Navigation surfaces are neutral, lifted toward the text color, so the
	// accent stays reserved for what is active: the playing track, the active
	// view, key hints. The now-playing row carries the accent tint instead.
	ink := ld(lipgloss.Color(colorTextPrimaryLight), lipgloss.Color(colorTextPrimaryDark))
	focus := mixColors(bg, ink, 0.12)
	selection := mixColors(bg, ink, 0.06)
	nowPlaying := mixColors(bg, base, 0.14)
	zebra := ld(lipgloss.Color(colorSurfaceL2Light), lipgloss.Color(colorSurfaceL2Dark))
	text := base
	target := lipgloss.Color("#000000")
	if hasDark {
		target = lipgloss.Color("#ffffff")
	}
	// Preserve the chosen fill color; tune its text variant for legibility on
	// both the canvas and the stronger focus surface, even for extreme overrides.
	for i := 1; i <= 100 && (colorContrast(text, bg) < 4.5 || colorContrast(text, focus) < 4.5 || colorContrast(text, zebra) < 4.5 || colorContrast(text, nowPlaying) < 4.5); i++ {
		text = mixColors(base, target, float64(i)/100)
	}
	onPrimary := lipgloss.Color("#000000")
	white := lipgloss.Color("#ffffff")
	if colorContrast(white, base) > colorContrast(onPrimary, base) {
		onPrimary = white
	}
	return themeColors{background: bg, primary: base, onPrimary: onPrimary,
		accentText: text, focus: focus, selection: selection, nowPlaying: nowPlaying}
}

// favoriteColor returns rose, or gold when the accent's hue is close enough
// to rose that favorites would read as playing.
func favoriteColor(accent, rose, gold color.Color) color.Color {
	h, sat := hueSat(accent)
	rh, _ := hueSat(rose)
	d := math.Abs(h - rh)
	if d > 180 {
		d = 360 - d
	}
	if sat >= 0.25 && d < 45 {
		return gold
	}
	return rose
}

// hueSat returns c's HSL hue in degrees and its saturation, 0..1.
func hueSat(c color.Color) (hue, sat float64) {
	r, g, b, _ := c.RGBA()
	rf, gf, bf := float64(r)/65535, float64(g)/65535, float64(b)/65535
	hi, lo := max(rf, gf, bf), min(rf, gf, bf)
	delta := hi - lo
	if delta == 0 {
		return 0, 0
	}
	l := (hi + lo) / 2
	sat = delta / (1 - math.Abs(2*l-1))
	switch hi {
	case rf:
		hue = math.Mod((gf-bf)/delta, 6)
	case gf:
		hue = (bf-rf)/delta + 2
	default:
		hue = (rf-gf)/delta + 4
	}
	hue *= 60
	if hue < 0 {
		hue += 360
	}
	return hue, sat
}

func mixColors(a, b color.Color, amount float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	channel := func(x, y uint32) uint8 {
		return uint8(math.Round((float64(x)*(1-amount) + float64(y)*amount) / 257))
	}
	return color.RGBA{R: channel(ar, br), G: channel(ag, bg), B: channel(ab, bb), A: 255}
}

func colorLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	linear := func(channel uint32) float64 {
		v := float64(channel) / 65535
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}

func colorContrast(a, b color.Color) float64 {
	x, y := colorLuminance(a), colorLuminance(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}

// RenderScreen paints unstyled text and all unused cells with the preset's
// canvas colors. Nested semantic foregrounds and row backgrounds are retained.
func (s Styles) RenderScreen(content string, width, height int) string {
	return s.RenderScreenFaded(content, width, height, 1, 0, 0)
}

// RenderScreenFaded is RenderScreen with lines fromRow up to (not including)
// toRow faded in from the canvas: at reveal 0 they are drawn in the
// background color, at 1 as they are. Kitty image placeholders keep their
// colors, which identify the image rather than tint it.
func (s Styles) RenderScreenFaded(content string, width, height int, reveal float64, fromRow, toRow int) string {
	content = lipgloss.NewStyle().Width(max(0, width)).Height(max(0, height)).Render(content)
	canvas := lipgloss.NewCanvas(lipgloss.Width(content), lipgloss.Height(content)).
		Compose(lipgloss.NewLayer(content))
	// SGR resets inside nested styles return to terminal defaults, not to an
	// enclosing Lip Gloss style. Resolve defaults on the parsed cells instead
	// of wrapping the ANSI string; retain explicit colors and text attributes.
	for y := range canvas.Height() {
		for x := range canvas.Width() {
			cell := canvas.CellAt(x, y)
			if cell.Width == 0 {
				continue // continuation of a wide character
			}
			if cell.Style.Fg == nil {
				cell.Style.Fg = s.TextPrimary.GetForeground()
			}
			if cell.Style.Bg == nil {
				cell.Style.Bg = s.Background
			}
			if y >= fromRow && y < toRow && reveal < 1 && !strings.HasPrefix(cell.Content, kittyPlaceholder) {
				cell.Style.Fg = mixColors(s.Background, cell.Style.Fg, reveal)
				cell.Style.Bg = mixColors(s.Background, cell.Style.Bg, reveal)
			}
		}
	}
	return canvas.Render()
}

// kittyPlaceholder starts a cell that draws part of a Kitty graphics image.
const kittyPlaceholder = "\U0010EEEE"

// Mix blends a toward b by amount (0 keeps a, 1 is b).
func Mix(a, b color.Color, amount float64) color.Color {
	return mixColors(a, b, math.Min(1, math.Max(0, amount)))
}

// Contrast is the WCAG contrast ratio between two colors, from 1 (none) to
// 21 (black on white).
func Contrast(a, b color.Color) float64 {
	return colorContrast(a, b)
}

// Hex formats c as #rrggbb.
func Hex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// ViewHue returns the style for the i-th primary view's number in the title
// bar. Every view shares one quiet tone: the active view is marked by the
// accent-filled tab, so the numbers need no colors of their own.
func (s Styles) ViewHue(int) lipgloss.Style {
	return s.TextSecondary
}
