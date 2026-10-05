package tui

import (
	musiclib "github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// bulkSelectedHashes returns the hashes of all bulk-selected tracks for the
// active view. Returns nil if there is no bulk selection or the view doesn't
// support it.
func (m Model) bulkSelectedHashes() []string {
	switch m.ActiveView {
	case viewLibrary:
		return hashMapToSlice(m.library.SelectedHashes)
	case viewQueue:
		return m.queue.SelectedHashes()
	case viewPlaylists:
		if len(m.playlists.SelectedTrackIDs) == 0 {
			return nil
		}
		hashes := make([]string, 0, len(m.playlists.SelectedTrackIDs))
		for _, t := range m.playlists.Tracks {
			if _, ok := m.playlists.SelectedTrackIDs[t.ID]; ok {
				hashes = append(hashes, t.Hash)
			}
		}
		return hashes
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return hashMapToSlice(m.browse.Artists.SelectedHashes)
		case browse.BrowseModeAlbums:
			return hashMapToSlice(m.browse.Albums.SelectedHashes)
		case browse.BrowseModeGenres:
			return hashMapToSlice(m.browse.Genres.SelectedHashes)
		}
		return nil
	case viewFavorites:
		if len(m.favorites.SelectedItems) == 0 {
			return nil
		}
		var items []musiclib.Track
		switch {
		case m.favorites.Level == favtui.ListLevel && m.favorites.Category == favtui.CategoryTracks:
			items = arttui.FilterTracks(m.favorites.Tracks, m.favorites.Filter.Text)
		case m.favorites.Level == favtui.TrackLevel:
			items = arttui.FilterTracks(m.favorites.DrilldownTracks, m.favorites.Filter.Text)
		default:
			return nil
		}
		var hashes []string
		for _, idx := range sortedIntSetKeys(m.favorites.SelectedItems) {
			if idx >= 0 && idx < len(items) {
				hashes = append(hashes, items[idx].Hash)
			}
		}
		if len(hashes) == 0 {
			return nil
		}
		return hashes
	case viewTags:
		return hashMapToSlice(m.tagsView.SelectedHashes)
	case viewMixes:
		if m.mixes.Level == 1 {
			return hashMapToSlice(m.mixes.TrackView.SelectedHashes)
		}
		return nil
	case viewInsights:
		if m.insights.AtTracks() {
			return hashMapToSlice(m.insights.TrackView.SelectedHashes)
		}
		return nil
	default:
		return nil
	}
}

// commonValue returns the shared value if all strings are equal, or empty string otherwise.
func commonValue(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	first := vals[0]
	for _, v := range vals[1:] {
		if v != first {
			return ""
		}
	}
	return first
}

// syncStatusBarMetadata updates the status bar artist and title when the
// currently-playing track (identified by hash) has been edited. This ensures
// the persistent left zone reflects renamed metadata immediately.
func (m Model) syncStatusBarMetadata(hash, artist, title string) Model {
	if m.playback.currentHash != "" && m.playback.currentHash == hash {
		m.statusBar.Artist = artist
		m.statusBar.Title = title
	}
	return m
}

// refreshDerivedViews updates the artists and albums lists derived from the
// current library index. Call after any metadata change that may have renamed
// artists, albums, or titles. Also rebuilds the library sort/filter caches so
// that active sort orders and filter matches reflect the updated metadata.
func (m Model) refreshDerivedViews() Model {
	m = m.followChangedAlbums()
	tracks := m.library.Index.Tracks()

	m.library = m.library.RefreshView()

	switch m.browse.Artists.Level {
	case arttui.ArtistLevel:
		m.browse.Artists.Artists = arttui.DeriveArtists(tracks)
		m.browse.Artists.ArtistCounts = arttui.DeriveArtistCounts(tracks)
		m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
		m.browse.Artists = m.browse.Artists.ClampCursor()
	case arttui.AlbumLevel:
		m.browse.Artists.Albums = arttui.DeriveAlbums(tracks, m.browse.Artists.SelectedArtist)
		m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
		m.browse.Artists = m.browse.Artists.ClampCursor()
	case arttui.TrackLevel:
		m.browse.Artists.Tracks = arttui.DeriveTracks(tracks, m.browse.Artists.SelectedArtist, m.browse.Artists.SelectedAlbum)
		m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
		m.browse.Artists = m.browse.Artists.ClampCursor()
	}

	switch m.browse.Albums.Level {
	case albtui.AlbumLevel:
		m.browse.Albums.Albums = albtui.DeriveAllAlbums(tracks)
		m.browse.Albums.AlbumCounts = albtui.DeriveAllAlbumCounts(tracks)
		m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()
		m.browse.Albums = m.browse.Albums.ClampCursor()
	case albtui.TrackLevel:
		m.browse.Albums.Tracks = albtui.DeriveAlbumTracks(tracks, m.browse.Albums.SelectedAlbum)
		m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()
		m.browse.Albums = m.browse.Albums.ClampCursor()
	}

	switch m.browse.Genres.Level {
	case gentui.GenreLevel:
		m.browse.Genres.Genres = gentui.DeriveGenres(tracks)
		m.browse.Genres.GenreCounts = gentui.DeriveGenreCounts(tracks)
		m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
		m.browse.Genres = m.browse.Genres.ClampCursor()
	case gentui.ArtistLevel:
		m.browse.Genres.Artists = gentui.DeriveGenreArtists(tracks, m.browse.Genres.SelectedGenre)
		m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
		m.browse.Genres = m.browse.Genres.ClampCursor()
	case gentui.TrackLevel:
		m.browse.Genres.Tracks = gentui.DeriveGenreArtistTracks(tracks, m.browse.Genres.SelectedGenre, m.browse.Genres.SelectedArtist)
		m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
		m.browse.Genres = m.browse.Genres.ClampCursor()
	}

	if m.playlists.Level == pltui.TrackLevel {
		for i := range m.playlists.Tracks {
			if t, ok := m.library.Index.FindByHash(m.playlists.Tracks[i].Hash); ok {
				m.playlists.Tracks[i].Title = t.Title
				m.playlists.Tracks[i].Artist = t.Artist
				m.playlists.Tracks[i].Album = t.Album
				m.playlists.Tracks[i].AlbumArtist = t.AlbumArtist
				m.playlists.Tracks[i].DurationMs = int64(t.Duration * 1000)
			}
		}
		m.playlists = m.playlists.RecomputePlaylistSort()
	}

	refreshTrackMetadata := func(ts []musiclib.Track) {
		for i := range ts {
			if t, ok := m.library.Index.FindByHash(ts[i].Hash); ok {
				ts[i].Title = t.Title
				ts[i].Artist = t.Artist
				ts[i].Album = t.Album
				ts[i].AlbumArtist = t.AlbumArtist
				ts[i].Genre = t.Genre
				ts[i].Duration = t.Duration
				ts[i].TrackNumber = t.TrackNumber
				ts[i].DiscNumber = t.DiscNumber
			}
		}
	}

	if m.library.Mode != libtui.LibraryModeAll && len(m.library.SourceTracks) > 0 {
		refreshTrackMetadata(m.library.SourceTracks)
		m.library = m.library.RefreshView()
	}

	if m.tagsView.Level == tagtui.TrackLevel {
		for i := range m.tagsView.Tracks {
			if t, ok := m.library.Index.FindByHash(m.tagsView.Tracks[i].Hash); ok {
				m.tagsView.Tracks[i].Title = t.Title
				m.tagsView.Tracks[i].Artist = t.Artist
				m.tagsView.Tracks[i].Album = t.Album
				m.tagsView.Tracks[i].AlbumArtist = t.AlbumArtist
				m.tagsView.Tracks[i].DurationMs = int64(t.Duration * 1000)
			}
		}
		m.tagsView = m.tagsView.RecomputeTagsSort()
	}

	if m.favorites.Level == favtui.ListLevel && m.favorites.Category == favtui.CategoryTracks {
		refreshTrackMetadata(m.favorites.Tracks)
		m.favorites = m.favorites.RecomputeFavoritesSort()
	}
	if m.favorites.Level == favtui.TrackLevel {
		refreshTrackMetadata(m.favorites.DrilldownTracks)
		m.favorites = m.favorites.RecomputeFavoritesSort()
	}

	// Refresh metadata for every cached mix, not just the active one — when
	// the user navigates to a previously-loaded mix the cached slice is
	// reused as-is, so stale title/artist/album/duration would leak in.
	for id, cached := range m.mixes.CachedMixes {
		refreshTrackMetadata(cached)
		m.mixes.CachedMixes[id] = cached
	}
	if m.mixes.Level == 1 {
		m.mixes.TrackView = m.mixes.TrackView.RefreshView()
	}

	// An open Insights drill holds a frozen snapshot taken by openDrill, so
	// without this the list the export/import loop is aimed at keeps rendering
	// pre-edit values until the drill is reopened.
	if m.insights.AtTracks() && len(m.insights.TrackView.SourceTracks) > 0 {
		refreshTrackMetadata(m.insights.TrackView.SourceTracks)
		m.insights.TrackView = m.insights.TrackView.RefreshView().ClampCursor()
	}

	// Queue genre filters must see edits, imports, undo, and data-health merges.
	for i := range m.queue.Entries {
		if track, ok := m.library.Index.FindByHash(m.queue.Entries[i].Hash); ok {
			m.queue.Entries[i].Genre = track.Genre
		}
	}
	m.clampQueueCursorAndScroll()
	return m
}
