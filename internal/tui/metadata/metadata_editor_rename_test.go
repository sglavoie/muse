package metadata

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestNewArtistRenameEditor verifies that a freshly constructed artist rename
// editor has the correct mode, focused field, initial artist value, and hashes.
func TestNewArtistRenameEditor(t *testing.T) {
	hashes := []string{"h1", "h2", "h3"}
	ed := NewArtistRenameEditor(hashes, "Alice")

	if ed.Mode() != ModeRenameArtist {
		t.Errorf("expected editorModeRenameArtist, got %v", ed.Mode())
	}
	if ed.focused != FieldArtist {
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

// TestNewAlbumRenameEditor verifies that a freshly constructed album rename
// editor has the correct mode, focused field, initial album value, and hashes.
func TestNewAlbumRenameEditor(t *testing.T) {
	hashes := []string{"h4", "h5"}
	ed := NewAlbumRenameEditor(hashes, "Gamma")

	if ed.Mode() != ModeRenameAlbum {
		t.Errorf("expected editorModeRenameAlbum, got %v", ed.Mode())
	}
	if ed.focused != FieldAlbum {
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

// TestRenameEditorDirtyFields verifies that modifying only the artist field
// marks it dirty while title and album remain nil.
func TestRenameEditorDirtyFields(t *testing.T) {
	ed := NewArtistRenameEditor([]string{"h1", "h2"}, "Alice")

	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})

	title, artist, album, _, _ := ed.DirtyFields()
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

	title, artist, album, _, _ := ed.DirtyFields()
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

// TestRenameEditorHandleKeyTabNoOp verifies that tab and shift+tab do not
// change the focused field when the editor is in rename mode.
func TestRenameEditorHandleKeyTabNoOp(t *testing.T) {
	ed := NewArtistRenameEditor([]string{"h1"}, "Alice")
	initialFocused := ed.focused

	edAfterTab, action, _ := ed.HandleKey(tea.KeyPressMsg{Code: '\t'})
	if edAfterTab.focused != initialFocused {
		t.Errorf("tab should not change focused in rename mode: was %v, got %v", initialFocused, edAfterTab.focused)
	}
	if action != ActionNone {
		t.Errorf("expected editorNone after tab, got %v", action)
	}

	edAfterShiftTab, action2, _ := ed.HandleKey(tea.KeyPressMsg{Code: '\t', Mod: tea.ModShift})
	if edAfterShiftTab.focused != initialFocused {
		t.Errorf("shift+tab should not change focused in rename mode: was %v, got %v", initialFocused, edAfterShiftTab.focused)
	}
	if action2 != ActionNone {
		t.Errorf("expected editorNone after shift+tab, got %v", action2)
	}
}

// TestRenameEditorEnterReturnsSave verifies that Enter triggers editorSave.
func TestRenameEditorEnterReturnsSave(t *testing.T) {
	ed := NewArtistRenameEditor([]string{"h1"}, "Alice")
	_, action, _ := ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != ActionSave {
		t.Errorf("expected editorSave on Enter, got %v", action)
	}
}

// TestRenameEditorEscReturnsCancel verifies that Esc triggers editorCancel.
func TestRenameEditorEscReturnsCancel(t *testing.T) {
	ed := NewAlbumRenameEditor([]string{"h1"}, "Gamma")
	_, action, _ := ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != ActionCancel {
		t.Errorf("expected editorCancel on Esc, got %v", action)
	}
}

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
	if strings.Contains(out, "tab/shift+tab") {
		t.Errorf("expected no tab hint in rename mode; got:\n%s", out)
	}
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

// TestRenameEditorNoDisplayDuration verifies that rename editors (artist/album)
// do not render the Duration field.
func TestRenameEditorNoDisplayDuration(t *testing.T) {
	s := newTestStyles()

	artist := NewArtistRenameEditor([]string{"h1"}, "Alice")
	out := ansi.Strip(artist.Render(80, 24, s))
	if strings.Contains(out, "Duration:") {
		t.Errorf("artist rename editor should not show Duration field; got:\n%s", out)
	}

	album := NewAlbumRenameEditor([]string{"h1"}, "Alpha")
	out = ansi.Strip(album.Render(80, 24, s))
	if strings.Contains(out, "Duration:") {
		t.Errorf("album rename editor should not show Duration field; got:\n%s", out)
	}
}
