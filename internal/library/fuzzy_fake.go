package library

import "strings"

// FakeFuzzySearcher is a deterministic in-memory FuzzySearcher for use in
// unit tests. It performs the same case-insensitive, per-field substring match
// as SahilmFuzzySearcher and returns results in stable index order. It supports
// field-specific queries via a:/t:/l:/g:/aa: prefixes.
type FakeFuzzySearcher struct{}

// Search returns one Match per track whose artist, title, album, or base
// filename contains query as a case-insensitive substring. An empty or
// whitespace-only query returns all tracks. Results are ordered by ascending
// TrackIndex.
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

	q := fq.General

	var matches []Match
	for i, t := range tracks {
		if _, matched := generalSubstringIndexes(t, q); matched {
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
			if _, matched := substringIndexes(t.Artist, fq.Artist); !matched {
				ok = false
			}
		}

		if ok && fq.Title != "" {
			if _, matched := substringIndexes(t.Title, fq.Title); !matched {
				ok = false
			}
		}

		if ok && fq.Album != "" {
			if _, matched := substringIndexes(t.Album, fq.Album); !matched {
				ok = false
			}
		}

		if ok && fq.Genre != "" {
			if _, matched := substringIndexes(t.Genre, fq.Genre); !matched {
				ok = false
			}
		}

		if ok && fq.AlbumArtist != "" {
			_, ok = substringIndexes(t.AlbumArtist, fq.AlbumArtist)
		}

		if ok && fq.General != "" {
			if _, matched := generalSubstringIndexes(t, fq.General); !matched {
				ok = false
			}
		}

		if ok {
			matches = append(matches, Match{TrackIndex: i})
		}
	}
	return matches
}
