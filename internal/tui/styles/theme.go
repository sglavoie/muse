package styles

import (
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
		}
	}
	return canvas.Render()
}
