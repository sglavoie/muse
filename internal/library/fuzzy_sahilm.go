package library

import (
	"path/filepath"

	"github.com/sahilm/fuzzy"
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

	src := &trackSource{tracks: tracks}
	fuzzyMatches := fuzzy.FindFrom(query, src)

	matches := make([]Match, 0, len(fuzzyMatches))
	for _, fm := range fuzzyMatches {
		matches = append(matches, Match{
			TrackIndex:     fm.Index,
			MatchedIndexes: fm.MatchedIndexes,
		})
	}
	return matches
}

// trackSource implements fuzzy.Source over a []Track by exposing a
// concatenated "artist title filename" string for each entry.
type trackSource struct {
	tracks []Track
}

func (ts *trackSource) Len() int { return len(ts.tracks) }

func (ts *trackSource) String(i int) string {
	t := ts.tracks[i]
	return t.Artist + " " + t.Title + " " + filepath.Base(t.Path)
}
