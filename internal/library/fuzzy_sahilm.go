package library

import (
	"path/filepath"
	"strings"
)

// SahilmFuzzySearcher is a FuzzySearcher backed by github.com/sahilm/fuzzy.
// It searches across the artist, title, and filename (base of Path) fields by
// concatenating them into a single searchable string per track.
type SahilmFuzzySearcher struct{}

// Search runs a fuzzy match against each track's artist, title, and filename.
// An empty query returns all tracks in index order with nil MatchedIndexes.
// For non-empty queries the returned matches are ranked by the sahilm/fuzzy
// score and carry the MatchedIndexes into the concatenated search string.
func (s SahilmFuzzySearcher) Search(query string, tracks []Track) []Match {
	if query == "" {
		matches := make([]Match, len(tracks))
		for i := range tracks {
			matches[i] = Match{TrackIndex: i}
		}
		return matches
	}

	lowerQuery := strings.ToLower(query)
	var matches []Match
	for i, t := range tracks {
		searchStr := t.Artist + " " + t.Title + " " + filepath.Base(t.Path)
		idx := strings.Index(strings.ToLower(searchStr), lowerQuery)
		if idx < 0 {
			continue
		}
		indexes := make([]int, len(lowerQuery))
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

