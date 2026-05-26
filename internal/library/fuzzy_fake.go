package library

import "strings"

// FakeFuzzySearcher is a deterministic in-memory FuzzySearcher for use in
// unit tests. It performs a case-insensitive substring match across the
// artist, title, album, and Path fields and returns results in stable index
// order. It supports field-specific queries via a:/t:/l: prefixes.
type FakeFuzzySearcher struct{}

// Search returns one Match per track whose artist, title, album, or Path
// contains query as a case-insensitive substring. An empty or whitespace-only
// query returns all tracks. Results are ordered by ascending TrackIndex.
func (f FakeFuzzySearcher) Search(query string, tracks []Track) []Match {
	if strings.TrimSpace(query) == "" {
		matches := make([]Match, len(tracks))
		for i := range tracks {
			matches[i] = Match{TrackIndex: i}
		}
		return matches
	}

	fq := ParseFilterQuery(query)

	if fq.IsFieldSpecific() {
		return f.searchFieldSpecific(fq, tracks)
	}

	q := strings.ToLower(fq.General)
	if q == "" {
		q = strings.ToLower(query)
	}

	var matches []Match
	for i, t := range tracks {
		if strings.Contains(strings.ToLower(t.Artist), q) ||
			strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.Album), q) ||
			strings.Contains(strings.ToLower(t.Path), q) {
			matches = append(matches, Match{TrackIndex: i})
		}
	}
	return matches
}

// searchFieldSpecific handles queries with at least one field prefix.
func (f FakeFuzzySearcher) searchFieldSpecific(fq FilterQuery, tracks []Track) []Match {
	var matches []Match
	for i, t := range tracks {
		ok := true

		if fq.Artist != "" {
			if !strings.Contains(strings.ToLower(t.Artist), strings.ToLower(fq.Artist)) {
				ok = false
			}
		}

		if ok && fq.Title != "" {
			if !strings.Contains(strings.ToLower(t.Title), strings.ToLower(fq.Title)) {
				ok = false
			}
		}

		if ok && fq.Album != "" {
			if !strings.Contains(strings.ToLower(t.Album), strings.ToLower(fq.Album)) {
				ok = false
			}
		}

		if ok && fq.Genre != "" {
			if !strings.Contains(strings.ToLower(t.Genre), strings.ToLower(fq.Genre)) {
				ok = false
			}
		}

		if ok && fq.General != "" {
			q := strings.ToLower(fq.General)
			if !strings.Contains(strings.ToLower(t.Artist), q) &&
				!strings.Contains(strings.ToLower(t.Title), q) &&
				!strings.Contains(strings.ToLower(t.Album), q) &&
				!strings.Contains(strings.ToLower(t.Path), q) {
				ok = false
			}
		}

		if ok {
			matches = append(matches, Match{TrackIndex: i})
		}
	}
	return matches
}
