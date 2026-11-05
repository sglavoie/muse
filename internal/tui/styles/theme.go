package styles

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

type themeColors struct {
	background color.Color
	primary    color.Color
	onPrimary  color.Color
	accentText color.Color
	focus      color.Color
	selection  color.Color
}

func newThemeColors(hasDark bool, primary string) themeColors {
	ld := lipgloss.LightDark(hasDark)
	bg := ld(lipgloss.Color("#f8f9fc"), lipgloss.Color("#191e28"))
	base := ld(lipgloss.Color(colorAccentPrimaryLight), lipgloss.Color(colorAccentPrimaryDark))
	if primary = strings.TrimSpace(primary); isHexColor(primary) {
		base = lipgloss.Color(primary)
	}
	focus := mixColors(bg, base, 0.12)
	selection := mixColors(bg, base, 0.06)
	zebra := ld(lipgloss.Color(colorSurfaceL2Light), lipgloss.Color(colorSurfaceL2Dark))
	text := base
	target := lipgloss.Color("#000000")
	if hasDark {
		target = lipgloss.Color("#ffffff")
	}
	// Preserve the chosen fill color; tune its text variant for legibility on
	// both the canvas and the stronger focus surface, even for extreme overrides.
	for i := 1; i <= 100 && (colorContrast(text, bg) < 4.5 || colorContrast(text, focus) < 4.5 || colorContrast(text, zebra) < 4.5); i++ {
		text = mixColors(base, target, float64(i)/100)
	}
	onPrimary := lipgloss.Color("#000000")
	white := lipgloss.Color("#ffffff")
	if colorContrast(white, base) > colorContrast(onPrimary, base) {
		onPrimary = white
	}
	return themeColors{background: bg, primary: base, onPrimary: onPrimary,
		accentText: text, focus: focus, selection: selection}
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

// viewHuesDark and viewHuesLight give each primary view its own quiet color,
// used for its number in the title bar so destinations are easy to find.
var (
	viewHuesDark  = []string{"#8ab4ff", "#c678dd", "#98c379", "#56b6c2", "#e0af68", "#e06c75", "#d19a66", "#61afef"}
	viewHuesLight = []string{"#315fbe", "#a626a4", "#287348", "#096e88", "#986801", "#b52f42", "#9a5a12", "#1f6fb2"}
)

// ViewHue returns the foreground style for the i-th primary view's marker.
func (s Styles) ViewHue(i int) lipgloss.Style {
	hues := viewHuesLight
	if s.HasDark {
		hues = viewHuesDark
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hues[((i%len(hues))+len(hues))%len(hues)]))
}

// columnTint nudges a text color toward the theme's accent text, enough to
// tell the artist column apart from titles at a glance while staying legible
// on the canvas, zebra and focus surfaces.
func columnTint(base color.Color, p themeColors) color.Color {
	for amount := 0.25; amount > 0; amount -= 0.05 {
		tinted := mixColors(base, p.accentText, amount)
		if colorContrast(tinted, p.background) >= 4.5 && colorContrast(tinted, p.focus) >= 4.5 {
			return tinted
		}
	}
	return base
}
