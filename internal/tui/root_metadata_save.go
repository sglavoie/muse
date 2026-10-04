package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/text"
)

// metadataEditedMsg is sent after the async DB write completes.
type metadataEditedMsg struct {
	err error
}

// handleMetadataSave reads values from the editor, updates in-memory state,
// and returns a command to persist the change to the database.
func (m Model) handleMetadataSave() (tea.Model, tea.Cmd) {
	ed := m.overlay.metadataEditor
	if ed.IsBulk() || ed.Mode() != metadata.ModeDefault {
		return m.handleBulkMetadataSave()
	}

	hash := ed.Hash()
	title := ed.Title()
	artist := ed.Artist()
	album := ed.Album()
	genre := ed.Genre()
	durStr := strings.TrimSpace(ed.Duration())

	newDuration, err := metadata.ParseDuration(durStr)
	if err != nil {
		m.statusBar.NotifyError(fmt.Sprintf("Invalid duration: %s", err))
		return m, nil
	}

	var origTitle, origArtist, origAlbum, origGenre string
	var origDuration float64
	var trackNumber, discNumber int
	if t, ok := m.library.Index.FindByHash(hash); ok {
		origTitle = t.Title
		origArtist = t.Artist
		origAlbum = t.Album
		origGenre = t.Genre
		origDuration = t.Duration
		trackNumber = t.TrackNumber
		discNumber = t.DiscNumber
	}
	// The editor displays whole seconds. Keep the precise library duration
	// unless the user actually changed the displayed duration.
	durationChanged := ed.DurationChanged()
	if !durationChanged {
		newDuration = origDuration
	}
	m.overlay.metadataEditor = nil
	if title == origTitle && artist == origArtist && album == origAlbum && genre == origGenre && !durationChanged {
		m.statusBar.Notify("No changes")
		return m, nil
	}

	m.library.Index.UpdateMetadata(hash, title, artist, album, genre)
	m.library.Index.UpdateDuration(hash, newDuration)
	m = m.syncRecentlyAddedFromLibrary()

	display := text.TrackDisplay(artist, title, "")
	newDurationMs := int64(newDuration * 1000)
	for i := range m.queue.Entries {
		if m.queue.Entries[i].Hash == hash {
			m.queue.Entries[i].Title = title
			m.queue.Entries[i].Artist = artist
			m.queue.Entries[i].Album = album
			m.queue.Entries[i].Display = display
			m.queue.Entries[i].DurationMs = newDurationMs
		}
	}

	m = m.syncStatusBarMetadata(hash, artist, title)
	m = m.preserveQueueSort()

	m.setUndoSlot(undoBulkEditMetadataAction{
		originals: map[string][5]string{hash: {origTitle, origArtist, origAlbum, origGenre, metadata.FormatDuration(origDuration)}},
		durations: map[string]float64{hash: origDuration},
	}, nil)

	m = m.refreshDerivedViews()

	if m.userEditedOverlay != nil {
		// The editor cannot change track/disc, so the overlay carries the
		// current numbers forward; leaving them zero would make the next scan
		// wipe a track number the file's own tags do supply.
		m.userEditedOverlay[hash] = db.UserEditedRow{
			Hash: hash, Title: title, Artist: artist, Album: album, Genre: genre,
			TrackNumber: trackNumber, DiscNumber: discNumber,
		}
	}

	mw := m.metadataWriter
	dw := m.durationWriter
	// Publishing here, after the index already reads edited, is what makes this
	// row outrank a genre merge that planned it before the edit and has not
	// reached the database yet.
	q := m.metadataWrites
	var ticket uint64
	if mw != nil {
		ticket = q.Publish(hash)
	}
	return m, func() tea.Msg {
		var firstErr error
		if mw != nil {
			_, err := q.Claim(hash, ticket, func() error {
				return mw.UpdateTrackMetadata(hash, title, artist, album, genre)
			})
			if err != nil {
				slog.Warn("metadata edit: DB write failed", "hash", hash, "err", err)
				firstErr = err
			}
		}
		if dw != nil && durationChanged {
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

	if dirtyTitle == nil && dirtyArtist == nil && dirtyAlbum == nil && dirtyGenre == nil && dirtyDuration == nil {
		m.overlay.metadataEditor = nil
		m.statusBar.Notify("No changes")
		return m, nil
	}

	var newDuration float64
	if dirtyDuration != nil {
		var err error
		newDuration, err = metadata.ParseDuration(*dirtyDuration)
		if err != nil {
			m.statusBar.NotifyError(fmt.Sprintf("Invalid duration: %s", err))
			return m, nil
		}
	}
	m.overlay.metadataEditor = nil

	originals := make(map[string][5]string, len(ed.Hashes()))
	durations := make(map[string]float64, len(ed.Hashes()))

	type update struct {
		hash, title, artist, album, genre string
		trackNumber, discNumber           int
		durationMs                        int64
		ticket                            uint64
	}
	var updates []update

	for _, h := range ed.Hashes() {
		t, ok := m.library.Index.FindByHash(h)
		if !ok {
			continue
		}
		originals[h] = [5]string{t.Title, t.Artist, t.Album, t.Genre, metadata.FormatDuration(t.Duration)}
		durations[h] = t.Duration

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

		m.library.Index.UpdateMetadata(h, newTitle, newArtist, newAlbum, newGenre)
		if dirtyDuration != nil {
			m.library.Index.UpdateDuration(h, trackDuration)
		}

		durationMs := int64(trackDuration * 1000)
		display := text.TrackDisplay(newArtist, newTitle, "")
		for i := range m.queue.Entries {
			if m.queue.Entries[i].Hash == h {
				m.queue.Entries[i].Title = newTitle
				m.queue.Entries[i].Artist = newArtist
				m.queue.Entries[i].Album = newAlbum
				m.queue.Entries[i].Display = display
				if dirtyDuration != nil {
					m.queue.Entries[i].DurationMs = durationMs
				}
			}
		}

		m = m.syncStatusBarMetadata(h, newArtist, newTitle)

		updates = append(updates, update{hash: h, title: newTitle, artist: newArtist, album: newAlbum, genre: newGenre, trackNumber: t.TrackNumber, discNumber: t.DiscNumber, durationMs: durationMs})
	}
	m = m.syncRecentlyAddedFromLibrary()
	m = m.preserveQueueSort()

	m = m.clearActiveViewSelection()

	m.setUndoSlot(undoBulkEditMetadataAction{
		originals: originals,
		durations: durations,
	}, nil)

	m = m.refreshDerivedViews()
	m = m.metadataSaveNotification(ed.Mode(), ed.Hashes(), dirtyArtist, dirtyAlbum)

	if m.userEditedOverlay != nil {
		for _, u := range updates {
			m.userEditedOverlay[u.hash] = db.UserEditedRow{
				Hash: u.hash, Title: u.title, Artist: u.artist, Album: u.album, Genre: u.genre,
				TrackNumber: u.trackNumber, DiscNumber: u.discNumber,
			}
		}
	}

	mw := m.metadataWriter
	dw := m.durationWriter
	q := m.metadataWrites
	if mw != nil {
		for i := range updates {
			updates[i].ticket = q.Publish(updates[i].hash)
		}
	}
	return m, func() tea.Msg {
		var firstErr error
		for _, u := range updates {
			if mw != nil {
				_, err := q.Claim(u.hash, u.ticket, func() error {
					return mw.UpdateTrackMetadata(u.hash, u.title, u.artist, u.album, u.genre)
				})
				if err != nil {
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
