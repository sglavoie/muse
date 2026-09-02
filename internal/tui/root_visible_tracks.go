package tui

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
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
	switch m.activeView {
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
	if m.mixes.level != 1 {
		return nil, "Open a mix before exporting", false
	}
	rows := m.mixes.trackView.Panel().VisibleTracks()
	if len(m.mixes.trackView.SelectedHashes) > 0 {
		rows = collectSelectedTracks(rows, m.mixes.trackView.SelectedHashes)
	}
	name := "Mix"
	for _, mix := range m.mixes.mixes {
		if mix.ID == m.mixes.activeMix {
			name = mix.Name
			break
		}
	}
	return rows, "Mixes ▸ " + name, true
}

func (m Model) insightsVisibleTracks() ([]library.Track, string, bool) {
	if !m.insights.atTracks() {
		return nil, "Drill into a track list before exporting", false
	}
	rows := m.insights.trackView.Panel().VisibleTracks()
	if len(m.insights.trackView.SelectedHashes) > 0 {
		rows = collectSelectedTracks(rows, m.insights.trackView.SelectedHashes)
	}
	label := m.insights.drillLabel
	if m.insights.level == insightsLevelSessionTracks && m.insights.sessionLabel != "" {
		label = m.insights.sessionLabel
	}
	if label == "" {
		return rows, "Insights", true
	}
	return rows, "Insights ▸ " + label, true
}

func (m Model) browseVisibleTracks() ([]library.Track, string, bool) {
	switch m.browse.mode {
	case BrowseModeAlbums:
		return m.browseAlbumsVisibleTracks()
	case BrowseModeGenres:
		return m.browseGenresVisibleTracks()
	default:
		return m.browseArtistsVisibleTracks()
	}
}

func (m Model) browseArtistsVisibleTracks() ([]library.Track, string, bool) {
	v := m.browse.artists
	switch v.level {
	case arttui.TrackLevel:
		rows := arttui.FilterTracks(v.tracks, v.filter.Text)
		if len(v.selectedHashes) > 0 {
			rows = collectSelectedTracks(rows, v.selectedHashes)
		}
		return rows, "Browse ▸ Artists ▸ " + v.selectedArtist + " ▸ " + v.selectedAlbum, true
	case arttui.AlbumLevel:
		album, ok := v.focusedAlbum()
		if !ok {
			return nil, "No album focused", false
		}
		rows := arttui.DeriveTracks(m.library.Index.Tracks(), v.selectedArtist, album)
		return rows, "Browse ▸ Artists ▸ " + v.selectedArtist + " ▸ " + album, true
	default:
		artist, ok := v.focusedArtist()
		if !ok {
			return nil, "No artist focused", false
		}
		rows := arttui.DeriveArtistTracks(m.library.Index.Tracks(), artist)
		return rows, "Browse ▸ Artists ▸ " + artist, true
	}
}

func (m Model) browseAlbumsVisibleTracks() ([]library.Track, string, bool) {
	v := m.browse.albums
	if v.level == albtui.TrackLevel {
		rows := arttui.FilterTracks(v.tracks, v.filter.Text)
		if len(v.selectedHashes) > 0 {
			rows = collectSelectedTracks(rows, v.selectedHashes)
		}
		return rows, "Browse ▸ Albums ▸ " + v.selectedAlbum, true
	}
	album, ok := v.focusedAlbum()
	if !ok {
		return nil, "No album focused", false
	}
	return albtui.DeriveAlbumTracks(m.library.Index.Tracks(), album), "Browse ▸ Albums ▸ " + album, true
}

func (m Model) browseGenresVisibleTracks() ([]library.Track, string, bool) {
	v := m.browse.genres
	switch v.level {
	case gentui.TrackLevel:
		rows := arttui.FilterTracks(v.tracks, v.filter.Text)
		if len(v.selectedHashes) > 0 {
			rows = collectSelectedTracks(rows, v.selectedHashes)
		}
		return rows, "Browse ▸ Genres ▸ " + v.selectedGenre + " ▸ " + v.selectedArtist, true
	case gentui.ArtistLevel:
		artist, ok := v.focusedArtist()
		if !ok {
			return nil, "No artist focused", false
		}
		rows := gentui.DeriveGenreArtistTracks(m.library.Index.Tracks(), v.selectedGenre, artist)
		return rows, "Browse ▸ Genres ▸ " + v.selectedGenre + " ▸ " + artist, true
	default:
		genre, ok := v.focusedGenre()
		if !ok {
			return nil, "No genre focused", false
		}
		return gentui.DeriveGenreTracks(m.library.Index.Tracks(), genre), "Browse ▸ Genres ▸ " + genre, true
	}
}

func (m Model) favoritesVisibleTracks() ([]library.Track, string, bool) {
	v := m.favorites
	switch {
	case v.level == favtui.ListLevel && v.category == favtui.CategoryTracks:
		rows := selectedByIndex(arttui.FilterTracks(v.tracks, v.filter.Text), v.selectedItems)
		return rows, "Favorites ▸ Tracks", true
	case v.level == favtui.TrackLevel:
		rows := selectedByIndex(arttui.FilterTracks(v.drilldownTracks, v.filter.Text), v.selectedItems)
		return rows, "Favorites ▸ " + v.drilldownName, true
	default:
		return nil, "Favorites shows no track list at this level", false
	}
}

func (m Model) tagsVisibleTracks() ([]library.Track, string, bool) {
	if m.tagsView.level != tagtui.TrackLevel {
		return nil, "Open a tag before exporting", false
	}
	rows := m.tagsView.displayTracks()
	selected := m.tagsView.selectedHashes
	tracks := make([]library.Track, 0, len(rows))
	for _, r := range rows {
		if len(selected) > 0 {
			if _, ok := selected[r.Hash]; !ok {
				continue
			}
		}
		tracks = append(tracks, trackFromPlaylistRow(r))
	}
	return tracks, "Tags ▸ " + m.tagsView.selectedTag.Name, true
}

func (m Model) playlistsVisibleTracks() ([]library.Track, string, bool) {
	if m.playlists.level != pltui.TrackLevel {
		return nil, "Open a playlist before exporting", false
	}
	rows := m.playlists.displayTracks()
	selected := m.playlists.selectedTrackIDs
	tracks := make([]library.Track, 0, len(rows))
	for _, r := range rows {
		if len(selected) > 0 {
			if _, ok := selected[r.ID]; !ok {
				continue
			}
		}
		tracks = append(tracks, trackFromPlaylistRow(r))
	}
	name := "Playlist"
	if m.playlists.listCursor >= 0 && m.playlists.listCursor < len(m.playlists.playlists) {
		name = m.playlists.playlists[m.playlists.listCursor].Name
	}
	return tracks, "Playlists ▸ " + name, true
}

func (m Model) queueVisibleTracks() ([]library.Track, string, bool) {
	entries := m.queue.filteredEntries()
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
		tracks = append(tracks, library.Track{
			Path:        e.Path,
			Hash:        e.Hash,
			Artist:      e.Artist,
			Title:       e.Title,
			Album:       e.Album,
			Duration:    float64(e.DurationMs) / 1000,
			TrackNumber: e.TrackNumber,
			DiscNumber:  e.DiscNumber,
		})
	}
	return tracks, "Queue", true
}

// selectedByIndex narrows rows to the display indices in selected, which is how
// Favorites records a bulk selection. An empty selection keeps every row.
func selectedByIndex(rows []library.Track, selected map[int]struct{}) []library.Track {
	if len(selected) == 0 {
		return rows
	}
	out := make([]library.Track, 0, len(selected))
	for _, idx := range sortedIntSetKeys(selected) {
		if idx >= 0 && idx < len(rows) {
			out = append(out, rows[idx])
		}
	}
	return out
}

// trackFromPlaylistRow adapts the row type Playlists and Tags hold. The result
// carries no Genre because db.PlaylistTrackRow has no such column; export
// hydrates each row through the library index to fill it in.
func trackFromPlaylistRow(r db.PlaylistTrackRow) library.Track {
	return library.Track{
		Path:        r.Path,
		Hash:        r.Hash,
		Artist:      r.Artist,
		Title:       r.Title,
		Album:       r.Album,
		Duration:    float64(r.DurationMs) / 1000,
		TrackNumber: r.TrackNumber,
		DiscNumber:  r.DiscNumber,
	}
}
