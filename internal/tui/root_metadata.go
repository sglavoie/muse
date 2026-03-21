package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
)

// metadataEditedMsg is sent after the async DB write completes.
type metadataEditedMsg struct {
	err error
}

// bulkSelectedHashes returns the hashes of all bulk-selected tracks for the
// active view. Returns nil if there is no bulk selection or the view doesn't
// support it.
func (m Model) bulkSelectedHashes() []string {
	switch m.activeView {
	case viewLibrary:
		if len(m.library.selectedHashes) == 0 {
			return nil
		}
		hashes := make([]string, 0, len(m.library.selectedHashes))
		for h := range m.library.selectedHashes {
			hashes = append(hashes, h)
		}
		return hashes
	case viewQueue:
		return m.queue.selectedHashes()
	case viewPlaylists:
		if len(m.playlists.selectedTrackIDs) == 0 {
			return nil
		}
		hashes := make([]string, 0, len(m.playlists.selectedTrackIDs))
		for _, t := range m.playlists.tracks {
			if _, ok := m.playlists.selectedTrackIDs[t.ID]; ok {
				hashes = append(hashes, t.Hash)
			}
		}
		return hashes
	default:
		return nil
	}
}

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

	// Bulk path: if multiple tracks are selected, open bulk editor.
	if hashes := m.bulkSelectedHashes(); len(hashes) >= 2 {
		var titles, artists, albums []string
		for _, h := range hashes {
			t, ok := m.library.index.FindByHash(h)
			if !ok {
				continue
			}
			titles = append(titles, t.Title)
			artists = append(artists, t.Artist)
			albums = append(albums, t.Album)
		}
		ed := NewBulkMetadataEditor(hashes, commonValue(titles), commonValue(artists), commonValue(albums))
		m.overlay.metadataEditor = &ed
		return m, nil, true
	}

	// Single-track path.
	track, ok := m.focusedTrackForPreview()
	if !ok {
		m.statusBar.notification = newNotification("No track to edit")
		return m, nil, true
	}
	// Enrich with album from index if missing (Queue/Playlists case).
	if track.Album == "" {
		if full, found := m.library.index.FindByHash(track.Hash); found {
			track.Album = full.Album
		}
	}
	ed := NewMetadataEditor(track.Hash, track.Title, track.Artist, track.Album)
	m.overlay.metadataEditor = &ed
	return m, nil, true
}

// commonValue returns the shared value if all strings are equal, or empty string otherwise.
func commonValue(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	first := vals[0]
	for _, v := range vals[1:] {
		if v != first {
			return ""
		}
	}
	return first
}

// handleMetadataSave reads values from the editor, updates in-memory state,
// and returns a command to persist the change to the database.
func (m Model) handleMetadataSave() (tea.Model, tea.Cmd) {
	ed := m.overlay.metadataEditor
	if ed.IsBulk() {
		return m.handleBulkMetadataSave()
	}

	hash := ed.Hash()
	title := ed.Title()
	artist := ed.Artist()
	album := ed.Album()

	m.overlay.metadataEditor = nil

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

// handleBulkMetadataSave saves dirty fields from the bulk metadata editor to
// all selected tracks, sets up an undo slot, and clears the selection.
func (m Model) handleBulkMetadataSave() (tea.Model, tea.Cmd) {
	ed := m.overlay.metadataEditor
	dirtyTitle, dirtyArtist, dirtyAlbum := ed.DirtyFields()
	m.overlay.metadataEditor = nil

	if dirtyTitle == nil && dirtyArtist == nil && dirtyAlbum == nil {
		m.statusBar.notification = newNotification("No changes")
		return m, nil
	}

	originals := make(map[string][3]string, len(ed.Hashes()))

	type update struct {
		hash, title, artist, album string
	}
	var updates []update

	for _, h := range ed.Hashes() {
		t, ok := m.library.index.FindByHash(h)
		if !ok {
			continue
		}
		originals[h] = [3]string{t.Title, t.Artist, t.Album}

		newTitle, newArtist, newAlbum := t.Title, t.Artist, t.Album
		if dirtyTitle != nil {
			newTitle = *dirtyTitle
		}
		if dirtyArtist != nil {
			newArtist = *dirtyArtist
		}
		if dirtyAlbum != nil {
			newAlbum = *dirtyAlbum
		}

		m.library.index.UpdateMetadata(h, newTitle, newArtist, newAlbum)

		display := fmt.Sprintf("%s – %s", newArtist, newTitle)
		for i := range m.queue.entries {
			if m.queue.entries[i].Hash == h {
				m.queue.entries[i].Title = newTitle
				m.queue.entries[i].Artist = newArtist
				m.queue.entries[i].Display = display
			}
		}

		updates = append(updates, update{h, newTitle, newArtist, newAlbum})
	}

	// Clear bulk selection on the active view.
	switch m.activeView {
	case viewLibrary:
		m.library.selectedHashes = nil
	case viewQueue:
		for i := range m.queue.entries {
			m.queue.entries[i].Selected = false
		}
	case viewPlaylists:
		m.playlists.selectedTrackIDs = nil
	}

	m.setUndoSlot(undoBulkEditMetadataAction{
		originals: originals,
	}, nil)

	w := m.metadataWriter
	if w == nil {
		return m, nil
	}
	return m, func() tea.Msg {
		var firstErr error
		for _, u := range updates {
			if err := w.UpdateTrackMetadata(u.hash, u.title, u.artist, u.album); err != nil {
				slog.Warn("bulk metadata edit: DB write failed", "hash", u.hash, "err", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
		return metadataEditedMsg{err: firstErr}
	}
}
