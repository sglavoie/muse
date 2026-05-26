package library

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

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
		field       int // 0=Artist, 1=Title, 2=Album, 3=Genre
		prefixStart int
		start       int
	}

	var segments []segment
	i := 0

	for i < len(raw) {
		// Only recognize a prefix at position 0 or after whitespace.
		if isFilterBoundary(raw, i) {
			matched := false
			for _, ps := range recognizedPrefixes {
				if hasPrefixFold(raw, i, ps.prefix) {
					segments = append(segments, segment{
						field:       ps.field,
						prefixStart: i,
						start:       i + len(ps.prefix),
					})
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
	if segments[0].prefixStart > 0 {
		fq.General = strings.TrimSpace(raw[:segments[0].prefixStart])
	}

	// Extract each segment's value.
	for j, seg := range segments {
		var end int
		if j+1 < len(segments) {
			end = segments[j+1].prefixStart
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

func hasPrefixFold(s string, pos int, prefix string) bool {
	end := pos + len(prefix)
	return end <= len(s) && strings.EqualFold(s[pos:end], prefix)
}

func isFilterBoundary(s string, pos int) bool {
	if pos == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s[:pos])
	return unicode.IsSpace(r)
}
