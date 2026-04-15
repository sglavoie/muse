package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestTagFilterPickerMultibyteFilter(t *testing.T) {
	tags := []db.TagRow{
		{ID: 1, Name: "café"},
		{ID: 2, Name: "rock"},
	}
	tp := NewTagFilterPicker(tags)

	// Type a multi-byte character; should be accepted.
	tp, _ = tp.HandleKey("é")
	if tp.filter != "é" {
		t.Fatalf("expected filter %q, got %q", "é", tp.filter)
	}
	if len(tp.filtered) != 1 {
		t.Fatalf("expected 1 filtered tag, got %d", len(tp.filtered))
	}

	// Backspace should remove the full character.
	tp, _ = tp.HandleKey("backspace")
	if tp.filter != "" {
		t.Errorf("expected empty filter after backspace, got %q", tp.filter)
	}
	if len(tp.filtered) != 2 {
		t.Errorf("expected all tags after clearing filter, got %d", len(tp.filtered))
	}
}

func TestTagFilterPickerBackspaceOnASCII(t *testing.T) {
	tags := []db.TagRow{
		{ID: 1, Name: "alpha"},
		{ID: 2, Name: "beta"},
	}
	tp := NewTagFilterPicker(tags)

	tp, _ = tp.HandleKey("a")
	tp, _ = tp.HandleKey("l")
	if tp.filter != "al" {
		t.Fatalf("expected filter %q, got %q", "al", tp.filter)
	}

	tp, _ = tp.HandleKey("backspace")
	if tp.filter != "a" {
		t.Errorf("expected filter %q after backspace, got %q", "a", tp.filter)
	}
}
