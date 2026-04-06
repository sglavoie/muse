package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// undoAction is the interface implemented by every reversible operation.
// description() returns the "press u to undo" status-bar notification shown
// when the action is created. perform() executes the actual undo logic.
type undoAction interface {
	kind() undoKind
	perform(m Model) (Model, tea.Cmd)
	description() string
}

// --- undoEnqueueAction ---

// undoEnqueueAction reverts a single-track enqueue (undoKindEnqueue).
type undoEnqueueAction struct {
	id    int64
	entry QueueEntry
	idx   int
}

func (a undoEnqueueAction) kind() undoKind { return undoKindEnqueue }

func (a undoEnqueueAction) description() string {
	return "Added " + a.entry.Display + " — press u to undo"
}

func (a undoEnqueueAction) perform(m Model) (Model, tea.Cmd) {
	// Find the enqueued entry by its database ID rather than by positional
	// index, because the queue may have been reordered since the enqueue.
	realIdx := -1
	for i, e := range m.queue.entries {
		if e.ID == a.id {
			realIdx = i
			break
		}
	}
	if realIdx >= 0 {
		if m.undoer != nil {
			if err := m.undoer.DequeueTrackSoft(a.id); err != nil {
				slog.Warn("undo enqueue: soft-delete failed", "err", err)
			}
			schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
		}
		m.queue.entries = append(m.queue.entries[:realIdx], m.queue.entries[realIdx+1:]...)
		m.clampQueueCursorAndScroll()
		if qr, ok := m.player.(playerQueueRemover); ok {
			_ = qr.RemoveFromQueue(realIdx) // player queue sync during undo, best-effort
		}
		// Adjust currentIndex after the removal.
		if realIdx < m.playback.currentIndex {
			m.playback.currentIndex--
		} else if realIdx == m.playback.currentIndex {
			m.playback.currentIndex = -1
		}
		// If the queue is now empty or the currently-playing track was the
		// one removed, stop playback so the status bar doesn't show a stale
		// track that is no longer in the queue.
		if len(m.queue.entries) == 0 || (m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash)) {
			m = m.resetPlaybackState()
		}
	}
	m.statusBar.notification = newNotification("Undone: removed " + a.entry.Display)
	return m, nil
}

// --- undoDequeueAction ---

// undoDequeueAction reverts a single-track dequeue (undoKindDequeue).
type undoDequeueAction struct {
	id    int64
	entry QueueEntry
	idx   int
}

func (a undoDequeueAction) kind() undoKind { return undoKindDequeue }

func (a undoDequeueAction) description() string {
	return "Track removed — press u to undo"
}

func (a undoDequeueAction) perform(m Model) (Model, tea.Cmd) {
	if m.undoer != nil {
		if err := m.undoer.UndoDequeue(a.id); err != nil {
			slog.Warn("undo dequeue failed", "err", err)
		}
	}
	idx := min(a.idx, len(m.queue.entries))
	entries := make([]QueueEntry, len(m.queue.entries)+1)
	copy(entries, m.queue.entries[:idx])
	entries[idx] = a.entry
	copy(entries[idx+1:], m.queue.entries[idx:])
	m.queue.entries = entries
	if m.playback.currentIndex >= 0 && idx <= m.playback.currentIndex {
		m.playback.currentIndex++
	}
	m.queue.cursor = idx
	// When a filter is active the cursor must index into the filtered
	// results, not the underlying entries slice. Clamp it so it stays
	// valid even when the restored entry does not match the filter text.
	n := len(m.queue.filteredEntries())
	if n == 0 {
		m.queue.cursor = 0
		m.queue.scrollOffset = 0
	} else if m.queue.cursor >= n {
		m.queue.cursor = n - 1
	}
	if m.queue.cursor < m.queue.scrollOffset {
		m.queue.scrollOffset = m.queue.cursor
	} else {
		maxRows := m.queue.visibleRows()
		if m.queue.cursor >= m.queue.scrollOffset+maxRows {
			m.queue.scrollOffset = m.queue.cursor - maxRows + 1
		}
	}
	m.syncPlayerQueue()
	m.statusBar.notification = newNotification("Undone: restored removed track")
	return m, nil
}

// --- undoClearAction ---

// undoClearAction reverts a full queue clear (undoKindClear).
type undoClearAction struct {
	ids     []int64
	entries []QueueEntry
	tracks  []player.QueueTrack
}

func (a undoClearAction) kind() undoKind { return undoKindClear }

func (a undoClearAction) description() string {
	return "Queue cleared — press u to undo"
}

func (a undoClearAction) perform(m Model) (Model, tea.Cmd) {
	if m.undoer != nil {
		if err := m.undoer.UndoClearQueue(a.ids); err != nil {
			slog.Warn("undo clear queue failed", "err", err)
		}
	}
	m.queue.entries = a.entries
	if qs, ok := m.player.(playerQueueSetter); ok {
		qs.SetQueue(a.tracks)
	}
	n := len(a.entries)
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: restored %d %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// --- undoBulkDequeueAction ---

// undoBulkDequeueAction reverts removal of multiple selected queue entries
// (undoKindBulkDequeue).
type undoBulkDequeueAction struct {
	entries []QueueEntry
	indices []int
	ids     []int64
}

func (a undoBulkDequeueAction) kind() undoKind { return undoKindBulkDequeue }

func (a undoBulkDequeueAction) description() string {
	n := len(a.entries)
	return fmt.Sprintf("%d %s removed — press u to undo", n, pluralize(n, "track", "tracks"))
}

func (a undoBulkDequeueAction) perform(m Model) (Model, tea.Cmd) {
	if m.undoer != nil {
		for _, id := range a.ids {
			if err := m.undoer.UndoDequeue(id); err != nil {
				slog.Warn("undo bulk dequeue failed", "err", err)
			}
		}
	}
	for i := range a.indices {
		idx := min(a.indices[i], len(m.queue.entries))
		entries := make([]QueueEntry, len(m.queue.entries)+1)
		copy(entries, m.queue.entries[:idx])
		entries[idx] = a.entries[i]
		copy(entries[idx+1:], m.queue.entries[idx:])
		m.queue.entries = entries
		if m.playback.currentIndex >= 0 && idx <= m.playback.currentIndex {
			m.playback.currentIndex++
		}
	}
	m.clampQueueCursorAndScroll()
	m.syncPlayerQueue()
	n := len(a.entries)
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: restored %d %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// --- undoBatchEnqueueAction ---

// undoBatchEnqueueAction reverts a batch enqueue from a playlist
// (undoKindBatchEnqueue).
type undoBatchEnqueueAction struct {
	ids          []int64
	count        int
	startIdx     int
	playlistName string
}

func (a undoBatchEnqueueAction) kind() undoKind { return undoKindBatchEnqueue }

func (a undoBatchEnqueueAction) description() string {
	return fmt.Sprintf("Added %d %s from %s — press u to undo",
		a.count, pluralize(a.count, "track", "tracks"), a.playlistName)
}

func (a undoBatchEnqueueAction) perform(m Model) (Model, tea.Cmd) {
	if m.batchEnqueuer != nil {
		if err := m.batchEnqueuer.DequeueTracksSoft(a.ids); err != nil {
			slog.Warn("undo batch enqueue: soft-delete failed", "err", err)
		}
		if m.undoer != nil {
			schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
		}
	}
	// Build a set of IDs to remove so we match the correct entries even if
	// the queue was reordered after the batch enqueue.
	idSet := make(map[int64]struct{}, len(a.ids))
	for _, id := range a.ids {
		idSet[id] = struct{}{}
	}
	var kept []QueueEntry
	var removedPlayerIndices []int
	for i, e := range m.queue.entries {
		if _, ok := idSet[e.ID]; ok {
			removedPlayerIndices = append(removedPlayerIndices, i)
		} else {
			kept = append(kept, e)
		}
	}
	m.queue.entries = kept
	m.clampQueueCursorAndScroll()
	// Recompute currentIndex: count how many removed entries were before it.
	if m.playback.currentIndex >= 0 {
		removedBefore := 0
		removedCurrent := false
		for _, ri := range removedPlayerIndices {
			if ri < m.playback.currentIndex {
				removedBefore++
			} else if ri == m.playback.currentIndex {
				removedCurrent = true
			}
		}
		if removedCurrent {
			m.playback.currentIndex = -1
		} else {
			m.playback.currentIndex -= removedBefore
		}
	}
	if qr, ok := m.player.(playerQueueRemover); ok {
		// Remove in reverse order so indices stay valid.
		for i := len(removedPlayerIndices) - 1; i >= 0; i-- {
			_ = qr.RemoveFromQueue(removedPlayerIndices[i])
		}
	}
	// If the queue is now empty or the currently-playing track was among
	// the removed entries, stop playback so the status bar doesn't show a
	// stale track that is no longer in the queue.
	if len(m.queue.entries) == 0 || (m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash)) {
		m = m.resetPlaybackState()
	}
	n := a.count
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: removed %d %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// --- undoPasteAction ---

// undoPasteAction reverts a queue paste by removing the pasted entries
// (undoKindPaste). It mirrors undoBatchEnqueueAction: soft-deletes the pasted
// IDs and removes matching entries from the in-memory queue.
type undoPasteAction struct {
	ids   []int64
	count int
}

func (a undoPasteAction) kind() undoKind { return undoKindPaste }

func (a undoPasteAction) description() string {
	return fmt.Sprintf("Pasted %d %s — press u to undo", a.count, pluralize(a.count, "track", "tracks"))
}

func (a undoPasteAction) perform(m Model) (Model, tea.Cmd) {
	if m.batchEnqueuer != nil {
		if err := m.batchEnqueuer.DequeueTracksSoft(a.ids); err != nil {
			slog.Warn("undo paste: soft-delete failed", "err", err)
		}
		if m.undoer != nil {
			schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
		}
	}
	idSet := make(map[int64]struct{}, len(a.ids))
	for _, id := range a.ids {
		idSet[id] = struct{}{}
	}
	var kept []QueueEntry
	var removedPlayerIndices []int
	for i, e := range m.queue.entries {
		if _, ok := idSet[e.ID]; ok {
			removedPlayerIndices = append(removedPlayerIndices, i)
		} else {
			kept = append(kept, e)
		}
	}
	m.queue.entries = kept
	m.clampQueueCursorAndScroll()
	if m.playback.currentIndex >= 0 {
		removedBefore := 0
		removedCurrent := false
		for _, ri := range removedPlayerIndices {
			if ri < m.playback.currentIndex {
				removedBefore++
			} else if ri == m.playback.currentIndex {
				removedCurrent = true
			}
		}
		if removedCurrent {
			m.playback.currentIndex = -1
		} else {
			m.playback.currentIndex -= removedBefore
		}
	}
	if qr, ok := m.player.(playerQueueRemover); ok {
		for i := len(removedPlayerIndices) - 1; i >= 0; i-- {
			_ = qr.RemoveFromQueue(removedPlayerIndices[i])
		}
	}
	if len(m.queue.entries) == 0 || (m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash)) {
		m = m.resetPlaybackState()
	}
	n := a.count
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: removed %d pasted %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// --- undoDeletePlaylistAction ---

// undoDeletePlaylistAction reverts a playlist soft-delete (undoKindDeletePlaylist).
type undoDeletePlaylistAction struct {
	entry db.PlaylistRow
	idx   int
}

func (a undoDeletePlaylistAction) kind() undoKind { return undoKindDeletePlaylist }

func (a undoDeletePlaylistAction) description() string {
	return "Playlist deleted — press u to undo"
}

func (a undoDeletePlaylistAction) perform(m Model) (Model, tea.Cmd) {
	if m.playlistDeleter != nil {
		if err := m.playlistDeleter.UndoDeletePlaylist(a.entry.ID); err != nil {
			slog.Warn("undo delete playlist failed", "err", err)
		}
	}
	idx := min(a.idx, len(m.playlists.playlists))
	playlists := make([]db.PlaylistRow, len(m.playlists.playlists)+1)
	copy(playlists, m.playlists.playlists[:idx])
	playlists[idx] = a.entry
	copy(playlists[idx+1:], m.playlists.playlists[idx:])
	m.playlists.playlists = playlists
	m.playlists.cursor = idx
	m.playlists = m.playlists.ensureCursorVisible()
	m.statusBar.notification = newNotification("Undone: restored playlist")
	return m, nil
}

// --- undoRemovePlaylistTrackAction ---

// undoRemovePlaylistTrackAction reverts removal of a single playlist track
// (undoKindRemovePlaylistTrack).
type undoRemovePlaylistTrackAction struct {
	entry db.PlaylistTrackRow
	idx   int
}

func (a undoRemovePlaylistTrackAction) kind() undoKind { return undoKindRemovePlaylistTrack }

func (a undoRemovePlaylistTrackAction) description() string {
	return "Track removed — press u to undo"
}

func (a undoRemovePlaylistTrackAction) perform(m Model) (Model, tea.Cmd) {
	if m.playlistDeleter != nil {
		if err := m.playlistDeleter.UndoRemovePlaylistTrack(a.entry.ID); err != nil {
			slog.Warn("undo remove playlist track failed", "err", err)
		}
	}
	idx := min(a.idx, len(m.playlists.tracks))
	tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks)+1)
	copy(tracks, m.playlists.tracks[:idx])
	tracks[idx] = a.entry
	copy(tracks[idx+1:], m.playlists.tracks[idx:])
	m.playlists.tracks = tracks
	m.playlists.cursor = idx
	m.playlists = m.playlists.ensureCursorVisible()
	m.statusBar.notification = newNotification("Undone: restored playlist track")
	return m, nil
}

// --- undoBulkRemovePlaylistTracksAction ---

// undoBulkRemovePlaylistTracksAction reverts removal of multiple selected
// playlist tracks (undoKindBulkRemovePlaylistTracks).
type undoBulkRemovePlaylistTracksAction struct {
	entries []db.PlaylistTrackRow
	indices []int
}

func (a undoBulkRemovePlaylistTracksAction) kind() undoKind {
	return undoKindBulkRemovePlaylistTracks
}

func (a undoBulkRemovePlaylistTracksAction) description() string {
	n := len(a.entries)
	return fmt.Sprintf("%d %s removed — press u to undo", n, pluralize(n, "track", "tracks"))
}

func (a undoBulkRemovePlaylistTracksAction) perform(m Model) (Model, tea.Cmd) {
	if m.playlistDeleter != nil {
		for _, t := range a.entries {
			if err := m.playlistDeleter.UndoRemovePlaylistTrack(t.ID); err != nil {
				slog.Warn("undo bulk remove playlist track failed", "err", err)
			}
		}
	}
	for i := range a.indices {
		idx := min(a.indices[i], len(m.playlists.tracks))
		tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks)+1)
		copy(tracks, m.playlists.tracks[:idx])
		tracks[idx] = a.entries[i]
		copy(tracks[idx+1:], m.playlists.tracks[idx:])
		m.playlists.tracks = tracks
	}
	if m.playlists.cursor < 0 {
		m.playlists.cursor = 0
	} else if m.playlists.cursor >= len(m.playlists.tracks) {
		m.playlists.cursor = len(m.playlists.tracks) - 1
	}
	m.playlists = m.playlists.ensureCursorVisible()
	n := len(a.entries)
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: restored %d playlist %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// --- undoBulkEditMetadataAction ---

// undoBulkEditMetadataAction reverts a bulk metadata edit (undoKindBulkEditMetadata).
// originals maps track hash → {title, artist, album, genre, duration (M:SS or "")}.
type undoBulkEditMetadataAction struct {
	originals map[string][5]string
}

func (a undoBulkEditMetadataAction) kind() undoKind { return undoKindBulkEditMetadata }

func (a undoBulkEditMetadataAction) description() string {
	n := len(a.originals)
	return fmt.Sprintf("Updated %d %s · u to undo", n, pluralize(n, "track", "tracks"))
}

func (a undoBulkEditMetadataAction) perform(m Model) (Model, tea.Cmd) {
	mw := m.metadataWriter
	dw := m.durationWriter
	for hash, orig := range a.originals {
		m.library.index.UpdateMetadata(hash, orig[0], orig[1], orig[2], orig[3])
		origDur, _ := parseDurationFromEditor(orig[4])
		m.library.index.UpdateDuration(hash, origDur)
		display := trackDisplay(orig[1], orig[0], "")
		origDurMs := int64(origDur * 1000)
		for i := range m.queue.entries {
			if m.queue.entries[i].Hash == hash {
				m.queue.entries[i].Title = orig[0]
				m.queue.entries[i].Artist = orig[1]
				m.queue.entries[i].Album = orig[2]
				m.queue.entries[i].Display = display
				m.queue.entries[i].DurationMs = origDurMs
			}
		}
		// Update status bar when the currently-playing track was reverted.
		m = m.syncStatusBarMetadata(hash, orig[1], orig[0])
	}
	m = m.syncRecentlyAddedFromLibrary()
	m = m.refreshDerivedViews()
	if mw != nil || dw != nil {
		originals := a.originals
		go func() {
			for h, orig := range originals {
				if mw != nil {
					if err := mw.UpdateTrackMetadata(h, orig[0], orig[1], orig[2], orig[3]); err != nil {
						slog.Warn("undo bulk metadata edit: DB write failed", "hash", h, "err", err)
					}
				}
				if dw != nil {
					origDur, _ := parseDurationFromEditor(orig[4])
					origDurMs := int64(origDur * 1000)
					if err := dw.UpdateTrackDuration(h, origDurMs); err != nil {
						slog.Warn("undo bulk metadata edit: duration DB write failed", "hash", h, "err", err)
					}
				}
			}
		}()
	}
	n := len(a.originals)
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone metadata edit for %d %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// --- undoRemoveFavoriteAction ---

// undoRemoveFavoriteAction reverts removal of a single favorite
// (undoKindRemoveFavorite).
type undoRemoveFavoriteAction struct {
	typ      string
	value    string
	track    library.Track
	idx      int
	level    favtui.Level
	category favtui.Category
}

func (a undoRemoveFavoriteAction) kind() undoKind { return undoKindRemoveFavorite }

func (a undoRemoveFavoriteAction) description() string {
	return fmt.Sprintf("Removed %s from favorites — press u to undo", a.typ)
}

func (a undoRemoveFavoriteAction) perform(m Model) (Model, tea.Cmd) {
	if m.favoriteWriter != nil {
		if err := m.favoriteWriter.AddFavorite(a.typ, a.value); err != nil {
			slog.Warn("undo remove favorite failed", "err", err)
		}
	}
	if a.typ == "track" && a.value != "" {
		if m.favoriteHashes == nil {
			m.favoriteHashes = make(map[string]struct{})
		}
		m.favoriteHashes[a.value] = struct{}{}
	}
	if a.typ == "artist" && a.value != "" {
		if m.favoriteArtists == nil {
			m.favoriteArtists = make(map[string]struct{})
		}
		m.favoriteArtists[a.value] = struct{}{}
	}
	if a.typ == "album" && a.value != "" {
		if m.favoriteAlbums == nil {
			m.favoriteAlbums = make(map[string]struct{})
		}
		m.favoriteAlbums[a.value] = struct{}{}
	}
	if m.activeView == viewFavorites && a.idx >= 0 &&
		m.favorites.level == a.level {
		switch a.level {
		case favtui.ListLevel:
			switch a.category {
			case favtui.CategoryArtists:
				m.favorites.artists = insertStringAt(m.favorites.artists, a.idx, a.value)
				m.favorites.artistCount = len(m.favorites.artists)
			case favtui.CategoryAlbums:
				m.favorites.albums = insertStringAt(m.favorites.albums, a.idx, a.value)
				m.favorites.albumCount = len(m.favorites.albums)
			case favtui.CategoryTracks:
				m.favorites.tracks = insertTrackAt(m.favorites.tracks, a.idx, a.track)
				m.favorites.trackCount = len(m.favorites.tracks)
			}
			m.favorites.cursor = a.idx
			m.favorites = m.favorites.clampCursor()
		case favtui.TrackLevel:
			m.favorites.drilldownTracks = insertTrackAt(m.favorites.drilldownTracks, a.idx, a.track)
			m.favorites.cursor = a.idx
			m.favorites = m.favorites.clampCursor()
		}
	}
	if m.activeView == viewFavorites && m.favoriteReader != nil {
		ac, al, t, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.artistCount = ac
			m.favorites.albumCount = al
			m.favorites.trackCount = t
		}
	}
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: added %s back to favorites", a.typ))
	return m, nil
}

// --- undoAddFavoriteAction ---

// undoAddFavoriteAction reverts a single favorite addition (undoKindAddFavorite).
type undoAddFavoriteAction struct {
	typ   string
	value string
}

func (a undoAddFavoriteAction) kind() undoKind { return undoKindAddFavorite }

func (a undoAddFavoriteAction) description() string {
	return fmt.Sprintf("Added %s to favorites — press u to undo", a.typ)
}

func (a undoAddFavoriteAction) perform(m Model) (Model, tea.Cmd) {
	if m.favoriteWriter != nil {
		if err := m.favoriteWriter.RemoveFavorite(a.typ, a.value); err != nil {
			slog.Warn("undo add favorite failed", "err", err)
		}
	}
	if a.typ == "track" && a.value != "" {
		delete(m.favoriteHashes, a.value)
	}
	if a.typ == "artist" && a.value != "" {
		delete(m.favoriteArtists, a.value)
	}
	if a.typ == "album" && a.value != "" {
		delete(m.favoriteAlbums, a.value)
	}
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: removed %s from favorites", a.typ))
	return m, nil
}

// --- undoBulkAddFavoritesAction ---

// undoBulkAddFavoritesAction reverts a bulk favorite addition
// (undoKindBulkAddFavorites).
type undoBulkAddFavoritesAction struct {
	typ    string
	values []string
}

func (a undoBulkAddFavoritesAction) kind() undoKind { return undoKindBulkAddFavorites }

func (a undoBulkAddFavoritesAction) description() string {
	n := len(a.values)
	return fmt.Sprintf("Added %d %ss to favorites — press u to undo", n, a.typ)
}

func (a undoBulkAddFavoritesAction) perform(m Model) (Model, tea.Cmd) {
	if m.favoriteWriter != nil {
		if err := m.favoriteWriter.RemoveFavoritesByTypeAndValues(a.typ, a.values); err != nil {
			slog.Warn("undo bulk add favorites failed", "err", err)
		}
	}
	if a.typ == "track" {
		for _, v := range a.values {
			delete(m.favoriteHashes, v)
		}
	}
	if a.typ == "artist" {
		for _, v := range a.values {
			delete(m.favoriteArtists, v)
		}
	}
	if a.typ == "album" {
		for _, v := range a.values {
			delete(m.favoriteAlbums, v)
		}
	}
	n := len(a.values)
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: removed %d %s(s) from favorites", n, a.typ))
	return m, nil
}

// --- undoTagToggleAction ---

// undoTagToggleAction reverts a tag assignment or removal (undoKindTagToggle).
// wasAdd is true when the original action added the tag; undo removes it.
// wasAdd is false when the original action removed the tag; undo re-adds it.
type undoTagToggleAction struct {
	tagID  int64
	hashes []string
	wasAdd bool
}

func (a undoTagToggleAction) kind() undoKind { return undoKindTagToggle }

func (a undoTagToggleAction) description() string {
	n := len(a.hashes)
	if a.wasAdd {
		return fmt.Sprintf("Tag added to %d %s — press u to undo", n, pluralize(n, "track", "tracks"))
	}
	return fmt.Sprintf("Tag removed from %d %s — press u to undo", n, pluralize(n, "track", "tracks"))
}

func (a undoTagToggleAction) perform(m Model) (Model, tea.Cmd) {
	tw := m.tagWriter
	if tw == nil {
		return m, nil
	}
	tagID, hashes, wasAdd := a.tagID, a.hashes, a.wasAdd
	return m, func() tea.Msg {
		var err error
		if wasAdd {
			// Original was add → undo removes.
			err = tw.RemoveTrackTagBulk(hashes, tagID)
		} else {
			// Original was remove → undo adds.
			err = tw.AddTrackTagBulk(hashes, tagID)
		}
		return tagToggledMsg{tagID: tagID, hashes: hashes, wasAdd: !wasAdd, err: err}
	}
}

// --- undoDeleteTagAction ---

// undoDeleteTagAction reverts a tag soft-delete (undoKindDeleteTag).
// Calls UndoDeleteTag to clear deleted_at, then restores the tag in the
// in-memory tag list and cache.
type undoDeleteTagAction struct {
	tag db.TagRow
	idx int // original position in the effective list
}

func (a undoDeleteTagAction) kind() undoKind { return undoKindDeleteTag }

func (a undoDeleteTagAction) description() string {
	return fmt.Sprintf("Tag %q deleted — press u to undo", a.tag.Name)
}

func (a undoDeleteTagAction) perform(m Model) (Model, tea.Cmd) {
	tw := m.tagWriter
	if tw != nil {
		if err := tw.UndoDeleteTag(a.tag.ID); err != nil {
			slog.Warn("undo delete tag: UndoDeleteTag failed", "id", a.tag.ID, "err", err)
		}
	}
	// Restore in the in-memory tag list.
	idx := min(a.idx, len(m.tags))
	restored := make([]db.TagRow, len(m.tags)+1)
	copy(restored, m.tags[:idx])
	restored[idx] = a.tag
	copy(restored[idx+1:], m.tags[idx:])
	m = m.rebuildTagCache(restored)

	// Also restore in the tagsView tag list.
	tvIdx := min(a.idx, len(m.tagsView.tags))
	tvTags := make([]db.TagRow, len(m.tagsView.tags)+1)
	copy(tvTags, m.tagsView.tags[:tvIdx])
	tvTags[tvIdx] = a.tag
	copy(tvTags[tvIdx+1:], m.tagsView.tags[tvIdx:])
	m.tagsView.tags = tvTags
	m.tagsView.cursor = tvIdx
	m.tagsView = m.tagsView.clampCursor()

	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: restored tag %q", a.tag.Name))
	return m, nil
}

// --- undoBulkRemoveFavoritesAction ---

// undoBulkRemoveFavoritesAction reverts bulk removal of favorites
// (undoKindBulkRemoveFavorites).
type undoBulkRemoveFavoritesAction struct {
	typ      string
	values   []string
	tracks   []library.Track
	indices  []int
	level    favtui.Level
	category favtui.Category
}

func (a undoBulkRemoveFavoritesAction) kind() undoKind { return undoKindBulkRemoveFavorites }

func (a undoBulkRemoveFavoritesAction) description() string {
	n := len(a.values)
	return fmt.Sprintf("Removed %d %ss from favorites — press u to undo", n, a.typ)
}

func (a undoBulkRemoveFavoritesAction) perform(m Model) (Model, tea.Cmd) {
	if m.favoriteWriter != nil {
		for _, v := range a.values {
			if err := m.favoriteWriter.AddFavorite(a.typ, v); err != nil {
				slog.Warn("undo bulk remove favorite failed", "err", err)
			}
		}
	}
	if a.typ == "track" {
		if m.favoriteHashes == nil {
			m.favoriteHashes = make(map[string]struct{})
		}
		for _, v := range a.values {
			m.favoriteHashes[v] = struct{}{}
		}
	}
	if a.typ == "artist" {
		if m.favoriteArtists == nil {
			m.favoriteArtists = make(map[string]struct{})
		}
		for _, v := range a.values {
			m.favoriteArtists[v] = struct{}{}
		}
	}
	if a.typ == "album" {
		if m.favoriteAlbums == nil {
			m.favoriteAlbums = make(map[string]struct{})
		}
		for _, v := range a.values {
			m.favoriteAlbums[v] = struct{}{}
		}
	}
	if m.activeView == viewFavorites && len(a.indices) > 0 &&
		m.favorites.level == a.level {
		switch a.level {
		case favtui.ListLevel:
			switch a.category {
			case favtui.CategoryArtists:
				for i, idx := range a.indices {
					m.favorites.artists = insertStringAt(m.favorites.artists, min(idx, len(m.favorites.artists)), a.values[i])
				}
				m.favorites.artistCount = len(m.favorites.artists)
			case favtui.CategoryAlbums:
				for i, idx := range a.indices {
					m.favorites.albums = insertStringAt(m.favorites.albums, min(idx, len(m.favorites.albums)), a.values[i])
				}
				m.favorites.albumCount = len(m.favorites.albums)
			case favtui.CategoryTracks:
				for i, idx := range a.indices {
					m.favorites.tracks = insertTrackAt(m.favorites.tracks, min(idx, len(m.favorites.tracks)), a.tracks[i])
				}
				m.favorites.trackCount = len(m.favorites.tracks)
			}
			m.favorites = m.favorites.clampCursor()
		case favtui.TrackLevel:
			for i, idx := range a.indices {
				m.favorites.drilldownTracks = insertTrackAt(m.favorites.drilldownTracks, min(idx, len(m.favorites.drilldownTracks)), a.tracks[i])
			}
			m.favorites = m.favorites.clampCursor()
		}
	}
	if m.activeView == viewFavorites && m.favoriteReader != nil {
		ac, al, t, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.artistCount = ac
			m.favorites.albumCount = al
			m.favorites.trackCount = t
		}
	}
	n := len(a.values)
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: added %d %s(s) back to favorites", n, a.typ))
	return m, nil
}
