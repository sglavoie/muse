package library

import (
	"path/filepath"
	"strings"
)

// SahilmFuzzySearcher is a FuzzySearcher backed by github.com/sahilm/fuzzy.
// It searches across the artist, title, album, and filename (base of Path)
// fields by concatenating them into a single searchable string per track.
type SahilmFuzzySearcher struct{}

// Search runs a fuzzy match against each track's artist, title, album, and
// filename. An empty query returns all tracks in index order with nil
// MatchedIndexes. For non-empty queries the returned matches are ranked by
// substring position and carry the MatchedIndexes into the concatenated search
// string. Field-specific queries (a:/t:/l: prefixes) match individual fields
// and populate per-field position slices instead.
func (s SahilmFuzzySearcher) Search(query string, tracks []Track) []Match {
	if query == "" {
		matches := make([]Match, len(tracks))
		for i := range tracks {
			matches[i] = Match{TrackIndex: i}
		}
		return matches
	}

	fq := ParseFilterQuery(query)

	if fq.IsFieldSpecific() {
		return s.searchFieldSpecific(fq, tracks)
	}

	// General path: search across concatenated artist+title+album+filename.
	searchTerm := strings.ToLower(fq.General)
	if searchTerm == "" {
		searchTerm = strings.ToLower(query)
	}

	searchRunes := []rune(searchTerm)

	var matches []Match
	for i, t := range tracks {
		searchStr := t.Artist + " " + t.Title + " " + t.Album + " " + filepath.Base(t.Path)
		idx := runeIndex(strings.ToLower(searchStr), searchTerm)
		if idx < 0 {
			continue
		}
		indexes := make([]int, len(searchRunes))
		for j := range indexes {
			indexes[j] = idx + j
		}
		matches = append(matches, Match{
			TrackIndex:     i,
			MatchedIndexes: indexes,
		})
	}
	return matches
}

// searchFieldSpecific handles queries with at least one field prefix.
func (s SahilmFuzzySearcher) searchFieldSpecific(fq FilterQuery, tracks []Track) []Match {
	var matches []Match
	for i, t := range tracks {
		m := Match{TrackIndex: i}
		ok := true

		if fq.Artist != "" {
			idx := runeIndex(strings.ToLower(t.Artist), strings.ToLower(fq.Artist))
			if idx < 0 {
				ok = false
			} else {
				m.ArtistPositions = consecutiveIndexes(idx, len([]rune(fq.Artist)))
			}
		}

		if ok && fq.Title != "" {
			idx := runeIndex(strings.ToLower(t.Title), strings.ToLower(fq.Title))
			if idx < 0 {
				ok = false
			} else {
				m.TitlePositions = consecutiveIndexes(idx, len([]rune(fq.Title)))
			}
		}

		if ok && fq.Album != "" {
			idx := runeIndex(strings.ToLower(t.Album), strings.ToLower(fq.Album))
			if idx < 0 {
				ok = false
			} else {
				m.AlbumPositions = consecutiveIndexes(idx, len([]rune(fq.Album)))
			}
		}

		if ok && fq.General != "" {
			searchStr := t.Artist + " " + t.Title + " " + t.Album + " " + filepath.Base(t.Path)
			if !strings.Contains(strings.ToLower(searchStr), strings.ToLower(fq.General)) {
				ok = false
			}
		}

		if ok {
			matches = append(matches, m)
		}
	}
	return matches
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

// consecutiveIndexes returns a slice [start, start+1, ..., start+length-1].
func consecutiveIndexes(start, length int) []int {
	idx := make([]int, length)
	for j := range idx {
		idx[j] = start + j
	}
	return idx
}
