package radio

import (
	cryptorand "crypto/rand"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) {
	return f(p)
}

func trackSeq(count int, artist, genre string) []library.Track {
	tracks := make([]library.Track, count)
	for i := range tracks {
		tracks[i] = track(string(rune('a'+i)), artist, "", genre, 0)
	}
	return tracks
}

func scoreAllForPickBatch(seed SeedTrack, tracks ...library.Track) []TrackScore {
	return ScoreAll(ScoreInput{
		Seed:      seed,
		AllTracks: tracks,
	})
}

func scoredTrack(hash, artist string, score float64) TrackScore {
	return TrackScore{Track: track(hash, artist, "", "", 0), Score: score}
}

func assertUniqueNonEmptyHashes(t *testing.T, tracks []library.Track) {
	t.Helper()

	seen := make(map[string]struct{}, len(tracks))
	for _, tr := range tracks {
		if tr.Hash == "" {
			continue
		}
		if _, ok := seen[tr.Hash]; ok {
			t.Fatalf("duplicate non-empty hash %q in result: %v", tr.Hash, tracks)
		}
		seen[tr.Hash] = struct{}{}
	}
}

func assertTrackHashes(t *testing.T, got []library.Track, want ...string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d tracks, got %d: %v", len(want), len(got), got)
	}
	for i, hash := range want {
		if got[i].Hash != hash {
			t.Fatalf("idx %d: expected %q, got %q", i, hash, got[i].Hash)
		}
	}
}

func assertTracksFromScoredPool(t *testing.T, result []library.Track, scored []TrackScore) {
	t.Helper()

	valid := make(map[string]struct{}, len(scored))
	for _, ts := range scored {
		valid[ts.Track.Hash] = struct{}{}
	}
	for _, tr := range result {
		if _, ok := valid[tr.Hash]; !ok {
			t.Fatalf("track %s not in scored pool", tr.Hash)
		}
	}
}

func normalizedArtistCounts(tracks []library.Track) map[string]int {
	counts := make(map[string]int)
	for _, tr := range tracks {
		if key := normalizedArtistKey(tr.Artist); key != "" {
			counts[key]++
		}
	}
	return counts
}

func assertMaxArtistCount(t *testing.T, tracks []library.Track, max int) {
	t.Helper()

	for artist, count := range normalizedArtistCounts(tracks) {
		if count > max {
			t.Fatalf("artist %q appears %d times, want at most %d", artist, count, max)
		}
	}
}

func withCryptoRandReader(t *testing.T, reader readerFunc) {
	t.Helper()

	orig := cryptorand.Reader
	cryptorand.Reader = reader
	t.Cleanup(func() {
		cryptorand.Reader = orig
	})
}

func useHighestCryptoRand(t *testing.T) {
	t.Helper()

	withCryptoRandReader(t, readerFunc(func(p []byte) (int, error) {
		for i := range p {
			p[i] = 0xff
		}
		if len(p) > 0 {
			p[0] = 0x1f
		}
		return len(p), nil
	}))
}
