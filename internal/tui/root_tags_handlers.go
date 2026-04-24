package tui

import (
	"github.com/sglavoie/muse/internal/db"
)

// handleTagViewTracksLoaded processes tagViewTracksLoadedMsg.
func (m Model) handleTagViewTracksLoaded(msg tagViewTracksLoadedMsg) Model {
	if msg.err != nil {
		m.statusBar.notify("Load tracks failed: " + msg.err.Error())
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
		m.statusBar.notify("Rename failed: " + msg.err.Error())
		return m
	}
	for i := range m.tagsView.tags {
		if m.tagsView.tags[i].ID == msg.id {
			m.tagsView.tags[i].Name = msg.name
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
	m.statusBar.notify("Tag renamed to " + msg.name)
	return m
}

// handleTagDeleted processes tagDeletedMsg: reverts on error or no-ops on
// success (the purge is now scheduled synchronously in handleTagDelete via
// scheduleSoftDelete).
func (m Model) handleTagDeleted(msg tagDeletedMsg) Model {
	if msg.err != nil {
		idx := min(msg.idx, len(m.tagsView.tags))
		restored := make([]db.TagRow, len(m.tagsView.tags)+1)
		copy(restored, m.tagsView.tags[:idx])
		restored[idx] = msg.tag
		copy(restored[idx+1:], m.tagsView.tags[idx:])
		m.tagsView.tags = restored
		m = m.rebuildTagCache(append(m.tags, msg.tag))
		m.statusBar.notify("Delete tag failed: " + msg.err.Error())
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
			m.tagsView.tags = tags
		}
		counts, _ := m.tagReader.ReadTagCounts()
		if counts == nil {
			counts = make(map[int64]int)
		}
		m.tagsView.tagCounts = counts
	}
	// Ensure the new tag appears in the view even if the DB read returned
	// stale data (WAL snapshot isolation on the read connection).
	if msg.id != 0 {
		found := false
		for _, t := range m.tagsView.tags {
			if t.ID == msg.id {
				found = true
				break
			}
		}
		if !found {
			m.tagsView.tags = append(m.tagsView.tags, db.TagRow{ID: msg.id, Name: msg.name})
		}
		if m.tagsView.tagCounts[msg.id] == 0 && len(msg.hashes) > 0 {
			m.tagsView.tagCounts[msg.id] = len(msg.hashes)
		}
	}
	return m
}
