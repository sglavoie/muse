package tui

import (
	"sort"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

func (m Model) followChangedAlbum(key string, previous []library.Track) string {
	if len(albtui.DeriveAlbumTracks(m.library.Index.Tracks(), key)) > 0 {
		return key
	}
	for _, old := range previous {
		if t, ok := m.library.Index.FindByHash(old.Hash); ok {
			return library.AlbumKey(t)
		}
	}
	// The old track list may have been cleared by an earlier scan batch.
	for _, t := range m.library.Index.Tracks() {
		if old, ok := m.watchedAlbumKeys[t.Hash]; ok && old == key {
			return library.AlbumKey(t)
		}
		if prior, ok := m.rescanSnapshot[t.Hash]; ok && prior.AlbumKey == key {
			return library.AlbumKey(t)
		}
	}
	return key
}

func (m Model) followChangedAlbums() Model {
	if m.browse.Artists.Level == arttui.TrackLevel {
		m.browse.Artists.SelectedAlbum = m.followChangedAlbum(m.browse.Artists.SelectedAlbum, m.browse.Artists.Tracks)
	}
	if m.browse.Albums.Level == albtui.TrackLevel {
		m.browse.Albums.SelectedAlbum = m.followChangedAlbum(m.browse.Albums.SelectedAlbum, m.browse.Albums.Tracks)
	}
	if m.favorites.Level == favtui.TrackLevel && m.favorites.Category == favtui.CategoryAlbums {
		m.favorites.DrilldownName = m.followChangedAlbum(m.favorites.DrilldownName, m.favorites.DrilldownTracks)
		m.favorites.DrilldownTracks = albtui.DeriveAlbumTracks(m.library.Index.Tracks(), m.favorites.DrilldownName)
	}
	return m
}

// Album edits and rescans can rekey persisted favorites. Reload only after the
// writer completes, retaining the old cache on a read error.
func (m Model) refreshAlbumFavorites() Model {
	if m.favoriteReader == nil {
		return m
	}
	rows, err := m.favoriteReader.ReadFavoritesByType("album")
	if err != nil {
		return m
	}
	m.favoriteAlbums = make(map[string]struct{}, len(rows))
	albums := make([]string, 0, len(rows))
	for _, row := range rows {
		m.favoriteAlbums[row.Value] = struct{}{}
		albums = append(albums, row.Value)
	}
	m.favorites.AlbumCount = len(albums)
	m.favorites.Albums = albums
	if m.favorites.Category == favtui.CategoryAlbums {
		m.favorites = m.favorites.RecomputeFavoritesSort()
		m.favorites = m.favorites.ClampCursor()
	}
	return m
}

func (m Model) changedAlbumKeys(tracks []library.Track) map[string]string {
	keys := make(map[string]string, len(tracks))
	for _, t := range tracks {
		keys[t.Hash] = m.watchedAlbumKeys[t.Hash]
		if old, ok := m.library.Index.FindByHash(t.Hash); ok {
			keys[t.Hash] = library.AlbumKey(old)
		}
	}
	return keys
}

// Watch events arrive before their DB batch commits. Mirror the writer's
// split/merge rule for highlights now; later reads use the persisted keys.
func (m Model) rekeyAlbumFavorites(oldKeys map[string]string, tracks []library.Track) Model {
	changed := make(map[string]bool)
	for _, t := range tracks {
		old := oldKeys[t.Hash]
		if _, favorite := m.favoriteAlbums[old]; favorite {
			key := library.AlbumKey(t)
			if old != key {
				m.favoriteAlbums[key] = struct{}{}
				changed[old] = true
			}
		}
	}
	for key := range changed {
		if len(albtui.DeriveAlbumTracks(m.library.Index.Tracks(), key)) == 0 {
			delete(m.favoriteAlbums, key)
		}
	}
	if len(changed) > 0 {
		m.favorites.Albums = nil
		for key := range m.favoriteAlbums {
			m.favorites.Albums = append(m.favorites.Albums, key)
		}
		sort.Strings(m.favorites.Albums)
		m.favorites.AlbumCount = len(m.favorites.Albums)
		if m.favorites.Category == favtui.CategoryAlbums {
			m.favorites = m.favorites.RecomputeFavoritesSort()
			m.favorites = m.favorites.ClampCursor()
		}
	}
	return m
}

// Retain only releases currently favorited or open in a drilldown. The watcher
// may send a move's deletion before its new path has been extracted.
func (m Model) rememberRemovedAlbums(paths []string) Model {
	for _, path := range paths {
		track, ok := m.library.Index.FindByPath(path)
		if !ok {
			continue
		}
		key := library.AlbumKey(track)
		if !m.referencesAlbum(key) {
			continue
		}
		if m.watchedAlbumKeys == nil {
			m.watchedAlbumKeys = make(map[string]string)
		}
		m.watchedAlbumKeys[track.Hash] = key
	}
	return m
}

func (m Model) referencesAlbum(key string) bool {
	_, favorite := m.favoriteAlbums[key]
	return favorite || key == m.browse.Albums.SelectedAlbum || key == m.browse.Artists.SelectedAlbum || key == m.favorites.DrilldownName
}
