package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/text"
)

// handleMetadataEditorKeyPress handles key events while the metadata editor
// overlay is active. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleMetadataEditorKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.metadataEditor == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m.markQuitting(), tea.Quit, true
	}
	updated, act, cmd := m.overlay.metadataEditor.HandleKey(msg)
	m.overlay.metadataEditor = &updated
	switch act {
	case metadata.ActionCancel:
		m.overlay.metadataEditor = nil
		return m, nil, true
	case metadata.ActionSave:
		model, cmd := m.handleMetadataSave()
		return model.(Model), cmd, true
	}
	return m, cmd, true
}

// handleMetadataKeyAction handles actionEditMetadata.
func (m Model) handleMetadataKeyAction(a keys.Action) (Model, tea.Cmd, bool) {
	if a != keys.ActionEditMetadata {
		return m, nil, false
	}

	if hashes := m.bulkSelectedHashes(); len(hashes) >= 2 {
		ed := m.bulkEditorForHashes(hashes)
		m.overlay.metadataEditor = &ed
		return m, nil, true
	}

	// A Data Health finding names a whole set of tracks, not a focused one, so
	// editing from it opens the bulk editor over everything the row counted.
	if row := m.insights.FocusedHealthRow(); row != nil && len(row.Hashes) > 0 {
		ed := m.bulkEditorForHashes(row.Hashes)
		m.overlay.metadataEditor = &ed
		return m, nil, true
	}

	if ed, ok := m.renameEditorForFocusedEntity(); ok {
		m.overlay.metadataEditor = &ed
		return m, nil, true
	}

	track, ok := m.focusedTrackForPreview()
	if !ok {
		m.statusBar.Notify("No track to edit")
		return m, nil, true
	}
	if full, found := m.library.Index.FindByHash(track.Hash); found {
		if track.Album == "" {
			track.Album = full.Album
		}
		track.Genre = full.Genre
	}
	ed := metadata.NewEditor(track.Hash, track.Title, track.Artist, track.Album, track.Genre, track.Duration)
	m.overlay.metadataEditor = &ed
	return m, nil, true
}

// bulkEditorForHashes seeds a bulk editor from the tracks behind the hashes,
// marking mixed fields with a display-only placeholder.
func (m Model) bulkEditorForHashes(hashes []string) metadata.Editor {
	var titles, artists, albums, genres, durations []string
	for _, h := range hashes {
		t, ok := m.library.Index.FindByHash(h)
		if !ok {
			continue
		}
		titles = append(titles, t.Title)
		artists = append(artists, t.Artist)
		albums = append(albums, t.Album)
		genres = append(genres, t.Genre)
		durations = append(durations, metadata.FormatDuration(t.Duration))
	}
	return metadata.NewBulkEditor(hashes, commonValue(titles), commonValue(artists), commonValue(albums), commonValue(genres), commonValue(durations)).
		WithMixedValues(titles, artists, albums, genres, durations)
}

// renameEditorForFocusedEntity returns a rename MetadataEditor when the active
// view and level support bulk renaming (Artists/Albums view at ArtistLevel or
// AlbumLevel). Returns false if the current context is not renameable.
func (m Model) renameEditorForFocusedEntity() (metadata.Editor, bool) {
	tracks := m.library.Index.Tracks()
	if m.ActiveView != viewBrowse {
		return metadata.Editor{}, false
	}
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		switch m.browse.Artists.Level {
		case arttui.ArtistLevel:
			displayName, ok := m.browse.Artists.FocusedArtist()
			if !ok {
				return metadata.Editor{}, false
			}
			rawName := arttui.RawName(displayName)
			matching := arttui.DeriveArtistTracks(tracks, rawName)
			hashes := make([]string, 0, len(matching))
			for _, t := range matching {
				hashes = append(hashes, t.Hash)
			}
			return metadata.NewArtistRenameEditor(hashes, displayName), true

		case arttui.AlbumLevel:
			displayName, ok := m.browse.Artists.FocusedAlbum()
			if !ok {
				return metadata.Editor{}, false
			}
			rawAlbum := arttui.RawName(displayName)
			matching := arttui.DeriveTracks(tracks, m.browse.Artists.SelectedArtist, rawAlbum)
			hashes := make([]string, 0, len(matching))
			for _, t := range matching {
				hashes = append(hashes, t.Hash)
			}
			return metadata.NewAlbumRenameEditor(hashes, library.AlbumTitle(displayName)), true
		}

	case browse.BrowseModeAlbums:
		if m.browse.Albums.Level == albtui.AlbumLevel {
			displayName, ok := m.browse.Albums.FocusedAlbum()
			if !ok {
				return metadata.Editor{}, false
			}
			rawAlbum := arttui.RawName(displayName)
			matching := albtui.DeriveAlbumTracks(tracks, rawAlbum)
			hashes := make([]string, 0, len(matching))
			for _, t := range matching {
				hashes = append(hashes, t.Hash)
			}
			return metadata.NewAlbumRenameEditor(hashes, library.AlbumTitle(displayName)), true
		}
	}
	return metadata.Editor{}, false
}

func (m Model) metadataSaveNotification(mode metadata.Mode, hashes []string, dirtyArtist, dirtyAlbum *string) Model {
	switch mode {
	case metadata.ModeRenameArtist:
		if dirtyArtist != nil {
			newName := *dirtyArtist
			if newName == "" {
				newName = arttui.UnknownArtist
			}
			for i, a := range m.browse.Artists.Artists {
				if a == newName {
					m.browse.Artists.Cursor = i
					m.browse.Artists.ScrollOffset = max(0, i-m.browse.Artists.Height/2)
					break
				}
			}
		}
		n := len(hashes)
		m.statusBar.Notify(fmt.Sprintf("Renamed %d %s", n, text.Pluralize(n, "track", "tracks")))
	case metadata.ModeRenameAlbum:
		if dirtyAlbum != nil {
			newName := *dirtyAlbum
			if len(hashes) > 0 {
				if t, ok := m.library.Index.FindByHash(hashes[0]); ok {
					newName = library.AlbumKey(t)
				}
			}
			if newName == "" {
				newName = arttui.UnknownAlbum
			}
			if m.ActiveView == viewBrowse {
				switch m.browse.Mode {
				case browse.BrowseModeArtists:
					for i, a := range m.browse.Artists.Albums {
						if a == newName {
							m.browse.Artists.Cursor = i
							m.browse.Artists.ScrollOffset = max(0, i-m.browse.Artists.Height/2)
							break
						}
					}
				case browse.BrowseModeAlbums:
					for i, a := range m.browse.Albums.Albums {
						if a == newName {
							m.browse.Albums.Cursor = i
							m.browse.Albums.ScrollOffset = max(0, i-m.browse.Albums.Height/2)
							break
						}
					}
				}
			}
		}
		n := len(hashes)
		m.statusBar.Notify(fmt.Sprintf("Renamed %d %s", n, text.Pluralize(n, "track", "tracks")))
	}
	return m
}
