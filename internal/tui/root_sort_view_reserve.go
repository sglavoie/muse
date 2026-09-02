package tui

import (
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func playlistsVisibleReserve(v pltui.PlaylistsView) int {
	reserve := 1
	if v.Level == pltui.ListLevel && v.SortState.Field != sorting.SortDefault {
		reserve++
	}
	if v.Level == pltui.TrackLevel {
		reserve = 4
	}
	return reserve
}

func drilldownVisibleReserve(isTopLevel, filterActive, hasSort bool) int {
	reserve := 3
	if isTopLevel {
		reserve = 0
		if filterActive {
			reserve = 1
		}
	}
	if hasSort {
		reserve++
	}
	return reserve
}

func artistsVisibleReserve(v browse.ArtistsView) int {
	switch v.Level {
	case arttui.AlbumLevel:
		return 2
	case arttui.TrackLevel:
		return 3
	default:
		return 1
	}
}

func albumsVisibleReserve(v browse.AlbumsView) int {
	if v.Level == albtui.TrackLevel {
		return 3
	}
	return 1
}

func favoritesVisibleReserve(v favtui.FavoritesView) int {
	var reserve int
	switch v.Level {
	case favtui.ListLevel:
		reserve = 2
		if v.Category == favtui.CategoryTracks {
			reserve++
		}
	case favtui.TrackLevel:
		reserve = 3
	}
	if v.SortState.Field != sorting.SortDefault {
		reserve++
	}
	return reserve
}

func genresVisibleReserve(v browse.GenresView) int {
	switch v.Level {
	case gentui.ArtistLevel:
		return 2
	case gentui.TrackLevel:
		return 3
	default:
		return 1
	}
}
