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
	// A non-matching query should allocate no result storage. On the first
	// match, reserve once: broad queries commonly match the whole library,
	// and repeatedly growing this wide struct slice copies hundreds of MB.
	matches := make([]Match, 0)
	positions := matchPositions{blockSize: min(4096, max(64, len(tracks)))}
	for i, track := range tracks {
		match, ok := matchPreparedTrack(track, fq, value, &positions)
		if ok {
			if len(matches) == 0 {
				matches = make([]Match, 0, len(tracks))
			}
			match.TrackIndex = i
			matches = append(matches, match)
		}
	}
	return matches
}

func matchPreparedTrack(t Track, q FilterQuery, value func(string) searchValue, positions *matchPositions) (Match, bool) {
	if !q.IsFieldSpecific() {
		positions, ok := preparedGeneral(t, q.General, value, positions)
		return Match{MatchedIndexes: positions}, ok
	}
	return matchPreparedFields(t, q, value, positions)
}

func matchPreparedFields(t Track, q FilterQuery, value func(string) searchValue, positions *matchPositions) (Match, bool) {
	// Do not store pointers to Match fields in a loop: that makes the entire
	// result escape to the heap for every candidate, including rejected tracks.
	var m Match
	fields := [...]struct{ text, query string }{
		{t.Artist, q.Artist}, {t.Title, q.Title}, {t.Album, q.Album},
		{t.Genre, q.Genre}, {t.AlbumArtist, q.AlbumArtist},
	}
	for i, field := range fields {
		if field.query == "" {
			continue
		}
		positions, ok := preparedIndexes(field.text, field.query, value(field.text), positions)
		if !ok {
			return Match{}, false
		}
		switch i {
		case 0:
			m.ArtistPositions = positions
		case 1:
			m.TitlePositions = positions
		case 2:
			m.AlbumPositions = positions
		case 3:
			m.GenrePositions = positions
		}
	}
	if q.General != "" {
		if _, ok := preparedGeneral(t, q.General, value, positions); !ok {
			return Match{}, false
		}
	}
	return m, true
}

func preparedGeneral(t Track, query string, value func(string) searchValue, positions *matchPositions) ([]int, bool) {
	if query == "" {
		return nil, true
	}
	offset := 0
	for _, field := range []string{t.Artist, t.Title, t.Album, filepath.Base(t.Path)} {
		positions, ok := preparedIndexes(field, query, value(field), positions)
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

func preparedIndexes(original, query string, value searchValue, positions *matchPositions) ([]int, bool) {
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
	return positions.indexes(start, end-start), true
}

// Search results own disjoint, capacity-limited spans in these blocks. This
// avoids one allocation per matching track without reusing memory across
// searches or aliasing writable highlight slices between matches.
type matchPositions struct {
	remaining []int
	blockSize int
}

func (p *matchPositions) indexes(start, length int) []int {
	if len(p.remaining) < length {
		p.remaining = make([]int, max(p.blockSize, length))
	}
	out := p.remaining[:length:length]
	p.remaining = p.remaining[length:]
	for i := range out {
		out[i] = start + i
	}
	return out
}
