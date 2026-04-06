package tui

import (
	"sort"

	"github.com/sglavoie/muse/internal/library"
)

// loadFavoriteTrackHashes reads all favorited track hashes from the DB and
// returns them as a set. Returns nil when reader is nil or on error.
func loadFavoriteTrackHashes(reader favoriteReader) map[string]struct{} {
	if reader == nil {
		return nil
	}
	rows, err := reader.ReadFavoritesByType("track")
	if err != nil {
		return nil
	}
	hashes := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		hashes[r.Value] = struct{}{}
	}
	return hashes
}

// loadFavoriteArtists reads all favorited artist names from the DB and
// returns them as a set. Returns nil when reader is nil or on error.
func loadFavoriteArtists(reader favoriteReader) map[string]struct{} {
	if reader == nil {
		return nil
	}
	rows, err := reader.ReadFavoritesByType("artist")
	if err != nil {
		return nil
	}
	artists := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		artists[r.Value] = struct{}{}
	}
	return artists
}

// loadFavoriteAlbums reads all favorited album names from the DB and
// returns them as a set. Returns nil when reader is nil or on error.
func loadFavoriteAlbums(reader favoriteReader) map[string]struct{} {
	if reader == nil {
		return nil
	}
	rows, err := reader.ReadFavoritesByType("album")
	if err != nil {
		return nil
	}
	albums := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		albums[r.Value] = struct{}{}
	}
	return albums
}

// deduplicateTracksByHash removes duplicate tracks (same hash) keeping the
// first occurrence. This prevents duplicates when the library contains the same
// album or artist tracks from multiple scan paths.
func deduplicateTracksByHash(tracks []library.Track) []library.Track {
	seen := make(map[string]struct{}, len(tracks))
	result := make([]library.Track, 0, len(tracks))
	for _, t := range tracks {
		if _, ok := seen[t.Hash]; !ok {
			seen[t.Hash] = struct{}{}
			result = append(result, t)
		}
	}
	return result
}

// indexOfString returns the index of the first occurrence of s in items,
// or 0 if not found. Used to translate a filtered display cursor back to
// the raw slice position for correct undo re-insertion.
func indexOfString(items []string, s string) int {
	for i, v := range items {
		if v == s {
			return i
		}
	}
	return 0
}

// indexOfTrackByHash returns the index of the first track with the given hash,
// or 0 if not found. Used to translate a filtered display cursor back to
// the raw slice position for correct undo re-insertion.
func indexOfTrackByHash(tracks []library.Track, hash string) int {
	for i, t := range tracks {
		if t.Hash == hash {
			return i
		}
	}
	return 0
}

// removeString removes the first occurrence of s from items.
func removeString(items []string, s string) []string {
	for i, v := range items {
		if v == s {
			return append(items[:i], items[i+1:]...)
		}
	}
	return items
}

// removeTrackByHash removes the first track with the given hash.
func removeTrackByHash(tracks []library.Track, hash string) []library.Track {
	for i, t := range tracks {
		if t.Hash == hash {
			return append(tracks[:i], tracks[i+1:]...)
		}
	}
	return tracks
}

// insertStringAt returns a new slice with s inserted at idx (clamped to len).
func insertStringAt(items []string, idx int, s string) []string {
	if idx < 0 {
		idx = 0
	}
	if idx > len(items) {
		idx = len(items)
	}
	result := make([]string, len(items)+1)
	copy(result, items[:idx])
	result[idx] = s
	copy(result[idx+1:], items[idx:])
	return result
}

// insertTrackAt returns a new slice with t inserted at idx (clamped to len).
func insertTrackAt(tracks []library.Track, idx int, t library.Track) []library.Track {
	if idx < 0 {
		idx = 0
	}
	if idx > len(tracks) {
		idx = len(tracks)
	}
	result := make([]library.Track, len(tracks)+1)
	copy(result, tracks[:idx])
	result[idx] = t
	copy(result[idx+1:], tracks[idx:])
	return result
}

// sortBulkRemovalByIndex sorts values, tracks, and indices by the indices in
// ascending order. This ensures that undo re-insertion (which processes indices
// left-to-right) places items back at their correct positions, because each
// insert shifts only items that come after the insertion point.
func sortBulkRemovalByIndex(values []string, tracks []library.Track, indices []int) {
	if len(indices) <= 1 {
		return
	}
	perm := make([]int, len(indices))
	for i := range perm {
		perm[i] = i
	}
	sort.Slice(perm, func(a, b int) bool {
		return indices[perm[a]] < indices[perm[b]]
	})
	sortedValues := make([]string, len(values))
	sortedIndices := make([]int, len(indices))
	for i, p := range perm {
		sortedValues[i] = values[p]
		sortedIndices[i] = indices[p]
	}
	copy(values, sortedValues)
	copy(indices, sortedIndices)
	if len(tracks) == len(indices) {
		sortedTracks := make([]library.Track, len(tracks))
		for i, p := range perm {
			sortedTracks[i] = tracks[p]
		}
		copy(tracks, sortedTracks)
	}
}
