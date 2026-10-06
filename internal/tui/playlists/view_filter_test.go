package playlists

import (
	"slices"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestFilteredSortedTracksKeepRowIdentity(t *testing.T) {
	v := PlaylistsView{Level: TrackLevel, Filter: filter.State{Text: "ALICE"},
		Tracks: []db.PlaylistTrackRow{
			{ID: 11, Hash: "a", Title: "Zulu", Artist: "Alice"},
			{ID: 12, Hash: "b", Title: "Beta", Artist: "Bob"},
			{ID: 13, Hash: "a", Title: "Zulu", Artist: "Alice"},
			{ID: 14, Hash: "c", Title: "Alpha", Artist: "Alice"},
		}, SortState: sorting.SortState{Field: sorting.SortTitle}}
	v = v.RecomputePlaylistSort()
	rows := v.DisplayTracks()
	if len(rows) != 3 || rows[0].ID != 14 || rows[1].ID != 11 || rows[2].ID != 13 {
		t.Fatal(rows)
	}
	v = v.HandleAction(keys.ActionGoToBottom)
	v = v.HandleAction(keys.ActionBulkSelectionToggle)
	if _, ok := v.SelectedTrackIDs[13]; !ok || len(v.SelectedTrackIDs) != 1 {
		t.Fatal(v.SelectedTrackIDs)
	}
	if v.TrackIndex(v.Cursor) != 2 {
		t.Fatal(v.Cursor, v.TrackIndex(v.Cursor))
	}
	v.Filter.Text = "no match"
	if _, ok := v.FocusedTrack(); ok {
		t.Fatal("no-match filter has a focused track")
	}
	v = v.HandleAction(keys.ActionBulkSelectionToggle)
	if len(v.SelectedTrackIDs) != 1 {
		t.Fatal("no-match toggle changed selection")
	}
}

func TestPlaylistFilterDrilldownRestoresList(t *testing.T) {
	v := PlaylistsView{Playlists: []db.PlaylistRow{{ID: 1, Name: "Rock"}, {ID: 2, Name: "Jazz"}, {ID: 3, Name: "Jazz piano"}},
		Filter: filter.State{Text: "jazz"}, Cursor: 1}
	v = v.OpenTracks([]db.PlaylistTrackRow{{ID: 10, Title: "Tune"}})
	if v.ListCursor != 2 || v.Filter.Text != "" || v.sourcePlaylistName() != "Jazz piano" {
		t.Fatal(v)
	}
	v.Filter.Text = "tune"
	v = v.BackToList()
	row, ok := v.FocusedPlaylist()
	if !ok || row.ID != 3 || v.Filter.Text != "jazz" {
		t.Fatal(row, v.Filter.Text)
	}
}

func TestPlaylistFieldFiltersPreserveDuplicateRows(t *testing.T) {
	v := PlaylistsView{Level: TrackLevel, Tracks: []db.PlaylistTrackRow{
		{ID: 1, Hash: "a", Title: "Live One", Artist: "Alice", Album: "Alpha", AlbumArtist: "Various Artists", Genre: "Ambient"},
		{ID: 2, Hash: "b", Title: "Two", Artist: "Bob", Album: "Beta", Genre: "Jazz"},
		{ID: 3, Hash: "a", Title: "Live One", Artist: "Alice", Album: "Alpha", AlbumArtist: "Various Artists", Genre: "Ambient"},
	}}
	for _, tc := range []struct {
		query string
		ids   []int64
	}{
		{"aa:various artists", []int64{1, 3}},
		{"albumartist:various a:alice", []int64{1, 3}},
		{"aa:missing", nil},
		{"a:ALICE", []int64{1, 3}}, {"artist:Bob", []int64{2}},
		{"live a:alice l:alpha g:ambient", []int64{1, 3}},
		{"title:two genre:jazz", []int64{2}}, {"album:beta", []int64{2}},
		{"g:", []int64{1, 2, 3}}, {"a:alice g:jazz", nil},
		{"Beta", []int64{2}}, {"ambient", nil},
	} {
		v.Filter.Text = tc.query
		var ids []int64
		for _, row := range v.DisplayTracks() {
			ids = append(ids, row.ID)
		}
		if !slices.Equal(ids, tc.ids) {
			t.Errorf("%q: %v, want %v", tc.query, ids, tc.ids)
		}
	}
	v.Level = ListLevel
	v.Playlists = []db.PlaylistRow{{Name: "a:Alice"}, {Name: "Alice"}}
	v.Filter.Text = "a:Alice"
	if rows := v.EffectivePlaylists(); len(rows) != 1 || rows[0].Name != "a:Alice" {
		t.Fatal(rows)
	}
}
