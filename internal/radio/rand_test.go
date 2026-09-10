package radio

import (
	"errors"
	"math"
	"testing"
)

func TestSequenceRand_YieldsValuesThenRepeatsTheLast(t *testing.T) {
	source := SequenceRand(0.1, 0.2)
	want := []float64{0.1, 0.2, 0.2, 0.2}
	for i, expected := range want {
		got, err := source.Float64()
		if err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
		if got != expected {
			t.Fatalf("draw %d = %v, want %v", i, got, expected)
		}
	}
}

func TestSequenceRand_EmptyDegradesToZero(t *testing.T) {
	got, err := SequenceRand().Float64()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("draw = %v, want 0", got)
	}
}

func TestFailingRand_Fails(t *testing.T) {
	if _, err := FailingRand().Float64(); !errors.Is(err, errNoRand) {
		t.Fatalf("err = %v, want errNoRand", err)
	}
}

func TestCryptoRand_StaysInTheUnitInterval(t *testing.T) {
	source := CryptoRand()
	for range 100 {
		got, err := source.Float64()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got < 0 || got >= 1 {
			t.Fatalf("draw = %v, want [0, 1)", got)
		}
	}
}

func TestUnitInterval(t *testing.T) {
	tests := []struct {
		name   string
		in     float64
		want   float64
		wantOk bool
	}{
		{"in range", 0.25, 0.25, true},
		{"zero", 0, 0, true},
		{"negative", -3, 0, true},
		{"one", 1, math.Nextafter(1, 0), true},
		{"above one", 42, math.Nextafter(1, 0), true},
		{"positive infinity", math.Inf(1), math.Nextafter(1, 0), true},
		{"negative infinity", math.Inf(-1), 0, true},
		{"nan", math.NaN(), 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := unitInterval(tc.in)
			if ok != tc.wantOk {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOk)
			}
			if ok && got != tc.want {
				t.Fatalf("value = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPickBatchWith_InjectedSourceIsDeterministic(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("a", "A", 30),
		scoredTrack("b", "B", 20),
		scoredTrack("c", "C", 10),
	}

	// The pool is the whole slice; weights are 30/20/10 out of 60, so the
	// draw boundaries sit at 0.5 and 0.833.
	tests := []struct {
		draw float64
		want string
	}{
		{0, "a"},
		{0.49, "a"},
		{0.6, "b"},
		{0.9, "c"},
	}

	for _, tc := range tests {
		got := PickBatchWith(scored, 1, 0.4, FixedRand(tc.draw))
		assertTrackHashes(t, got, tc.want)
	}
}

func TestPickBatchWith_FailedDrawTakesTheHighestScore(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("a", "A", 10),
		scoredTrack("b", "B", 30),
	}
	// ScoreAll sorts descending; this slice is deliberately unsorted so the
	// fallback's "first eligible" is observable.
	got := PickBatchWith(scored, 1, 0.4, FailingRand())
	assertTrackHashes(t, got, "a")
}

func TestPickBatchWith_NilSourceFallsBackToCrypto(t *testing.T) {
	scored := []TrackScore{scoredTrack("a", "A", 10)}
	got := PickBatchWith(scored, 1, 0.4, nil)
	assertTrackHashes(t, got, "a")
}
