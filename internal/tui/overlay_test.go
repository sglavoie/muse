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

func TestOverlayBoxRender_CentersHintIndependently(t *testing.T) {
	box := OverlayBox{
		Title: " T ",
		Rows:  []OverlayRow{{Text: "x"}},
		Hint:  "esc to dismiss",
	}
	stripped := ansi.Strip(box.Render(80, 24, newTestStyles()))
	lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
	hintLine := lines[len(lines)-1]
	leadingSpaces := len(hintLine) - len(strings.TrimLeft(hintLine, " "))
	want := (80 - len("esc to dismiss")) / 2
	if leadingSpaces != want {
		t.Fatalf("hint leading spaces = %d, want %d\noutput:\n%s", leadingSpaces, want, stripped)
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

func TestOverlayBoxRender_MultiByteRowAlignment(t *testing.T) {
	// All rows in an overlay box should have identical display width
	// regardless of whether the content contains multi-byte characters.
	box := OverlayBox{
		Title: " Test ",
		Rows: []OverlayRow{
			{Text: "ASCII only row"},
			{Text: "Björk — Jóga"}, // accented chars (2 bytes each)
			{Text: "日本語テスト"},       // CJK chars (3 bytes, 2 display columns each)
			{Label: "Artist:", Text: "Björk"},
			{Label: "Title:", Text: "日本語"},
		},
	}
	out := box.Render(80, 24, newTestStyles())
	stripped := ansi.Strip(out)
	lines := strings.Split(stripped, "\n")

	// Find content lines (between the borders).
	var contentLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, borderV) && strings.HasSuffix(line, borderV) {
			contentLines = append(contentLines, line)
		}
	}

	if len(contentLines) < 2 {
		t.Fatalf("expected at least 2 content lines, got %d", len(contentLines))
	}

	// Every bordered line should have the same display width.
	refWidth := ansi.StringWidth(contentLines[0])
	for i, line := range contentLines[1:] {
		w := ansi.StringWidth(line)
		if w != refWidth {
			t.Errorf("line %d display width = %d, want %d (same as line 0)\nline 0: %q\nline %d: %q",
				i+1, w, refWidth, contentLines[0], i+1, line)
		}
	}
}

func TestOverlayBoxRender_LabelledRowsMultiByte(t *testing.T) {
	// Verify labelled rows with multi-byte labels align correctly.
	box := OverlayBox{
		Title: " Box ",
		Rows: []OverlayRow{
			{Label: "Artist:", Text: "Data One"},
			{Label: "Title:", Text: "Data Two"},
		},
	}
	out := box.Render(80, 24, newTestStyles())
	stripped := ansi.Strip(out)

	// Both data rows should have the same display width.
	lines := strings.Split(stripped, "\n")
	var dataLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, borderV) && strings.HasSuffix(line, borderV) && strings.Contains(line, "Data") {
			dataLines = append(dataLines, line)
		}
	}

	if len(dataLines) != 2 {
		t.Fatalf("expected 2 data lines, got %d", len(dataLines))
	}

	w0 := ansi.StringWidth(dataLines[0])
	w1 := ansi.StringWidth(dataLines[1])
	if w0 != w1 {
		t.Errorf("labelled row widths differ: %d vs %d\nline 0: %q\nline 1: %q",
			w0, w1, dataLines[0], dataLines[1])
	}
}
