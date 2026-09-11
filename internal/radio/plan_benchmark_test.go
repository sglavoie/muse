package radio

import (
	"fmt"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// Scale benchmarks for the planner.
//
// The sizes are the ones the product has to survive rather than the ones the
// code is comfortable with. LibraryIndex documents a 50,000-track soft bound,
// so 100,000 is deliberately past it: continuous radio plans off a full library
// snapshot on every refill, and a planner that degrades at twice the documented
// bound would degrade in the one place the user cannot avoid.
//
// What these measure is the whole per-batch cost, because that is what the event
// loop waits on. Partition walks every candidate; HydrationTargets decides which
// hashes get a behavioral read; PlanFrom scores and selects. They are benchmarked
// separately as well as end to end so a regression can be attributed instead of
// merely noticed — a jump in allocations per batch means something started
// copying the library, and only the split says which pass did it.
//
// Every case pins Rand to FailingRand so selection follows score order. A
// weighted draw would add a variable number of random reads per batch and make
// the numbers wobble for reasons that have nothing to do with scale.

// benchSizes are the library scales session 14 records evidence for.
var benchSizes = []int{1_000, 10_000, 100_000}

// benchArtists bounds artist diversity so the per-artist cap is actually
// consulted at every size. One artist per track would make PickBatch's
// bookkeeping free and hide the cost the real library pays.
const benchArtists = 200

// benchGenres is the genre pool the filter benchmarks select from.
var benchGenres = []string{"Rock", "Jazz", "Electronic", "Folk", "Classical"}

// benchTracks generates count deterministic candidates spread across artists,
// genres, and discovery classes.
//
// The mix is one third never-played, one third stale, one third familiar, so both
// pools stay large enough that a discovery target of any size is fillable and the
// partition never collapses into the single-pool case.
func benchTracks(count int) []library.Track {
	tracks := make([]library.Track, count)
	for i := range count {
		hash := fmt.Sprintf("%064x", i)
		t := library.Track{
			Hash:     hash,
			Path:     "/music/" + hash + ".mp3",
			Artist:   fmt.Sprintf("Artist %03d", i%benchArtists),
			Title:    fmt.Sprintf("Track %06d", i),
			Album:    fmt.Sprintf("Album %04d", i%(benchArtists*2)),
			Genre:    benchGenres[i%len(benchGenres)],
			Duration: 180 + float64(i%120),
		}
		switch i % 3 {
		case 1:
			t.PlayCount = 4 + i%7
			t.LastPlayedAt = daysAgo(120 + i%200)
		case 2:
			t.PlayCount = 9 + i%11
			t.LastPlayedAt = daysAgo(1 + i%30)
		}
		tracks[i] = t
	}
	return tracks
}

// benchTrackTags assigns each track a small tag set drawn from a fixed pool, so
// the shared-tag score and the tag filters both have something to work with.
func benchTrackTags(tracks []library.Track) map[string]map[int64]struct{} {
	tags := make(map[string]map[int64]struct{}, len(tracks))
	for i, t := range tracks {
		tags[t.Hash] = map[int64]struct{}{
			int64(i%17) + 1: {},
			int64(i%5) + 20: {},
		}
	}
	return tags
}

// benchHashSet returns the hashes of every nth track, which is how the
// benchmarks build the protected, recent, favorite, and dismissed sets without
// making any of them the whole library.
func benchHashSet(tracks []library.Track, nth int) map[string]struct{} {
	set := make(map[string]struct{}, len(tracks)/nth+1)
	for i := 0; i < len(tracks); i += nth {
		set[tracks[i].Hash] = struct{}{}
	}
	return set
}

// benchCompletionRates fills the behavioral map for every candidate. Production
// only hydrates the hashes HydrationTargets reports, so this is the pessimistic
// case: a fully hydrated map costs the most per lookup.
func benchCompletionRates(tracks []library.Track) map[string]float64 {
	rates := make(map[string]float64, len(tracks))
	for i, t := range tracks {
		rates[t.Hash] = float64(i%100) / 100
	}
	return rates
}

// benchCatalog answers for every tag the generated data uses.
type benchCatalog struct{}

func (benchCatalog) HasTag(id int64) bool { return id > 0 && id <= 24 }

// benchInput builds the PlanInput one size is measured with.
//
// It takes no *testing.T on purpose: the shared planInput helper is test-only
// and the rune-based planTracks generator wraps past 26 hashes, neither of which
// survives a 100,000-track run.
func benchInput(tracks []library.Track) PlanInput {
	return PlanInput{
		Request: PlanRequest{
			Settings:    DefaultSettings(),
			Seed:        planTestSeed(),
			BatchSize:   5,
			RequestedAt: testNow,
		},
		Candidates:      tracks,
		TrackTags:       benchTrackTags(tracks),
		FavoriteHashes:  benchHashSet(tracks, 11),
		RecentHashes:    benchHashSet(tracks, 31),
		ProtectedHashes: benchHashSet(tracks, 97),
		DismissedHashes: benchHashSet(tracks, 53),
		CompletionRates: benchCompletionRates(tracks),
		CoOccurrences:   map[string]int{},
		TimeGenreBonus:  map[string]float64{"Rock": 0.2, "Jazz": 0.1},
		TagCatalog:      benchCatalog{},
		Rand:            FailingRand(),
	}
}

// benchName labels a sub-benchmark by scale.
func benchName(size int) string { return fmt.Sprintf("%dk", size/1000) }

// BenchmarkPlanBatch measures one complete refill: partition, hydration
// targeting, scoring, and selection, exactly as a live batch runs it.
func BenchmarkPlanBatch(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(benchName(size), func(b *testing.B) {
			in := benchInput(benchTracks(size))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				res := PlanBatch(in)
				if len(res.Tracks) != in.Request.BatchSize {
					b.Fatalf("expected a full batch, got %d (%s)", len(res.Tracks), res.Outcome)
				}
			}
		})
	}
}

// BenchmarkPartition isolates the candidate walk: filters, discovery
// classification, and the protected-hash removal.
func BenchmarkPartition(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(benchName(size), func(b *testing.B) {
			in := benchInput(benchTracks(size))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if pools := Partition(in); pools.Len() == 0 {
					b.Fatal("expected a non-empty partition")
				}
			}
		})
	}
}

// BenchmarkPartitionFiltered measures the same walk with populated filters, the
// shape a tuned session runs with.
func BenchmarkPartitionFiltered(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(benchName(size), func(b *testing.B) {
			in := benchInput(benchTracks(size))
			in.Request.Settings.Include = FilterSet{
				Genres: []string{"Rock", "Jazz"},
				TagIDs: []int64{1, 2, 3},
			}
			in.Request.Settings.Exclude = FilterSet{Artists: []string{"Artist 007"}}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if pools := Partition(in); pools.Len() == 0 {
					b.Fatal("expected a non-empty partition")
				}
			}
		})
	}
}

// BenchmarkHydrationTargets measures the per-pool top-N walk that decides which
// hashes earn a behavioral read. The partition is done outside the loop because
// the live path keeps the Pools it already has.
func BenchmarkHydrationTargets(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(benchName(size), func(b *testing.B) {
			in := benchInput(benchTracks(size))
			pools := Partition(in)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if got := pools.HydrationTargets(in, 15); len(got) == 0 {
					b.Fatal("expected hydration targets")
				}
			}
		})
	}
}

// BenchmarkPlanFrom measures scoring and selection alone, over a partition taken
// once. This is the half that grows with pool size rather than library size, and
// separating it is what makes a regression attributable.
func BenchmarkPlanFrom(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(benchName(size), func(b *testing.B) {
			in := benchInput(benchTracks(size))
			pools := Partition(in)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				res := PlanFrom(in, pools)
				if len(res.Tracks) != in.Request.BatchSize {
					b.Fatalf("expected a full batch, got %d (%s)", len(res.Tracks), res.Outcome)
				}
			}
		})
	}
}

// TestPlanBatchAtLibraryScale is the regression guard that rides along with the
// benchmarks: benchmarks are not run by the test suite, so without this nothing
// would notice a planner that became quadratic at scale.
//
// The budget is deliberately loose. It is there to catch an accidental
// library-squared walk, not to police milliseconds on a shared machine under
// -race, and a threshold tight enough to be interesting would be flaky enough to
// be ignored.
func TestPlanBatchAtLibraryScale(t *testing.T) {
	const budget = 10 * time.Second

	in := benchInput(benchTracks(100_000))

	start := time.Now()
	res := PlanBatch(in)
	elapsed := time.Since(start)

	if len(res.Tracks) != in.Request.BatchSize {
		t.Fatalf("expected %d tracks, got %d (%s)", in.Request.BatchSize, len(res.Tracks), res.Outcome)
	}
	if res.Outcome != PlanOutcomeOK {
		t.Fatalf("expected an ok plan at scale, got %s: %v", res.Outcome, res.Reasons)
	}
	if elapsed > budget {
		t.Fatalf("planning 100k candidates took %s, over the %s budget", elapsed, budget)
	}
}

// TestHydrationTargetsStayBoundedAtScale pins the property the behavioral reads
// depend on: the hash list handed to storage is a function of the per-group
// count, not of the library size. If it ever grew with the library, every refill
// would widen its database reads as the user's collection grew.
func TestHydrationTargetsStayBoundedAtScale(t *testing.T) {
	const perGroup = 15

	var previous int
	for _, size := range benchSizes {
		in := benchInput(benchTracks(size))
		got := len(Partition(in).HydrationTargets(in, perGroup))

		// Two pools, plus the seed hash, is the whole budget.
		if max := 2*perGroup + 1; got > max {
			t.Fatalf("%d candidates: %d hydration targets, over the %d ceiling", size, got, max)
		}
		if previous != 0 && got != previous {
			t.Fatalf("%d candidates: %d hydration targets, expected the same %d as the smaller library",
				size, got, previous)
		}
		previous = got
	}
}
