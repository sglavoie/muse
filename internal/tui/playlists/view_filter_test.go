package playlists

import (
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
