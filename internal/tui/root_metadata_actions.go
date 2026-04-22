package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

// handleMetadataEditorKeyPress handles key events while the metadata editor
// overlay is active. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleMetadataEditorKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.metadataEditor == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, act, cmd := m.overlay.metadataEditor.HandleKey(msg)
	m.overlay.metadataEditor = &updated
	switch act {
	case editorCancel:
		m.overlay.metadataEditor = nil
		return m, nil, true
	case editorSave:
		model, cmd := m.handleMetadataSave()
		return model.(Model), cmd, true
	}
	return m, cmd, true
}

// handleMetadataKeyAction handles actionEditMetadata.
func (m Model) handleMetadataKeyAction(a action) (Model, tea.Cmd, bool) {
	if a != actionEditMetadata {
		return m, nil, false
	}

	if hashes := m.bulkSelectedHashes(); len(hashes) >= 2 {
		var titles, artists, albums, genres, durations []string
		for _, h := range hashes {
			t, ok := m.library.index.FindByHash(h)
			if !ok {
				continue
			}
			titles = append(titles, t.Title)
			artists = append(artists, t.Artist)
			albums = append(albums, t.Album)
			genres = append(genres, t.Genre)
			durations = append(durations, formatDurationForEditor(t.Duration))
		}
		ed := NewBulkMetadataEditor(hashes, commonValue(titles), commonValue(artists), commonValue(albums), commonValue(genres), commonValue(durations))
		m.overlay.metadataEditor = &ed
		return m, nil, true
	}

	if ed, ok := m.renameEditorForFocusedEntity(); ok {
		m.overlay.metadataEditor = &ed
		return m, nil, true
	}

	track, ok := m.focusedTrackForPreview()
	if !ok {
		m.statusBar.notify("No track to edit")
		return m, nil, true
	}
	if full, found := m.library.index.FindByHash(track.Hash); found {
		if track.Album == "" {
			track.Album = full.Album
		}
		track.Genre = full.Genre
	}
	ed := NewMetadataEditor(track.Hash, track.Title, track.Artist, track.Album, track.Genre, track.Duration)
	m.overlay.metadataEditor = &ed
	return m, nil, true
}

// renameEditorForFocusedEntity returns a rename MetadataEditor when the active
// view and level support bulk renaming (Artists/Albums view at ArtistLevel or
// AlbumLevel). Returns false if the current context is not renameable.
func (m Model) renameEditorForFocusedEntity() (MetadataEditor, bool) {
	tracks := m.library.index.Tracks()
	switch m.activeView {
	case viewArtists:
		switch m.artists.level {
		case arttui.ArtistLevel:
			displayName, ok := m.artists.focusedArtist()
			if !ok {
				return MetadataEditor{}, false
			}
			rawName := arttui.RawName(displayName)
			matching := arttui.DeriveArtistTracks(tracks, rawName)
			hashes := make([]string, 0, len(matching))
			for _, t := range matching {
				hashes = append(hashes, t.Hash)
			}
			return NewArtistRenameEditor(hashes, displayName), true

		case arttui.AlbumLevel:
			displayName, ok := m.artists.focusedAlbum()
			if !ok {
				return MetadataEditor{}, false
			}
			rawAlbum := arttui.RawName(displayName)
			matching := arttui.DeriveTracks(tracks, m.artists.selectedArtist, rawAlbum)
			hashes := make([]string, 0, len(matching))
			for _, t := range matching {
				hashes = append(hashes, t.Hash)
			}
			return NewAlbumRenameEditor(hashes, displayName), true
		}

	case viewAlbums:
		if m.albums.level == albtui.AlbumLevel {
			displayName, ok := m.albums.focusedAlbum()
			if !ok {
				return MetadataEditor{}, false
			}
			rawAlbum := arttui.RawName(displayName)
			matching := albtui.DeriveAlbumTracks(tracks, rawAlbum)
			hashes := make([]string, 0, len(matching))
			for _, t := range matching {
				hashes = append(hashes, t.Hash)
			}
			return NewAlbumRenameEditor(hashes, displayName), true
		}
	}
	return MetadataEditor{}, false
}

func (m Model) metadataSaveNotification(mode editorMode, hashes []string, dirtyArtist, dirtyAlbum *string) Model {
	switch mode {
	case editorModeRenameArtist:
		if dirtyArtist != nil {
			newName := *dirtyArtist
			if newName == "" {
				newName = arttui.UnknownArtist
			}
			for i, a := range m.artists.artists {
				if a == newName {
					m.artists.cursor = i
					m.artists.scrollOffset = max(0, i-m.artists.height/2)
					break
				}
			}
		}
		n := len(hashes)
		m.statusBar.notify(fmt.Sprintf("Renamed %d %s", n, pluralize(n, "track", "tracks")))
	case editorModeRenameAlbum:
		if dirtyAlbum != nil {
			newName := *dirtyAlbum
			if newName == "" {
				newName = arttui.UnknownAlbum
			}
			switch m.activeView {
			case viewArtists:
				for i, a := range m.artists.albums {
					if a == newName {
						m.artists.cursor = i
						m.artists.scrollOffset = max(0, i-m.artists.height/2)
						break
					}
				}
			case viewAlbums:
				for i, a := range m.albums.albums {
					if a == newName {
						m.albums.cursor = i
						m.albums.scrollOffset = max(0, i-m.albums.height/2)
						break
					}
				}
			}
		}
		n := len(hashes)
		m.statusBar.notify(fmt.Sprintf("Renamed %d %s", n, pluralize(n, "track", "tracks")))
	}
	return m
}
