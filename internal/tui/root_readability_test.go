package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestCompactTrackTablesReserveReadableIdentity(t *testing.T) {
	for name, columns := range map[string]func(int) libtui.Columns{
		"library": libtui.Compute, "recent": libtui.ComputeWithDateAdded, "history": libtui.ComputeWithPlayedAt,
	} {
		c := columns(60)
		if c.Title < 20 || c.Artist < 12 || c.Album != 0 {
			t.Fatalf("%s loses song identity: %+v", name, c)
		}
	}
	p, f, tg := pltui.ComputeTrack(60), favtui.ComputeTrack(60), tagtui.ComputeTrack(60)
	for _, c := range [][3]int{{p.Title, p.Artist, p.Album}, {f.Title, f.Artist, f.Album}, {tg.Title, tg.Artist, tg.Album}} {
		if c[0] < 20 || c[1] < 12 || c[2] != 0 {
			t.Fatalf("compact collection loses song identity: %v", c)
		}
	}
	if c := libtui.ComputeWithPlayedAt(60); c.DateAdded != 10 {
		t.Fatal("compact history must retain the date without time")
	}
	if c := libtui.ComputeWithPlayedAt(120); c.Album == 0 || c.DateAdded != 16 {
		t.Fatal("wide history must restore full detail")
	}
}

func TestEmptyFooterKeepsFilterRecoveryAndHiddenSelection(t *testing.T) {
	m := newRichFixture()
	m.termWidth = 100
	m.showFooterHints = true
	m.ActiveView = viewQueue
	m.queue.Filter.Text = "zzzz-no-such-song"
	out := ansi.Strip(m.renderFooterHint())
	if !strings.Contains(out, "esc clear/back") || strings.Contains(out, "d remove") {
		t.Fatal(out)
	}
	m.queue.Entries[0].Selected = true
	out = ansi.Strip(m.renderFooterHint())
	if !strings.Contains(out, "X clear") {
		t.Fatalf("hidden selection actions were lost: %s", out)
	}
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = nil
	out = ansi.Strip(m.renderFooterHint())
	if !strings.Contains(out, "esc back") || strings.Contains(out, "c create") {
		t.Fatal(out)
	}
}
