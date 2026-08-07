package tui

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestFavoriteTargetPickerSingleRowsAndHotkeys(t *testing.T) {
	p := newFavoriteTargetPicker(
		[]library.Track{{Hash: "h1", Title: "Blue in Green", Artist: "Miles Davis", Album: "Kind of Blue"}},
		map[string]struct{}{},
		map[string]struct{}{"Miles Davis": {}},
		map[string]struct{}{},
		false,
		favoritePickerOrigin{},
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
	msg := cmd().(favoriteTargetPickedMsg)
	if msg.typ != "artist" || msg.add || len(msg.values) != 1 || msg.values[0] != "Miles Davis" {
		t.Fatalf("artist selection = %+v", msg)
	}
}

func TestFavoriteTargetPickerBulkCountsAndNormalize(t *testing.T) {
	p := newFavoriteTargetPicker(
		[]library.Track{
			{Hash: "h1", Title: "One", Artist: "A", Album: "Shared"},
			{Hash: "h2", Title: "Two", Artist: "B", Album: "Shared"},
			{Hash: "h3", Title: "Three", Artist: "B", Album: "Shared"},
		},
		map[string]struct{}{"h1": {}},
		map[string]struct{}{"A": {}, "B": {}},
		map[string]struct{}{"Shared": {}},
		true,
		favoritePickerOrigin{},
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
	msg := cmd().(favoriteTargetPickedMsg)
	if !msg.add || strings.Join(msg.values, ",") != "h2,h3" {
		t.Fatalf("mixed title changes = %+v, want add [h2 h3]", msg)
	}
}

func TestFavoriteTargetPickerDisablesMissingMetadataAndSkipsRows(t *testing.T) {
	p := newFavoriteTargetPicker(
		[]library.Track{
			{Hash: "h1", Title: "One", Artist: "Unknown Artist", Album: "A"},
			{Hash: "h2", Title: "Two", Artist: "Known", Album: ""},
		},
		map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, true,
		favoritePickerOrigin{},
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

func TestFavoriteTargetPickerCancelAndRenderNarrow(t *testing.T) {
	p := newFavoriteTargetPicker(
		[]library.Track{{Hash: "h1", Title: strings.Repeat("界", 40), Artist: "A", Album: "B"}},
		map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, false,
		favoritePickerOrigin{},
	)
	for _, key := range []string{"esc", "F"} {
		_, cmd := p.HandleKey(key)
		if _, ok := cmd().(favoriteTargetDismissedMsg); !ok {
			t.Fatalf("%s did not dismiss picker", key)
		}
	}
	raw := p.Render(minTerminalWidth, minTerminalHeight, styles.NewStyles(true))
	if issues := DetectLayoutIssues(raw, PlainScreen(raw), minTerminalWidth, minTerminalHeight); len(issues) != 0 {
		t.Fatalf("favorite target picker should fit at minimum supported size, got %+v:\n%s", issues, PlainScreen(raw))
	}
}
