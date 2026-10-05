package favorites

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestFavoriteTargetPickerSingleRowsAndHotkeys(t *testing.T) {
	p := NewTargetPicker(
		[]library.Track{{Hash: "h1", Title: "Blue in Green", Artist: "Miles Davis", Album: "Kind of Blue"}},
		map[string]struct{}{},
		map[string]struct{}{"Miles Davis": {}},
		map[string]struct{}{},
		false,
		PickerOrigin{},
	)

	if got := p.rowText(p.rows[0]); !strings.Contains(got, "Blue in Green · none · Add") {
		t.Fatalf("title row = %q", got)
	}
	if got := p.rowText(p.rows[1]); !strings.Contains(got, "Miles Davis · all · Remove") {
		t.Fatalf("artist row = %q", got)
	}

	_, cmd := p.HandleKey("r")
	if cmd == nil {
		t.Fatal("artist hotkey did not select")
	}
	msg := cmd().(TargetPickedMsg)
	if msg.Typ != "artist" || msg.Add || len(msg.Values) != 1 || msg.Values[0] != "Miles Davis" {
		t.Fatalf("artist selection = %+v", msg)
	}
}

func TestFavoriteTargetPickerBulkCountsAndNormalize(t *testing.T) {
	p := NewTargetPicker(
		[]library.Track{
			{Hash: "h1", Title: "One", Artist: "A", Album: "Shared",
				AlbumArtist: "Various Artists"},
			{Hash: "h2", Title: "Two", Artist: "B", Album: "Shared",
				AlbumArtist: "Various Artists"},
			{Hash: "h3", Title: "Three", Artist: "B", Album: "Shared",
				AlbumArtist: "Various Artists"},
		},
		map[string]struct{}{"h1": {}},
		map[string]struct{}{"A": {}, "B": {}},
		map[string]struct{}{library.AlbumKey(library.Track{Album: "Shared",
			AlbumArtist: "Various Artists"}): {}},
		true,
		PickerOrigin{},
	)

	if got := p.rowText(p.rows[0]); !strings.Contains(got, "3 tracks · mixed · Add") {
		t.Fatalf("bulk title row = %q", got)
	}
	if got := p.rowText(p.rows[1]); !strings.Contains(got, "2 artists · all · Remove") {
		t.Fatalf("bulk artist row = %q", got)
	}
	if got := p.rowText(p.rows[2]); !strings.Contains(got, "1 album · all · Remove") {
		t.Fatalf("bulk album row = %q", got)
	}

	_, cmd := p.HandleKey("t")
	msg := cmd().(TargetPickedMsg)
	if !msg.Add || strings.Join(msg.Values, ",") != "h2,h3" {
		t.Fatalf("mixed title changes = %+v, want add [h2 h3]", msg)
	}
}

func TestFavoriteTargetPickerDisablesMissingMetadataAndSkipsRows(t *testing.T) {
	p := NewTargetPicker(
		[]library.Track{
			{Hash: "h1", Title: "One", Artist: "Unknown Artist", Album: "A"},
			{Hash: "h2", Title: "Two", Artist: "Known", Album: ""},
		},
		map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, true,
		PickerOrigin{},
	)
	if p.rows[1].enabled || p.rows[2].enabled {
		t.Fatalf("missing metadata rows enabled: artist=%v album=%v", p.rows[1].enabled, p.rows[2].enabled)
	}
	if moved := p.move(1); moved.cursor != 0 {
		t.Fatalf("cursor moved onto disabled row: %d", moved.cursor)
	}
	if _, cmd := p.HandleKey("r"); cmd != nil {
		t.Fatal("disabled artist hotkey selected a target")
	}
}
