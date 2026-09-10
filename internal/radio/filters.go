package radio

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

// unknownArtistSentinel mirrors the library's placeholder for files without an
// artist tag (internal/library/index.go). The domain keeps its own copy rather
// than importing an unexported constant; blank artists fold onto the same key
// so an explicit "Unknown Artist" filter covers both spellings.
const unknownArtistSentinel = "Unknown Artist"

// FilterSet is one side (include or exclude) of the DJ pickers. Values inside a
// field are OR'd together.
type FilterSet struct {
	TagIDs  []int64
	Genres  []string
	Artists []string
}

// IsEmpty reports whether the set constrains nothing.
func (f FilterSet) IsEmpty() bool {
	return len(f.TagIDs) == 0 && len(f.Genres) == 0 && len(f.Artists) == 0
}

// Validate reports the first non-positive tag id in the set. Genres and
// artists are free text and cannot be invalid, only empty.
func (f FilterSet) Validate() error {
	for _, id := range f.TagIDs {
		if id <= 0 {
			return fmt.Errorf("%w: %d", ErrInvalidTagID, id)
		}
	}
	return nil
}

// Normalized returns a canonical copy: tag ids deduplicated and sorted
// ascending, genres and artists trimmed, deduplicated case-insensitively
// (first spelling wins) and sorted by their folded key. Non-positive tag ids
// are dropped because they can never resolve — call Validate first to report
// them instead of losing them silently.
func (f FilterSet) Normalized() FilterSet {
	return FilterSet{
		TagIDs:  normalizedTagIDs(f.TagIDs),
		Genres:  normalizedFilterValues(f.Genres, normalizedGenreKey),
		Artists: normalizedFilterValues(f.Artists, artistFilterKey),
	}
}

// Equal reports whether the two sets constrain the same things, ignoring order
// and letter case.
func (f FilterSet) Equal(other FilterSet) bool {
	left, right := f.Normalized(), other.Normalized()
	if len(left.TagIDs) != len(right.TagIDs) ||
		len(left.Genres) != len(right.Genres) ||
		len(left.Artists) != len(right.Artists) {
		return false
	}
	for i, id := range left.TagIDs {
		if right.TagIDs[i] != id {
			return false
		}
	}
	return equalFolded(left.Genres, right.Genres, normalizedGenreKey) &&
		equalFolded(left.Artists, right.Artists, artistFilterKey)
}

// TagIDSet returns the set's tag ids as a lookup set, dropping non-positive
// ids. Returns nil when no usable id is present.
func (f FilterSet) TagIDSet() TagIDSet {
	ids := normalizedTagIDs(f.TagIDs)
	if len(ids) == 0 {
		return nil
	}
	set := make(TagIDSet, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// ResolveTags reports the set's unresolved tag references.
func (f FilterSet) ResolveTags(catalog TagCatalog) error {
	return resolveTagIDs(catalog, f.TagIDs)
}

// TagCatalog reports whether a tag id still exists. Storage sessions implement
// it against SQLite; the domain never depends on a database.
type TagCatalog interface {
	HasTag(id int64) bool
}

// TagIDSet is an in-memory TagCatalog.
type TagIDSet map[int64]struct{}

// HasTag implements TagCatalog.
func (s TagIDSet) HasTag(id int64) bool {
	_, ok := s[id]
	return ok
}

// UnresolvedTagError lists tag ids referenced by settings that no longer exist
// in the catalog. It unwraps to ErrUnresolvedTag.
type UnresolvedTagError struct {
	TagIDs []int64
}

// Error implements error.
func (e *UnresolvedTagError) Error() string {
	ids := make([]string, 0, len(e.TagIDs))
	for _, id := range e.TagIDs {
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	return fmt.Sprintf("%s: %s", ErrUnresolvedTag.Error(), strings.Join(ids, ", "))
}

// Unwrap lets callers match with errors.Is(err, ErrUnresolvedTag).
func (e *UnresolvedTagError) Unwrap() error { return ErrUnresolvedTag }

// resolveTagIDs collects every referenced id the catalog does not know. A nil
// catalog resolves nothing: with no way to confirm a reference, every id is
// reported rather than assumed valid.
func resolveTagIDs(catalog TagCatalog, groups ...[]int64) error {
	seen := make(map[int64]struct{})
	var unresolved []int64
	for _, group := range groups {
		for _, id := range group {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			if catalog != nil && catalog.HasTag(id) {
				continue
			}
			unresolved = append(unresolved, id)
		}
	}
	if len(unresolved) == 0 {
		return nil
	}
	sort.Slice(unresolved, func(i, j int) bool { return unresolved[i] < unresolved[j] })
	return &UnresolvedTagError{TagIDs: unresolved}
}

// TrackFacts is the minimal per-track metadata filter evaluation needs. Built
// from a library track plus its resolved tag ids so the domain stays free of
// storage types.
type TrackFacts struct {
	Hash   string
	Artist string
	Genres []string
	TagIDs map[int64]struct{}
}

// FactsFromTrack projects a library track (and its tag ids) into TrackFacts.
func FactsFromTrack(t library.Track, tagIDs map[int64]struct{}) TrackFacts {
	return TrackFacts{
		Hash:   t.Hash,
		Artist: t.Artist,
		Genres: library.SplitGenres(t.Genre),
		TagIDs: tagIDs,
	}
}

// FilterVerdict is the outcome of evaluating a track against the filters.
type FilterVerdict struct {
	// Eligible is true when no exclusion matched and every populated
	// inclusion field matched.
	Eligible bool
	// Blocked explains the first failure. Zero when Eligible.
	Blocked Reason
	// Matched lists the inclusion matches in field order (tags, genres,
	// artists). Empty when no inclusion field is populated.
	Matched Reasons
}

// CompiledFilters is an include/exclude pair normalized once and prepared for
// repeated matching. Planning sessions scan the whole library per batch, so
// compile once and reuse rather than paying normalization per candidate.
type CompiledFilters struct {
	include compiledSet
	exclude compiledSet
}

// CompileFilters normalizes and indexes the two filter sets.
func CompileFilters(include, exclude FilterSet) CompiledFilters {
	return CompiledFilters{
		include: compileSet(include),
		exclude: compileSet(exclude),
	}
}

// Match evaluates facts against the compiled filters. Values are OR'd within a
// field, populated inclusion fields are AND'd together, and exclusions win over
// inclusions. Missing metadata never satisfies an inclusion: a track with no
// genres cannot match a genre inclusion.
func (c CompiledFilters) Match(facts TrackFacts) FilterVerdict {
	for _, field := range c.exclude.fields(facts) {
		if !field.populated {
			continue
		}
		if value, ok := field.match(); ok {
			return FilterVerdict{Blocked: Reason{Code: field.excludeCode, Value: value}}
		}
	}

	var matched Reasons
	for _, field := range c.include.fields(facts) {
		if !field.populated {
			continue
		}
		value, ok := field.match()
		if !ok {
			return FilterVerdict{Blocked: Reason{Code: field.missingCode, Detail: field.detail}}
		}
		matched = append(matched, Reason{Code: field.includeCode, Value: value})
	}
	return FilterVerdict{Eligible: true, Matched: matched}
}

// MatchFilters evaluates a single track against the filters. Prefer
// CompileFilters when scanning more than a handful of candidates.
func MatchFilters(include, exclude FilterSet, facts TrackFacts) FilterVerdict {
	return CompileFilters(include, exclude).Match(facts)
}

// Conflicts returns the values present in both sets, normalized. Because
// exclusions win, every returned value is unsatisfiable.
func Conflicts(include, exclude FilterSet) FilterSet {
	inc, exc := include.Normalized(), exclude.Normalized()
	return FilterSet{
		TagIDs:  intersectTagIDs(inc.TagIDs, exc.TagIDs),
		Genres:  intersectValues(inc.Genres, exc.Genres, normalizedGenreKey),
		Artists: intersectValues(inc.Artists, exc.Artists, artistFilterKey),
	}
}

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
		k := key(value)
		if k == "" {
			continue
		}
		if filterValue, ok := idx[k]; ok {
			return filterValue, true
		}
	}
	return "", false
}

// filterField binds one picker field to the way it is matched and reported.
type filterField struct {
	populated   bool
	match       func() (string, bool)
	includeCode ReasonCode
	excludeCode ReasonCode
	missingCode ReasonCode
	detail      string
}

// fields describes the three picker fields of a compiled set.
func (c compiledSet) fields(facts TrackFacts) []filterField {
	return []filterField{
		{
			populated:   len(c.tagIDs) > 0,
			match:       func() (string, bool) { return matchTagIDs(c.tagIDs, facts.TagIDs) },
			includeCode: ReasonIncludeTag,
			excludeCode: ReasonExcludeTag,
			missingCode: ReasonMissingTag,
			detail:      "no tag in the inclusion list",
		},
		{
			populated:   len(c.genres) > 0,
			match:       func() (string, bool) { return c.genres.match(facts.Genres, normalizedGenreKey) },
			includeCode: ReasonIncludeGenre,
			excludeCode: ReasonExcludeGenre,
			missingCode: ReasonMissingGenre,
			detail:      "no genre in the inclusion list",
		},
		{
			populated:   len(c.artists) > 0,
			match:       func() (string, bool) { return c.artists.match([]string{facts.Artist}, artistFilterKey) },
			includeCode: ReasonIncludeArtist,
			excludeCode: ReasonExcludeArtist,
			missingCode: ReasonMissingArtist,
			detail:      "artist not in the inclusion list",
		},
	}
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
		return normalizedArtistKey(unknownArtistSentinel)
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
