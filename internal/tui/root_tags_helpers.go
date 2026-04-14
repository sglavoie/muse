package tui

import (
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/styles"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// tagPickerHashes returns the hashes the tag picker should operate on.
// isBulk is true when the returned hashes came from a multi-track selection.
func (m Model) tagPickerHashes() (hashes []string, isBulk bool) {
	selected := m.selectedTrackHashes()
	if len(selected) > 1 {
		return selected, true
	}
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
	case viewQueue:
		var hashes []string
		for _, e := range m.queue.entries {
			if e.Selected {
				hashes = append(hashes, e.Hash)
			}
		}
		return hashes
	case viewArtists:
		return hashMapToSlice(m.artists.selectedHashes)
	case viewAlbums:
		return hashMapToSlice(m.albums.selectedHashes)
	case viewGenres:
		return hashMapToSlice(m.genres.selectedHashes)
	case viewTags:
		return hashMapToSlice(m.tagsView.selectedHashes)
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

// hasTag reports whether m.tags already contains a tag with the given ID.
func (m Model) hasTag(id int64) bool {
	for _, t := range m.tags {
		if t.ID == id {
			return true
		}
	}
	return false
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

// nonEmptyTags returns the subset of tags that have at least one non-missing
// track according to tagCounts. When tagCounts is nil all tags are returned.
func nonEmptyTags(tags []db.TagRow, counts map[int64]int) []db.TagRow {
	if counts == nil {
		return tags
	}
	filtered := make([]db.TagRow, 0, len(tags))
	for _, t := range tags {
		if counts[t.ID] > 0 {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// removeTagFromCaches removes a single tag from m.tags and m.tagsView.tags,
// then rebuilds the lookup maps. Used when a tag is auto-deleted because it
// became empty.
func (m Model) removeTagFromCaches(tagID int64) Model {
	filtered := make([]db.TagRow, 0, len(m.tags))
	for _, t := range m.tags {
		if t.ID != tagID {
			filtered = append(filtered, t)
		}
	}
	m = m.rebuildTagCache(filtered)

	tvFiltered := make([]db.TagRow, 0, len(m.tagsView.tags))
	for _, t := range m.tagsView.tags {
		if t.ID != tagID {
			tvFiltered = append(tvFiltered, t)
		}
	}
	m.tagsView.tags = tvFiltered
	m.tagsView = m.tagsView.clampCursor()

	if m.tagsView.tagCounts != nil {
		delete(m.tagsView.tagCounts, tagID)
	}
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

// purgeEmptyTagsCmd returns a tea.Cmd that soft-deletes any tags with zero
// non-missing tracks. Runs once at startup to clean up pre-existing orphans.
func purgeEmptyTagsCmd(tags []db.TagRow, tr tagReader, tw tagWriter) tea.Cmd {
	if tr == nil || tw == nil || len(tags) == 0 {
		return nil
	}
	return func() tea.Msg {
		counts, err := tr.ReadTagCounts()
		if err != nil {
			slog.Warn("startup: read tag counts for purge", "err", err)
			return nil
		}
		var purged []int64
		for _, t := range tags {
			if counts[t.ID] == 0 {
				if deleted, err := tw.DeleteTagIfEmpty(t.ID); err != nil {
					slog.Warn("startup: auto-delete empty tag", "id", t.ID, "err", err)
				} else if deleted {
					purged = append(purged, t.ID)
				}
			}
		}
		if len(purged) == 0 {
			return nil
		}
		slog.Info("startup: purged empty tags", "count", len(purged))
		return emptyTagsPurgedMsg{tagIDs: purged}
	}
}
