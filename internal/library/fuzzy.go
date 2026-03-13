package library

// Match represents a single fuzzy search result for a track.
type Match struct {
	// TrackIndex is the position of the matched track in the searched slice.
	TrackIndex int
	// MatchedIndexes holds the character positions within the matched field
	// that correspond to the query runes, used for in-place highlight rendering.
	MatchedIndexes []int
}

// FuzzySearcher filters a track slice by a query string using fuzzy matching
// across the artist, title, and filename (Path) fields. An empty query returns
// all tracks in index order.
type FuzzySearcher interface {
	Search(query string, tracks []Track) []Match
}
