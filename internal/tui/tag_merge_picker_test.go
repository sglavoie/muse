package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestTagMergePickerBackspaceMultibyte(t *testing.T) {
	src := db.TagRow{ID: 1, Name: "src"}
	all := []db.TagRow{
		src,
		{ID: 2, Name: "cafe"},
		{ID: 3, Name: "été"},
	}
	tp := NewTagMergePicker(src, all)

	// Type a multi-byte character.
	tp, _ = tp.HandleKey("é")
	if tp.filter != "é" {
		t.Fatalf("expected filter %q, got %q", "é", tp.filter)
	}

	// Backspace should remove the entire character, not just the last byte.
	tp, _ = tp.HandleKey("backspace")
	if tp.filter != "" {
		t.Errorf("expected empty filter after backspace, got %q", tp.filter)
	}

	// Type two multi-byte characters and backspace once.
	tp, _ = tp.HandleKey("é")
	tp, _ = tp.HandleKey("t")
	tp, _ = tp.HandleKey("backspace")
	if tp.filter != "é" {
		t.Errorf("expected filter %q after partial backspace, got %q", "é", tp.filter)
	}
}

func TestTagMergePickerRefilterClampsOnBackspace(t *testing.T) {
	src := db.TagRow{ID: 1, Name: "src"}
	all := []db.TagRow{
		src,
		{ID: 2, Name: "alpha"},
		{ID: 3, Name: "beta"},
		{ID: 4, Name: "gamma"},
	}
	tp := NewTagMergePicker(src, all)

	// Type a filter that matches only one destination.
	tp, _ = tp.HandleKey("g")
	tp, _ = tp.HandleKey("a")
	if len(tp.filtered) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(tp.filtered))
	}

	// Cursor should be clamped to 0.
	if tp.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", tp.cursor)
	}

	// Backspace widens the filter; cursor should remain valid.
	tp, _ = tp.HandleKey("backspace")
	if tp.cursor < 0 || tp.cursor >= len(tp.filtered) {
		t.Errorf("cursor %d out of bounds for %d filtered items", tp.cursor, len(tp.filtered))
	}
}
