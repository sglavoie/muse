package tui

import (
	"log/slog"
	"sort"
	"strings"
	"time"

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
		return hashMapToSlice(m.library.SelectedHashes)
	case viewQueue:
		var hashes []string
		for _, e := range m.queue.entries {
			if e.Selected {
				hashes = append(hashes, e.Hash)
			}
		}
		return hashes
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			return hashMapToSlice(m.browse.artists.selectedHashes)
		case BrowseModeAlbums:
			return hashMapToSlice(m.browse.albums.selectedHashes)
		case BrowseModeGenres:
			return hashMapToSlice(m.browse.genres.selectedHashes)
		}
		return nil
	case viewPlaylists:
		if len(m.playlists.SelectedTrackIDs) == 0 {
			return nil
		}
		hashes := make([]string, 0, len(m.playlists.SelectedTrackIDs))
		for _, t := range m.playlists.Tracks {
			if _, ok := m.playlists.SelectedTrackIDs[t.ID]; ok {
				hashes = append(hashes, t.Hash)
			}
		}
		return hashes
	case viewFavorites:
		if len(m.favorites.SelectedItems) == 0 {
			return nil
		}
		var items []musiclib.Track
		switch {
		case m.favorites.Level == favtui.ListLevel && m.favorites.Category == favtui.CategoryTracks:
			items = arttui.FilterTracks(m.favorites.Tracks, m.favorites.Filter.Text)
		case m.favorites.Level == favtui.TrackLevel:
			items = arttui.FilterTracks(m.favorites.DrilldownTracks, m.favorites.Filter.Text)
		default:
			return nil
		}
		var hashes []string
		for _, idx := range sortedIntSetKeys(m.favorites.SelectedItems) {
			if idx >= 0 && idx < len(items) {
				hashes = append(hashes, items[idx].Hash)
			}
		}
		return hashes
	case viewTags:
		return hashMapToSlice(m.tagsView.SelectedHashes)
	case viewMixes:
		if m.mixes.level == 1 {
			return hashMapToSlice(m.mixes.trackView.SelectedHashes)
		}
		return nil
	case viewInsights:
		if m.insights.atTracks() {
			return hashMapToSlice(m.insights.trackView.SelectedHashes)
		}
		return nil
	}
	return nil
}

// hashMapToSlice converts a hash set to a sorted slice.
func hashMapToSlice(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	s := make([]string, 0, len(m))
	for k := range m {
		s = append(s, k)
	}
	sort.Strings(s)
	return s
}

// focusedTrackHash returns the content hash of the track currently under the
// cursor, or "" when the cursor is not on a track row.
func (m Model) focusedTrackHash() string {
	switch m.activeView {
	case viewLibrary:
		if track, ok := m.library.Panel().FocusedTrack(); ok {
			return track.Hash
		}
	case viewQueue:
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			return filtered[m.queue.cursor].Hash
		}
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			if m.browse.artists.level == arttui.TrackLevel {
				if track, ok := m.browse.artists.focusedTrack(); ok {
					return track.Hash
				}
			}
		case BrowseModeAlbums:
			if m.browse.albums.level == albtui.TrackLevel {
				if track, ok := m.browse.albums.focusedTrack(); ok {
					return track.Hash
				}
			}
		case BrowseModeGenres:
			if m.browse.genres.level == gentui.TrackLevel {
				if track, ok := m.browse.genres.focusedTrack(); ok {
					return track.Hash
				}
			}
		}
	case viewPlaylists:
		if track, ok := m.playlists.FocusedTrack(); ok {
			return track.Hash
		}
	case viewFavorites:
		switch m.favorites.Level {
		case favtui.ListLevel:
			if m.favorites.Category == favtui.CategoryTracks {
				if track, ok := m.favorites.FocusedTrack(); ok {
					return track.Hash
				}
			}
		case favtui.TrackLevel:
			if track, ok := m.favorites.FocusedTrack(); ok {
				return track.Hash
			}
		}
	case viewTags:
		if m.tagsView.Level == tagtui.TrackLevel {
			if track, ok := m.tagsView.FocusedTrack(); ok {
				return track.Hash
			}
		}
	case viewMixes:
		if t := m.mixes.focusedTrack(); t != nil {
			return t.Hash
		}
	case viewInsights:
		if t := m.insights.focusedTrack(); t != nil {
			return t.Hash
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

	tvFiltered := make([]db.TagRow, 0, len(m.tagsView.Tags))
	for _, t := range m.tagsView.Tags {
		if t.ID != tagID {
			tvFiltered = append(tvFiltered, t)
		}
	}
	m.tagsView.Tags = tvFiltered
	m.tagsView = m.tagsView.ClampCursor()

	if m.tagsView.TagCounts != nil {
		delete(m.tagsView.TagCounts, tagID)
	}

	// Clear the cross-view tag filter when it pointed at the removed tag.
	// Without this, the title bar would render "[tag: ]" because the tag's
	// name lookup now returns the empty string, and the cached
	// tagFilterHashes maps on Library/Queue would dangle indefinitely.
	if m.activeTagFilter != nil && *m.activeTagFilter == tagID {
		m = m.clearTagFilter()
	}
	return m
}

// syncTagsViewTags copies the global tag list into the tags view on switch.
func (m Model) syncTagsViewTags() Model {
	m.tagsView.Tags = m.tags
	return m
}

// playlistRowsToLibTracks converts PlaylistTrackRow slice to library.Track slice.
func playlistRowsToLibTracks(rows []db.PlaylistTrackRow) []musiclib.Track {
	tracks := make([]musiclib.Track, len(rows))
	for i, r := range rows {
		tracks[i] = musiclib.Track{
			Hash:        r.Hash,
			Path:        r.Path,
			Artist:      r.Artist,
			Title:       r.Title,
			Album:       r.Album,
			Duration:    float64(r.DurationMs) / 1000,
			TrackNumber: r.TrackNumber,
			DiscNumber:  r.DiscNumber,
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
// non-missing tracks, then hard-deletes all soft-deleted tags. Runs once at
// startup to clean up orphans left over from previous sessions.
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
		// Hard-delete all soft-deleted tags (no undo window at startup).
		if err := tw.PurgeDeletedTags(time.Now().Unix() + 1); err != nil {
			slog.Warn("startup: hard-delete soft-deleted tags", "err", err)
		}
		if len(purged) == 0 {
			return nil
		}
		slog.Info("startup: purged empty tags", "count", len(purged))
		return emptyTagsPurgedMsg{tagIDs: purged}
	}
}
