package library

import (
	"path/filepath"
	"strings"
	"unicode"
)

// SahilmFuzzySearcher is a FuzzySearcher backed by github.com/sahilm/fuzzy.
// It searches across the artist, title, album, and filename (base of Path)
// fields by concatenating them into a single searchable string per track.
type SahilmFuzzySearcher struct{}

// Search runs a fuzzy subsequence match against each track's artist, title,
// album, and filename. An empty or whitespace-only query returns all tracks in
// index order with nil MatchedIndexes. Field-specific queries (a:/t:/l:
// prefixes) match individual fields and populate per-field position slices
// instead.
func (s SahilmFuzzySearcher) Search(query string, tracks []Track) []Match {
	if strings.TrimSpace(query) == "" {
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

	var matches []Match
	for i, t := range tracks {
		searchStr := t.Artist + " " + t.Title + " " + t.Album + " " + filepath.Base(t.Path)
		indexes, ok := fuzzyIndexes(searchStr, searchTerm)
		if !ok {
			continue
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
			indexes, matched := fuzzyIndexes(t.Artist, fq.Artist)
			if !matched {
				ok = false
			} else {
				m.ArtistPositions = indexes
			}
		}

		if ok && fq.Title != "" {
			indexes, matched := fuzzyIndexes(t.Title, fq.Title)
			if !matched {
				ok = false
			} else {
				m.TitlePositions = indexes
			}
		}

		if ok && fq.Album != "" {
			indexes, matched := fuzzyIndexes(t.Album, fq.Album)
			if !matched {
				ok = false
			} else {
				m.AlbumPositions = indexes
			}
		}

		if ok && fq.Genre != "" {
			if !strings.Contains(strings.ToLower(t.Genre), strings.ToLower(fq.Genre)) {
				ok = false
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

// fuzzyIndexes reports whether query is a case-insensitive subsequence of s
// and returns the rune positions in s that satisfy the match.
func fuzzyIndexes(s, query string) ([]int, bool) {
	queryRunes := []rune(query)
	if len(queryRunes) == 0 {
		return nil, true
	}

	if idx := runeIndex(strings.ToLower(s), strings.ToLower(query)); idx >= 0 {
		return consecutiveIndexes(idx, len(queryRunes)), true
	}

	indexes := make([]int, 0, len(queryRunes))
	qi := 0
	for pos, r := range []rune(s) {
		if unicode.ToLower(r) != unicode.ToLower(queryRunes[qi]) {
			continue
		}
		indexes = append(indexes, pos)
		qi++
		if qi == len(queryRunes) {
			return indexes, true
		}
	}
	return nil, false
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
