package library

import (
	"path/filepath"
	"runtime"
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
	return searchTracks(query, tracks, s.lookup)
}

// lookup gives one search worker its view of the cache. Workers only read
// s.values while a search runs and keep their misses to themselves; the
// returned flush folds those misses into s.values, and searchTracks calls it
// only after every worker has finished, so the map is never read and written
// at once.
func (s *CachedFuzzySearcher) lookup() (func(string) searchValue, func()) {
	var added map[string]searchValue
	value := func(text string) searchValue {
		if cached, ok := s.values[text]; ok {
			return cached
		}
		if cached, ok := added[text]; ok {
			return cached
		}
		value := prepareSearchValue(text)
		if len(s.values)+len(added) < maxSearchValues {
			if added == nil {
				added = make(map[string]searchValue)
			}
			added[text] = value
		}
		return value
	}
	flush := func() {
		for text, value := range added {
			if len(s.values) >= maxSearchValues {
				return
			}
			s.values[text] = value
		}
	}
	return value, flush
}

// uncachedLookup is the lookup for searches that keep no cache between calls.
func uncachedLookup() (func(string) searchValue, func()) {
	return prepareSearchValue, func() {}
}

// searchChunkSize is the number of tracks each search worker scans. Libraries
// smaller than two chunks are searched on the calling goroutine, where a
// keystroke already costs well under a millisecond.
const searchChunkSize = 4096

// searchTracks matches query against every track. Large libraries are split
// into chunks scanned in parallel, one lookup per chunk from newLookup; the
// matches are concatenated in chunk order, so TrackIndex order is unchanged.
func searchTracks(query string, tracks []Track, newLookup func() (func(string) searchValue, func())) []Match {
	fq := ParseFilterQuery(query)
	for _, term := range []*string{&fq.Artist, &fq.Title, &fq.Album, &fq.Genre, &fq.AlbumArtist, &fq.General} {
		*term = FoldSearchText(*term)
	}
	chunks := min(runtime.GOMAXPROCS(0), len(tracks)/searchChunkSize)
	if chunks < 2 {
		value, flush := newLookup()
		defer flush()
		return searchChunk(fq, tracks, 0, value)
	}

	size := (len(tracks) + chunks - 1) / chunks
	results := make([][]Match, chunks)
	flushes := make([]func(), chunks)
	var wg sync.WaitGroup
	for c := range chunks {
		lo, hi := c*size, min((c+1)*size, len(tracks))
		value, flush := newLookup()
		flushes[c] = flush
		wg.Go(func() { results[c] = searchChunk(fq, tracks[lo:hi], lo, value) })
	}
	wg.Wait()
	for _, flush := range flushes {
		flush()
	}

	total := 0
	for _, r := range results {
		total += len(r)
	}
	// A non-nil empty result is a completed search, not an uncached search.
	if total == 0 {
		return make([]Match, 0)
	}
	matches := make([]Match, 0, total)
	for _, r := range results {
		matches = append(matches, r...)
	}
	return matches
}

// searchChunk matches fq against tracks, numbering matches from offset.
func searchChunk(fq FilterQuery, tracks []Track, offset int, value func(string) searchValue) []Match {
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
			match.TrackIndex = offset + i
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
	// Extend the highlight over combining marks that follow the match in the
	// original, walking it in place rather than converting it to []rune.
	rest := original
	for range end {
		_, n := utf8.DecodeRuneInString(rest)
		rest = rest[n:]
	}
	for rest != "" {
		r, n := utf8.DecodeRuneInString(rest)
		if !unicode.Is(unicode.Mn, r) {
			break
		}
		end++
		rest = rest[n:]
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
