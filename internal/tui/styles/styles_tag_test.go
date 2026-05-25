package styles_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestTagColorStyleTrimsColor(t *testing.T) {
	got := colorRGBA(t, styles.TagColorStyle("  #ff6600\t").GetForeground())
	want := parseHex(t, "#ff6600")
	if got != want {
		t.Fatalf("TagColorStyle foreground = %#v, want %#v", got, want)
	}
}

func TestTagColorStyleAcceptsShortHexColor(t *testing.T) {
	got := colorRGBA(t, styles.TagColorStyle("#f60").GetForeground())
	want := parseHex(t, "#ff6600")
	if got != want {
		t.Fatalf("TagColorStyle foreground = %#v, want %#v", got, want)
	}
}

func TestColoredDotWithInvalidColorIsPlain(t *testing.T) {
	for _, c := range []string{"123", "blue", "#gggggg", "#12345"} {
		t.Run(c, func(t *testing.T) {
			got := styles.ColoredDot(c)
			if got != "●" {
				t.Fatalf("ColoredDot(%q) = %q, want plain dot", c, got)
			}
		})
	}
}

func TestColoredDotWithBlankColorIsPlain(t *testing.T) {
	got := styles.ColoredDot(" \t\n ")
	if got != "●" {
		t.Fatalf("ColoredDot with blank color = %q, want plain dot", got)
	}
}
