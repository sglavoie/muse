package sorting

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestSortPlaylists(t *testing.T) {
	playlists := []db.PlaylistRow{
		{ID: 1, Name: "Rock", TrackCount: 10},
		{ID: 2, Name: "Alpha", TrackCount: 5},
		{ID: 3, Name: "Jazz", TrackCount: 20},
	}

	t.Run("sort by name asc", func(t *testing.T) {
		sorted := SortPlaylists(playlists, SortName, SortAsc)
		if len(sorted) != 3 {
			t.Fatalf("expected 3 playlists, got %d", len(sorted))
		}
		if sorted[0].Name != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0].Name)
		}
		if sorted[1].Name != "Jazz" {
			t.Errorf("second should be Jazz, got %q", sorted[1].Name)
		}
		if sorted[2].Name != "Rock" {
			t.Errorf("third should be Rock, got %q", sorted[2].Name)
		}
	})

	t.Run("sort by name desc", func(t *testing.T) {
		sorted := SortPlaylists(playlists, SortName, SortDesc)
		if sorted[0].Name != "Rock" {
			t.Errorf("first should be Rock, got %q", sorted[0].Name)
		}
		if sorted[2].Name != "Alpha" {
			t.Errorf("last should be Alpha, got %q", sorted[2].Name)
		}
	})

	t.Run("sort by track count asc", func(t *testing.T) {
		sorted := SortPlaylists(playlists, SortTrackCount, SortAsc)
		if sorted[0].TrackCount != 5 {
			t.Errorf("first should have 5 tracks, got %d", sorted[0].TrackCount)
		}
		if sorted[2].TrackCount != 20 {
			t.Errorf("last should have 20 tracks, got %d", sorted[2].TrackCount)
		}
	})

	t.Run("sort by track count desc", func(t *testing.T) {
		sorted := SortPlaylists(playlists, SortTrackCount, SortDesc)
		if sorted[0].TrackCount != 20 {
			t.Errorf("first should have 20 tracks, got %d", sorted[0].TrackCount)
		}
		if sorted[2].TrackCount != 5 {
			t.Errorf("last should have 5 tracks, got %d", sorted[2].TrackCount)
		}
	})

	t.Run("default returns nil", func(t *testing.T) {
		sorted := SortPlaylists(playlists, SortDefault, SortAsc)
		if sorted != nil {
			t.Error("SortDefault should return nil")
		}
	})

	t.Run("original not modified", func(t *testing.T) {
		_ = SortPlaylists(playlists, SortName, SortAsc)
		if playlists[0].Name != "Rock" {
			t.Error("original slice was modified")
		}
	})
}

func TestSortPlaylistTrackIndices(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 1, Title: "Zulu", Artist: "Charlie", Album: "Gamma", DurationMs: 3000},
		{ID: 2, Title: "Alpha", Artist: "Bravo", Album: "Beta", DurationMs: 1000},
		{ID: 3, Title: "Mike", Artist: "Alpha", Album: "Alpha", DurationMs: 2000},
	}

	t.Run("sort by title asc", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortTitle, SortAsc)
		if order == nil {
			t.Fatal("expected non-nil order")
		}
		if tracks[order[0]].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", tracks[order[0]].Title)
		}
		if tracks[order[1]].Title != "Mike" {
			t.Errorf("second should be Mike, got %q", tracks[order[1]].Title)
		}
		if tracks[order[2]].Title != "Zulu" {
			t.Errorf("third should be Zulu, got %q", tracks[order[2]].Title)
		}
	})

	t.Run("sort by artist desc", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortArtist, SortDesc)
		if tracks[order[0]].Artist != "Charlie" {
			t.Errorf("first should be Charlie, got %q", tracks[order[0]].Artist)
		}
		if tracks[order[2]].Artist != "Alpha" {
			t.Errorf("last should be Alpha, got %q", tracks[order[2]].Artist)
		}
	})

	t.Run("sort by duration desc", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortDuration, SortDesc)
		if tracks[order[0]].DurationMs != 3000 {
			t.Errorf("first should have 3000ms, got %d", tracks[order[0]].DurationMs)
		}
		if tracks[order[2]].DurationMs != 1000 {
			t.Errorf("last should have 1000ms, got %d", tracks[order[2]].DurationMs)
		}
	})

	t.Run("sort by album asc", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortAlbum, SortAsc)
		if tracks[order[0]].Album != "Alpha" {
			t.Errorf("first should be Alpha, got %q", tracks[order[0]].Album)
		}
		if tracks[order[1]].Album != "Beta" {
			t.Errorf("second should be Beta, got %q", tracks[order[1]].Album)
		}
		if tracks[order[2]].Album != "Gamma" {
			t.Errorf("third should be Gamma, got %q", tracks[order[2]].Album)
		}
	})

	t.Run("default returns nil", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortDefault, SortAsc)
		if order != nil {
			t.Error("SortDefault should return nil")
		}
	})
}
