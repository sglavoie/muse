package tui

import (
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

func libraryViewVisibleReserve(v LibraryView) int {
	if v.width == 0 {
		if v.filter.Active || v.filter.Text != "" {
			return 1
		}
		return 0
	}
	return 2
}

func playlistsVisibleReserve(v PlaylistsView) int {
	reserve := 1
	if v.level == pltui.ListLevel && v.sortState.Field != SortDefault {
		reserve++
	}
	if v.level == pltui.TrackLevel {
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

func artistsVisibleReserve(v ArtistsView) int {
	switch v.level {
	case arttui.AlbumLevel:
		return 2
	case arttui.TrackLevel:
		return 3
	default:
		return 1
	}
}

func albumsVisibleReserve(v AlbumsView) int {
	if v.level == albtui.TrackLevel {
		return 3
	}
	return 1
}

func favoritesVisibleReserve(v FavoritesView) int {
	var reserve int
	switch v.level {
	case favtui.ListLevel:
		reserve = 2
		if v.category == favtui.CategoryTracks {
			reserve++
		}
	case favtui.TrackLevel:
		reserve = 3
	}
	if v.sortState.Field != SortDefault {
		reserve++
	}
	return reserve
}

func genresVisibleReserve(v GenresView) int {
	switch v.level {
	case gentui.ArtistLevel:
		return 2
	case gentui.TrackLevel:
		return 3
	default:
		return 1
	}
}
