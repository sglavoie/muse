package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
)

// metadataUpdater persists user-edited metadata to the database.
type metadataUpdater interface {
	UpdateTrackMetadata(hash, title, artist, album string) error
}

// SetMetadataWriter sets the metadataUpdater used to persist metadata edits.
func (m *Model) SetMetadataWriter(w metadataUpdater) {
	m.metadataWriter = w
}

// metadataEditedMsg is sent after the async DB write completes.
type metadataEditedMsg struct {
	err error
}

// handleMetadataKeyAction handles actionEditMetadata.
func (m Model) handleMetadataKeyAction(a action) (Model, tea.Cmd, bool) {
	if a != actionEditMetadata {
		return m, nil, false
	}
	track, ok := m.focusedTrackForPreview()
	if !ok {
		m.statusBar.notification = transientNotification{
			msg:       "No track to edit",
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true
	}
	// Enrich with album from index if missing (Queue/Playlists case).
	if track.Album == "" {
		if full, found := m.library.index.FindByHash(track.Hash); found {
			track.Album = full.Album
		}
	}
	ed := NewMetadataEditor(track.Hash, track.Title, track.Artist, track.Album)
	m.metadataEditor = &ed
	return m, nil, true
}

// handleMetadataSave reads values from the editor, updates in-memory state,
// and returns a command to persist the change to the database.
func (m Model) handleMetadataSave() (tea.Model, tea.Cmd) {
	ed := m.metadataEditor
	hash := ed.Hash()
	title := ed.Title()
	artist := ed.Artist()
	album := ed.Album()

	m.metadataEditor = nil

	// Update in-memory library index.
	m.library.index.UpdateMetadata(hash, title, artist, album)

	// Update matching queue entries.
	display := fmt.Sprintf("%s – %s", artist, title)
	for i := range m.queue.entries {
		if m.queue.entries[i].Hash == hash {
			m.queue.entries[i].Title = title
			m.queue.entries[i].Artist = artist
			m.queue.entries[i].Display = display
		}
	}

	// Persist to database asynchronously.
	w := m.metadataWriter
	if w == nil {
		return m, nil
	}
	return m, func() tea.Msg {
		err := w.UpdateTrackMetadata(hash, title, artist, album)
		if err != nil {
			slog.Warn("metadata edit: DB write failed", "hash", hash, "err", err)
		}
		return metadataEditedMsg{err: err}
	}
}
