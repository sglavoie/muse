package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func newTestStyles() styles.Styles {
	return styles.NewStyles(true)
}

func TestPreviewOverlayRender_BasicContent(t *testing.T) {
	p := PreviewOverlay{
		artist:  "Test Artist",
		title:   "Test Song",
		elapsed: 83,   // 1:23
		total:   296,  // 4:56
	}
	s := newTestStyles()
	out := p.Render(80, 24, s)
	stripped := ansi.Strip(out)

	if !strings.Contains(stripped, "Preview") {
		t.Errorf("expected 'Preview' header in output, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "Test Artist") {
		t.Error("expected artist in output")
	}
	if !strings.Contains(stripped, "Test Song") {
		t.Error("expected title in output")
	}
	if !strings.Contains(stripped, "1:23") || !strings.Contains(stripped, "4:56") {
		t.Error("expected formatted elapsed/total times in output")
	}
	if !strings.Contains(stripped, "P or esc to dismiss") {
		t.Error("expected dismiss hint in output")
	}
}

func TestPreviewOverlayRender_UnknownTrack(t *testing.T) {
	p := PreviewOverlay{}
	s := newTestStyles()
	out := p.Render(80, 24, s)

	if !strings.Contains(out, "(unknown)") {
		t.Error("expected '(unknown)' for empty artist/title")
	}
}

func TestPreviewOverlayRender_ArtistOnly(t *testing.T) {
	p := PreviewOverlay{artist: "Solo Artist"}
	s := newTestStyles()
	out := p.Render(80, 24, s)

	if !strings.Contains(out, "Solo Artist") {
		t.Error("expected artist name in output")
	}
}

func TestPreviewOverlayRender_TitleOnly(t *testing.T) {
	p := PreviewOverlay{title: "Untitled Track"}
	s := newTestStyles()
	out := p.Render(80, 24, s)

	if !strings.Contains(out, "Untitled Track") {
		t.Error("expected title in output")
	}
}

func TestPreviewOverlayRender_CenteredVertically(t *testing.T) {
	p := PreviewOverlay{artist: "A", title: "B"}
	s := newTestStyles()
	out := p.Render(80, 40, s)

	// The output should start with blank lines for vertical centering.
	lines := strings.Split(out, "\n")
	blankCount := 0
	for _, line := range lines {
		if line == "" {
			blankCount++
		} else {
			break
		}
	}
	if blankCount < 5 {
		t.Errorf("expected vertical centering padding (>= 5 blank lines), got %d", blankCount)
	}
}

func TestPreviewOverlayRender_SmallTerminal(t *testing.T) {
	p := PreviewOverlay{artist: "A", title: "B"}
	s := newTestStyles()
	// Should not panic with a very small terminal.
	out := p.Render(10, 5, s)
	if out == "" {
		t.Error("expected non-empty output even with small terminal")
	}
}
