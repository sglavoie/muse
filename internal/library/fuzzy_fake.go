package library

import "strings"

// FakeFuzzySearcher is a deterministic in-memory FuzzySearcher for use in
// unit tests. It performs a case-insensitive substring match across the
// artist, title, and Path fields and returns results in stable index order.
type FakeFuzzySearcher struct{}

// Search returns one Match per track whose artist, title, or Path contains
// query as a case-insensitive substring. An empty query returns all tracks.
// Results are ordered by ascending TrackIndex, ensuring deterministic output.
func (f FakeFuzzySearcher) Search(query string, tracks []Track) []Match {
	if query == "" {
		matches := make([]Match, len(tracks))
		for i := range tracks {
			matches[i] = Match{TrackIndex: i}
		}
		return matches
	}

	q := strings.ToLower(query)
	var matches []Match
	for i, t := range tracks {
		if strings.Contains(strings.ToLower(t.Artist), q) ||
			strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.Path), q) {
			matches = append(matches, Match{TrackIndex: i})
		}
	}
	return matches
}
