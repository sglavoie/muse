package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// This file covers what Enter on a queue row means once the player owns a
// shuffle timeline. Selecting the row that is already playing restarts it
// through ReplayCurrent, which leaves the timeline untouched; selecting anything
// else is a new selection and keeps reanchoring through PlayFromIndex. The
// distinction is invisible with shuffle off — both restart the same audio — and
// is the whole difference under shuffle, where reanchoring re-permutes every row
// the user can still see coming.

// replayModel returns a model over an n-row queue whose player traverses it in
// order and sits on the row at traversal position pos, with the TUI agreeing
// that the queue row at that position is what is playing.
func replayModel(order []int, pos int) (Model, *fakeTraversalPlayer) {
	p := newFakeTraversalPlayer(order, pos)
	p.resolvePrefetch()
	m := newModelWithTracks()
	m.player = p
	m.queue.Traversal = queueTraversalSource(p)
	m.queue.Entries = replayEntries(len(order))
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = order[pos]
	m.playback.currentHash = m.queue.Entries[order[pos]].Hash
	return m, p
}

// replayEntries builds n queue rows with distinct identities.
func replayEntries(n int) []queue.Entry {
	entries := make([]queue.Entry, n)
	for i := range entries {
		entries[i] = queue.Entry{
			ID:      int64(i + 1),
			Hash:    "hash-" + string(rune('a'+i)),
			Path:    "/music/" + string(rune('a'+i)) + ".flac",
			Title:   "T" + string(rune('a'+i)),
			Artist:  "Artist " + string(rune('a'+i)),
			Display: "row " + string(rune('a'+i)),
		}
	}
	return entries
}

// runSelect drives Enter on a queue row all the way through: the view resolves
// the row, root handles the resulting message, and the async player command runs
// so its result message is available to feed back in.
func runSelect(t *testing.T, m Model, displayRow int) (Model, playback.PlayFromIndexResultMsg) {
	t.Helper()
	stubFileExistsTrue(t)
	m.ActiveView = viewQueue
	m.queue.Cursor = displayRow
	_, selectCmd := m.queue.HandleAction(keys.ActionSelect)
	if selectCmd == nil {
		t.Fatal("select produced no command")
	}
	playMsg, ok := selectCmd().(playback.PlayQueueIndexMsg)
	if !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", selectCmd())
	}
	m, cmd := m.handlePlayQueueIndexMsg(playMsg)
	if cmd == nil {
		t.Fatal("play produced no command")
	}
	result, ok := cmd().(playback.PlayFromIndexResultMsg)
	if !ok {
		t.Fatalf("expected PlayFromIndexResultMsg, got %T", cmd())
	}
	return m, result
}

// upcoming lists the queue rows the timeline will still visit, which is exactly
// what the Queue panel shows below the now-playing row.
func upcoming(p *fakeTraversalPlayer) []int {
	t := p.QueueTraversal()
	return append([]int(nil), t.Order[t.CurrentPosition+1:]...)
}

// TestSelectCurrentRowReplaysWithoutMovingTheTimeline is the headline case: the
// user presses Enter on the row that is already playing to hear it again, and
// the order of everything still upcoming is untouched.
func TestSelectCurrentRowReplaysWithoutMovingTheTimeline(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4
	before := upcoming(p)
	beforeOrder := p.QueueTraversal().Order
	beforePos := p.QueueTraversal().CurrentPosition
	m.playback.elapsedSecs = 137

	// The current row sits at display position 2 under the timeline order.
	m, result := runSelect(t, m, 2)

	if p.replayCalls != 1 {
		t.Errorf("ReplayCurrent calls = %d, want 1", p.replayCalls)
	}
	if len(p.playFromIndexCalls) != 0 {
		t.Errorf("PlayFromIndex must not be called for the current row, got %v", p.playFromIndexCalls)
	}
	if !result.Replay {
		t.Error("result must be marked as a replay")
	}
	if result.Index != 4 || result.Hash != m.queue.Entries[4].Hash {
		t.Errorf("result names row %d/%q, want 4/%q", result.Index, result.Hash, m.queue.Entries[4].Hash)
	}
	if m.playback.elapsedSecs != 0 {
		t.Errorf("elapsed = %v, want 0 — a replay restarts at zero", m.playback.elapsedSecs)
	}
	if m.playback.state != playback.StatePlaying {
		t.Errorf("state = %v, want playing", m.playback.state)
	}
	assertTimelineUnmoved(t, p, beforeOrder, beforePos, before)
}

// assertTimelineUnmoved checks the cycle is exactly where it was: same order,
// same cursor, and therefore the same rows still upcoming. All three matter —
// a reanchor that happened to land on the same cursor would still re-permute
// the tail, which is the reshuffle the user would see.
func assertTimelineUnmoved(t *testing.T, p *fakeTraversalPlayer, order []int, pos int, upcomingBefore []int) {
	t.Helper()
	after := p.QueueTraversal()
	if !slices.Equal(after.Order, order) {
		t.Errorf("traversal order = %v, want it unchanged at %v", after.Order, order)
	}
	if after.CurrentPosition != pos {
		t.Errorf("current position = %d, want it unchanged at %d", after.CurrentPosition, pos)
	}
	if got := upcoming(p); !slices.Equal(got, upcomingBefore) {
		t.Errorf("upcoming order = %v, want it unchanged at %v", got, upcomingBefore)
	}
}

// TestSelectOtherRowAnchorsAFreshCycle verifies the other half of the rule: a
// genuine selection still goes through PlayFromIndex, which is what reanchors
// the cycle around the chosen row.
func TestSelectOtherRowAnchorsAFreshCycle(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4

	m, result := runSelect(t, m, 4) // display row 4 is queue row 2

	if p.replayCalls != 0 {
		t.Errorf("ReplayCurrent calls = %d, want 0 for a different row", p.replayCalls)
	}
	if !slices.Equal(p.playFromIndexCalls, []int{2}) {
		t.Errorf("PlayFromIndex calls = %v, want [2]", p.playFromIndexCalls)
	}
	if result.Replay {
		t.Error("result must not be marked as a replay")
	}
	if m.playback.currentIndex != 2 {
		t.Errorf("currentIndex = %d, want 2", m.playback.currentIndex)
	}
	if pos := p.QueueTraversal().CurrentPosition; pos != 4 {
		t.Errorf("current position = %d, want the cursor moved onto the selected row (4)", pos)
	}
}

// TestSelectCurrentRowReplaysWhilePausedAndStopped verifies the two other states
// that sit on a loaded entry also replay. Idle does not: nothing is loaded, so
// there is no cycle to preserve and the selection anchors a new one.
func TestSelectCurrentRowReplaysWhilePausedAndStopped(t *testing.T) {
	tests := []struct {
		name       string
		state      playback.State
		wantReplay bool
	}{
		{"playing", playback.StatePlaying, true},
		{"paused", playback.StatePaused, true},
		{"stopped", playback.StateStopped, true},
		{"idle", playback.StateIdle, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, p := replayModel([]int{3, 1, 4, 0, 2}, 2)
			m.playback.state = tc.state

			m, result := runSelect(t, m, 2)

			gotReplay := p.replayCalls == 1
			if gotReplay != tc.wantReplay {
				t.Errorf("replayed = %v, want %v", gotReplay, tc.wantReplay)
			}
			if result.Replay != tc.wantReplay {
				t.Errorf("result.Replay = %v, want %v", result.Replay, tc.wantReplay)
			}
			if anchored := len(p.playFromIndexCalls) == 1; anchored == tc.wantReplay {
				t.Errorf("PlayFromIndex calls = %v, want the opposite of replay", p.playFromIndexCalls)
			}
			if m.playback.state != playback.StatePlaying {
				t.Errorf("state = %v, want playing after a selection", m.playback.state)
			}
		})
	}
}

// TestSelectWhileExhaustedAnchorsAFreshCycle covers the exhausted queue: the
// cycle finished, the model went idle, and pressing Enter on the row that played
// last must start over rather than replay something the player is no longer
// holding.
func TestSelectWhileExhaustedAnchorsAFreshCycle(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 4)
	m.playback.state = playback.StateIdle
	m.playback.queueExhausted = true

	m, result := runSelect(t, m, 4)

	if p.replayCalls != 0 {
		t.Errorf("ReplayCurrent calls = %d, want 0 while exhausted", p.replayCalls)
	}
	if !slices.Equal(p.playFromIndexCalls, []int{2}) {
		t.Errorf("PlayFromIndex calls = %v, want [2]", p.playFromIndexCalls)
	}
	if result.Replay {
		t.Error("result must not be marked as a replay")
	}
	if m.playback.state != playback.StatePlaying {
		t.Errorf("state = %v, want playing", m.playback.state)
	}
}

// TestSelectWithNoCurrentRowAnchors covers a model with nothing current at all
// (currentIndex -1 after the playing row's file vanished): every row is a new
// selection.
func TestSelectWithNoCurrentRowAnchors(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 2)
	m.playback.currentIndex = -1
	m.playback.currentHash = ""

	_, result := runSelect(t, m, 2)

	if p.replayCalls != 0 {
		t.Errorf("ReplayCurrent calls = %d, want 0 with no current row", p.replayCalls)
	}
	if !slices.Equal(p.playFromIndexCalls, []int{4}) {
		t.Errorf("PlayFromIndex calls = %v, want [4]", p.playFromIndexCalls)
	}
	if result.Replay {
		t.Error("result must not be marked as a replay")
	}
}

// TestSelectDuplicateRowOfCurrentTrackAnchors verifies identity is per
// occurrence: a second queue row holding the same track is a different row, so
// selecting it is a selection and not a replay.
func TestSelectDuplicateRowOfCurrentTrackAnchors(t *testing.T) {
	m, p := replayModel([]int{0, 1, 2}, 0) // playing queue row 0
	// Row 2 is a duplicate of row 0: same track, its own queue occurrence.
	m.queue.Entries[2].Hash = m.queue.Entries[0].Hash
	m.queue.Entries[2].Path = m.queue.Entries[0].Path

	_, result := runSelect(t, m, 2)

	if p.replayCalls != 0 {
		t.Errorf("ReplayCurrent calls = %d, want 0 for a duplicate occurrence", p.replayCalls)
	}
	if !slices.Equal(p.playFromIndexCalls, []int{2}) {
		t.Errorf("PlayFromIndex calls = %v, want [2]", p.playFromIndexCalls)
	}
	if result.Replay {
		t.Error("selecting a duplicate row is a new selection, not a replay")
	}
}

// TestSelectCurrentRowUnderSortAndFilterStillReplays proves the operation is
// chosen from queue identity, not from where the row happens to be drawn: a
// metadata sort and a filter both move the current row around the panel without
// turning Enter on it into a reanchoring selection.
func TestSelectCurrentRowUnderSortAndFilterStillReplays(t *testing.T) {
	t.Run("sorted", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4
		m.queue.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
		m.queue = m.queue.RecomputeDisplayOrder()
		row := m.queue.FindEntryFilteredPosition(4)

		_, result := runSelect(t, m, row)

		if p.replayCalls != 1 {
			t.Errorf("ReplayCurrent calls = %d, want 1", p.replayCalls)
		}
		if !result.Replay || result.Index != 4 {
			t.Errorf("result = %+v, want a replay of row 4", result)
		}
	})

	t.Run("filtered", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4
		m.queue.Filter = filter.State{Active: true, Text: m.queue.Entries[4].Title}
		row := m.queue.FindEntryFilteredPosition(4)
		if len(m.queue.FilteredEntries()) == len(m.queue.Entries) {
			t.Fatal("test setup: the filter hid nothing")
		}

		_, result := runSelect(t, m, row)

		if p.replayCalls != 1 {
			t.Errorf("ReplayCurrent calls = %d, want 1", p.replayCalls)
		}
		if !result.Replay || result.Index != 4 {
			t.Errorf("result = %+v, want a replay of row 4", result)
		}
	})
}

// TestSelectCurrentRowFallsBackToPlayFromIndex covers a player without the
// optional interface: the selection must still start the track, through the one
// operation such a player has.
func TestSelectCurrentRowFallsBackToPlayFromIndex(t *testing.T) {
	p := &stubPlayer{}
	m := newModelWithTracks()
	m.player = p
	m.queue.Entries = replayEntries(3)
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 1
	m.playback.currentHash = m.queue.Entries[1].Hash

	_, result := runSelect(t, m, 1)

	if !slices.Equal(p.playFromIndexCalls, []int{1}) {
		t.Errorf("PlayFromIndex calls = %v, want [1]", p.playFromIndexCalls)
	}
	if result.Replay {
		t.Error("a player without ReplayCurrent must not report a replay")
	}
}

// TestReplayFailureResetsPlaybackAndNamesTheOperation verifies a refused replay
// recovers exactly like a refused play, and says which one failed.
func TestReplayFailureResetsPlaybackAndNamesTheOperation(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 2)
	p.replayErr = errors.New("mpv gone")

	m, result := runSelect(t, m, 2)
	if result.Err == nil {
		t.Fatal("expected a failure result")
	}
	m = m.handlePlayFromIndexResultMsg(result)

	if m.playback.state != playback.StateIdle {
		t.Errorf("state = %v, want idle after a failed replay", m.playback.state)
	}
	if m.playback.currentIndex != -1 {
		t.Errorf("currentIndex = %d, want -1 after a failed replay", m.playback.currentIndex)
	}
	if msg := m.statusBar.TopNotification().Msg; !strings.HasPrefix(msg, "Replay failed: ") {
		t.Errorf("notification = %q, want it to name the replay", msg)
	}
}

// TestStalePlayFailureAfterReplayIsIgnored covers the ordering the async
// commands make possible: the user replays, then immediately jumps elsewhere,
// and the replay's failure lands last. It names the row that is no longer
// current, so it must be discarded rather than resetting the new track.
func TestStalePlayFailureAfterReplayIsIgnored(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 2)
	p.replayErr = errors.New("mpv gone")

	m, replayResult := runSelect(t, m, 2)
	// The jump lands first and succeeds.
	m, jumpResult := runSelect(t, m, 4)
	if jumpResult.Err != nil {
		t.Fatalf("jump failed: %v", jumpResult.Err)
	}
	m = m.handlePlayFromIndexResultMsg(jumpResult)
	// The replay's failure arrives afterwards.
	m = m.handlePlayFromIndexResultMsg(replayResult)

	if m.playback.state != playback.StatePlaying {
		t.Errorf("state = %v, want playing — the stale replay failure must not reset it", m.playback.state)
	}
	if m.playback.currentIndex != 2 {
		t.Errorf("currentIndex = %d, want 2", m.playback.currentIndex)
	}
	if msg := m.statusBar.TopNotification().Msg; strings.Contains(msg, "failed") {
		t.Errorf("notification = %q, want no failure surfaced for the abandoned replay", msg)
	}
}

// TestReplayEndsTheListenBoundary verifies the pre-reset listen boundary is kept
// for replays too: what the user heard before pressing Enter is a finished
// listen, and it must be captured before the optimistic reset zeroes it.
func TestReplayEndsTheListenBoundary(t *testing.T) {
	m, _ := replayModel([]int{3, 1, 4, 0, 2}, 2)
	m = m.armPendingListen(m.queue.Entries[4].Hash, m.queue.Entries[4].Artist)
	m.playback.listenedSecs = 120
	m.playback.peakTotalSecs = 200

	m, _ = runSelect(t, m, 2)

	if len(m.playback.flushes) != 1 {
		t.Fatalf("queued listens = %d, want the outgoing listen captured before the progress reset", len(m.playback.flushes))
	}
	flushed := m.playback.flushes[0]
	if flushed.hash != m.queue.Entries[4].Hash {
		t.Errorf("listen hash = %q, want the replayed row's %q", flushed.hash, m.queue.Entries[4].Hash)
	}
	if flushed.listenedSecs != 120 {
		t.Errorf("listened secs = %v, want the 120s measured before the replay", flushed.listenedSecs)
	}
	if m.playback.pending.armed {
		t.Error("the listen must be disarmed so the follow-up TrackChangedMsg cannot re-capture it")
	}
	if m.playback.listenedSecs != 0 {
		t.Errorf("listenedSecs = %v, want the replay to restart the measurement at zero", m.playback.listenedSecs)
	}
}
