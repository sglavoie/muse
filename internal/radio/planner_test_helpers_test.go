package radio

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// planTestSeed is the seed every planner test scores against. It shares its
// genre with the generated candidates so scores stay comfortably positive and
// the weighted draw is exercised rather than the zero-score fallback.
func planTestSeed() SeedSnapshot {
	return SeedSnapshot{
		TrackHash: "seed",
		Title:     "Seed",
		Artist:    "Seed Artist",
		Album:     "Seed Album",
		Genres:    []string{"Rock"},
		Duration:  200,
	}
}

// neverPlayed builds a discovery candidate: no counted plays at all.
func neverPlayed(hash, artist string) library.Track {
	return library.Track{Hash: hash, Path: "/m/" + hash + ".mp3", Artist: artist, Title: hash, Genre: "Rock", Duration: 200}
}

// stalePlayed builds a discovery candidate whose last counted play predates the
// 90-day cutoff.
func stalePlayed(hash, artist string) library.Track {
	t := neverPlayed(hash, artist)
	t.PlayCount = 12
	t.LastPlayedAt = daysAgo(200)
	return t
}

// recentlyPlayed builds a familiar candidate.
func recentlyPlayed(hash, artist string) library.Track {
	t := neverPlayed(hash, artist)
	t.PlayCount = 12
	t.LastPlayedAt = daysAgo(3)
	return t
}

// planTracks generates count candidates with a per-track artist, so artist
// diversity never limits a test that is not about diversity.
func planTracks(prefix string, count int, build func(hash, artist string) library.Track) []library.Track {
	tracks := make([]library.Track, 0, count)
	for i := range count {
		hash := prefix + string(rune('a'+i))
		tracks = append(tracks, build(hash, "Artist "+hash))
	}
	return tracks
}

// planInput builds a deterministic PlanInput at the Balanced preset.
func planInput(t *testing.T, candidates []library.Track) PlanInput {
	t.Helper()

	return PlanInput{
		Request: PlanRequest{
			Settings:    DefaultSettings(),
			Seed:        planTestSeed(),
			BatchSize:   5,
			RequestedAt: testNow,
		},
		Candidates: candidates,
		// A failing source pins selection to the highest-scored eligible
		// candidate, which makes assertions about *which* tracks were chosen
		// stable without weakening the pool, cap, or partition logic.
		Rand: FailingRand(),
	}
}

// withDiscoveryPercent returns a copy of in retargeted at pct.
func withDiscoveryPercent(in PlanInput, pct int) PlanInput {
	in.Request.Settings = in.Request.Settings.WithDiscoveryPercent(pct)
	return in
}

func planHashes(result PlanResult) []string {
	hashes := make([]string, 0, len(result.Tracks))
	for _, pt := range result.Tracks {
		hashes = append(hashes, pt.Track.Hash)
	}
	return hashes
}

func assertPlanHashes(t *testing.T, result PlanResult, want ...string) {
	t.Helper()

	got := planHashes(result)
	if len(got) != len(want) {
		t.Fatalf("expected %d tracks %v, got %d: %v", len(want), want, len(got), got)
	}
	for i, hash := range want {
		if got[i] != hash {
			t.Fatalf("idx %d: expected %q, got %q (all: %v)", i, hash, got[i], got)
		}
	}
}

func assertNoDuplicateTracks(t *testing.T, result PlanResult) {
	t.Helper()

	seen := make(map[string]struct{}, len(result.Tracks))
	for _, pt := range result.Tracks {
		key := trackSelectionKey(pt.Track)
		if _, dup := seen[key]; dup {
			t.Fatalf("duplicate selection %q in %v", key, planHashes(result))
		}
		seen[key] = struct{}{}
	}
}

func hashSet(hashes ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(hashes))
	for _, h := range hashes {
		set[h] = struct{}{}
	}
	return set
}

func containsHash(result PlanResult, hash string) bool {
	for _, pt := range result.Tracks {
		if pt.Track.Hash == hash {
			return true
		}
	}
	return false
}
