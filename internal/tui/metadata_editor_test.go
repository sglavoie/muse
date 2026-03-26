package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// --- NewArtistRenameEditor ---

// TestNewArtistRenameEditor verifies that a freshly constructed artist rename
// editor has the correct mode, focused field, initial artist value, and hashes.
func TestNewArtistRenameEditor(t *testing.T) {
	hashes := []string{"h1", "h2", "h3"}
	ed := NewArtistRenameEditor(hashes, "Alice")

	if ed.Mode() != editorModeRenameArtist {
		t.Errorf("expected editorModeRenameArtist, got %v", ed.Mode())
	}
	if ed.focused != fieldArtist {
		t.Errorf("expected focused=fieldArtist, got %v", ed.focused)
	}
	if ed.Artist() != "Alice" {
		t.Errorf("expected artist %q, got %q", "Alice", ed.Artist())
	}
	if ed.Title() != "" {
		t.Errorf("expected empty title, got %q", ed.Title())
	}
	if ed.Album() != "" {
		t.Errorf("expected empty album, got %q", ed.Album())
	}
	if len(ed.Hashes()) != 3 {
		t.Fatalf("expected 3 hashes, got %d", len(ed.Hashes()))
	}
	for i, h := range hashes {
		if ed.Hashes()[i] != h {
			t.Errorf("hashes[%d]: expected %q, got %q", i, h, ed.Hashes()[i])
		}
	}
	if ed.renameTitle != " ✎ Rename Artist " {
		t.Errorf("expected renameTitle %q, got %q", " ✎ Rename Artist ", ed.renameTitle)
	}
}

// --- NewAlbumRenameEditor ---

// TestNewAlbumRenameEditor verifies that a freshly constructed album rename
// editor has the correct mode, focused field, initial album value, and hashes.
func TestNewAlbumRenameEditor(t *testing.T) {
	hashes := []string{"h4", "h5"}
	ed := NewAlbumRenameEditor(hashes, "Gamma")

	if ed.Mode() != editorModeRenameAlbum {
		t.Errorf("expected editorModeRenameAlbum, got %v", ed.Mode())
	}
	if ed.focused != fieldAlbum {
		t.Errorf("expected focused=fieldAlbum, got %v", ed.focused)
	}
	if ed.Album() != "Gamma" {
		t.Errorf("expected album %q, got %q", "Gamma", ed.Album())
	}
	if ed.Title() != "" {
		t.Errorf("expected empty title, got %q", ed.Title())
	}
	if ed.Artist() != "" {
		t.Errorf("expected empty artist, got %q", ed.Artist())
	}
	if len(ed.Hashes()) != 2 {
		t.Fatalf("expected 2 hashes, got %d", len(ed.Hashes()))
	}
	if ed.renameTitle != " ✎ Rename Album " {
		t.Errorf("expected renameTitle %q, got %q", " ✎ Rename Album ", ed.renameTitle)
	}
}

// --- DirtyFields in rename mode ---

// TestRenameEditorDirtyFields verifies that modifying only the artist field
// marks it dirty while title and album remain nil.
func TestRenameEditorDirtyFields(t *testing.T) {
	ed := NewArtistRenameEditor([]string{"h1", "h2"}, "Alice")

	// Simulate typing a new character.
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})

	title, artist, album, _ := ed.DirtyFields()
	if title != nil {
		t.Errorf("expected title dirty=nil, got %q", *title)
	}
	if artist == nil {
		t.Fatal("expected artist dirty, got nil")
	}
	if *artist != "Alicex" {
		t.Errorf("expected artist %q, got %q", "Alicex", *artist)
	}
	if album != nil {
		t.Errorf("expected album dirty=nil, got %q", *album)
	}
}

// TestRenameEditorAlbumDirtyFields verifies that modifying only the album field
// marks it dirty while title and artist remain nil.
func TestRenameEditorAlbumDirtyFields(t *testing.T) {
	ed := NewAlbumRenameEditor([]string{"h3"}, "Gamma")

	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})

	title, artist, album, _ := ed.DirtyFields()
	if title != nil {
		t.Errorf("expected title dirty=nil, got %q", *title)
	}
	if artist != nil {
		t.Errorf("expected artist dirty=nil, got %q", *artist)
	}
	if album == nil {
		t.Fatal("expected album dirty, got nil")
	}
	if *album != "Gammaz" {
		t.Errorf("expected album %q, got %q", "Gammaz", *album)
	}
}

// --- Tab / shift+tab are no-ops in rename mode ---

// TestRenameEditorHandleKeyTabNoOp verifies that tab and shift+tab do not
// change the focused field when the editor is in rename mode.
func TestRenameEditorHandleKeyTabNoOp(t *testing.T) {
	ed := NewArtistRenameEditor([]string{"h1"}, "Alice")
	initialFocused := ed.focused

	edAfterTab, action, _ := ed.HandleKey(tea.KeyPressMsg{Code: '\t'})
	if edAfterTab.focused != initialFocused {
		t.Errorf("tab should not change focused in rename mode: was %v, got %v", initialFocused, edAfterTab.focused)
	}
	if action != editorNone {
		t.Errorf("expected editorNone after tab, got %v", action)
	}

	edAfterShiftTab, action2, _ := ed.HandleKey(tea.KeyPressMsg{Code: '\t', Mod: tea.ModShift})
	if edAfterShiftTab.focused != initialFocused {
		t.Errorf("shift+tab should not change focused in rename mode: was %v, got %v", initialFocused, edAfterShiftTab.focused)
	}
	if action2 != editorNone {
		t.Errorf("expected editorNone after shift+tab, got %v", action2)
	}
}

// TestDefaultEditorHandleKeyTabCycles verifies that tab does cycle fields in
// the default (non-rename) mode, to avoid regressions.
func TestDefaultEditorHandleKeyTabCycles(t *testing.T) {
	ed := NewMetadataEditor("h1", "Title", "Artist", "Album", "")
	if ed.focused != fieldTitle {
		t.Fatalf("expected initial focus=fieldTitle, got %v", ed.focused)
	}

	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: '\t'})
	if ed.focused != fieldArtist {
		t.Errorf("after tab: expected fieldArtist, got %v", ed.focused)
	}

	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: '\t', Mod: tea.ModShift})
	if ed.focused != fieldTitle {
		t.Errorf("after shift+tab: expected fieldTitle, got %v", ed.focused)
	}
}

// --- Render ---

// TestRenameEditorRender verifies that the artist rename editor renders only
// the Artist field, uses the custom title, and omits the tab hint.
func TestRenameEditorRender(t *testing.T) {
	ed := NewArtistRenameEditor([]string{"h1", "h2"}, "Alice")
	s := newTestStyles()
	out := ansi.Strip(ed.Render(80, 24, s))

	if !strings.Contains(out, "Rename Artist") {
		t.Errorf("expected 'Rename Artist' in output; got:\n%s", out)
	}
	if !strings.Contains(out, "Artist:") {
		t.Errorf("expected 'Artist:' field in output; got:\n%s", out)
	}
	if strings.Contains(out, "Title:") {
		t.Errorf("expected 'Title:' to be absent in rename mode; got:\n%s", out)
	}
	if strings.Contains(out, "Album:") {
		t.Errorf("expected 'Album:' to be absent in rename mode; got:\n%s", out)
	}
	// Hint line should NOT contain tab/shift+tab.
	if strings.Contains(out, "tab/shift+tab") {
		t.Errorf("expected no tab hint in rename mode; got:\n%s", out)
	}
	// But save/cancel hints should still be there.
	if !strings.Contains(out, "enter save") {
		t.Errorf("expected 'enter save' hint in output; got:\n%s", out)
	}
	if !strings.Contains(out, "esc cancel") {
		t.Errorf("expected 'esc cancel' hint in output; got:\n%s", out)
	}
}

// TestRenameAlbumEditorRender verifies that the album rename editor renders
// only the Album field with the correct custom title.
func TestRenameAlbumEditorRender(t *testing.T) {
	ed := NewAlbumRenameEditor([]string{"h3"}, "Gamma")
	s := newTestStyles()
	out := ansi.Strip(ed.Render(80, 24, s))

	if !strings.Contains(out, "Rename Album") {
		t.Errorf("expected 'Rename Album' in output; got:\n%s", out)
	}
	if !strings.Contains(out, "Album:") {
		t.Errorf("expected 'Album:' field in output; got:\n%s", out)
	}
	if strings.Contains(out, "Title:") {
		t.Errorf("expected 'Title:' to be absent; got:\n%s", out)
	}
	if strings.Contains(out, "Artist:") {
		t.Errorf("expected 'Artist:' to be absent; got:\n%s", out)
	}
}

// TestDefaultEditorRenderShowsAllFields verifies that the default (full) editor
// still shows all three fields (regression guard).
func TestDefaultEditorRenderShowsAllFields(t *testing.T) {
	ed := NewMetadataEditor("h1", "My Title", "My Artist", "My Album", "")
	s := newTestStyles()
	out := ansi.Strip(ed.Render(80, 24, s))

	for _, want := range []string{"Title:", "Artist:", "Album:", "Genre:", "tab/shift+tab", "Edit Metadata"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in default editor output; got:\n%s", want, out)
		}
	}
}

// TestRenameEditorEnterReturnsSave verifies that Enter triggers editorSave.
func TestRenameEditorEnterReturnsSave(t *testing.T) {
	ed := NewArtistRenameEditor([]string{"h1"}, "Alice")
	_, action, _ := ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != editorSave {
		t.Errorf("expected editorSave on Enter, got %v", action)
	}
}

// TestRenameEditorEscReturnsCancel verifies that Esc triggers editorCancel.
func TestRenameEditorEscReturnsCancel(t *testing.T) {
	ed := NewAlbumRenameEditor([]string{"h1"}, "Gamma")
	_, action, _ := ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != editorCancel {
		t.Errorf("expected editorCancel on Esc, got %v", action)
	}
}

// TestMetadataEditorRenderNarrowTerminalNoPanic verifies that rendering with a
// very narrow terminal (where the title is wider than boxInner) does not panic.
// Before the fix, titlePad could go negative, causing strings.Repeat to panic.
func TestMetadataEditorRenderNarrowTerminalNoPanic(t *testing.T) {
	// Create an editor with many tracks so the title " ✎ Edit Metadata (50 tracks) "
	// is wide enough to exceed boxInner on a tiny terminal.
	hashes := make([]string, 50)
	for i := range hashes {
		hashes[i] = "hash"
	}
	ed := NewBulkMetadataEditor(hashes, "T", "A", "B", "")
	s := newTestStyles()

	// Render at 20 columns wide — the title is wider than this.
	// This should not panic.
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
		// Title is focused by default.
		ed := NewMetadataEditor("h1", longTitle, "Artist", "Album", "Rock")
		s := newTestStyles()

		// Sub-case 1: cursor at end (default).
		out := ansi.Strip(ed.Render(termWidth, 24, s))
		for _, line := range strings.Split(out, "\n") {
			w := ansi.StringWidth(line)
			if w > termWidth {
				t.Errorf("cursor-at-end: line exceeds terminal width %d (got %d): %q", termWidth, w, line)
			}
		}

		// Sub-case 2: cursor moved to the middle of the text.
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
		ed := NewMetadataEditor("h1", longTitle, "Artist", "Album", "Rock")
		// Tab to Artist so Title is non-focused and will be wrapped.
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: '\t'})

		s := newTestStyles()
		out := ansi.Strip(ed.Render(termWidth, 24, s))

		outLines := strings.Split(out, "\n")

		// The full title text must be present across wrapped lines.
		flat := strings.Join(outLines, " ")
		for _, word := range strings.Fields(longTitle) {
			if !strings.Contains(flat, word) {
				t.Errorf("expected word %q from title in output; got:\n%s", word, out)
			}
		}

		// "Title:" label appears on exactly one line.
		titleLabelCount := 0
		for _, line := range outLines {
			if strings.Contains(line, "Title:") {
				titleLabelCount++
			}
		}
		if titleLabelCount != 1 {
			t.Errorf("expected Title: label on exactly 1 line, found %d", titleLabelCount)
		}

		// boxInner = min(88-8, 80) = 80; bordered lines are 82 chars wide.
		// With centering: leftPad = (88-82)/2 = 3, so total = 85.
		// No line should exceed termWidth.
		for _, line := range outLines {
			w := ansi.StringWidth(line)
			if w > termWidth {
				t.Errorf("non-focused: line exceeds terminal width %d (got %d): %q", termWidth, w, line)
			}
		}
	})
}

// TestDirtyFieldsTrimsWhitespace verifies that DirtyFields trims leading and
// trailing whitespace from modified field values.
func TestDirtyFieldsTrimsWhitespace(t *testing.T) {
	ed := NewMetadataEditor("h1", "Alice", "", "", "")

	// Type a trailing space — cursor starts at end, so this appends a space.
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: ' ', Text: " "})

	title, artist, album, _ := ed.DirtyFields()
	if title == nil {
		t.Fatal("expected title to be dirty after typing a space")
	}
	if *title != "Alice" {
		t.Errorf("expected trimmed title %q, got %q", "Alice", *title)
	}
	if artist != nil {
		t.Errorf("expected artist=nil (not dirty), got %q", *artist)
	}
	if album != nil {
		t.Errorf("expected album=nil (not dirty), got %q", *album)
	}
}
