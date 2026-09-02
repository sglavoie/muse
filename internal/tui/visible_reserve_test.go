package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestLibraryViewVisibleReserve(t *testing.T) {
	tests := []struct {
		name string
		view libtui.View
		want int
	}{
		{
			name: "normal width no filter",
			view: libtui.View{Width: 80},
			want: 2,
		},
		{
			name: "normal width filter active",
			view: libtui.View{Width: 80, Filter: filter.State{Active: true}},
			want: 2,
		},
		{
			name: "normal width filter text set but not active",
			view: libtui.View{Width: 80, Filter: filter.State{Text: "foo"}},
			want: 2,
		},
		{
			name: "normal width filter active with text",
			view: libtui.View{Width: 80, Filter: filter.State{Active: true, Text: "bar"}},
			want: 2,
		},
		{
			name: "zero width no filter",
			view: libtui.View{},
			want: 0,
		},
		{
			name: "zero width filter active",
			view: libtui.View{Filter: filter.State{Active: true}},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := libtui.VisibleReserve(tt.view)
			if got != tt.want {
				t.Errorf("libraryViewVisibleReserve() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPlaylistsVisibleReserve(t *testing.T) {
	tests := []struct {
		name string
		view PlaylistsView
		want int
	}{
		{
			name: "list level no sort",
			view: PlaylistsView{level: pltui.ListLevel, sortState: sorting.SortState{Field: sorting.SortDefault}},
			want: 1,
		},
		{
			name: "list level with sort",
			view: PlaylistsView{level: pltui.ListLevel, sortState: sorting.SortState{Field: sorting.SortName}},
			want: 2,
		},
		{
			name: "track level",
			view: PlaylistsView{level: pltui.TrackLevel},
			want: 4,
		},
		{
			name: "track level ignores sort state",
			view: PlaylistsView{level: pltui.TrackLevel, sortState: sorting.SortState{Field: sorting.SortArtist}},
			want: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := playlistsVisibleReserve(tt.view)
			if got != tt.want {
				t.Errorf("playlistsVisibleReserve() = %d, want %d", got, tt.want)
			}
		})
	}
}
