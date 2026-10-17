package tui

import (
	"log/slog"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
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
	switch m.ActiveView {
	case viewLibrary:
		return hashMapToSlice(m.library.SelectedHashes)
	case viewQueue:
		var hashes []string
		for _, e := range m.queue.Entries {
			if e.Selected {
				hashes = append(hashes, e.Hash)
			}
		}
		return hashes
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return hashMapToSlice(m.browse.Artists.SelectedHashes)
		case browse.BrowseModeAlbums:
			return hashMapToSlice(m.browse.Albums.SelectedHashes)
		case browse.BrowseModeGenres:
			return hashMapToSlice(m.browse.Genres.SelectedHashes)
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
		var hashes []string
		for _, track := range m.favorites.SelectedTracks() {
			hashes = append(hashes, track.Hash)
		}
		return hashes
	case viewTags:
		return hashMapToSlice(m.tagsView.SelectedHashes)
	case viewMixes:
		if m.mixes.Level == 1 {
			return hashMapToSlice(m.mixes.TrackView.SelectedHashes)
		}
		return nil
	case viewInsights:
		if m.insights.AtTracks() {
			return hashMapToSlice(m.insights.TrackView.SelectedHashes)
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
	switch m.ActiveView {
	case viewLibrary:
		if track, ok := m.library.Panel().FocusedTrack(); ok {
			return track.Hash
		}
	case viewQueue:
		filtered := m.queue.FilteredEntries()
		if m.queue.Cursor >= 0 && m.queue.Cursor < len(filtered) {
			return filtered[m.queue.Cursor].Hash
		}
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			if m.browse.Artists.Level == arttui.TrackLevel {
				if track, ok := m.browse.Artists.FocusedTrack(); ok {
					return track.Hash
				}
			}
		case browse.BrowseModeAlbums:
			if m.browse.Albums.Level == albtui.TrackLevel {
				if track, ok := m.browse.Albums.FocusedTrack(); ok {
					return track.Hash
				}
			}
		case browse.BrowseModeGenres:
			if m.browse.Genres.Level == gentui.TrackLevel {
				if track, ok := m.browse.Genres.FocusedTrack(); ok {
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
		if t := m.mixes.FocusedTrack(); t != nil {
			return t.Hash
		}
	case viewInsights:
		if t := m.insights.FocusedTrack(); t != nil {
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
	// A tag filter selects on exactly this map, so a toggle can move a track in
	// or out of the candidate set.
	m.djInvalidate()
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
//
// Every tag create, rename, delete, undelete, merge, and refresh funnels
// through here, which makes it the one place Auto-DJ has to learn that the tag
// catalog moved: a filter referencing a tag that just disappeared must start
// reporting as unresolved rather than keep planning against a stale answer.
func (m Model) rebuildTagCache(tags []db.TagRow) Model {
	m.tags = tags
	m.tagCacheErr = nil
	m.tagNames = make(map[int64]string, len(tags))
	m.tagColors = make(map[int64]string, len(tags))
	for _, t := range tags {
		m.tagNames[t.ID] = t.Name
		m.tagColors[t.ID] = t.Color
	}
	m.djInvalidate()
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
			AlbumArtist: r.AlbumArtist,
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
// non-missing tracks. Hard deletion waits for the next startup sweep. Runs once at
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
		// The synchronous startup sweep handles old tombstones. This command can
		// finish after the user has already armed an undo, so it must not purge.

		if len(purged) == 0 {
			return nil
		}
		slog.Info("startup: purged empty tags", "count", len(purged))
		return emptyTagsPurgedMsg{tagIDs: purged}
	}
}
