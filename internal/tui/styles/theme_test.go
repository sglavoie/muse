package styles_test

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestCustomPrimaryColors(t *testing.T) {
	for _, dark := range []bool{true, false} {
		for _, primary := range []string{"#cc44aa", "#ffffff", "#000000", "#ffff00", "#001166"} {
			s := styles.NewTheme(dark, primary)
			if colorRGBA(t, s.ActiveTab.GetBackground()) != parseHex(t, primary) {
				t.Fatalf("primary fill does not preserve %s", primary)
			}
			if ratio := contrastRatio(colorRGBA(t, s.ActiveTab.GetForeground()), colorRGBA(t, s.Primary)); ratio < 4.5 {
				t.Errorf("dark=%v %s active tab contrast %.2f", dark, primary, ratio)
			}
			for _, fg := range []color.Color{s.Heading.GetForeground(), s.HelpKey.GetForeground(), s.ProgressFilled.GetForeground(), s.AccentMark.GetForeground()} {
				for _, bg := range []color.Color{s.Background, s.Accent.GetBackground(), s.ZebraBg} {
					if ratio := contrastRatio(colorRGBA(t, fg), colorRGBA(t, bg)); ratio < 4.5 {
						t.Errorf("dark=%v %s accent text contrast %.2f", dark, primary, ratio)
					}
				}
			}
			def := styles.NewStyles(dark)
			if colorRGBA(t, s.Accent.GetBackground()) == colorRGBA(t, def.Accent.GetBackground()) ||
				colorRGBA(t, s.SelectionBg) == colorRGBA(t, def.SelectionBg) {
				t.Error("custom primary did not tint focus and selection")
			}
			if colorRGBA(t, s.Error.GetForeground()) != colorRGBA(t, def.Error.GetForeground()) {
				t.Error("primary changed semantic error color")
			}
		}
	}
}

func TestRenderScreenFillsCanvasAndPreservesStyles(t *testing.T) {
	for _, dark := range []bool{false, true} {
		s := styles.NewStyles(dark)
		accent := s.ActiveTab.Render("Active")
		rendered := s.RenderScreen(accent+" plain\n"+s.Error.Render("Error"), 20, 5)
		lines := strings.Split(ansi.Strip(rendered), "\n")
		if len(lines) != 5 {
			t.Fatalf("got %d rows, want 5", len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) != 20 {
				t.Errorf("canvas row width = %d", ansi.StringWidth(line))
			}
		}
		canvas := lipgloss.NewCanvas(20, 5).Compose(lipgloss.NewLayer(rendered))
		if got := canvas.CellAt(0, 0).Style; colorRGBA(t, got.Bg) != colorRGBA(t, s.Primary) || colorRGBA(t, got.Fg) != colorRGBA(t, s.ActiveTab.GetForeground()) {
			t.Fatal("canvas lost the active tab colors")
		}
		if rendered == ansi.Strip(rendered) {
			t.Fatal("canvas is unstyled")
		}
	}
}

func TestScreenResolvesColorsAfterNestedStyleResets(t *testing.T) {
	for _, dark := range []bool{false, true} {
		s := styles.NewStyles(dark)
		content := s.Heading.Render("Heading") + " plain\n" +
			s.Dim.Render("Dim") + " plain\n" +
			s.Accent.Render("Focus") + " border\n" +
			s.TextPrimary.Background(s.ZebraBg).Render("Stripe") + " plain\n" +
			s.TextPrimary.Render("Artist") + " " + s.TextSecondary.Render("Album")
		canvas := lipgloss.NewCanvas(20, 6).Compose(lipgloss.NewLayer(s.RenderScreen(content, 20, 6)))
		for y := range canvas.Height() {
			for x := range canvas.Width() {
				cell := canvas.CellAt(x, y)
				wantBg := s.Background
				switch {
				case y == 2 && x < 5:
					wantBg = s.Accent.GetBackground()
				case y == 3 && x < 6:
					wantBg = s.ZebraBg
				}
				if cell.Style.Bg == nil || cell.Style.Fg == nil {
					t.Fatalf("dark=%v cell (%d,%d) %q uses terminal default colors after a style reset", dark, x, y, cell.Content)
				}
				if colorRGBA(t, cell.Style.Bg) != colorRGBA(t, wantBg) {
					t.Fatalf("dark=%v cell (%d,%d) %q has wrong background", dark, x, y, cell.Content)
				}
			}
		}
		if colorRGBA(t, canvas.CellAt(8, 0).Style.Fg) != colorRGBA(t, s.TextPrimary.GetForeground()) {
			t.Fatal("plain text after a heading lost the theme foreground")
		}
	}
}
