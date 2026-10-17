package tui

import (
	"github.com/sglavoie/muse/internal/db"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleTagViewTracksLoaded processes tagViewTracksLoadedMsg.
// The async ReadTracksByTag command can race with view navigation: if the
// user has switched away from the Tags view (or already drilled in/back out
// to a different state) by the time the result arrives, applying the load
// would surprise them by flipping the level to TrackLevel for a stale tag.
// Drop the message in those cases and keep the refreshed counts only.
func (m Model) handleTagViewTracksLoaded(msg tagViewTracksLoadedMsg) Model {
	if msg.err != nil {
		m.statusBar.NotifyError("Load tracks failed: " + msg.err.Error())
		return m
	}
	if m.ActiveView != viewTags || m.tagsView.Level != tagtui.TagLevel {
		if msg.counts != nil {
			m.tagsView.TagCounts = msg.counts
		}
		return m
	}
	m.tagsView = m.tagsView.OpenTracks(msg.tag, msg.tracks)
	if msg.counts != nil {
		m.tagsView.TagCounts = msg.counts
	}
	return m
}

// handleTagViewCountsLoaded refreshes tag counts in the view.
func (m Model) handleTagViewCountsLoaded(msg tagViewCountsLoadedMsg) Model {
	if msg.err != nil {
		return m
	}
	m.tagsView.TagCounts = msg.counts
	return m
}

// handleTagRenamed processes tagRenamedMsg: updates the in-memory tag list.
func (m Model) handleTagRenamed(msg tagRenamedMsg) Model {
	if msg.err != nil {
		m.statusBar.NotifyError("Rename failed: " + msg.err.Error())
		return m
	}
	for i := range m.tagsView.Tags {
		if m.tagsView.Tags[i].ID == msg.id {
			m.tagsView.Tags[i].Name = msg.name
			break
		}
	}
	for i := range m.tags {
		if m.tags[i].ID == msg.id {
			m.tags[i].Name = msg.name
			break
		}
	}
	m.tagNames[msg.id] = msg.name
	m = m.remapSmartPlaylistTag(msg.id, db.TagRow{ID: msg.id, Name: msg.name})
	m.statusBar.Notify("Tag renamed to " + msg.name)
	return m
}

// handleTagDeleted processes tagDeletedMsg: reverts on error or no-ops on
// success (the purge is now scheduled synchronously in handleTagDelete via
// scheduleSoftDelete).
func (m Model) handleTagDeleted(msg tagDeletedMsg) Model {
	if msg.err != nil {
		idx := min(msg.idx, len(m.tagsView.Tags))
		restored := make([]db.TagRow, len(m.tagsView.Tags)+1)
		copy(restored, m.tagsView.Tags[:idx])
		restored[idx] = msg.tag
		copy(restored[idx+1:], m.tagsView.Tags[idx:])
		m.tagsView.Tags = restored
		m = m.rebuildTagCache(append(m.tags, msg.tag))
		m.statusBar.NotifyError("Delete tag failed: " + msg.err.Error())
		return m
	}
	return m
}

// refreshTagsViewAfterCreate refreshes the tag list in the Tags view.
func (m Model) refreshTagsViewAfterCreate(msg tagCreatedMsg) Model {
	if msg.err != nil {
		return m
	}
	if m.tagReader != nil {
		tags, err := m.tagReader.ReadTags()
		if err == nil {
			m.tagsView.Tags = tags
		}
		counts, _ := m.tagReader.ReadTagCounts()
		if counts == nil {
			counts = make(map[int64]int)
		}
		m.tagsView.TagCounts = counts
	}
	// Ensure the new tag appears in the view even if the DB read returned
	// stale data (WAL snapshot isolation on the read connection).
	if msg.id != 0 {
		found := false
		for _, t := range m.tagsView.Tags {
			if t.ID == msg.id {
				found = true
				break
			}
		}
		if !found {
			m.tagsView.Tags = append(m.tagsView.Tags, db.TagRow{ID: msg.id, Name: msg.name})
		}
		if m.tagsView.TagCounts[msg.id] == 0 && len(msg.hashes) > 0 {
			m.tagsView.TagCounts[msg.id] = len(msg.hashes)
		}
	}
	return m
}
