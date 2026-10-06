package metadata

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestDefaultEditorHandleKeyTabCycles verifies that tab does cycle fields in
// the default (non-rename) mode, to avoid regressions.
func TestDefaultEditorHandleKeyTabCycles(t *testing.T) {
	ed := NewEditor("h1", "Title", "Artist", "Album", "", 0)
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

// TestDefaultEditorTabCyclesSixFields verifies that tab cycles through all
// six fields including Duration and Album Artist.
func TestDefaultEditorTabCyclesSixFields(t *testing.T) {
	ed := NewEditor("h1", "T", "A", "B", "G", 225)
	if ed.focused != fieldTitle {
		t.Fatalf("expected initial focus=fieldTitle, got %v", ed.focused)
	}
	expectedCycle := []Field{fieldArtist, fieldAlbum, fieldGenre, FieldDuration, FieldAlbumArtist, fieldTitle}
	for i, want := range expectedCycle {
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: '\t'})
		if ed.focused != want {
			t.Errorf("tab %d: expected %v, got %v", i+1, want, ed.focused)
		}
	}
}

// TestDurationDirtyFieldReturnsPointer verifies that editing the Duration
// field marks it dirty and DirtyFields returns a non-nil pointer.
func TestDurationDirtyFieldReturnsPointer(t *testing.T) {
	ed := NewEditor("h1", "T", "A", "B", "G", 0)

	for range 4 {
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: '\t'})
	}
	if ed.focused != FieldDuration {
		t.Fatalf("expected focus on fieldDuration, got %v", ed.focused)
	}

	for _, ch := range "3:45" {
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}

	_, _, _, _, dur := ed.DirtyFields()
	if dur == nil {
		t.Fatal("expected duration to be dirty, got nil")
	}
	if *dur != "3:45" {
		t.Errorf("expected dirty duration %q, got %q", "3:45", *dur)
	}
}

// TestDirtyFieldsTrimsWhitespace verifies that DirtyFields trims leading and
// trailing whitespace from modified field values.
func TestDirtyFieldsTrimsWhitespace(t *testing.T) {
	ed := NewEditor("h1", "Alice", "", "", "", 0)

	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: ' ', Text: " "})

	title, artist, album, _, _ := ed.DirtyFields()
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
