package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
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
	case viewRecentlyAdded:
		if len(m.recentlyAdded.selectedHashes) == 0 {
			return nil
		}
		hashes := make([]string, 0, len(m.recentlyAdded.selectedHashes))
		for h := range m.recentlyAdded.selectedHashes {
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
	case viewArtists:
		if len(m.artists.selectedHashes) == 0 {
			return nil
		}
		hashes := make([]string, 0, len(m.artists.selectedHashes))
		for h := range m.artists.selectedHashes {
			hashes = append(hashes, h)
		}
		return hashes
	case viewAlbums:
		if len(m.albums.selectedHashes) == 0 {
			return nil
		}
		hashes := make([]string, 0, len(m.albums.selectedHashes))
		for h := range m.albums.selectedHashes {
			hashes = append(hashes, h)
		}
		return hashes
	case viewFavorites:
		if len(m.favorites.selectedItems) == 0 {
			return nil
		}
		var items []musiclib.Track
		switch {
		case m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks:
			items = arttui.FilterTracks(m.favorites.tracks, m.favorites.filter.Text)
		case m.favorites.level == favtui.TrackLevel:
			items = arttui.FilterTracks(m.favorites.drilldownTracks, m.favorites.filter.Text)
		default:
			return nil // artist/album list levels have no track hashes
		}
		var hashes []string
		for idx := range m.favorites.selectedItems {
			if idx >= 0 && idx < len(items) {
				hashes = append(hashes, items[idx].Hash)
			}
		}
		if len(hashes) == 0 {
			return nil
		}
		return hashes
	case viewGenres:
		if len(m.genres.selectedHashes) == 0 {
			return nil
		}
		hashes := make([]string, 0, len(m.genres.selectedHashes))
		for h := range m.genres.selectedHashes {
			hashes = append(hashes, h)
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

	// Rename path: at artist or album level, open a focused rename editor.
	if ed, ok := m.renameEditorForFocusedEntity(); ok {
		m.overlay.metadataEditor = &ed
		return m, nil, true
	}

	// Single-track path.
	track, ok := m.focusedTrackForPreview()
	if !ok {
		m.statusBar.notification = newNotification("No track to edit")
		return m, nil, true
	}
	// Enrich album and genre from index if missing (Queue/Playlists case).
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

	// Validate duration before closing the editor.
	newDuration, err := parseDurationFromEditor(durStr)
	if err != nil {
		m.statusBar.notification = newNotification(fmt.Sprintf("Invalid duration: %s", err))
		return m, nil
	}

	m.overlay.metadataEditor = nil

	// Capture original values before mutation for undo support.
	var origTitle, origArtist, origAlbum, origGenre string
	var origDuration float64
	if t, ok := m.library.index.FindByHash(hash); ok {
		origTitle = t.Title
		origArtist = t.Artist
		origAlbum = t.Album
		origGenre = t.Genre
		origDuration = t.Duration
	}

	// Update in-memory library index.
	m.library.index.UpdateMetadata(hash, title, artist, album, genre)
	m.library.index.UpdateDuration(hash, newDuration)
	m = m.syncRecentlyAddedFromLibrary()

	// Update matching queue entries.
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

	// Update the status bar when the currently-playing track was edited.
	m = m.syncStatusBarMetadata(hash, artist, title)

	// Set up undo slot so the user can revert the single-track edit.
	m.setUndoSlot(undoBulkEditMetadataAction{
		originals: map[string][5]string{hash: {origTitle, origArtist, origAlbum, origGenre, formatDurationForEditor(origDuration)}},
	}, nil)

	// Refresh derived views (Artists, Albums) so they reflect the edit.
	m = m.refreshDerivedViews()

	// Keep the user-edited overlay in sync so future rescans/watcher
	// events preserve this edit in the in-memory index.
	if m.userEditedOverlay != nil {
		m.userEditedOverlay[hash] = db.UserEditedRow{
			Hash: hash, Title: title, Artist: artist, Album: album, Genre: genre,
		}
	}

	// Persist to database asynchronously.
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

// syncStatusBarMetadata updates the status bar artist and title when the
// currently-playing track (identified by hash) has been edited. This ensures
// the persistent left zone reflects renamed metadata immediately.
func (m Model) syncStatusBarMetadata(hash, artist, title string) Model {
	if m.playback.currentHash != "" && m.playback.currentHash == hash {
		m.statusBar.artist = artist
		m.statusBar.title = title
	}
	return m
}

// refreshDerivedViews updates the artists and albums lists derived from the
// current library index. Call after any metadata change that may have renamed
// artists, albums, or titles. Also rebuilds the library sort/filter caches so
// that active sort orders and filter matches reflect the updated metadata.
func (m Model) refreshDerivedViews() Model {
	tracks := m.library.index.Tracks()

	// Refresh the library view's sorted and filtered caches so that an
	// active sort order or filter query reflects the updated metadata.
	if m.library.sortState.Field != SortDefault {
		m.library = m.library.recomputeSortedTracks()
	}
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}

	switch m.artists.level {
	case arttui.ArtistLevel:
		m.artists.artists = arttui.DeriveArtists(tracks)
		m.artists = m.artists.recomputeArtistsSort()
		m.artists = m.artists.clampCursor()
	case arttui.AlbumLevel:
		m.artists.albums = arttui.DeriveAlbums(tracks, m.artists.selectedArtist)
		m.artists = m.artists.recomputeArtistsSort()
		m.artists = m.artists.clampCursor()
	case arttui.TrackLevel:
		m.artists.tracks = arttui.DeriveTracks(tracks, m.artists.selectedArtist, m.artists.selectedAlbum)
		m.artists = m.artists.recomputeArtistsSort()
		m.artists = m.artists.clampCursor()
	}
	switch m.albums.level {
	case albtui.AlbumLevel:
		m.albums.albums = albtui.DeriveAllAlbums(tracks)
		m.albums = m.albums.recomputeAlbumsSort()
		m.albums = m.albums.clampCursor()
	case albtui.TrackLevel:
		m.albums.tracks = albtui.DeriveAlbumTracks(tracks, m.albums.selectedAlbum)
		m.albums = m.albums.recomputeAlbumsSort()
		m.albums = m.albums.clampCursor()
	}
	switch m.genres.level {
	case gentui.GenreLevel:
		m.genres.genres = gentui.DeriveGenres(tracks)
		m.genres = m.genres.recomputeGenresSort()
		m.genres = m.genres.clampCursor()
	case gentui.ArtistLevel:
		m.genres.artists = gentui.DeriveGenreArtists(tracks, m.genres.selectedGenre)
		m.genres = m.genres.recomputeGenresSort()
		m.genres = m.genres.clampCursor()
	case gentui.TrackLevel:
		m.genres.tracks = gentui.DeriveGenreArtistTracks(tracks, m.genres.selectedGenre, m.genres.selectedArtist)
		m.genres = m.genres.recomputeGenresSort()
		m.genres = m.genres.clampCursor()
	}

	// Refresh playlist track metadata so the playlists view reflects edits.
	if m.playlists.level == pltui.TrackLevel {
		for i := range m.playlists.tracks {
			if t, ok := m.library.index.FindByHash(m.playlists.tracks[i].Hash); ok {
				m.playlists.tracks[i].Title = t.Title
				m.playlists.tracks[i].Artist = t.Artist
				m.playlists.tracks[i].Album = t.Album
			}
		}
		m.playlists = m.playlists.recomputePlaylistSort()
	}

	// Refresh favorites track metadata so the favorites view reflects edits.
	refreshTrackMetadata := func(ts []musiclib.Track) {
		for i := range ts {
			if t, ok := m.library.index.FindByHash(ts[i].Hash); ok {
				ts[i].Title = t.Title
				ts[i].Artist = t.Artist
				ts[i].Album = t.Album
				ts[i].Genre = t.Genre
			}
		}
	}
	if m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks {
		refreshTrackMetadata(m.favorites.tracks)
		m.favorites = m.favorites.recomputeFavoritesSort()
	}
	if m.favorites.level == favtui.TrackLevel {
		refreshTrackMetadata(m.favorites.drilldownTracks)
		m.favorites = m.favorites.recomputeFavoritesSort()
	}

	return m
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

	// Validate the duration string once before processing any tracks.
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

		// Update status bar when the currently-playing track was edited.
		m = m.syncStatusBarMetadata(h, newArtist, newTitle)

		updates = append(updates, update{h, newTitle, newArtist, newAlbum, newGenre, durationMs})
	}
	m = m.syncRecentlyAddedFromLibrary()

	// Clear bulk selection on the active view.
	switch m.activeView {
	case viewLibrary:
		m.library.selectedHashes = nil
	case viewRecentlyAdded:
		m.recentlyAdded.selectedHashes = nil
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
	}

	m.setUndoSlot(undoBulkEditMetadataAction{
		originals: originals,
	}, nil)

	m = m.refreshDerivedViews()

	// For rename mode, reposition the cursor on the new name and set a
	// rename-specific notification.
	switch ed.Mode() {
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
		n := len(ed.Hashes())
		m.statusBar.notification = newNotification(fmt.Sprintf("Renamed %d %s", n, pluralize(n, "track", "tracks")))
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
		n := len(ed.Hashes())
		m.statusBar.notification = newNotification(fmt.Sprintf("Renamed %d %s", n, pluralize(n, "track", "tracks")))
	}

	// Keep the user-edited overlay in sync for bulk edits.
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
