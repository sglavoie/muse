package tui

import (
	"testing"

	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestFavoritesVisibleReserve(t *testing.T) {
	tests := []struct {
		name string
		v    favtui.FavoritesView
		want int
	}{
		{"category level", favtui.FavoritesView{Level: favtui.CategoryLevel}, 1},
		{"list level artists no filter no sort", favtui.FavoritesView{Level: favtui.ListLevel, Category: favtui.CategoryArtists}, 2},
		{"list level artists with filter", favtui.FavoritesView{Level: favtui.ListLevel, Category: favtui.CategoryArtists, Filter: filter.State{Active: true}}, 2},
		{"list level artists with committed filter text", favtui.FavoritesView{Level: favtui.ListLevel, Category: favtui.CategoryArtists, Filter: filter.State{Text: "foo"}}, 2},
		{"list level artists with sort", favtui.FavoritesView{Level: favtui.ListLevel, Category: favtui.CategoryArtists, SortState: sorting.SortState{Field: sorting.SortName}}, 3},
		{"list level artists with filter and sort", favtui.FavoritesView{Level: favtui.ListLevel, Category: favtui.CategoryArtists, Filter: filter.State{Active: true}, SortState: sorting.SortState{Field: sorting.SortName}}, 3},
		{"list level tracks no filter no sort", favtui.FavoritesView{Level: favtui.ListLevel, Category: favtui.CategoryTracks}, 3},
		{"list level tracks with filter", favtui.FavoritesView{Level: favtui.ListLevel, Category: favtui.CategoryTracks, Filter: filter.State{Active: true}}, 3},
		{"list level tracks with sort", favtui.FavoritesView{Level: favtui.ListLevel, Category: favtui.CategoryTracks, SortState: sorting.SortState{Field: sorting.SortTitle}}, 4},
		{"track level no sort", favtui.FavoritesView{Level: favtui.TrackLevel}, 3},
		{"track level with sort", favtui.FavoritesView{Level: favtui.TrackLevel, SortState: sorting.SortState{Field: sorting.SortTitle}}, 4},
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
