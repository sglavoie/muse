package queue

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/filter"
)

func TestAlbumArtistFilterPreservesQueueEntryIdentity(t *testing.T) {
	v := View{Entries: []Entry{
		{ID: 1, Hash: "same", Artist: "Guest", AlbumArtist: "Björk Ensemble", Selected: true},
		{ID: 2, Hash: "other", Artist: "Björk Ensemble"},
		{ID: 3, Hash: "same", Artist: "Guest", AlbumArtist: "Björk Ensemble"},
	}, Filter: filter.State{Text: "aa:bjork ensemble a:guest"}}
	rows := v.FilteredEntries()
	if len(rows) != 2 || rows[0].ID != 1 || rows[1].ID != 3 || !rows[0].Selected {
		t.Fatalf("filter lost row identity or selection: %+v", rows)
	}
	v.Filter.Text = "albumartist:missing"
	if len(v.FilteredEntries()) != 0 {
		t.Fatal("nonmatching Album Artist kept rows")
	}
	v.Filter.Text = "aa:"
	if len(v.FilteredEntries()) != 3 {
		t.Fatal("empty prefix should keep every row")
	}
}
