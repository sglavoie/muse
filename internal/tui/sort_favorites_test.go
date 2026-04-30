package tui

import (
	"testing"

	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
)

func TestFavoritesVisibleReserve(t *testing.T) {
	tests := []struct {
		name string
		v    FavoritesView
		want int
	}{
		{"category level", FavoritesView{level: favtui.CategoryLevel}, 0},
		{"list level artists no filter no sort", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists}, 2},
		{"list level artists with filter", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, filter: filter.State{Active: true}}, 2},
		{"list level artists with committed filter text", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, filter: filter.State{Text: "foo"}}, 2},
		{"list level artists with sort", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, sortState: SortState{Field: SortName}}, 3},
		{"list level artists with filter and sort", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, filter: filter.State{Active: true}, sortState: SortState{Field: SortName}}, 3},
		{"list level tracks no filter no sort", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryTracks}, 3},
		{"list level tracks with filter", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryTracks, filter: filter.State{Active: true}}, 3},
		{"list level tracks with sort", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryTracks, sortState: SortState{Field: SortTitle}}, 4},
		{"track level no sort", FavoritesView{level: favtui.TrackLevel}, 3},
		{"track level with sort", FavoritesView{level: favtui.TrackLevel, sortState: SortState{Field: SortTitle}}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := favoritesVisibleReserve(tt.v)
			if got != tt.want {
				t.Errorf("favoritesVisibleReserve = %d, want %d", got, tt.want)
			}
		})
	}
}
