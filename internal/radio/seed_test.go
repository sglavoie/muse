package radio_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

var seedFixedNow = time.Date(2026, 4, 12, 15, 30, 0, 0, time.UTC)

// TestSeedKindClassifiesEveryOrigin pins the hash namespacing. Kind is derived
// rather than stored, so a snapshot read back from the database has to classify
// the same way it did when it was captured — including one written by a build
// that knew a synthetic namespace this one does not.
func TestSeedKindClassifiesEveryOrigin(t *testing.T) {
	cases := []struct {
		name      string
		hash      string
		want      radio.SeedKind
		synthetic bool
	}{
		{"the zero snapshot", "", radio.SeedKindNone, false},
		// A real content hash is hexadecimal and can never contain a colon, so
		// it is the one non-empty case that must not read as synthetic.
		{"a library track", "8f4b2c", radio.SeedKindTrack, false},
		{"the whole library", radio.LibrarySeedHash, radio.SeedKindLibrary, true},
		{"a collection", radio.CollectionSeedPrefix + "Mixes ▸ Late night", radio.SeedKindCollection, true},
		{"an unknown synthetic namespace", radio.SyntheticSeedPrefix + "future:1", radio.SeedKindCollection, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := radio.SeedSnapshot{TrackHash: tc.hash}
			if got := s.Kind(); got != tc.want {
				t.Errorf("Kind() = %q, want %q", got, tc.want)
			}
			if got := s.IsSynthetic(); got != tc.synthetic {
				t.Errorf("IsSynthetic() = %v, want %v", got, tc.synthetic)
			}
		})
	}
}

// TestCoOccurrenceHashSkipsSyntheticSeeds guards the one genuinely
// seed-relative signal. A synthetic hash matches no play-history row, so asking
// for it spends a query to learn nothing and reports the session as seeded when
// its seed signal is unavailable by construction.
func TestCoOccurrenceHashSkipsSyntheticSeeds(t *testing.T) {
	real := radio.SeedSnapshot{TrackHash: "8f4b2c"}
	if got := real.CoOccurrenceHash(); got != "8f4b2c" {
		t.Errorf("a real seed reported co-occurrence hash %q, want its own hash", got)
	}
	for _, hash := range []string{radio.LibrarySeedHash, radio.CollectionSeedPrefix + "Mix"} {
		s := radio.SeedSnapshot{TrackHash: hash}
		if got := s.CoOccurrenceHash(); got != "" {
			t.Errorf("synthetic seed %q offered co-occurrence hash %q, want none", hash, got)
		}
	}
}

// TestCollectionLabelRoundTrip checks the collection a session was started from
// survives in the hash, so a restored session can still say what it plays from.
func TestCollectionLabelRoundTrip(t *testing.T) {
	seed := radio.CaptureCollectionSeed("Mixes ▸ Late night", nil, nil, radio.FixedClock(seedFixedNow))

	label, ok := seed.CollectionLabel()
	if !ok {
		t.Fatalf("a collection seed reported no label: %+v", seed)
	}
	if label != "Mixes ▸ Late night" {
		t.Errorf("label = %q, want the collection it was aggregated from", label)
	}
	if _, ok := (radio.SeedSnapshot{TrackHash: "8f4b2c"}).CollectionLabel(); ok {
		t.Error("a track seed claimed a collection label")
	}
	if _, ok := radio.WholeLibrarySeed(radio.FixedClock(seedFixedNow)).CollectionLabel(); ok {
		t.Error("the whole-library seed claimed a collection label")
	}
}

// TestSyntheticSeedsAreRestorableAndPlannable is the reason synthetic hashes
// exist at all: IsZero is keyed on the hash and PlanRequest.Validate refuses a
// zero seed, so a collection or whole-library start with no hash could neither
// be planned from nor survive a restart.
func TestSyntheticSeedsAreRestorableAndPlannable(t *testing.T) {
	seeds := map[string]radio.SeedSnapshot{
		"whole library": radio.WholeLibrarySeed(radio.FixedClock(seedFixedNow)),
		"collection": radio.CaptureCollectionSeed("Tags ▸ chill", []library.Track{
			{Hash: "a", Artist: "Alice", Genre: "Rock", Duration: 200},
		}, nil, radio.FixedClock(seedFixedNow)),
	}
	for name, seed := range seeds {
		t.Run(name, func(t *testing.T) {
			if seed.IsZero() {
				t.Fatalf("%s seed is zero, so it cannot be persisted: %+v", name, seed)
			}
			if seed.CapturedAt != seedFixedNow {
				t.Errorf("CapturedAt = %v, want the injected clock %v", seed.CapturedAt, seedFixedNow)
			}
			req := radio.PlanRequest{
				Settings:    radio.DefaultSettings(),
				Seed:        seed,
				BatchSize:   10,
				RequestedAt: seedFixedNow,
			}
			if err := req.Validate(); err != nil {
				t.Errorf("the planner refused a %s seed: %v", name, err)
			}
		})
	}
}

// TestCaptureCollectionSeedAggregates covers what a collection seed actually
// describes: the dominant artist, the union of genres, the median duration, and
// the union of tags across the whole set.
func TestCaptureCollectionSeedAggregates(t *testing.T) {
	tracks := []library.Track{
		{Hash: "a", Artist: "Alice", Genre: "Rock", Duration: 100},
		{Hash: "b", Artist: "Alice", Genre: "Jazz;Rock", Duration: 200},
		{Hash: "c", Artist: "Bob", Genre: "Folk", Duration: 300},
	}
	tags := map[string]map[int64]struct{}{
		"a": {7: {}},
		"c": {3: {}, 7: {}},
	}

	seed := radio.CaptureCollectionSeed("  Library  ", tracks, tags, radio.FixedClock(seedFixedNow))

	if got, want := seed.TrackHash, radio.CollectionSeedPrefix+"Library"; got != want {
		t.Errorf("TrackHash = %q, want %q", got, want)
	}
	if seed.Title != "Library" {
		t.Errorf("Title = %q, want the trimmed label", seed.Title)
	}
	if seed.Artist != "Alice" {
		t.Errorf("Artist = %q, want the most common one", seed.Artist)
	}
	if want := []string{"Folk", "Jazz", "Rock"}; !reflect.DeepEqual(seed.Genres, want) {
		t.Errorf("Genres = %v, want the sorted union %v", seed.Genres, want)
	}
	if seed.Duration != 200 {
		t.Errorf("Duration = %v, want the median 200", seed.Duration)
	}
	if want := []int64{3, 7}; !reflect.DeepEqual(seed.TagIDs, want) {
		t.Errorf("TagIDs = %v, want the sorted union %v", seed.TagIDs, want)
	}
}

// TestAggregateSeedDedupesByHash pins the correction the shared helper carries:
// a collection that lists the same track twice must not give its artist twice
// the weight, or a mix with one repeat would be seeded from the wrong artist.
func TestAggregateSeedDedupesByHash(t *testing.T) {
	tracks := []library.Track{
		{Hash: "a", Artist: "Alice", Genre: "Rock", Duration: 100},
		{Hash: "a", Artist: "Alice", Genre: "Rock", Duration: 100},
		{Hash: "b", Artist: "Bob", Genre: "Rock", Duration: 100},
	}

	// Deduplicated the tally is 1-1 and the name breaks the tie; counted twice
	// Alice would win outright.
	if got := radio.AggregateSeed(tracks, nil).Artist; got != "Alice" {
		t.Fatalf("Artist = %q, want Alice", got)
	}
	tracks[0].Artist, tracks[1].Artist = "Zed", "Zed"
	if got := radio.AggregateSeed(tracks, nil).Artist; got != "Bob" {
		t.Errorf("Artist = %q: the repeated track was counted twice, want Bob on the tie-break", got)
	}
}

// TestAggregateSeedIsDeterministic runs the same tied input repeatedly. Go
// randomizes map iteration, so a tally resolved by visit order would produce a
// different seed from one press to the next and make a session unreproducible.
func TestAggregateSeedIsDeterministic(t *testing.T) {
	tracks := []library.Track{
		{Hash: "a", Artist: "Zed", Genre: "Rock;Jazz", Duration: 100},
		{Hash: "b", Artist: "Alice", Genre: "Folk", Duration: 200},
		{Hash: "c", Artist: "Mel", Genre: "Ambient", Duration: 300},
	}
	tags := map[string]map[int64]struct{}{"a": {9: {}, 2: {}}, "b": {5: {}}}

	want := radio.AggregateSeed(tracks, tags)
	if want.Artist != "Alice" {
		t.Fatalf("Artist = %q, want the lexicographically first of the tied artists", want.Artist)
	}
	if want.Hash != "" {
		t.Errorf("Hash = %q, want empty: an aggregate describes no single track", want.Hash)
	}
	for i := 0; i < 200; i++ {
		if got := radio.AggregateSeed(tracks, tags); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d produced %+v, want %+v", i, got, want)
		}
	}
}

// TestAggregateSeedIgnoresUnknownDurations checks the median is taken over
// durations that exist. A library row whose duration was never extracted reads
// as 0, and counting those would drag every collection seed toward silence.
func TestAggregateSeedIgnoresUnknownDurations(t *testing.T) {
	agg := radio.AggregateSeed([]library.Track{
		{Hash: "a", Artist: "Alice"},
		{Hash: "b", Artist: "Alice", Duration: 240},
		{Hash: "c", Artist: "Alice"},
	}, nil)

	if agg.Duration != 240 {
		t.Errorf("Duration = %v, want the only known duration 240", agg.Duration)
	}
}

// TestAggregateSeedEmptySet is the degenerate case every fallback path reaches:
// it must produce a usable zero aggregate rather than panic.
func TestAggregateSeedEmptySet(t *testing.T) {
	agg := radio.AggregateSeed(nil, nil)

	if agg.Artist != "" || agg.Genres != nil || agg.Duration != 0 || agg.TagIDs != nil {
		t.Errorf("an empty set aggregated to %+v, want the zero aggregate", agg)
	}
}
