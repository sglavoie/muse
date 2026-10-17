package library

import (
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type searchValue struct {
	folded    string
	positions []int
}

func prepareSearchValue(s string) searchValue {
	folded, positions := foldSearch(s)
	return searchValue{folded: folded, positions: positions}
}

// CachedFuzzySearcher reuses normalization across keystrokes. Keys are complete
// field values, so edits, rescans and reordering cannot leave stale matches.
// Its bounded cache is safe to share between model copies and goroutines.
type CachedFuzzySearcher struct {
	mu     sync.Mutex
	values map[string]searchValue
}

const maxSearchValues = 250_000

func (s *CachedFuzzySearcher) Search(query string, tracks []Track) []Match {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil || len(s.values) >= maxSearchValues {
		s.values = make(map[string]searchValue)
	}
	return searchTracks(query, tracks, s.value)
}

func (s *CachedFuzzySearcher) value(text string) searchValue {
	if cached, ok := s.values[text]; ok {
		return cached
	}
	value := prepareSearchValue(text)
	if len(s.values) < maxSearchValues {
		s.values[text] = value
	}
	return value
}

func searchTracks(query string, tracks []Track, value func(string) searchValue) []Match {
	fq := ParseFilterQuery(query)
	for _, term := range []*string{&fq.Artist, &fq.Title, &fq.Album, &fq.Genre, &fq.AlbumArtist, &fq.General} {
		*term = FoldSearchText(*term)
	}
	// A non-nil empty result is a completed search, not an uncached search.
	matches := make([]Match, 0)
	for i, track := range tracks {
		match, ok := matchPreparedTrack(track, fq, value)
		if ok {
			match.TrackIndex = i
			matches = append(matches, match)
		}
	}
	return matches
}

func matchPreparedTrack(t Track, q FilterQuery, value func(string) searchValue) (Match, bool) {
	m := Match{}
	if !q.IsFieldSpecific() {
		var ok bool
		m.MatchedIndexes, ok = preparedGeneral(t, q.General, value)
		return m, ok
	}
	fields := []struct {
		text, query string
		positions   *[]int
	}{
		{t.Artist, q.Artist, &m.ArtistPositions}, {t.Title, q.Title, &m.TitlePositions},
		{t.Album, q.Album, &m.AlbumPositions}, {t.Genre, q.Genre, &m.GenrePositions},
		{t.AlbumArtist, q.AlbumArtist, nil},
	}
	for _, field := range fields {
		if field.query == "" {
			continue
		}
		positions, ok := preparedIndexes(field.text, field.query, value(field.text))
		if !ok {
			return Match{}, false
		}
		if field.positions != nil {
			*field.positions = positions
		}
	}
	if q.General != "" {
		if _, ok := preparedGeneral(t, q.General, value); !ok {
			return Match{}, false
		}
	}
	return m, true
}

func preparedGeneral(t Track, query string, value func(string) searchValue) ([]int, bool) {
	if query == "" {
		return nil, true
	}
	offset := 0
	for _, field := range []string{t.Artist, t.Title, t.Album, filepath.Base(t.Path)} {
		positions, ok := preparedIndexes(field, query, value(field))
		if ok {
			for i := range positions {
				positions[i] += offset
			}
			return positions, true
		}
		offset += utf8.RuneCountInString(field) + 1
	}
	return nil, false
}

func preparedIndexes(original, query string, value searchValue) ([]int, bool) {
	index := strings.Index(value.folded, query)
	if index < 0 {
		return nil, false
	}
	index = utf8.RuneCountInString(value.folded[:index])
	length := utf8.RuneCountInString(query)
	start, end := value.positions[index], value.positions[index+length-1]+1
	runes := []rune(original)
	for end < len(runes) && unicode.Is(unicode.Mn, runes[end]) {
		end++
	}
	return consecutiveIndexes(start, end-start), true
}
