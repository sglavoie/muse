package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
	musiclib "github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleTagPickerKeyPress intercepts key presses when the tag picker is open.
func (m Model) handleTagPickerKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.tagPicker == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.tagPicker.HandleKey(msg.String(), msg)
	m.overlay.tagPicker = &updated
	return m, cmd, true
}

// openTagPicker opens the tag picker overlay for the focused track (single
// mode) or for the current bulk selection. Returns without opening when there
// is no focused track and no bulk selection.
func (m Model) openTagPicker() (Model, tea.Cmd) {
	hashes, isBulk := m.tagPickerHashes()
	if len(hashes) == 0 {
		m.statusBar.notification = newNotification("No track focused")
		return m, nil
	}

	var tp TagPicker
	var cmd tea.Cmd

	if isBulk {
		tp, cmd = NewTagPickerBulk(m.tags, hashes, m.trackTags)
	} else {
		trackTagIDs := m.trackTags[hashes[0]]
		if trackTagIDs == nil {
			trackTagIDs = map[int64]struct{}{}
		}
		tp, cmd = NewTagPicker(m.tags, hashes[0], trackTagIDs)
	}

	m.overlay.tagPicker = &tp
	return m, cmd
}

// tagPickerHashes returns the hashes the tag picker should operate on.
// isBulk is true when the returned hashes came from a multi-track selection.
func (m Model) tagPickerHashes() (hashes []string, isBulk bool) {
	// Check active visual/bulk selection first.
	selected := m.selectedTrackHashes()
	if len(selected) > 1 {
		return selected, true
	}
	// Fall back to focused track.
	if hash := m.focusedTrackHash(); hash != "" {
		return []string{hash}, false
	}
	return nil, false
}

// selectedTrackHashes returns hashes for all currently bulk-selected tracks
// across whichever view supports selection.
func (m Model) selectedTrackHashes() []string {
	switch m.activeView {
	case viewLibrary:
		return hashMapToSlice(m.library.selectedHashes)
	case viewRecentlyAdded:
		return hashMapToSlice(m.recentlyAdded.selectedHashes)
	case viewHistory:
		return hashMapToSlice(m.history.selectedHashes)
	}
	return nil
}

// hashMapToSlice converts a hash set to an ordered slice.
func hashMapToSlice(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	s := make([]string, 0, len(m))
	for k := range m {
		s = append(s, k)
	}
	return s
}

// focusedTrackHash returns the content hash of the track currently under the
// cursor, or "" when the cursor is not on a track row.
func (m Model) focusedTrackHash() string {
	switch m.activeView {
	case viewLibrary:
		if track, ok := m.library.view().FocusedTrack(); ok {
			return track.Hash
		}
	case viewRecentlyAdded:
		if track, ok := m.recentlyAdded.view().FocusedTrack(); ok {
			return track.Hash
		}
	case viewHistory:
		if track, ok := m.history.view().FocusedTrack(); ok {
			return track.Hash
		}
	case viewQueue:
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			return filtered[m.queue.cursor].Hash
		}
	case viewArtists:
		if m.artists.level == arttui.TrackLevel {
			if track, ok := m.artists.focusedTrack(); ok {
				return track.Hash
			}
		}
	case viewAlbums:
		if m.albums.level == albtui.TrackLevel {
			if track, ok := m.albums.focusedTrack(); ok {
				return track.Hash
			}
		}
	case viewPlaylists:
		if track, ok := m.playlists.focusedTrack(); ok {
			return track.Hash
		}
	case viewFavorites:
		switch m.favorites.level {
		case favtui.ListLevel:
			if m.favorites.category == favtui.CategoryTracks {
				if track, ok := m.favorites.focusedTrack(); ok {
					return track.Hash
				}
			}
		case favtui.TrackLevel:
			if track, ok := m.favorites.focusedTrack(); ok {
				return track.Hash
			}
		}
	case viewGenres:
		if m.genres.level == gentui.TrackLevel {
			if track, ok := m.genres.focusedTrack(); ok {
				return track.Hash
			}
		}
	case viewTags:
		if m.tagsView.level == tagtui.TrackLevel {
			if track, ok := m.tagsView.focusedTrack(); ok {
				return track.Hash
			}
		}
	}
	return ""
}

// handleTagPickerToggle processes TagPickerToggleMsg: writes to DB, updates
// the in-memory cache, updates the picker check state, and arms undo.
func (m Model) handleTagPickerToggle(msg TagPickerToggleMsg) (Model, tea.Cmd) {
	tw := m.tagWriter
	if tw == nil {
		return m, nil
	}

	tagID, hashes, add := msg.TagID, msg.Hashes, msg.Add

	// Optimistic in-memory update.
	m = m.applyTagToggle(tagID, hashes, add)

	// Update picker check state if still open.
	if m.overlay.tagPicker != nil {
		var newState checkState
		if add {
			newState = checkChecked
		} else {
			newState = checkUnchecked
		}
		updated := m.overlay.tagPicker.UpdateItemState(tagID, newState)
		m.overlay.tagPicker = &updated
	}

	// Arm undo.
	m.setUndoSlot(undoTagToggleAction{tagID: tagID, hashes: hashes, wasAdd: add}, nil)

	// Persist asynchronously.
	return m, func() tea.Msg {
		var err error
		if add {
			if len(hashes) == 1 {
				err = tw.AddTrackTag(hashes[0], tagID)
			} else {
				err = tw.AddTrackTagBulk(hashes, tagID)
			}
		} else {
			if len(hashes) == 1 {
				err = tw.RemoveTrackTag(hashes[0], tagID)
			} else {
				err = tw.RemoveTrackTagBulk(hashes, tagID)
			}
		}
		if err != nil {
			slog.Warn("tag toggle: DB write failed", "tagID", tagID, "add", add, "err", err)
		}
		return tagToggledMsg{tagID: tagID, hashes: hashes, wasAdd: add, err: err}
	}
}

// handleTagPickerCreate processes TagPickerCreateMsg: creates the tag in the
// DB, refreshes the cache, and re-opens the picker with the new tag.
func (m Model) handleTagPickerCreate(msg TagPickerCreateMsg) (Model, tea.Cmd) {
	tw := m.tagWriter
	tr := m.tagReader
	if tw == nil || tr == nil {
		return m, nil
	}

	name := strings.TrimSpace(msg.Name)
	if name == "" {
		return m, nil
	}

	return m, func() tea.Msg {
		id, err := tw.CreateTag(name, "")
		if err != nil {
			slog.Warn("tag picker: create tag failed", "name", name, "err", err)
			return tagCreatedMsg{err: err}
		}
		return tagCreatedMsg{id: id, name: name}
	}
}

// handleTagCreated processes tagCreatedMsg: updates the cache and re-opens
// the tag picker so the user sees the new tag.
func (m Model) handleTagCreated(msg tagCreatedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.statusBar.notification = newNotification("Create tag failed: " + msg.err.Error())
		return m, nil
	}

	// Re-read tags to update cache.
	if m.tagReader != nil {
		tags, err := m.tagReader.ReadTags()
		if err == nil {
			m = m.rebuildTagCache(tags)
		}
	}

	// Re-open the picker with updated tag list.
	return m.openTagPicker()
}

// handleTagToggled processes tagToggledMsg: reverts the optimistic update on
// error, and is a no-op on success (the cache was already updated optimistically).
func (m Model) handleTagToggled(msg tagToggledMsg) (Model, tea.Cmd) {
	if msg.err == nil {
		return m, nil
	}
	// Revert the optimistic update.
	m = m.applyTagToggle(msg.tagID, msg.hashes, !msg.wasAdd)
	if m.overlay.tagPicker != nil {
		var prevState checkState
		if msg.wasAdd {
			prevState = checkUnchecked
		} else {
			prevState = checkChecked
		}
		updated := m.overlay.tagPicker.UpdateItemState(msg.tagID, prevState)
		m.overlay.tagPicker = &updated
	}
	m.statusBar.notification = newNotification("Tag toggle failed: " + msg.err.Error())
	return m, nil
}

// handleTagsRefreshed processes tagsRefreshedMsg: replaces the in-memory
// tag and trackTags caches with the freshly loaded data.
func (m Model) handleTagsRefreshed(msg tagsRefreshedMsg) Model {
	if msg.err != nil {
		slog.Warn("tags refresh failed", "err", msg.err)
		return m
	}
	m = m.rebuildTagCache(msg.tags)
	if msg.trackTags != nil {
		m.trackTags = msg.trackTags
	}
	return m
}

// applyTagToggle updates the in-memory trackTags cache for a tag toggle.
func (m Model) applyTagToggle(tagID int64, hashes []string, add bool) Model {
	if m.trackTags == nil {
		m.trackTags = make(map[string]map[int64]struct{})
	}
	for _, h := range hashes {
		if add {
			if m.trackTags[h] == nil {
				m.trackTags[h] = make(map[int64]struct{})
			}
			m.trackTags[h][tagID] = struct{}{}
		} else {
			delete(m.trackTags[h], tagID)
		}
	}
	return m
}

// rebuildTagCache replaces the in-memory tag list and lookup maps.
func (m Model) rebuildTagCache(tags []db.TagRow) Model {
	m.tags = tags
	m.tagNames = make(map[int64]string, len(tags))
	m.tagColors = make(map[int64]string, len(tags))
	for _, t := range tags {
		m.tagNames[t.ID] = t.Name
		m.tagColors[t.ID] = t.Color
	}
	return m
}

// ---------- Tags view actions ----------

// handleTagOpen handles actionTagOpen in the Tags view.
// At TagLevel: drills into tracks for the focused tag.
// At TrackLevel: enqueues the focused track.
func (m Model) handleTagOpen() (Model, tea.Cmd) {
	if m.activeView != viewTags {
		return m, nil
	}
	switch m.tagsView.level {
	case tagtui.TagLevel:
		tag, ok := m.tagsView.focusedTag()
		if !ok {
			return m, nil
		}
		tr := m.tagReader
		if tr == nil {
			m.statusBar.notification = newNotification("Tag reader not available")
			return m, nil
		}
		return m, func() tea.Msg {
			tracks, err := tr.ReadTracksByTag(tag.ID)
			if err != nil {
				return tagViewTracksLoadedMsg{err: err}
			}
			counts, _ := tr.ReadTagCounts()
			return tagViewTracksLoadedMsg{tag: tag, tracks: tracks, counts: counts}
		}
	case tagtui.TrackLevel:
		cmd := m.tagsView.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries))
		return m, cmd
	}
	return m, nil
}

// handleTagBack navigates one level up in the Tags view.
func (m Model) handleTagBack() Model {
	if m.activeView != viewTags {
		return m
	}
	m.tagsView = m.tagsView.back()
	return m
}

// handleTagCreate starts the inline create input.
func (m Model) handleTagCreate() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.level != tagtui.TagLevel {
		return m, nil
	}
	if m.tagsView.isCreating() || m.tagsView.isRenaming() {
		return m, nil
	}
	m.tagsView = m.tagsView.startCreate()
	return m, nil
}

// handleTagRename starts the inline rename input for the focused tag.
func (m Model) handleTagRename() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.level != tagtui.TagLevel {
		return m, nil
	}
	if m.tagsView.isCreating() || m.tagsView.isRenaming() {
		return m, nil
	}
	tag, ok := m.tagsView.focusedTag()
	if !ok {
		return m, nil
	}
	m.tagsView = m.tagsView.startRename(tag.Name)
	return m, nil
}

// handleTagDelete soft-deletes the focused tag and arms undo.
func (m Model) handleTagDelete() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.level != tagtui.TagLevel {
		return m, nil
	}
	tag, ok := m.tagsView.focusedTag()
	if !ok {
		return m, nil
	}
	idx := m.tagsView.cursor

	// Remove from tagsView list immediately.
	newTags := make([]db.TagRow, 0, len(m.tagsView.tags)-1)
	for _, t := range m.tagsView.tags {
		if t.ID != tag.ID {
			newTags = append(newTags, t)
		}
	}
	m.tagsView.tags = newTags
	m.tagsView = m.tagsView.clampCursor()

	// Remove from global tag cache.
	newGlobalTags := make([]db.TagRow, 0, len(m.tags)-1)
	for _, t := range m.tags {
		if t.ID != tag.ID {
			newGlobalTags = append(newGlobalTags, t)
		}
	}
	m = m.rebuildTagCache(newGlobalTags)

	// Arm undo.
	m.setUndoSlot(undoDeleteTagAction{tag: tag, idx: idx}, nil)

	tw := m.tagWriter
	return m, func() tea.Msg {
		var err error
		if tw != nil {
			err = tw.DeleteTagSoft(tag.ID)
		}
		return tagDeletedMsg{tag: tag, idx: idx, err: err}
	}
}

// handleTagEnqueueAll enqueues all tracks for the focused tag.
func (m Model) handleTagEnqueueAll() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.level != tagtui.TagLevel {
		return m, nil
	}
	tag, ok := m.tagsView.focusedTag()
	if !ok {
		return m, nil
	}
	tr := m.tagReader
	be := m.batchEnqueuer
	if tr == nil || be == nil {
		m.statusBar.notification = newNotification("Enqueue not available")
		return m, nil
	}
	wasEmpty := len(m.queue.entries) == 0
	tagName := tag.Name
	tagID := tag.ID
	return m, func() tea.Msg {
		rows, err := tr.ReadTracksByTag(tagID)
		if err != nil {
			return TransientNotifyMsg("Enqueue failed: " + err.Error())
		}
		libTracks := playlistRowsToLibTracks(rows)
		ids, err := be.AppendTracks(libTracks, tagName)
		if err != nil {
			return TransientNotifyMsg("Enqueue failed: " + err.Error())
		}
		return tagEnqueueAllMsg{tagName: tagName, count: len(libTracks), ids: ids, wasEmpty: wasEmpty}
	}
}

// handleTagViewTracksLoaded processes tagViewTracksLoadedMsg.
func (m Model) handleTagViewTracksLoaded(msg tagViewTracksLoadedMsg) Model {
	if msg.err != nil {
		m.statusBar.notification = newNotification("Load tracks failed: " + msg.err.Error())
		return m
	}
	m.tagsView = m.tagsView.openTracks(msg.tag, msg.tracks)
	if msg.counts != nil {
		m.tagsView.tagCounts = msg.counts
	}
	return m
}

// handleTagViewCountsLoaded refreshes tag counts in the view.
func (m Model) handleTagViewCountsLoaded(msg tagViewCountsLoadedMsg) Model {
	if msg.err != nil {
		return m
	}
	m.tagsView.tagCounts = msg.counts
	return m
}

// handleTagRenamed processes tagRenamedMsg: updates the in-memory tag list.
func (m Model) handleTagRenamed(msg tagRenamedMsg) Model {
	if msg.err != nil {
		m.statusBar.notification = newNotification("Rename failed: " + msg.err.Error())
		return m
	}
	// Update tagsView list.
	for i := range m.tagsView.tags {
		if m.tagsView.tags[i].ID == msg.id {
			m.tagsView.tags[i].Name = msg.name
			break
		}
	}
	// Update global tag cache.
	for i := range m.tags {
		if m.tags[i].ID == msg.id {
			m.tags[i].Name = msg.name
			break
		}
	}
	m.tagNames[msg.id] = msg.name
	m.statusBar.notification = newNotification("Tag renamed to " + msg.name)
	return m
}

// handleTagDeleted processes tagDeletedMsg: shows notification or reverts on error.
func (m Model) handleTagDeleted(msg tagDeletedMsg) Model {
	if msg.err != nil {
		// Revert the optimistic removal.
		idx := min(msg.idx, len(m.tagsView.tags))
		restored := make([]db.TagRow, len(m.tagsView.tags)+1)
		copy(restored, m.tagsView.tags[:idx])
		restored[idx] = msg.tag
		copy(restored[idx+1:], m.tagsView.tags[idx:])
		m.tagsView.tags = restored
		m = m.rebuildTagCache(append(m.tags, msg.tag))
		m.statusBar.notification = newNotification("Delete tag failed: " + msg.err.Error())
		return m
	}
	m.statusBar.notification = newNotification("Tag deleted — press u to undo")
	return m
}

// handleTagCreateInputKeyPress is the keyPressHandler signature wrapper for
// the tag create input (used in orderedKeyPressHandlers).
func (m Model) handleTagCreateInputKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	return m.handleTagCreateInput(msg)
}

// handleTagRenameInputKeyPress is the keyPressHandler signature wrapper for
// the tag rename input (used in orderedKeyPressHandlers).
func (m Model) handleTagRenameInputKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	return m.handleTagRenameInput(msg)
}

// handleTagCreateInput processes enter/esc on the inline create input.
// Returns (updated model, cmd, consumed).
func (m Model) handleTagCreateInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.tagsView.isCreating() || m.activeView != viewTags {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		name := m.tagsView.createInputValue()
		m.tagsView = m.tagsView.cancelCreate()
		if name == "" {
			return m, nil, true
		}
		tw := m.tagWriter
		return m, func() tea.Msg {
			id, err := tw.CreateTag(name, "")
			return tagCreatedMsg{id: id, name: name, err: err}
		}, true
	case "esc", "ctrl+c":
		m.tagsView = m.tagsView.cancelCreate()
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.tagsView, cmd = m.tagsView.updateCreateInput(msg)
		return m, cmd, true
	}
}

// handleTagRenameInput processes enter/esc on the inline rename input.
// Returns (updated model, cmd, consumed).
func (m Model) handleTagRenameInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.tagsView.isRenaming() || m.activeView != viewTags {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		name := m.tagsView.renameInputValue()
		tag, _ := m.tagsView.focusedTag()
		m.tagsView = m.tagsView.cancelRename()
		if name == "" || tag.ID == 0 {
			return m, nil, true
		}
		tw := m.tagWriter
		tagID := tag.ID
		return m, func() tea.Msg {
			err := tw.RenameTag(tagID, name)
			return tagRenamedMsg{id: tagID, name: name, err: err}
		}, true
	case "esc", "ctrl+c":
		m.tagsView = m.tagsView.cancelRename()
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.tagsView, cmd = m.tagsView.updateRenameInput(msg)
		return m, cmd, true
	}
}

// handleTagCreated in the Tags view context: refresh the view tag list.
// (This augments the existing handleTagCreated which re-opens the picker.)
func (m Model) refreshTagsViewAfterCreate(msg tagCreatedMsg) Model {
	if msg.err != nil {
		return m
	}
	// Append the new tag and reload counts.
	if m.tagReader != nil {
		tags, err := m.tagReader.ReadTags()
		if err == nil {
			m.tagsView.tags = tags
		}
		counts, _ := m.tagReader.ReadTagCounts()
		m.tagsView.tagCounts = counts
	}
	return m
}

// loadTagCountsCmd returns a Cmd that loads tag counts from the DB.
func (m Model) loadTagCountsCmd() tea.Cmd {
	tr := m.tagReader
	if tr == nil {
		return nil
	}
	return func() tea.Msg {
		counts, err := tr.ReadTagCounts()
		return tagViewCountsLoadedMsg{counts: counts, err: err}
	}
}

// handleTagEnqueueAll2 processes tagEnqueueAllMsg: updates the queue state.
func (m Model) handleTagEnqueueAll2(msg tagEnqueueAllMsg) Model {
	if msg.err != nil {
		m.statusBar.notification = newNotification("Enqueue failed: " + msg.err.Error())
		return m
	}
	if msg.count == 0 {
		m.statusBar.notification = newNotification("No tracks to enqueue")
		return m
	}
	// Reload queue from DB to pick up the newly enqueued tracks.
	// Using a simple notification for now; proper queue sync handled by
	// the batch enqueuer's DB write and the next queue reload.
	n := msg.count
	word := pluralize(n, "track", "tracks")
	m.statusBar.notification = newNotification(
		fmt.Sprintf("Enqueued %d %s from tag %q", n, word, msg.tagName))
	return m
}

// syncTagsViewTags copies the global tag list into the tags view on switch.
func (m Model) syncTagsViewTags() Model {
	m.tagsView.tags = m.tags
	return m
}

// playlistRowsToLibTracks converts PlaylistTrackRow slice to library.Track slice.
func playlistRowsToLibTracks(rows []db.PlaylistTrackRow) []musiclib.Track {
	tracks := make([]musiclib.Track, len(rows))
	for i, r := range rows {
		tracks[i] = musiclib.Track{
			Hash:     r.Hash,
			Path:     r.Path,
			Artist:   r.Artist,
			Title:    r.Title,
			Album:    r.Album,
			Duration: float64(r.DurationMs) / 1000,
		}
	}
	return tracks
}

// handleTagMergeKeyPress intercepts key presses when the tag merge picker is open.
func (m Model) handleTagMergeKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.tagMergePicker == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.tagMergePicker.HandleKey(msg.String())
	m.overlay.tagMergePicker = &updated
	return m, cmd, true
}

// handleTagMerge opens the merge picker for the focused tag.
func (m Model) handleTagMerge() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.level != tagtui.TagLevel {
		return m, nil
	}
	if m.tagsView.isCreating() || m.tagsView.isRenaming() {
		return m, nil
	}
	tag, ok := m.tagsView.focusedTag()
	if !ok {
		return m, nil
	}
	if len(m.tags) < 2 {
		m.statusBar.notification = newNotification("Need at least 2 tags to merge")
		return m, nil
	}
	picker := NewTagMergePicker(tag, m.tags)
	m.overlay.tagMergePicker = &picker
	return m, nil
}

// handleTagMergePicked executes the irreversible merge after user confirmation.
func (m Model) handleTagMergePicked(msg tagMergePickedMsg) (Model, tea.Cmd) {
	m.overlay.tagMergePicker = nil
	tw := m.tagWriter
	if tw == nil {
		return m, nil
	}
	src, dest := msg.src, msg.dest
	return m, func() tea.Msg {
		err := tw.MergeTags(src.ID, dest.ID)
		return tagMergedMsg{src: src, dest: dest, err: err}
	}
}

// handleTagMerged processes tagMergedMsg: refreshes the tag cache and notifies.
func (m Model) handleTagMerged(msg tagMergedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.statusBar.notification = newNotification("Merge failed: " + msg.err.Error())
		return m, nil
	}

	// Reload tags from DB so the cache and view are consistent.
	if m.tagReader != nil {
		tags, err := m.tagReader.ReadTags()
		if err == nil {
			m = m.rebuildTagCache(tags)
			m.tagsView.tags = tags
		}
		counts, _ := m.tagReader.ReadTagCounts()
		m.tagsView.tagCounts = counts
	}
	m.tagsView = m.tagsView.clampCursor()
	m.statusBar.notification = newNotification(
		fmt.Sprintf("Merged %q into %q", msg.src.Name, msg.dest.Name))
	return m, nil
}

// buildTagsLine builds a human-readable string of colored tag names for the
// given track hash. Returns "" when the track has no tags. Each tag is rendered
// as "● Name" with the tag's color applied to the dot and name.
func (m Model) buildTagsLine(hash string) string {
	if hash == "" {
		return ""
	}
	tagIDs, ok := m.trackTags[hash]
	if !ok || len(tagIDs) == 0 {
		return ""
	}
	// Iterate over m.tags in order to produce a stable, alphabetical display.
	var parts []string
	for _, tag := range m.tags {
		if _, has := tagIDs[tag.ID]; !has {
			continue
		}
		dot := styles.ColoredDot(tag.Color)
		parts = append(parts, dot+" "+tag.Name)
	}
	return strings.Join(parts, "  ")
}

// loadTrackTagsCmd returns a Cmd that loads all track-tag assignments from
// the DB into the in-memory trackTags cache (called on startup and after tag
// mutations that require a full cache refresh).
func (m Model) loadTrackTagsCmd() tea.Cmd {
	tr := m.tagReader
	if tr == nil {
		return nil
	}
	return func() tea.Msg {
		tags, err := tr.ReadTags()
		if err != nil {
			return tagsRefreshedMsg{err: err}
		}
		trackTags := make(map[string]map[int64]struct{})
		for _, tag := range tags {
			hashes, herr := tr.ReadTrackHashesByTag(tag.ID)
			if herr != nil {
				slog.Warn("load track tags: read hashes failed", "tagID", tag.ID, "err", herr)
				continue
			}
			for _, h := range hashes {
				if trackTags[h] == nil {
					trackTags[h] = make(map[int64]struct{})
				}
				trackTags[h][tag.ID] = struct{}{}
			}
		}
		return tagsRefreshedMsg{tags: tags, trackTags: trackTags}
	}
}
