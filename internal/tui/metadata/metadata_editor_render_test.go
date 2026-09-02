package metadata

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestDefaultEditorRenderShowsAllFields verifies that the default (full) editor
// still shows all three fields (regression guard).
func TestDefaultEditorRenderShowsAllFields(t *testing.T) {
	ed := NewEditor("h1", "My Title", "My Artist", "My Album", "", 0)
	s := newTestStyles()
	out := ansi.Strip(ed.Render(80, 24, s))

	for _, want := range []string{"Title:", "Artist:", "Album:", "Genre:", "Duration:", "tab/shift+tab", "Edit Metadata"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in default editor output; got:\n%s", want, out)
		}
	}
}

// TestMetadataEditorRenderNarrowTerminalNoPanic verifies that rendering with a
// very narrow terminal (where the title is wider than boxInner) does not panic.
func TestMetadataEditorRenderNarrowTerminalNoPanic(t *testing.T) {
	hashes := make([]string, 50)
	for i := range hashes {
		hashes[i] = "hash"
	}
	ed := NewBulkEditor(hashes, "T", "A", "B", "", "")
	s := newTestStyles()

	out := ed.Render(20, 10, s)
	if out == "" {
		t.Fatal("Render returned empty string for narrow terminal")
	}
}

// TestMetadataEditorRenderWrapsLongValue verifies that both focused and
// non-focused fields with text longer than valueWidth do not exceed the box
// width. Focused fields scroll/clip; non-focused fields wrap to multiple rows.
func TestMetadataEditorRenderWrapsLongValue(t *testing.T) {
	longTitle := "This is a very long title that should definitely wrap to multiple lines in the metadata editor popup box"
	termWidth := 88

	t.Run("focused field does not overflow", func(t *testing.T) {
		ed := NewEditor("h1", longTitle, "Artist", "Album", "Rock", 0)
		s := newTestStyles()

		out := ansi.Strip(ed.Render(termWidth, 24, s))
		for _, line := range strings.Split(out, "\n") {
			w := ansi.StringWidth(line)
			if w > termWidth {
				t.Errorf("cursor-at-end: line exceeds terminal width %d (got %d): %q", termWidth, w, line)
			}
		}

		edMid := ed
		for range 30 {
			edMid, _, _ = edMid.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
		}
		outMid := ansi.Strip(edMid.Render(termWidth, 24, s))
		for _, line := range strings.Split(outMid, "\n") {
			w := ansi.StringWidth(line)
			if w > termWidth {
				t.Errorf("cursor-in-middle: line exceeds terminal width %d (got %d): %q", termWidth, w, line)
			}
		}
	})

	t.Run("non-focused field wraps correctly", func(t *testing.T) {
		ed := NewEditor("h1", longTitle, "Artist", "Album", "Rock", 0)
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: '\t'})

		s := newTestStyles()
		out := ansi.Strip(ed.Render(termWidth, 24, s))

		outLines := strings.Split(out, "\n")
		flat := strings.Join(outLines, " ")
		for _, word := range strings.Fields(longTitle) {
			if !strings.Contains(flat, word) {
				t.Errorf("expected word %q from title in output; got:\n%s", word, out)
			}
		}

		titleLabelCount := 0
		for _, line := range outLines {
			if strings.Contains(line, "Title:") {
				titleLabelCount++
			}
		}
		if titleLabelCount != 1 {
			t.Errorf("expected Title: label on exactly 1 line, found %d", titleLabelCount)
		}

		for _, line := range outLines {
			w := ansi.StringWidth(line)
			if w > termWidth {
				t.Errorf("non-focused: line exceeds terminal width %d (got %d): %q", termWidth, w, line)
			}
		}
	})
}
