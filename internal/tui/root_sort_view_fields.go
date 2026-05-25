package tui

import (
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func artistsSortFieldsForLevel(level arttui.Level) []SortField {
	switch level {
	case arttui.ArtistLevel:
		return artistArtistSortFields
	case arttui.AlbumLevel:
		return artistAlbumSortFields
	case arttui.TrackLevel:
		return artistTrackSortFields
	default:
		return artistArtistSortFields
	}
}

func albumsSortFieldsForLevel(level albtui.Level) []SortField {
	switch level {
	case albtui.AlbumLevel:
		return albumAlbumSortFields
	case albtui.TrackLevel:
		return albumTrackSortFields
	default:
		return albumAlbumSortFields
	}
}

func favoritesSortFieldsForLevel(v FavoritesView) []SortField {
	switch v.level {
	case favtui.CategoryLevel:
		return nil
	case favtui.ListLevel:
		switch v.category {
		case favtui.CategoryArtists, favtui.CategoryAlbums:
			return favListSortFields
		case favtui.CategoryTracks:
			return favTrackSortFields
		}
		return nil
	case favtui.TrackLevel:
		return favTrackSortFields
	default:
		return nil
	}
}

func genresSortFieldsForLevel(level gentui.Level) []SortField {
	switch level {
	case gentui.GenreLevel:
		return genreGenreSortFields
	case gentui.ArtistLevel:
		return genreArtistSortFields
	case gentui.TrackLevel:
		return genreTrackSortFields
	default:
		return genreGenreSortFields
	}
}

func tagsSortFieldsForLevel(level tagtui.Level) []SortField {
	switch level {
	case tagtui.TrackLevel:
		return tagsTrackSortFields
	default:
		return nil
	}
}
