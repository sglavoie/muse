package tui

import (
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
)

// rederiveArtistsData rebuilds the artists view's backing data from the
// library index. Call when cycling sort back to default so that the original
// (pre-sort) order is restored - the in-place sort replaced the slices and
// clearing sortState alone does not recover the original order.
func (m Model) rederiveArtistsData() Model {
	tracks := m.library.Index.Tracks()
	switch m.browse.artists.level {
	case arttui.ArtistLevel:
		m.browse.artists.artists = arttui.DeriveArtists(tracks)
	case arttui.AlbumLevel:
		m.browse.artists.albums = arttui.DeriveAlbums(tracks, m.browse.artists.selectedArtist)
	case arttui.TrackLevel:
		m.browse.artists.tracks = arttui.DeriveTracks(tracks, m.browse.artists.selectedArtist, m.browse.artists.selectedAlbum)
	}
	return m
}

// rederiveAlbumsData rebuilds the albums view's backing data from the library
// index. See rederiveArtistsData for rationale.
func (m Model) rederiveAlbumsData() Model {
	tracks := m.library.Index.Tracks()
	switch m.browse.albums.level {
	case albtui.AlbumLevel:
		m.browse.albums.albums = albtui.DeriveAllAlbums(tracks)
	case albtui.TrackLevel:
		m.browse.albums.tracks = albtui.DeriveAlbumTracks(tracks, m.browse.albums.selectedAlbum)
	}
	return m
}

// rederiveFavoritesData rebuilds the favorites view's track lists from the
// library index so the original order is restored. For artist/album lists
// at ListLevel the data is re-loaded from the database because the in-place
// sort replaces the slice and the original created_at DESC order cannot be
// recovered from the library index alone.
func (m Model) rederiveFavoritesData() Model {
	tracks := m.library.Index.Tracks()
	switch m.favorites.Level {
	case favtui.ListLevel:
		switch m.favorites.Category {
		case favtui.CategoryArtists:
			if m.favoriteReader != nil {
				rows, err := m.favoriteReader.ReadFavoritesByType("artist")
				if err == nil {
					artists := make([]string, len(rows))
					for i, r := range rows {
						artists[i] = r.Value
					}
					m.favorites.Artists = artists
				}
			}
		case favtui.CategoryAlbums:
			if m.favoriteReader != nil {
				rows, err := m.favoriteReader.ReadFavoritesByType("album")
				if err == nil {
					albums := make([]string, len(rows))
					for i, r := range rows {
						albums[i] = r.Value
					}
					m.favorites.Albums = albums
				}
			}
		case favtui.CategoryTracks:
			hashSet := make(map[string]struct{}, len(m.favorites.Tracks))
			for _, t := range m.favorites.Tracks {
				hashSet[t.Hash] = struct{}{}
			}
			var rebuilt []library.Track
			for _, t := range tracks {
				if _, ok := hashSet[t.Hash]; ok {
					rebuilt = append(rebuilt, t)
					delete(hashSet, t.Hash)
				}
			}
			m.favorites.Tracks = rebuilt
		}
	case favtui.TrackLevel:
		hashSet := make(map[string]struct{}, len(m.favorites.DrilldownTracks))
		for _, t := range m.favorites.DrilldownTracks {
			hashSet[t.Hash] = struct{}{}
		}
		var rebuilt []library.Track
		for _, t := range tracks {
			if _, ok := hashSet[t.Hash]; ok {
				rebuilt = append(rebuilt, t)
				delete(hashSet, t.Hash)
			}
		}
		m.favorites.DrilldownTracks = rebuilt
	}
	return m
}

// rederiveGenresData rebuilds the genres view's backing data from the library
// index. See rederiveArtistsData for rationale.
func (m Model) rederiveGenresData() Model {
	tracks := m.library.Index.Tracks()
	switch m.browse.genres.level {
	case gentui.GenreLevel:
		m.browse.genres.genres = gentui.DeriveGenres(tracks)
	case gentui.ArtistLevel:
		m.browse.genres.artists = gentui.DeriveGenreArtists(tracks, m.browse.genres.selectedGenre)
	case gentui.TrackLevel:
		m.browse.genres.tracks = gentui.DeriveGenreArtistTracks(tracks, m.browse.genres.selectedGenre, m.browse.genres.selectedArtist)
	}
	return m
}
