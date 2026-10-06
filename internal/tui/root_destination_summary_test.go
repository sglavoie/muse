package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestDestinationSummaryUsesCapturedTracksIncludingHiddenSelection(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "a", Title: "Alpha", Duration: 90.5})
	m.library.Index.Insert(library.Track{Hash: "b", Title: "Beta", Duration: 30.5})
	m.library.Index.Insert(library.Track{Hash: "c", Title: "Other", Duration: 999})
	m.library.SelectedHashes = map[string]struct{}{"a": {}, "b": {}}
	m.library.Filter.Text = "Alpha"
	next, _, handled := m.handleLibraryBulkAction(keys.ActionBulkAction)
	m = next.(Model)
	if !handled || m.overlay.destPicker == nil {
		t.Fatal("picker not opened")
	}
	if len(m.overlay.destPickerTracks) != 2 {
		t.Fatal("hidden selection lost")
	}
	out := ansi.Strip(m.overlay.destPicker.Render(60, 15, styles.NewStyles(true)))
	if !strings.Contains(out, "Add 2 tracks · 2:01") {
		t.Fatalf("summary: %s", out)
	}
}
