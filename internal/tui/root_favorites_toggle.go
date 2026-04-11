package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// handleToggleFavorite handles the 'F' key across all views.
func (m Model) handleToggleFavorite() (Model, tea.Cmd, bool) {
	if m.favoriteReader == nil || m.favoriteWriter == nil {
		return m, nil, false
	}

	reader := m.favoriteReader
	writer := m.favoriteWriter

	switch m.activeView {
	case viewLibrary:
		if len(m.library.selectedHashes) > 0 {
			hashes := make([]string, 0, len(m.library.selectedHashes))
			for h := range m.library.selectedHashes {
				hashes = append(hashes, h)
			}
			m.library.selectedHashes = nil
			return m, m.toggleFavoriteBulk(reader, writer, "track", hashes), true
		}
		if t, ok := m.library.view().FocusedTrack(); ok {
			return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
		}

	case viewRecentlyAdded:
		if len(m.recentlyAdded.selectedHashes) > 0 {
			hashes := make([]string, 0, len(m.recentlyAdded.selectedHashes))
			for h := range m.recentlyAdded.selectedHashes {
				hashes = append(hashes, h)
			}
			m.recentlyAdded.selectedHashes = nil
			return m, m.toggleFavoriteBulk(reader, writer, "track", hashes), true
		}
		if t, ok := m.recentlyAdded.view().FocusedTrack(); ok {
			return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
		}

	case viewArtists:
		switch m.artists.level {
		case arttui.ArtistLevel:
			if name, ok := m.artists.focusedArtist(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "artist", name), true
			}
		case arttui.AlbumLevel:
			if name, ok := m.artists.focusedAlbum(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "album", name), true
			}
		case arttui.TrackLevel:
			if t, ok := m.artists.focusedTrack(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
			}
		}

	case viewAlbums:
		switch m.albums.level {
		case albtui.AlbumLevel:
			if name, ok := m.albums.focusedAlbum(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "album", name), true
			}
		case albtui.TrackLevel:
			if t, ok := m.albums.focusedTrack(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
			}
		}

	case viewQueue:
		if m.queue.hasSelection() {
			hashes := m.queue.selectedHashes()
			for i := range m.queue.entries {
				m.queue.entries[i].Selected = false
			}
			return m, m.toggleFavoriteBulk(reader, writer, "track", hashes), true
		}
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			e := filtered[m.queue.cursor]
			return m, m.toggleFavoriteSingle(reader, writer, "track", e.Hash), true
		}

	case viewPlaylists:
		if m.playlists.level == pltui.TrackLevel {
			if m.playlists.hasTrackSelection() {
				var hashes []string
				for _, t := range m.playlists.tracks {
					if _, ok := m.playlists.selectedTrackIDs[t.ID]; ok {
						hashes = append(hashes, t.Hash)
					}
				}
				m.playlists.selectedTrackIDs = nil
				return m, m.toggleFavoriteBulk(reader, writer, "track", hashes), true
			}
			if t, ok := m.playlists.focusedTrack(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
			}
		}

	case viewGenres:
		switch m.genres.level {
		case gentui.ArtistLevel:
			if name, ok := m.genres.focusedArtist(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "artist", name), true
			}
		case gentui.TrackLevel:
			if len(m.genres.selectedHashes) > 0 {
				hashes := make([]string, 0, len(m.genres.selectedHashes))
				for h := range m.genres.selectedHashes {
					hashes = append(hashes, h)
				}
				m.genres.selectedHashes = nil
				return m, m.toggleFavoriteBulk(reader, writer, "track", hashes), true
			}
			if t, ok := m.genres.focusedTrack(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
			}
		}

	case viewFavorites:
		return m.handleFavoritesRemove()
	}

	return m, nil, true
}

// toggleFavoriteSingle returns a Cmd that checks IsFavorite and toggles.
func (m Model) toggleFavoriteSingle(reader favoriteReader, writer favoriteWriter, typ, value string) tea.Cmd {
	return func() tea.Msg {
		isFav, err := reader.IsFavorite(typ, value)
		if err != nil {
			return favoriteToggledMsg{typ: typ, value: value, err: err}
		}
		if isFav {
			err = writer.RemoveFavorite(typ, value)
		} else {
			err = writer.AddFavorite(typ, value)
		}
		msg := favoriteToggledMsg{typ: typ, value: value, added: !isFav, err: err}
		if isFav {
			msg.removedIdx = -1
		}
		return msg
	}
}

// toggleFavoriteBulk returns a Cmd that toggles multiple values. Each value is
// checked individually so that the toggle semantics apply per-item.
func (m Model) toggleFavoriteBulk(reader favoriteReader, writer favoriteWriter, typ string, values []string) tea.Cmd {
	return func() tea.Msg {
		var added, removed int
		var addedValues, removedValues []string
		for _, v := range values {
			isFav, err := reader.IsFavorite(typ, v)
			if err != nil {
				continue
			}
			if isFav {
				if err := writer.RemoveFavorite(typ, v); err != nil {
					slog.Warn("favorite remove failed", "err", err)
				}
				removed++
				removedValues = append(removedValues, v)
			} else {
				if err := writer.AddFavorite(typ, v); err != nil {
					slog.Warn("favorite add failed", "err", err)
				}
				added++
				addedValues = append(addedValues, v)
			}
		}
		msg := favoriteToggledMsg{
			typ:        typ,
			count:      added + removed,
			added:      added > 0,
			removedIdx: -1,
		}
		if added == 0 && len(removedValues) > 0 {
			msg.removedValues = removedValues
		}
		if removed == 0 && len(addedValues) > 0 {
			msg.addedValues = addedValues
		}
		return msg
	}
}
