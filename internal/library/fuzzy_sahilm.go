package library

import (
	"path/filepath"
	"strings"
	"unicode"
)

// SahilmFuzzySearcher performs deterministic substring filtering across track
// metadata. The historical name is retained for compatibility with callers.
type SahilmFuzzySearcher struct{}

// Search runs a case-insensitive substring match against each track's artist,
// title, album, and base filename independently. An empty or whitespace-only
// query returns all tracks in index order with nil MatchedIndexes.
// Field-specific queries (a:/t:/l:/g:/aa: prefixes) match individual fields and
// populate per-field position slices instead.
func (s SahilmFuzzySearcher) Search(query string, tracks []Track) []Match {
	return searchTracks(query, tracks, uncachedLookup)
}

// generalSubstringIndexes checks each searchable field independently so a
// query can never be assembled from characters in different fields. Returned
// indexes use the legacy concatenated artist/title/album/filename coordinate
// space consumed by the renderer.
func generalSubstringIndexes(t Track, query string) ([]int, bool) {
	fields := [...]string{t.Artist, t.Title, t.Album, filepath.Base(t.Path)}
	offset := 0
	for _, field := range fields {
		indexes, matched := substringIndexes(field, query)
		if matched {
			for i := range indexes {
				indexes[i] += offset
			}
			return indexes, true
		}
		offset += len([]rune(field)) + 1
	}
	return nil, false
}

// substringIndexes reports whether query occurs contiguously in s using a
// case-insensitive comparison and returns rune-based positions for rendering.
func substringIndexes(s, query string) ([]int, bool) {
	folded, positions := foldSearch(s)
	queryRunes := []rune(FoldSearchText(query))
	if len(queryRunes) == 0 {
		return nil, true
	}
	idx := runeIndex(folded, string(queryRunes))
	if idx < 0 {
		return nil, false
	}
	start, end := positions[idx], positions[idx+len(queryRunes)-1]+1
	// Include combining accents belonging to the matched final character.
	original := []rune(s)
	for end < len(original) && unicode.Is(unicode.Mn, original[end]) {
		end++
	}
	return consecutiveIndexes(start, end-start), true
}

// consecutiveIndexes returns a slice [start, start+1, ..., start+length-1].
func consecutiveIndexes(start, length int) []int {
	idx := make([]int, length)
	for j := range idx {
		idx[j] = start + j
	}
	return idx
}

// runeIndex returns the rune (not byte) position of the first occurrence of
// substr in s, or -1 if substr is not present. This is the rune-aware
// equivalent of strings.Index, needed because highlight rendering works with
// rune positions (padHighlight iterates over []rune).
func runeIndex(s, substr string) int {
	byteIdx := strings.Index(s, substr)
	if byteIdx < 0 {
		return -1
	}
	// Count runes in s[:byteIdx] to convert byte offset to rune offset.
	return len([]rune(s[:byteIdx]))
}
