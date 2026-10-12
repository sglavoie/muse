package radio

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

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

// RemapTag re-points every reference to src at dest.
//
// It exists because merging two tags is not a deletion: the references that
// named the source have to follow it, or a filter the user built survives the
// merge as an unresolved id that pauses the DJ. A set already naming dest
// collapses to one entry rather than carrying it twice, and the result is
// normalized so a remapped set is indistinguishable from one the user picked in
// that shape.
//
// The receiver is never mutated: a live plan may be reading the same backing
// array off the update loop.
func (f FilterSet) RemapTag(src, dest int64) FilterSet {
	ids, changed := remapTagIDs(f.TagIDs, src, dest)
	if !changed {
		return f
	}
	f.TagIDs = ids
	return f
}

// remapTagIDs substitutes dest for src in ids, reporting whether src was
// present at all. A no-op returns the original slice so an unaffected caller
// keeps its identity and allocates nothing.
func remapTagIDs(ids []int64, src, dest int64) ([]int64, bool) {
	if src == dest || src <= 0 || dest <= 0 {
		return ids, false
	}
	found := false
	for _, id := range ids {
		if id == src {
			found = true
			break
		}
	}
	if !found {
		return ids, false
	}
	swapped := make([]int64, len(ids))
	for i, id := range ids {
		if id == src {
			id = dest
		}
		swapped[i] = id
	}
	return normalizedTagIDs(swapped), true
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
	for _, field := range filterFields {
		if !c.exclude.populated(field.kind) {
			continue
		}
		if value, ok := c.exclude.match(field.kind, facts); ok {
			return FilterVerdict{Blocked: Reason{Code: field.excludeCode, Value: value}}
		}
	}

	var matched Reasons
	for _, field := range filterFields {
		if !c.include.populated(field.kind) {
			continue
		}
		value, ok := c.include.match(field.kind, facts)
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
