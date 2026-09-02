package tui

import (
	"sort"

	"github.com/sglavoie/muse/internal/tui/sorting"
)

// SortQueueIndices returns a permutation array where displayOrder[i] is the
// index into entries for display row i. Returns nil when field is SortDefault.
func SortQueueIndices(entries []QueueEntry, field sorting.SortField, dir sorting.SortDir) []int {
	if field == sorting.SortDefault || len(entries) == 0 {
		return nil
	}
	indices := make([]int, len(entries))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(a, b int) bool {
		ea, eb := entries[indices[a]], entries[indices[b]]
		switch field {
		case sorting.SortTitle:
			return sorting.CompareStringsUnknownLast(ea.Title, eb.Title, dir)
		case sorting.SortArtist:
			return sorting.CompareStringsUnknownLast(ea.Artist, eb.Artist, dir)
		case sorting.SortAlbum:
			return sorting.CompareStringsUnknownLast(ea.Album, eb.Album, dir)
		case sorting.SortDuration:
			return sorting.CompareNumericZeroLast(ea.DurationMs, eb.DurationMs, dir)
		case sorting.SortSource:
			return sorting.CompareStringsUnknownLast(ea.SourcePlaylist, eb.SourcePlaylist, dir)
		default:
			return false
		}
	})
	return indices
}
