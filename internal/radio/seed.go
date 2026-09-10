package radio

import (
	"sort"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

// Synthetic seed hashes.
//
// A seed snapshot is only restorable across a restart when it carries a hash:
// SeedSnapshot.IsZero is keyed on it and PlanRequest.Validate refuses a zero
// seed. A collection start and a whole-library start have no library track to
// borrow a hash from, so they are given a namespaced one instead. The namespace
// is what keeps them apart from a real BLAKE3 hash, which is hexadecimal and can
// never contain a colon.
const (
	// SyntheticSeedPrefix marks every hash that names no library track.
	SyntheticSeedPrefix = "synthetic:"
	// CollectionSeedPrefix is the namespace of a seed aggregated from a
	// user-targeted collection; the remainder is the collection's label.
	CollectionSeedPrefix = SyntheticSeedPrefix + "collection:"
	// LibrarySeedHash is the seed of a session started with nothing current.
	// It carries no artist, genre, tag, or duration, so scoring falls back to
	// the behavioral signals across the whole library.
	LibrarySeedHash = SyntheticSeedPrefix + "library"
)

// SeedKind names where a seed came from. It is derived from the hash rather
// than stored, so a snapshot read back from the database classifies the same
// way it did when it was captured.
type SeedKind string

const (
	// SeedKindNone is the kind of the zero snapshot.
	SeedKindNone SeedKind = "none"
	// SeedKindTrack is a seed captured from one library track.
	SeedKindTrack SeedKind = "track"
	// SeedKindCollection is a seed aggregated from a targeted collection.
	SeedKindCollection SeedKind = "collection"
	// SeedKindLibrary is the whole-library seed used when nothing is current.
	SeedKindLibrary SeedKind = "library"
)

// Kind classifies the snapshot.
func (s SeedSnapshot) Kind() SeedKind {
	hash := strings.TrimSpace(s.TrackHash)
	switch {
	case hash == "":
		return SeedKindNone
	case hash == LibrarySeedHash:
		return SeedKindLibrary
	case strings.HasPrefix(hash, CollectionSeedPrefix):
		return SeedKindCollection
	case strings.HasPrefix(hash, SyntheticSeedPrefix):
		// An unknown synthetic namespace is still not a library track, so it
		// must not be treated as one.
		return SeedKindCollection
	default:
		return SeedKindTrack
	}
}

// IsSynthetic reports whether the seed names no library track.
func (s SeedSnapshot) IsSynthetic() bool {
	return strings.HasPrefix(strings.TrimSpace(s.TrackHash), SyntheticSeedPrefix)
}

// CoOccurrenceHash returns the hash to ask the play history about, or "" when
// there is nothing to ask.
//
// Co-occurrence is the one genuinely seed-relative signal: it answers "what did
// this listener play alongside *this track*". A synthetic hash matches no row in
// the history, so asking for it would spend a query to learn nothing. Every
// other behavioral signal describes the listener and is loaded regardless.
func (s SeedSnapshot) CoOccurrenceHash() string {
	if s.IsSynthetic() {
		return ""
	}
	return strings.TrimSpace(s.TrackHash)
}

// CollectionLabel returns the collection a synthetic collection seed was
// aggregated from. The second result is false for every other kind.
func (s SeedSnapshot) CollectionLabel() (string, bool) {
	hash := strings.TrimSpace(s.TrackHash)
	if !strings.HasPrefix(hash, CollectionSeedPrefix) {
		return "", false
	}
	return strings.TrimPrefix(hash, CollectionSeedPrefix), true
}

// WholeLibrarySeed returns the seed for a session started with no current
// track. It is deliberately a real, restorable snapshot rather than the zero
// value: the planner refuses an empty seed, and "start from nothing in
// particular" has to survive a restart like any other start.
func WholeLibrarySeed(clock Clock) SeedSnapshot {
	return SeedSnapshot{TrackHash: LibrarySeedHash, Title: "Whole library", CapturedAt: nowFrom(clock)}
}

// CaptureCollectionSeed aggregates a targeted collection — a bulk selection, a
// mix, an album, a playlist, a tag — into one restorable seed snapshot.
//
// label names the collection and becomes the snapshot's title as well as the
// tail of its synthetic hash, so restoring the session restores the same seed
// even after the collection's membership has changed. Duplicate tracks are
// collapsed by content hash before anything is counted: a collection that lists
// the same track twice must not give its artist twice the weight.
func CaptureCollectionSeed(label string, tracks []library.Track, trackTags map[string]map[int64]struct{}, clock Clock) SeedSnapshot {
	agg := AggregateSeed(tracks, trackTags)
	ids := make([]int64, 0, len(agg.TagIDs))
	for id := range agg.TagIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) == 0 {
		ids = nil
	}
	return SeedSnapshot{
		TrackHash:  CollectionSeedPrefix + strings.TrimSpace(label),
		Title:      strings.TrimSpace(label),
		Artist:     agg.Artist,
		Genres:     agg.Genres,
		TagIDs:     ids,
		Duration:   agg.Duration,
		CapturedAt: nowFrom(clock),
	}
}

// AggregateSeed computes the aggregate metadata of a track set: its most common
// artist, the union of its genres, its median positive duration, and the union
// of its tags. The returned SeedTrack carries no hash, because the aggregate
// describes no single track.
//
// Every step is deterministic. The artist tally is resolved by count and then
// by name, so two artists appearing equally often always yield the same seed
// instead of whichever one Go's map iteration happened to visit first; the genre
// union is sorted for the same reason. A seed that changes between two runs over
// identical input would make a session unreproducible and a test meaningless.
//
// Tracks are deduplicated by content hash first. Hash-less rows cannot be
// compared and are all kept.
func AggregateSeed(tracks []library.Track, trackTags map[string]map[int64]struct{}) SeedTrack {
	unique := dedupeTracksByHash(tracks)

	artistCount := make(map[string]int, len(unique))
	genreSet := make(map[string]struct{})
	tagSet := make(map[int64]struct{})
	durs := make([]float64, 0, len(unique))
	for _, t := range unique {
		if t.Artist != "" {
			artistCount[t.Artist]++
		}
		for _, g := range library.SplitGenres(t.Genre) {
			genreSet[g] = struct{}{}
		}
		for id := range trackTags[t.Hash] {
			tagSet[id] = struct{}{}
		}
		if t.Duration > 0 {
			durs = append(durs, t.Duration)
		}
	}

	genres := make([]string, 0, len(genreSet))
	for g := range genreSet {
		genres = append(genres, g)
	}
	sort.Strings(genres)
	if len(genres) == 0 {
		genres = nil
	}

	sort.Float64s(durs)
	var medianDur float64
	if len(durs) > 0 {
		medianDur = durs[len(durs)/2]
	}
	if len(tagSet) == 0 {
		tagSet = nil
	}

	return SeedTrack{
		Artist:   topArtist(artistCount),
		Genres:   genres,
		Duration: medianDur,
		TagIDs:   tagSet,
		// Hash intentionally empty: an aggregate is not a real track.
	}
}

// dedupeTracksByHash keeps the first occurrence of each content hash and every
// hash-less row, preserving input order.
func dedupeTracksByHash(tracks []library.Track) []library.Track {
	seen := make(map[string]struct{}, len(tracks))
	out := make([]library.Track, 0, len(tracks))
	for _, t := range tracks {
		if t.Hash != "" {
			if _, dup := seen[t.Hash]; dup {
				continue
			}
			seen[t.Hash] = struct{}{}
		}
		out = append(out, t)
	}
	return out
}

// topArtist returns the most frequent artist, breaking ties on the name so the
// result never depends on map iteration order.
func topArtist(counts map[string]int) string {
	var top string
	var best int
	for artist, count := range counts {
		// Counts are always positive, so the first artist visited always wins
		// the count comparison against the zero initial best; the name
		// comparison only ever decides a genuine tie.
		if count > best || (count == best && artist < top) {
			best = count
			top = artist
		}
	}
	return top
}
