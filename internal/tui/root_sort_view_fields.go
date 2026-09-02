package tui

import (
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/sorting"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func artistsSortFieldsForLevel(level arttui.Level) []sorting.SortField {
	switch level {
	case arttui.ArtistLevel:
		return sorting.ArtistArtistSortFields
	case arttui.AlbumLevel:
		return sorting.ArtistAlbumSortFields
	case arttui.TrackLevel:
		return sorting.ArtistTrackSortFields
	default:
		return sorting.ArtistArtistSortFields
	}
}

func albumsSortFieldsForLevel(level albtui.Level) []sorting.SortField {
	switch level {
	case albtui.AlbumLevel:
		return sorting.AlbumAlbumSortFields
	case albtui.TrackLevel:
		return sorting.AlbumTrackSortFields
	default:
		return sorting.AlbumAlbumSortFields
	}
}

func favoritesSortFieldsForLevel(v favtui.FavoritesView) []sorting.SortField {
	switch v.Level {
	case favtui.CategoryLevel:
		return nil
	case favtui.ListLevel:
		switch v.Category {
		case favtui.CategoryArtists, favtui.CategoryAlbums:
			return sorting.FavListSortFields
		case favtui.CategoryTracks:
			return sorting.FavTrackSortFields
		}
		return nil
	case favtui.TrackLevel:
		return sorting.FavTrackSortFields
	default:
		return nil
	}
}

func genresSortFieldsForLevel(level gentui.Level) []sorting.SortField {
	switch level {
	case gentui.GenreLevel:
		return sorting.GenreGenreSortFields
	case gentui.ArtistLevel:
		return sorting.GenreArtistSortFields
	case gentui.TrackLevel:
		return sorting.GenreTrackSortFields
	default:
		return sorting.GenreGenreSortFields
	}
}

func tagsSortFieldsForLevel(level tagtui.Level) []sorting.SortField {
	switch level {
	case tagtui.TrackLevel:
		return sorting.TagsTrackSortFields
	default:
		return nil
	}
}
