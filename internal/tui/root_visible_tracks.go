package tui

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	instui "github.com/sglavoie/muse/internal/tui/insights"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// visibleTracks returns the rows the active view is showing, in display order
// (filter + sort applied), or the bulk selection when one is non-empty. When ok
// is false the view has no materialized track list to offer and label carries
// the reason, which the caller surfaces as a notification.
//
// Two divergences from bulkActionTracks() are deliberate:
//   - There is no focused-track fallback. A bulk action with nothing selected
//     acts on the row under the cursor; an export with nothing selected means
//     "everything I am looking at", which is the whole point of the feature.
//   - The fuzzy filter is honored, where bulkLibraryTracks ignores it. What is
//     on screen is what gets exported.
func (m Model) visibleTracks() (tracks []library.Track, label string, ok bool) {
	switch m.ActiveView {
	case viewLibrary:
		return m.libraryVisibleTracks()
	case viewMixes:
		return m.mixesVisibleTracks()
	case viewInsights:
		return m.insightsVisibleTracks()
	case viewBrowse:
		return m.browseVisibleTracks()
	case viewFavorites:
		return m.favoritesVisibleTracks()
	case viewTags:
		return m.tagsVisibleTracks()
	case viewPlaylists:
		return m.playlistsVisibleTracks()
	case viewQueue:
		return m.queueVisibleTracks()
	default:
		return nil, "This view has no track list to export", false
	}
}

func (m Model) libraryVisibleTracks() ([]library.Track, string, bool) {
	rows := m.library.Panel().VisibleTracks()
	if len(m.library.SelectedHashes) > 0 {
		rows = collectSelectedTracks(rows, m.library.SelectedHashes)
	}
	return rows, libtui.StripLabel(m.library.Mode), true
}

func (m Model) mixesVisibleTracks() ([]library.Track, string, bool) {
	if m.mixes.Level != 1 {
		return nil, "Open a mix before exporting", false
	}
	rows := m.mixes.TrackView.Panel().VisibleTracks()
	if len(m.mixes.TrackView.SelectedHashes) > 0 {
		rows = collectSelectedTracks(rows, m.mixes.TrackView.SelectedHashes)
	}
	name := "Mix"
	for _, mix := range m.mixes.Mixes {
		if mix.ID == m.mixes.ActiveMix {
			name = mix.Name
			break
		}
	}
	return rows, "Mixes ▸ " + name, true
}

func (m Model) insightsVisibleTracks() ([]library.Track, string, bool) {
	if !m.insights.AtTracks() {
		return nil, "Drill into a track list before exporting", false
	}
	rows := m.insights.TrackView.Panel().VisibleTracks()
	if len(m.insights.TrackView.SelectedHashes) > 0 {
		rows = collectSelectedTracks(rows, m.insights.TrackView.SelectedHashes)
	}
	label := m.insights.DrillLabel
	if m.insights.Level == instui.LevelSessionTracks && m.insights.SessionLabel != "" {
		label = m.insights.SessionLabel
	}
	if label == "" {
		return rows, "Insights", true
	}
	return rows, "Insights ▸ " + label, true
}

func (m Model) browseVisibleTracks() ([]library.Track, string, bool) {
	switch m.browse.Mode {
	case browse.BrowseModeAlbums:
		return m.browseAlbumsVisibleTracks()
	case browse.BrowseModeGenres:
		return m.browseGenresVisibleTracks()
	default:
		return m.browseArtistsVisibleTracks()
	}
}

func (m Model) browseArtistsVisibleTracks() ([]library.Track, string, bool) {
	v := m.browse.Artists
	switch v.Level {
	case arttui.TrackLevel:
		rows := arttui.FilterTracks(v.Tracks, v.Filter.Text)
		if len(v.SelectedHashes) > 0 {
			rows = collectSelectedTracks(rows, v.SelectedHashes)
		}
		return rows, "Browse ▸ Artists ▸ " + v.SelectedArtist + " ▸ " + library.AlbumLabel(v.SelectedAlbum), true
	case arttui.AlbumLevel:
		album, ok := v.FocusedAlbum()
		if !ok {
			return nil, "No album focused", false
		}
		rows := arttui.DeriveTracks(m.library.Index.Tracks(), v.SelectedArtist, album)
		return rows, "Browse ▸ Artists ▸ " + v.SelectedArtist + " ▸ " + library.AlbumLabel(album), true
	default:
		artist, ok := v.FocusedArtist()
		if !ok {
			return nil, "No artist focused", false
		}
		rows := arttui.DeriveArtistTracks(m.library.Index.Tracks(), artist)
		return rows, "Browse ▸ Artists ▸ " + artist, true
	}
}

func (m Model) browseAlbumsVisibleTracks() ([]library.Track, string, bool) {
	v := m.browse.Albums
	if v.Level == albtui.TrackLevel {
		rows := arttui.FilterTracks(v.Tracks, v.Filter.Text)
		if len(v.SelectedHashes) > 0 {
			rows = collectSelectedTracks(rows, v.SelectedHashes)
		}
		return rows, "Browse ▸ Albums ▸ " + library.AlbumLabel(v.SelectedAlbum), true
	}
	album, ok := v.FocusedAlbum()
	if !ok {
		return nil, "No album focused", false
	}
	return albtui.DeriveAlbumTracks(m.library.Index.Tracks(), album), "Browse ▸ Albums ▸ " + library.AlbumLabel(album), true
}

func (m Model) browseGenresVisibleTracks() ([]library.Track, string, bool) {
	v := m.browse.Genres
	switch v.Level {
	case gentui.TrackLevel:
		rows := arttui.FilterTracks(v.Tracks, v.Filter.Text)
		if len(v.SelectedHashes) > 0 {
			rows = collectSelectedTracks(rows, v.SelectedHashes)
		}
		return rows, "Browse ▸ Genres ▸ " + v.SelectedGenre + " ▸ " + v.SelectedArtist, true
	case gentui.ArtistLevel:
		artist, ok := v.FocusedArtist()
		if !ok {
			return nil, "No artist focused", false
		}
		rows := gentui.DeriveGenreArtistTracks(m.library.Index.Tracks(), v.SelectedGenre, artist)
		return rows, "Browse ▸ Genres ▸ " + v.SelectedGenre + " ▸ " + artist, true
	default:
		genre, ok := v.FocusedGenre()
		if !ok {
			return nil, "No genre focused", false
		}
		return gentui.DeriveGenreTracks(m.library.Index.Tracks(), genre), "Browse ▸ Genres ▸ " + genre, true
	}
}

func (m Model) favoritesVisibleTracks() ([]library.Track, string, bool) {
	v := m.favorites
	switch {
	case v.Level == favtui.ListLevel && v.Category == favtui.CategoryTracks:
		rows := arttui.FilterTracks(v.Tracks, v.Filter.Text)
		if v.HasSelection() {
			rows = v.SelectedTracks()
		}
		return rows, "Favorites ▸ Tracks", true
	case v.Level == favtui.TrackLevel:
		rows := arttui.FilterTracks(v.DrilldownTracks, v.Filter.Text)
		if v.HasSelection() {
			rows = v.SelectedTracks()
		}
		return rows, "Favorites ▸ " + library.AlbumLabel(v.DrilldownName), true
	default:
		return nil, "Favorites shows no track list at this level", false
	}
}

func (m Model) tagsVisibleTracks() ([]library.Track, string, bool) {
	if m.tagsView.Level != tagtui.TrackLevel {
		return nil, "Open a tag before exporting", false
	}
	rows := m.tagsView.DisplayTracks()
	selected := m.tagsView.SelectedHashes
	tracks := make([]library.Track, 0, len(rows))
	for _, r := range rows {
		if len(selected) > 0 {
			if _, ok := selected[r.Hash]; !ok {
				continue
			}
		}
		tracks = append(tracks, m.hydrateTrack(trackFromPlaylistRow(r)))
	}
	return tracks, "Tags ▸ " + m.tagsView.SelectedTag.Name, true
}

func (m Model) playlistsVisibleTracks() ([]library.Track, string, bool) {
	if m.playlists.Level != pltui.TrackLevel {
		return nil, "Open a playlist before exporting", false
	}
	rows := m.playlists.DisplayTracks()
	selected := m.playlists.SelectedTrackIDs
	tracks := make([]library.Track, 0, len(rows))
	for _, r := range rows {
		if len(selected) > 0 {
			if _, ok := selected[r.ID]; !ok {
				continue
			}
		}
		tracks = append(tracks, m.hydrateTrack(trackFromPlaylistRow(r)))
	}
	name := "Playlist"
	if m.playlists.ListCursor >= 0 && m.playlists.ListCursor < len(m.playlists.Playlists) {
		name = m.playlists.Playlists[m.playlists.ListCursor].Name
	}
	return tracks, "Playlists ▸ " + name, true
}

func (m Model) queueVisibleTracks() ([]library.Track, string, bool) {
	entries := m.queue.FilteredEntries()
	anySelected := false
	for _, e := range entries {
		if e.Selected {
			anySelected = true
			break
		}
	}
	tracks := make([]library.Track, 0, len(entries))
	for _, e := range entries {
		if anySelected && !e.Selected {
			continue
		}
		tracks = append(tracks, m.hydrateTrack(library.Track{
			Path:        e.Path,
			Hash:        e.Hash,
			Artist:      e.Artist,
			Title:       e.Title,
			Album:       e.Album,
			AlbumArtist: e.AlbumArtist,
			Duration:    float64(e.DurationMs) / 1000,
			TrackNumber: e.TrackNumber,
			DiscNumber:  e.DiscNumber,
		}))
	}
	return tracks, "Queue", true
}

// hydrateTrack replaces a view's own adapted row with the library index entry
// for the same content hash.
//
// Queue entries and db.PlaylistTrackRow carry no Genre, PlayCount, or
// LastPlayedAt column at all, so a row adapted straight from either reads as a
// genre-less track that has never been played. Export merely lost a column to
// that; a caller that starts Auto-DJ from a selection would have every one of
// those rows fail a genre inclusion and classify as pure discovery. The row
// itself remains the fallback for a hash the index no longer holds, which is
// what keeps a missing file exportable.
func (m Model) hydrateTrack(row library.Track) library.Track {
	if row.Hash == "" {
		return row
	}
	live, ok := m.library.Index.FindByHash(row.Hash)
	if !ok {
		return row
	}
	return live
}

// trackFromPlaylistRow adapts the row type Playlists and Tags hold.
func trackFromPlaylistRow(r db.PlaylistTrackRow) library.Track {
	return library.Track{
		Path:        r.Path,
		Hash:        r.Hash,
		Artist:      r.Artist,
		Title:       r.Title,
		Album:       r.Album,
		AlbumArtist: r.AlbumArtist,
		Genre:       r.Genre,
		Duration:    float64(r.DurationMs) / 1000,
		TrackNumber: r.TrackNumber,
		DiscNumber:  r.DiscNumber,
	}
}
