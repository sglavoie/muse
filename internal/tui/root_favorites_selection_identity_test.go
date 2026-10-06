package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestFavoritesHiddenSelectionTargets(t *testing.T) {
	for _, level := range []favtui.Level{favtui.ListLevel, favtui.TrackLevel} {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			tracks := []library.Track{
				{Hash: "a", Title: "Alpha", Artist: "A"},
				{Hash: "b", Title: "Beta", Artist: "B"},
				{Hash: "c", Title: "Charlie", Artist: "C"},
			}
			m := New(context.Background(), WithLibraryTracks(tracks), WithFavoriteWriter(&stubFavoriteWriter{}))
			m.ActiveView = viewFavorites
			m.favorites.Level, m.favorites.Category = level, favtui.CategoryTracks
			m.favorites.Tracks, m.favorites.DrilldownTracks = tracks, tracks
			m.favorites = m.favorites.ToggleSelection(0).ToggleSelection(2)
			m = pressEnter(typeText(pressSlash(m), "Beta"))
			m.favorites.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
			m.favorites = m.favorites.RecomputeFavoritesSort()
			want := []string{"c", "a"}
			for name, got := range map[string][]string{
				"metadata": m.bulkSelectedHashes(), "tags": m.selectedTrackHashes(),
			} {
				if !slices.Equal(got, want) {
					t.Fatalf("%s targets %v, want %v", name, got, want)
				}
			}
			for _, tracks := range [][]library.Track{m.bulkFavoriteTracks(), m.favorites.SelectedTracks()} {
				if len(tracks) != 2 || tracks[0].Hash != "c" || tracks[1].Hash != "a" {
					t.Fatalf("queue targets: %v", tracks)
				}
			}
			exported, _, ok := m.favoritesVisibleTracks()
			if !ok || len(exported) != 2 || exported[0].Hash != "c" || exported[1].Hash != "a" {
				t.Fatalf("export targets: %v", exported)
			}
			for _, width := range []int{60, 100} {
				m.favorites.SetSize(width, 12)
				screen := ansi.Strip(m.favorites.Render(*m.styles, nil, nil, nil, "", ""))
				if !strings.Contains(screen, "2 selected · 2 hidden") {
					t.Fatalf("missing hidden selection: %s", screen)
				}
				for _, line := range strings.Split(screen, "\n") {
					if ansi.StringWidth(line) > width {
						t.Fatalf("overflow: %q", line)
					}
				}
			}
			// Visual selection adds the visible row while retaining hidden selections.
			m = enterVisualMode(m)
			m = doNav(m, keys.ActionCursorDown)
			if len(m.favorites.SelectedItems) != 3 {
				t.Fatal(m.favorites.SelectedItems)
			}
			m = exitVisualMode(m)
			m.favorites = m.favorites.ToggleSelection(0)
			_, cmd, handled := m.handleFavoritesBulkAction()
			if !handled || cmd == nil {
				t.Fatal("missing bulk remove")
			}
			msg := cmd().(favoriteToggledMsg)
			if !slices.Equal(msg.removedValues, want) || !slices.Equal(msg.removedIndices, []int{0, 2}) {
				t.Fatalf("remove targets: %v at %v", msg.removedValues, msg.removedIndices)
			}
		})
	}
}

func TestFavoritesHiddenCollectionRemovalAfterSort(t *testing.T) {
	for _, category := range []favtui.Category{favtui.CategoryArtists, favtui.CategoryAlbums} {
		m := New(context.Background(), WithFavoriteWriter(&stubFavoriteWriter{}))
		m.ActiveView = viewFavorites
		m.favorites.Level, m.favorites.Category = favtui.ListLevel, category
		m.favorites.Artists = []string{"Alpha", "Beta", "Charlie"}
		m.favorites.Albums = []string{"Alpha", "Beta", "Charlie"}
		m.favorites = m.favorites.ToggleSelection(0).ToggleSelection(2)
		m = pressEnter(typeText(pressSlash(m), "Beta"))
		m.favorites.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
		m.favorites = m.favorites.RecomputeFavoritesSort()
		_, cmd, _ := m.handleFavoritesBulkAction()
		if cmd == nil {
			t.Fatal("missing bulk remove")
		}
		msg := cmd().(favoriteToggledMsg)
		if !slices.Equal(msg.removedValues, []string{"Charlie", "Alpha"}) || !slices.Equal(msg.removedIndices, []int{0, 2}) {
			t.Fatalf("collection remove: %v at %v", msg.removedValues, msg.removedIndices)
		}
	}
}

func TestPlaylistGenreFilterRefreshesAfterMetadataChanges(t *testing.T) {
	m := newRichFixture()
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Hash: "rh1", Title: "Track A1", Genre: "Ambient"},
		{ID: 2, Hash: "rh1", Title: "Track A1", Genre: "Ambient"},
	})
	m.playlists.Filter.Text = "g:jazz"
	if len(m.playlists.DisplayTracks()) != 0 {
		t.Fatal("initial genre matched")
	}
	m.library.Index.UpdateMetadata("rh1", "Track A1", "Alice", "Alpha", "Jazz")
	m = m.refreshDerivedViews()
	rows := m.playlists.DisplayTracks()
	if len(rows) != 2 || rows[0].ID != 1 || rows[1].ID != 2 {
		t.Fatal(rows)
	}
	m.library.Index.UpdateMetadata("rh1", "Track A1", "Alice", "Alpha", "Ambient")
	m = m.refreshDerivedViews()
	if len(m.playlists.DisplayTracks()) != 0 {
		t.Fatal("restored genre still matched")
	}
}
