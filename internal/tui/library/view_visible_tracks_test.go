package library

import (
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
)

func TestVisibleTracksReturnsOverrideOrderUnfiltered(t *testing.T) {
	tracks := []musiclib.Track{{Hash: "a"}, {Hash: "b"}, {Hash: "c"}}
	v := View{OverrideTracks: tracks}

	got := v.VisibleTracks()
	if len(got) != 3 || got[0].Hash != "a" || got[2].Hash != "c" {
		t.Fatalf("expected the override order verbatim, got %+v", got)
	}
}

func TestVisibleTracksAppliesFilterAndAgreesWithFocusedTrack(t *testing.T) {
	tracks := []musiclib.Track{{Hash: "a"}, {Hash: "b"}, {Hash: "c"}}
	v := View{
		OverrideTracks:  tracks,
		Filter:          filter.State{Text: "q"},
		FilteredMatches: []musiclib.Match{{TrackIndex: 2}, {TrackIndex: 0}},
		Cursor:          1,
	}

	got := v.VisibleTracks()
	if len(got) != 2 || got[0].Hash != "c" || got[1].Hash != "a" {
		t.Fatalf("expected match order [c a], got %+v", got)
	}

	focused, ok := v.FocusedTrack()
	if !ok || focused.Hash != got[v.Cursor].Hash {
		t.Fatalf("FocusedTrack %v/%v disagrees with VisibleTracks row %d", focused, ok, v.Cursor)
	}
}

func TestVisibleTracksDropsOutOfRangeMatches(t *testing.T) {
	tracks := []musiclib.Track{{Hash: "a"}, {Hash: "b"}}
	v := View{
		OverrideTracks:  tracks,
		Filter:          filter.State{Text: "q"},
		FilteredMatches: []musiclib.Match{{TrackIndex: 1}, {TrackIndex: 7}, {TrackIndex: -1}},
	}

	got := v.VisibleTracks()
	if len(got) != 1 || got[0].Hash != "b" {
		t.Fatalf("stale matches must be dropped, got %+v", got)
	}
}
