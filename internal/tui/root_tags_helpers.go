package tui

import (
	"strings"

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
