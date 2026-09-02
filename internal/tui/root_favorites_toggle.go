package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
)

// handleToggleFavorite handles the 'F' key across all views.
func (m Model) handleToggleFavorite() (Model, tea.Cmd, bool) {
	if m.favoriteReader == nil || m.favoriteWriter == nil {
		return m, nil, false
	}

	reader := m.favoriteReader
	writer := m.favoriteWriter
	if tracks := m.favoritePickerTracks(); len(tracks) > 0 {
		m = m.openFavoriteTargetPicker()
		return m, nil, true
	}

	switch m.ActiveView {
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			switch m.browse.Artists.Level {
			case arttui.ArtistLevel:
				if name, ok := m.browse.Artists.FocusedArtist(); ok {
					return m, m.toggleFavoriteSingle(reader, writer, "artist", name), true
				}
			case arttui.AlbumLevel:
				if name, ok := m.browse.Artists.FocusedAlbum(); ok {
					return m, m.toggleFavoriteSingle(reader, writer, "album", name), true
				}
			}
		case browse.BrowseModeAlbums:
			if m.browse.Albums.Level == albtui.AlbumLevel {
				if name, ok := m.browse.Albums.FocusedAlbum(); ok {
					return m, m.toggleFavoriteSingle(reader, writer, "album", name), true
				}
			}
		case browse.BrowseModeGenres:
			if m.browse.Genres.Level == gentui.ArtistLevel {
				if name, ok := m.browse.Genres.FocusedArtist(); ok {
					return m, m.toggleFavoriteSingle(reader, writer, "artist", name), true
				}
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
