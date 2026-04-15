package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
)

// metadataEditedMsg is sent after the async DB write completes.
type metadataEditedMsg struct {
	err error
}

// handleMetadataSave reads values from the editor, updates in-memory state,
// and returns a command to persist the change to the database.
func (m Model) handleMetadataSave() (tea.Model, tea.Cmd) {
	ed := m.overlay.metadataEditor
	if ed.IsBulk() || ed.Mode() != editorModeDefault {
		return m.handleBulkMetadataSave()
	}

	hash := ed.Hash()
	title := ed.Title()
	artist := ed.Artist()
	album := ed.Album()
	genre := ed.Genre()
	durStr := strings.TrimSpace(ed.Duration())

	newDuration, err := parseDurationFromEditor(durStr)
	if err != nil {
		m.statusBar.notification = newNotification(fmt.Sprintf("Invalid duration: %s", err))
		return m, nil
	}

	m.overlay.metadataEditor = nil

	var origTitle, origArtist, origAlbum, origGenre string
	var origDuration float64
	if t, ok := m.library.index.FindByHash(hash); ok {
		origTitle = t.Title
		origArtist = t.Artist
		origAlbum = t.Album
		origGenre = t.Genre
		origDuration = t.Duration
	}

	m.library.index.UpdateMetadata(hash, title, artist, album, genre)
	m.library.index.UpdateDuration(hash, newDuration)
	m = m.syncRecentlyAddedFromLibrary()

	display := trackDisplay(artist, title, "")
	newDurationMs := int64(newDuration * 1000)
	for i := range m.queue.entries {
		if m.queue.entries[i].Hash == hash {
			m.queue.entries[i].Title = title
			m.queue.entries[i].Artist = artist
			m.queue.entries[i].Album = album
			m.queue.entries[i].Display = display
			m.queue.entries[i].DurationMs = newDurationMs
		}
	}

	m = m.syncStatusBarMetadata(hash, artist, title)

	m.setUndoSlot(undoBulkEditMetadataAction{
		originals: map[string][5]string{hash: {origTitle, origArtist, origAlbum, origGenre, formatDurationForEditor(origDuration)}},
	}, nil)

	m = m.refreshDerivedViews()

	if m.userEditedOverlay != nil {
		m.userEditedOverlay[hash] = db.UserEditedRow{
			Hash: hash, Title: title, Artist: artist, Album: album, Genre: genre,
		}
	}

	mw := m.metadataWriter
	dw := m.durationWriter
	return m, func() tea.Msg {
		var firstErr error
		if mw != nil {
			if err := mw.UpdateTrackMetadata(hash, title, artist, album, genre); err != nil {
				slog.Warn("metadata edit: DB write failed", "hash", hash, "err", err)
				firstErr = err
			}
		}
		if dw != nil {
			if err := dw.UpdateTrackDuration(hash, newDurationMs); err != nil {
				slog.Warn("metadata edit: duration DB write failed", "hash", hash, "err", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
		return metadataEditedMsg{err: firstErr}
	}
}

// handleBulkMetadataSave saves dirty fields from the bulk metadata editor to
// all selected tracks, sets up an undo slot, and clears the selection.
func (m Model) handleBulkMetadataSave() (tea.Model, tea.Cmd) {
	ed := m.overlay.metadataEditor
	dirtyTitle, dirtyArtist, dirtyAlbum, dirtyGenre, dirtyDuration := ed.DirtyFields()
	m.overlay.metadataEditor = nil

	if dirtyTitle == nil && dirtyArtist == nil && dirtyAlbum == nil && dirtyGenre == nil && dirtyDuration == nil {
		m.statusBar.notification = newNotification("No changes")
		return m, nil
	}

	var newDuration float64
	if dirtyDuration != nil {
		var err error
		newDuration, err = parseDurationFromEditor(*dirtyDuration)
		if err != nil {
			m.statusBar.notification = newNotification(fmt.Sprintf("Invalid duration: %s", err))
			return m, nil
		}
	}

	originals := make(map[string][5]string, len(ed.Hashes()))

	type update struct {
		hash, title, artist, album, genre string
		durationMs                        int64
	}
	var updates []update

	for _, h := range ed.Hashes() {
		t, ok := m.library.index.FindByHash(h)
		if !ok {
			continue
		}
		originals[h] = [5]string{t.Title, t.Artist, t.Album, t.Genre, formatDurationForEditor(t.Duration)}

		newTitle, newArtist, newAlbum, newGenre := t.Title, t.Artist, t.Album, t.Genre
		trackDuration := t.Duration
		if dirtyTitle != nil {
			newTitle = *dirtyTitle
		}
		if dirtyArtist != nil {
			newArtist = *dirtyArtist
		}
		if dirtyAlbum != nil {
			newAlbum = *dirtyAlbum
		}
		if dirtyGenre != nil {
			newGenre = *dirtyGenre
		}
		if dirtyDuration != nil {
			trackDuration = newDuration
		}

		m.library.index.UpdateMetadata(h, newTitle, newArtist, newAlbum, newGenre)
		if dirtyDuration != nil {
			m.library.index.UpdateDuration(h, trackDuration)
		}

		durationMs := int64(trackDuration * 1000)
		display := trackDisplay(newArtist, newTitle, "")
		for i := range m.queue.entries {
			if m.queue.entries[i].Hash == h {
				m.queue.entries[i].Title = newTitle
				m.queue.entries[i].Artist = newArtist
				m.queue.entries[i].Album = newAlbum
				m.queue.entries[i].Display = display
				if dirtyDuration != nil {
					m.queue.entries[i].DurationMs = durationMs
				}
			}
		}

		m = m.syncStatusBarMetadata(h, newArtist, newTitle)

		updates = append(updates, update{h, newTitle, newArtist, newAlbum, newGenre, durationMs})
	}
	m = m.syncRecentlyAddedFromLibrary()

	switch m.activeView {
	case viewLibrary:
		m.library.selectedHashes = nil
	case viewRecentlyAdded:
		m.recentlyAdded.selectedHashes = nil
	case viewHistory:
		m.history.selectedHashes = nil
	case viewQueue:
		for i := range m.queue.entries {
			m.queue.entries[i].Selected = false
		}
	case viewPlaylists:
		m.playlists.selectedTrackIDs = nil
	case viewArtists:
		m.artists.selectedHashes = nil
	case viewAlbums:
		m.albums.selectedHashes = nil
	case viewFavorites:
		m.favorites.selectedItems = nil
	case viewGenres:
		m.genres.selectedHashes = nil
	case viewTags:
		m.tagsView.selectedHashes = nil
	}

	m.setUndoSlot(undoBulkEditMetadataAction{
		originals: originals,
	}, nil)

	m = m.refreshDerivedViews()
	m = m.metadataSaveNotification(ed.Mode(), ed.Hashes(), dirtyArtist, dirtyAlbum)

	if m.userEditedOverlay != nil {
		for _, u := range updates {
			m.userEditedOverlay[u.hash] = db.UserEditedRow{
				Hash: u.hash, Title: u.title, Artist: u.artist, Album: u.album, Genre: u.genre,
			}
		}
	}

	mw := m.metadataWriter
	dw := m.durationWriter
	return m, func() tea.Msg {
		var firstErr error
		for _, u := range updates {
			if mw != nil {
				if err := mw.UpdateTrackMetadata(u.hash, u.title, u.artist, u.album, u.genre); err != nil {
					slog.Warn("bulk metadata edit: DB write failed", "hash", u.hash, "err", err)
					if firstErr == nil {
						firstErr = err
					}
				}
			}
			if dw != nil && dirtyDuration != nil {
				if err := dw.UpdateTrackDuration(u.hash, u.durationMs); err != nil {
					slog.Warn("bulk metadata edit: duration DB write failed", "hash", u.hash, "err", err)
					if firstErr == nil {
						firstErr = err
					}
				}
			}
		}
		return metadataEditedMsg{err: firstErr}
	}
}
