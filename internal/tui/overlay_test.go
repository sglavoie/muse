package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestOverlayBoxRender_ContainsTitle(t *testing.T) {
	box := OverlayBox{
		Title: " ♪ Test ",
		Rows:  []OverlayRow{{Text: "content"}},
		Hint:  "press esc",
	}
	out := ansi.Strip(box.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "Test") {
		t.Errorf("expected title text in output, got:\n%s", out)
	}
}

func TestOverlayBoxRender_ContainsRowContent(t *testing.T) {
	box := OverlayBox{
		Title: " Title ",
		Rows: []OverlayRow{
			{Text: "first row"},
			{Text: "styled row", Styled: true},
		},
	}
	out := ansi.Strip(box.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "first row") {
		t.Error("expected first row in output")
	}
	if !strings.Contains(out, "styled row") {
		t.Error("expected styled row in output")
	}
}

func TestOverlayBoxRender_ContainsHint(t *testing.T) {
	box := OverlayBox{
		Title: " T ",
		Rows:  []OverlayRow{{Text: "x"}},
		Hint:  "esc to dismiss",
	}
	out := ansi.Strip(box.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "esc to dismiss") {
		t.Error("expected hint text in output")
	}
}

func TestOverlayBoxRender_VerticalCentering(t *testing.T) {
	box := OverlayBox{
		Title: " T ",
		Rows:  []OverlayRow{{Text: "x"}},
	}
	out := box.Render(80, 40, newTestStyles())
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

func TestOverlayBoxRender_SmallTerminalNoPanic(t *testing.T) {
	box := OverlayBox{
		Title: " ♪ Preview ",
		Rows:  []OverlayRow{{Text: "some content"}},
		Hint:  "dismiss",
	}
	out := box.Render(10, 5, newTestStyles())
	if out == "" {
		t.Error("expected non-empty output even with small terminal")
	}
}
