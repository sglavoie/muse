package library

import "strings"

// FilterQuery holds the parsed components of a library filter string.
// A query like "rock a:beatles l:abbey" is parsed into General="rock",
// Artist="beatles", Album="abbey".
type FilterQuery struct {
	General string
	Artist  string
	Title   string
	Album   string
	Genre   string
}

// IsFieldSpecific reports whether at least one of Artist, Title, Album, or Genre is set.
func (fq FilterQuery) IsFieldSpecific() bool {
	return fq.Artist != "" || fq.Title != "" || fq.Album != "" || fq.Genre != ""
}

// prefixSpec maps a recognized prefix to the FilterQuery field it targets.
type prefixSpec struct {
	prefix string
	field  int // 0=Artist, 1=Title, 2=Album, 3=Genre
}

// recognizedPrefixes lists all accepted filter prefixes, longest first so that
// "artist:" is checked before "a:" (avoiding ambiguous partial matches).
var recognizedPrefixes = []prefixSpec{
	{"artist:", 0},
	{"title:", 1},
	{"album:", 2},
	{"genre:", 3},
	{"a:", 0},
	{"t:", 1},
	{"l:", 2},
	{"g:", 3},
}

// ParseFilterQuery parses a raw filter string into a FilterQuery.
//
// Recognized prefixes (a:/artist:, t:/title:, l:/album:, g:/genre:) are
// detected only at position 0 or immediately after a space. Each prefix
// captures all text until the next recognized prefix or end of string. Text
// before the first prefix goes into General. Duplicate prefixes: last one wins.
func ParseFilterQuery(raw string) FilterQuery {
	type segment struct {
		field int // -1 = General, 0=Artist, 1=Title, 2=Album, 3=Genre
		start int
	}

	lower := strings.ToLower(raw)
	var segments []segment
	i := 0

	for i < len(lower) {
		// Only recognize a prefix at position 0 or after a space.
		if i == 0 || lower[i-1] == ' ' {
			matched := false
			for _, ps := range recognizedPrefixes {
				if strings.HasPrefix(lower[i:], ps.prefix) {
					segments = append(segments, segment{field: ps.field, start: i + len(ps.prefix)})
					i += len(ps.prefix)
					matched = true
					break
				}
			}
			if matched {
				continue
			}
		}
		i++
	}

	var fq FilterQuery

	if len(segments) == 0 {
		// No prefixes found — everything is General.
		fq.General = strings.TrimSpace(raw)
		return fq
	}

	// Text before the first segment is General.
	if segments[0].start > 0 {
		// The start includes the prefix length; walk back to find where the
		// prefix token began (it's at start - len(prefix)).
		firstPrefixPos := findPrefixStart(lower, segments[0].start)
		fq.General = strings.TrimSpace(raw[:firstPrefixPos])
	}

	// Extract each segment's value.
	for j, seg := range segments {
		var end int
		if j+1 < len(segments) {
			end = findPrefixStart(lower, segments[j+1].start)
		} else {
			end = len(raw)
		}
		value := strings.TrimSpace(raw[seg.start:end])
		switch seg.field {
		case 0:
			fq.Artist = value
		case 1:
			fq.Title = value
		case 2:
			fq.Album = value
		case 3:
			fq.Genre = value
		}
	}

	return fq
}

// findPrefixStart returns the index where the prefix token starts, given that
// contentStart is the index right after the prefix's colon.
func findPrefixStart(lower string, contentStart int) int {
	// Walk backwards from contentStart past the prefix (e.g. "a:" or "artist:").
	for _, ps := range recognizedPrefixes {
		pLen := len(ps.prefix)
		candidate := contentStart - pLen
		if candidate >= 0 && strings.HasPrefix(lower[candidate:], ps.prefix) {
			// Must be at position 0 or preceded by a space.
			if candidate == 0 || lower[candidate-1] == ' ' {
				return candidate
			}
		}
	}
	return contentStart
}
