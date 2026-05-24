package library

// Match represents a single fuzzy search result for a track.
type Match struct {
	// TrackIndex is the position of the matched track in the searched slice.
	TrackIndex int
	// MatchedIndexes holds the character positions within the matched field
	// that correspond to the query runes, used for in-place highlight rendering.
	MatchedIndexes []int
	// ArtistPositions holds 0-indexed character positions within the artist
	// string that matched a field-specific query (a:/artist: prefix).
	ArtistPositions []int
	// TitlePositions holds 0-indexed character positions within the title
	// string that matched a field-specific query (t:/title: prefix).
	TitlePositions []int
	// AlbumPositions holds 0-indexed character positions within the album
	// string that matched a field-specific query (l:/album: prefix).
	AlbumPositions []int
}

// FuzzySearcher filters a track slice by a query string using fuzzy matching
// across the artist, title, and filename (Path) fields. An empty or
// whitespace-only query returns all tracks in index order.
type FuzzySearcher interface {
	Search(query string, tracks []Track) []Match
}
