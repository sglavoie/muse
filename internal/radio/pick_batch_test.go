package radio

import "testing"

func TestPickBatch_BasicCount(t *testing.T) {
	scored := scoreAllForPickBatch(SeedTrack{Hash: "seed"}, trackSeq(10, "ArtistX", "")...)

	result := PickBatch(scored, 5, 0.4)
	if len(result) == 0 {
		t.Fatalf("expected non-empty result")
	}
	if len(result) > 5 {
		t.Fatalf("expected at most 5 tracks, got %d", len(result))
	}
}

func TestPickBatch_SmallLibrary(t *testing.T) {
	scored := scoreAllForPickBatch(
		SeedTrack{Hash: "seed"},
		track("t1", "ArtistA", "", "", 0),
		track("t2", "ArtistB", "", "", 0),
	)

	if result := PickBatch(scored, 10, 0.4); len(result) > 2 {
		t.Fatalf("small library: expected at most 2 tracks, got %d", len(result))
	}
}

func TestPickBatch_EmptyInputs(t *testing.T) {
	cases := []struct {
		name   string
		scored []TrackScore
		n      int
	}{
		{name: "nil scored", scored: nil, n: 5},
		{name: "empty scored", scored: []TrackScore{}, n: 5},
		{name: "zero n", scored: []TrackScore{scoredTrack("t1", "", 10)}, n: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PickBatch(tc.scored, tc.n, 0.4); got != nil {
				t.Fatalf("expected nil, got %v", got)
			}
		})
	}
}

func TestPickBatch_HugeNDoesNotOverflowPoolSize(t *testing.T) {
	scored := []TrackScore{scoredTrack("t1", "ArtistA", 10)}
	hugeN := int(^uint(0)>>1)/3 + 1

	assertTrackHashes(t, PickBatch(scored, hugeN, 1.0), "t1")
}

func TestPickBatch_HugeNWithNoArtistCapReturnsAvailableTracks(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("t1", "ArtistA", -1),
		scoredTrack("t2", "ArtistA", -2),
		scoredTrack("t3", "ArtistA", -3),
	}
	hugeN := int(^uint(0) >> 1)

	assertTrackHashes(t, PickBatch(scored, hugeN, 1.0), "t1", "t2", "t3")
}

func TestPickBatch_FromPool(t *testing.T) {
	scored := scoreAllForPickBatch(SeedTrack{Hash: "seed"}, trackSeq(10, "ArtistX", "")...)

	assertTracksFromScoredPool(t, PickBatch(scored, 3, 0.4), scored)
}
