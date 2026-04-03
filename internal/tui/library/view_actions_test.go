package library

import (
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
)

// TestFocusedTrackNoFilter verifies that FocusedTrack returns the track at
// the cursor position when no filter is active.
func TestFocusedTrackNoFilter(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Cursor: 1}
	track, ok := v.FocusedTrack()
	if !ok {
		t.Fatal("FocusedTrack: expected ok=true, got false")
	}
	tracks := idx.Tracks()
	if track.Artist != tracks[1].Artist || track.Title != tracks[1].Title {
		t.Errorf("FocusedTrack: expected %q/%q, got %q/%q",
			tracks[1].Artist, tracks[1].Title, track.Artist, track.Title)
	}
}

// TestFocusedTrackEmptyLibrary verifies that FocusedTrack returns false when
// the library has no tracks.
func TestFocusedTrackEmptyLibrary(t *testing.T) {
	v := View{}
	_, ok := v.FocusedTrack()
	if ok {
		t.Error("FocusedTrack: expected ok=false for empty library, got true")
	}
}

// TestFocusedTrackWithActiveFilter verifies that FocusedTrack indexes into the
// filtered result set, not the full track list, when a filter is active.
func TestFocusedTrackWithActiveFilter(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
		Filter: filter.State{Active: true, Text: "Alice"},
		Cursor: 0,
	}
	track, ok := v.FocusedTrack()
	if !ok {
		t.Fatal("FocusedTrack with filter: expected ok=true, got false")
	}
	if track.Artist != "Alice" {
		t.Errorf("FocusedTrack with filter: expected Artist %q, got %q", "Alice", track.Artist)
	}
}

// TestEnqueueFocusedSendsCorrectTrack verifies that the tea.Cmd returned by
// EnqueueFocused calls AppendTrack with the focused track and returns a
// TrackEnqueuedMsg containing that track.
func TestEnqueueFocusedSendsCorrectTrack(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Cursor: 0}
	focused := idx.Tracks()[0]

	qa := &fakeQueueAppender{}
	_, cmd := v.EnqueueFocused(qa, 0)
	if cmd == nil {
		t.Fatal("EnqueueFocused: expected non-nil Cmd, got nil")
	}

	msg := cmd()
	enqueued, ok := msg.(TrackEnqueuedMsg)
	if !ok {
		t.Fatalf("EnqueueFocused: cmd returned %T, want TrackEnqueuedMsg", msg)
	}
	if enqueued.Err != nil {
		t.Errorf("EnqueueFocused: unexpected error: %v", enqueued.Err)
	}
	if enqueued.Track.Artist != focused.Artist || enqueued.Track.Title != focused.Title {
		t.Errorf("EnqueueFocused: track mismatch: got %q/%q, want %q/%q",
			enqueued.Track.Artist, enqueued.Track.Title, focused.Artist, focused.Title)
	}
	if len(qa.tracks) != 1 || qa.tracks[0].Artist != focused.Artist {
		t.Errorf("EnqueueFocused: AppendTrack called with wrong track or wrong number of times")
	}
}

// TestEnqueueFocusedDuplicatesAccepted verifies that enqueueing the same track
// twice is accepted without error, so duplicate queue entries are permitted.
func TestEnqueueFocusedDuplicatesAccepted(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Cursor: 0}
	qa := &fakeQueueAppender{}

	_, cmd1 := v.EnqueueFocused(qa, 0)
	_, cmd2 := v.EnqueueFocused(qa, 1)

	msg1 := cmd1()
	if e, ok := msg1.(TrackEnqueuedMsg); !ok || e.Err != nil {
		t.Fatalf("first enqueue failed: %v", msg1)
	}
	msg2 := cmd2()
	if e, ok := msg2.(TrackEnqueuedMsg); !ok || e.Err != nil {
		t.Fatalf("second enqueue of same track failed: %v", msg2)
	}
	if len(qa.tracks) != 2 {
		t.Errorf("expected 2 AppendTrack calls for duplicates, got %d", len(qa.tracks))
	}
}

// TestEnqueueFocusedWasEmptyTrueWhenQueueEmpty verifies that
// TrackEnqueuedMsg.WasEmpty is true when prevQueueLen is 0.
func TestEnqueueFocusedWasEmptyTrueWhenQueueEmpty(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Cursor: 0}
	qa := &fakeQueueAppender{}

	_, cmd := v.EnqueueFocused(qa, 0)
	msg := cmd().(TrackEnqueuedMsg)
	if !msg.WasEmpty {
		t.Error("WasEmpty should be true when prevQueueLen is 0")
	}
}

// TestEnqueueFocusedWasEmptyFalseWhenQueueNonEmpty verifies that
// TrackEnqueuedMsg.WasEmpty is false when prevQueueLen is greater than zero.
func TestEnqueueFocusedWasEmptyFalseWhenQueueNonEmpty(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Cursor: 0}
	qa := &fakeQueueAppender{}

	_, cmd := v.EnqueueFocused(qa, 3)
	msg := cmd().(TrackEnqueuedMsg)
	if msg.WasEmpty {
		t.Error("WasEmpty should be false when prevQueueLen > 0")
	}
}

// TestEnqueueFocusedNilCmdOnEmptyLibrary verifies that EnqueueFocused returns
// a nil Cmd when the library contains no tracks.
func TestEnqueueFocusedNilCmdOnEmptyLibrary(t *testing.T) {
	v := View{}
	qa := &fakeQueueAppender{}
	_, cmd := v.EnqueueFocused(qa, 0)
	if cmd != nil {
		t.Error("EnqueueFocused on empty library: expected nil Cmd, got non-nil")
	}
}

// TestEnqueueFocusedWithActiveFilterSelectsFilteredTrack verifies that when a
// filter is active, EnqueueFocused enqueues the track at cursor position in
// the filtered result set rather than the unfiltered full list. This locks in
// the FocusedTrack filter-branch behaviour and acts as a regression guard for
// any code that reconstructs a View for enqueue purposes.
func TestEnqueueFocusedWithActiveFilterSelectsFilteredTrack(t *testing.T) {
	idx := buildIndex(sampleTracks)
	// FakeFuzzySearcher matches "Alice" only against the Alice track (index 0
	// in the sorted index). Cursor 0 should therefore resolve to Alice, not to
	// whatever track sits at position 0 in the unfiltered list.
	v := View{
		Index:  idx,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
		Filter: filter.State{Active: true, Text: "Alice"},
		Cursor: 0,
	}

	qa := &fakeQueueAppender{}
	_, cmd := v.EnqueueFocused(qa, 0)
	if cmd == nil {
		t.Fatal("EnqueueFocused with active filter: expected non-nil Cmd, got nil")
	}
	msg, ok := cmd().(TrackEnqueuedMsg)
	if !ok {
		t.Fatalf("EnqueueFocused: cmd returned unexpected type")
	}
	if msg.Err != nil {
		t.Fatalf("EnqueueFocused with filter: unexpected error: %v", msg.Err)
	}
	if msg.Track.Artist != "Alice" {
		t.Errorf("EnqueueFocused with active filter: expected Artist %q, got %q", "Alice", msg.Track.Artist)
	}
}
