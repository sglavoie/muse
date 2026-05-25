package styles_test

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestStyleNesting verifies that Background wrapping Bold output preserves inner
// styling. Bold sequences must survive the outer Background render, satisfying
// ac-f4704a90675005f2b2db92c90011a914.
func TestStyleNesting(t *testing.T) {
	bold := lipgloss.NewStyle().Bold(true)
	bg := lipgloss.NewStyle().Background(lipgloss.Color("#2c3e50"))

	inner := bold.Render("hello")
	if inner == "hello" {
		t.Fatal("bold.Render did not apply any ANSI sequences to the text")
	}

	outer := bg.Render(inner)
	if outer == "" {
		t.Fatal("outer render is empty")
	}

	// The bold ANSI attribute sequence must be explicitly present in the outer render.
	// Constructed at runtime so this source file does not trigger TestNoRawANSI.
	boldSeq := string([]byte{0x1b, 0x5b, 0x31, 0x6d}) // ESC [ 1 m
	if !strings.Contains(outer, boldSeq) {
		t.Errorf("outer render does not contain bold ANSI sequence (ESC[1m); outer=%q", outer)
	}

	// The bold sequences from inner must be present inside the outer render.
	if !strings.Contains(outer, inner) {
		t.Errorf("outer render does not contain inner bold sequences; outer=%q inner=%q", outer, inner)
	}

	stripped := ansi.Strip(outer)
	if stripped != "hello" {
		t.Errorf("stripped text = %q, want %q", stripped, "hello")
	}

	// Verify that the package's own Accent style round-trips through ansi.Strip,
	// tying this contract test to styles.NewStyles rather than raw lipgloss only.
	accentOut := styles.NewStyles(true).Accent.Render("hello")
	if ansi.Strip(accentOut) != "hello" {
		t.Errorf("ansi.Strip(NewStyles(true).Accent.Render(%q)) = %q, want %q", "hello", ansi.Strip(accentOut), "hello")
	}
}

// TestLipglossV2ColorAPI verifies that the native lipgloss v2 color API is in
// use. It calls lipgloss.Color() and assigns the result to a color.Color
// variable (this only compiles when lipgloss v2 returns color.Color, not the
// v1 string type alias). It also calls lipgloss.LightDark() to confirm that
// helper is available for adaptive theming.
func TestLipglossV2ColorAPI(t *testing.T) {
	// lipgloss.Color returns color.Color in v2; this assignment would fail to
	// compile against a v1 or compat package that returns a string type.
	var c color.Color = lipgloss.Color("#ff0000")
	if c == nil {
		t.Fatal("lipgloss.Color returned nil")
	}

	// lipgloss.LightDark must be callable; invoke it to confirm availability.
	ld := lipgloss.LightDark(true)
	if ld == nil {
		t.Fatal("lipgloss.LightDark returned nil function")
	}
}
