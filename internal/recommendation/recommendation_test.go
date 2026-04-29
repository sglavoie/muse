package recommendation_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
)

func TestRegistry_length(t *testing.T) {
	defs := recommendation.Registry()
	if len(defs) == 0 {
		t.Fatal("Registry() returned empty slice")
	}
}

func TestRegistry_order(t *testing.T) {
	defs := recommendation.Registry()
	want := []recommendation.MixID{
		recommendation.MixRediscover,
		recommendation.MixHiddenGems,
		recommendation.MixOnRepeat,
		recommendation.MixDeepCuts,
		recommendation.MixNeverPlayed,
	}
	if len(defs) != len(want) {
		t.Fatalf("Registry() returned %d defs, want %d", len(defs), len(want))
	}
	for i, d := range defs {
		if d.Info.ID != want[i] {
			t.Errorf("Registry()[%d].Info.ID = %v, want %v", i, d.Info.ID, want[i])
		}
		if d.Info.Name == "" {
			t.Errorf("Registry()[%d].Info.Name is empty", i)
		}
		if d.Info.Description == "" {
			t.Errorf("Registry()[%d].Info.Description is empty", i)
		}
	}
}

func TestRegistry_uniqueIDs(t *testing.T) {
	seen := make(map[recommendation.MixID]bool)
	for _, d := range recommendation.Registry() {
		if seen[d.Info.ID] {
			t.Errorf("duplicate MixID %v in Registry()", d.Info.ID)
		}
		seen[d.Info.ID] = true
	}
}

func TestFilterSeen_nilSeen(t *testing.T) {
	tracks := []library.Track{{Hash: "a"}, {Hash: "b"}}
	got := recommendation.FilterSeen(tracks, nil)
	if len(got) != 2 {
		t.Fatalf("FilterSeen with nil seen: want 2 tracks, got %d", len(got))
	}
}

func TestFilterSeen_emptySeen(t *testing.T) {
	tracks := []library.Track{{Hash: "a"}, {Hash: "b"}}
	got := recommendation.FilterSeen(tracks, map[string]struct{}{})
	if len(got) != 2 {
		t.Fatalf("FilterSeen with empty seen: want 2, got %d", len(got))
	}
}

func TestFilterSeen_removes(t *testing.T) {
	tracks := []library.Track{{Hash: "a"}, {Hash: "b"}, {Hash: "c"}}
	seen := map[string]struct{}{"b": {}}
	got := recommendation.FilterSeen(tracks, seen)
	if len(got) != 2 {
		t.Fatalf("FilterSeen: want 2, got %d", len(got))
	}
	for _, track := range got {
		if track.Hash == "b" {
			t.Error("FilterSeen should have removed hash b")
		}
	}
}

func TestFilterSeen_allRemoved(t *testing.T) {
	tracks := []library.Track{{Hash: "a"}}
	seen := map[string]struct{}{"a": {}}
	got := recommendation.FilterSeen(tracks, seen)
	if len(got) != 0 {
		t.Fatalf("FilterSeen all seen: want 0, got %d", len(got))
	}
}

func TestFilterSeen_preservesOrder(t *testing.T) {
	tracks := []library.Track{{Hash: "a"}, {Hash: "b"}, {Hash: "c"}, {Hash: "d"}}
	seen := map[string]struct{}{"b": {}}
	got := recommendation.FilterSeen(tracks, seen)
	want := []string{"a", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("want %d tracks, got %d", len(want), len(got))
	}
	for i, track := range got {
		if track.Hash != want[i] {
			t.Errorf("got[%d].Hash = %q, want %q", i, track.Hash, want[i])
		}
	}
}
