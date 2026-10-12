package radio

import (
	"sort"
	"strconv"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

// compiledSet is one normalized filter side indexed for lookup.
type compiledSet struct {
	tagIDs  []int64
	genres  valueIndex
	artists valueIndex
}

func compileSet(f FilterSet) compiledSet {
	normalized := f.Normalized()
	return compiledSet{
		tagIDs:  normalized.TagIDs,
		genres:  newValueIndex(normalized.Genres, normalizedGenreKey),
		artists: newValueIndex(normalized.Artists, artistFilterKey),
	}
}

// valueIndex maps a folded filter value onto the spelling to report back.
type valueIndex map[string]string

func newValueIndex(values []string, key func(string) string) valueIndex {
	if len(values) == 0 {
		return nil
	}
	index := make(valueIndex, len(values))
	for _, value := range values {
		k := key(value)
		if k == "" {
			continue
		}
		if _, dup := index[k]; !dup {
			index[k] = value
		}
	}
	if len(index) == 0 {
		return nil
	}
	return index
}

// match reports the filter value matching any of the track's values. Track
// values are visited in order, so the reported value is stable for a given
// track. Blank track values are skipped unless key maps them onto a real key
// (as artistFilterKey does for unknown artists).
func (idx valueIndex) match(trackValues []string, key func(string) string) (string, bool) {
	if len(idx) == 0 {
		return "", false
	}
	for _, value := range trackValues {
		if filterValue, ok := idx.matchOne(value, key); ok {
			return filterValue, true
		}
	}
	return "", false
}

// matchOne is match for a field that holds a single track value, which spares
// the planner a one-element slice per candidate.
func (idx valueIndex) matchOne(trackValue string, key func(string) string) (string, bool) {
	if len(idx) == 0 {
		return "", false
	}
	k := key(trackValue)
	if k == "" {
		return "", false
	}
	filterValue, ok := idx[k]
	return filterValue, ok
}

// filterKind names one of the three picker fields.
type filterKind int

const (
	filterKindTag filterKind = iota
	filterKindGenre
	filterKindArtist
)

// filterField binds one picker field to the reasons it reports.
type filterField struct {
	kind        filterKind
	includeCode ReasonCode
	excludeCode ReasonCode
	missingCode ReasonCode
	detail      string
}

// filterFields is the order the three picker fields are evaluated in, for both
// sides. It is package-level data rather than something Match builds: matching
// runs once per candidate per batch, and describing the fields with per-call
// closures was the planner's largest allocation source in BenchmarkPartition.
var filterFields = [...]filterField{
	{
		kind:        filterKindTag,
		includeCode: ReasonIncludeTag,
		excludeCode: ReasonExcludeTag,
		missingCode: ReasonMissingTag,
		detail:      "no tag in the inclusion list",
	},
	{
		kind:        filterKindGenre,
		includeCode: ReasonIncludeGenre,
		excludeCode: ReasonExcludeGenre,
		missingCode: ReasonMissingGenre,
		detail:      "no genre in the inclusion list",
	},
	{
		kind:        filterKindArtist,
		includeCode: ReasonIncludeArtist,
		excludeCode: ReasonExcludeArtist,
		missingCode: ReasonMissingArtist,
		detail:      "artist not in the inclusion list",
	},
}

// populated reports whether this side filters on the given field at all.
func (c compiledSet) populated(kind filterKind) bool {
	switch kind {
	case filterKindTag:
		return len(c.tagIDs) > 0
	case filterKindGenre:
		return len(c.genres) > 0
	case filterKindArtist:
		return len(c.artists) > 0
	}
	return false
}

// match reports the filter value of the given field that the facts satisfy.
func (c compiledSet) match(kind filterKind, facts TrackFacts) (string, bool) {
	switch kind {
	case filterKindTag:
		return matchTagIDs(c.tagIDs, facts.TagIDs)
	case filterKindGenre:
		return c.genres.match(facts.Genres, normalizedGenreKey)
	case filterKindArtist:
		return c.artists.matchOne(facts.Artist, artistFilterKey)
	}
	return "", false
}

// matchTagIDs reports the first filter tag id the track carries. The filter
// list is normally far shorter than the track's tag set, so it drives the loop.
func matchTagIDs(filterIDs []int64, trackTags map[int64]struct{}) (string, bool) {
	for _, id := range filterIDs {
		if _, ok := trackTags[id]; ok {
			return strconv.FormatInt(id, 10), true
		}
	}
	return "", false
}

// artistFilterKey folds an artist for filter comparison. Blank artists map onto
// the library's "Unknown Artist" sentinel so both spellings of "untagged" are
// picked by the same filter entry — and only when that entry is explicitly
// listed.
func artistFilterKey(artist string) string {
	key := normalizedArtistKey(artist)
	if key == "" {
		return normalizedArtistKey(library.UnknownArtist)
	}
	return key
}

func normalizedTagIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	normalized := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	if len(normalized) == 0 {
		return nil
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	return normalized
}

func normalizedFilterValues(values []string, key func(string) string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		k := key(trimmed)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	sort.Slice(normalized, func(i, j int) bool {
		return key(normalized[i]) < key(normalized[j])
	})
	return normalized
}

func equalFolded(left, right []string, key func(string) string) bool {
	for i, value := range left {
		if key(value) != key(right[i]) {
			return false
		}
	}
	return true
}

func intersectTagIDs(left, right []int64) []int64 {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	rightSet := make(map[int64]struct{}, len(right))
	for _, id := range right {
		rightSet[id] = struct{}{}
	}
	var shared []int64
	for _, id := range left {
		if _, ok := rightSet[id]; ok {
			shared = append(shared, id)
		}
	}
	return shared
}

func intersectValues(left, right []string, key func(string) string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	rightKeys := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightKeys[key(value)] = struct{}{}
	}
	var shared []string
	for _, value := range left {
		if _, ok := rightKeys[key(value)]; ok {
			shared = append(shared, value)
		}
	}
	return shared
}
