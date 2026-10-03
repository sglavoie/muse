package player

import (
	"slices"
	"testing"
)

func TestRemainingIndicesIncludeInsertionsBehindCursor(t *testing.T) {
	m := &Manager{queue: []QueueTrack{{QueueID: 1}, {QueueID: 2}, {QueueID: 3}, {QueueID: 4}}, currentIndex: 2}
	ids := queueIdentities(m.queue)
	m.unreachedAdditions = map[string]bool{ids[1]: true}
	got := m.QueueTraversal()
	if !slices.Equal(got.RemainingIndices, []int{1, 3}) || len(got.RemainingIndices) != got.Remaining {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
	// Returning to the insertion must not count the completed prefix again.
	m.pinSequentialFrontierLocked()
	m.currentIndex = 1
	delete(m.unreachedAdditions, ids[1])
	got = m.QueueTraversal()
	if !slices.Equal(got.RemainingIndices, []int{3}) {
		t.Fatalf("completed prefix counted again: %+v", got)
	}
	got.RemainingIndices[0] = 0
	if !slices.Equal(m.QueueTraversal().RemainingIndices, []int{3}) {
		t.Fatal("snapshot aliases manager state")
	}
}

func TestRemainingIndicesExcludeUnscheduledShuffleRows(t *testing.T) {
	m := &Manager{queue: []QueueTrack{{QueueID: 1}, {QueueID: 2}, {QueueID: 3}, {QueueID: 4}}, currentIndex: 2, shuffleEnabled: true}
	ids := queueIdentities(m.queue)
	m.shuffleHistory = []string{ids[0], ids[2]}
	m.shufflePos = 1
	m.shuffleUnseen = []string{ids[1]}
	got := m.QueueTraversal()
	if !slices.Equal(got.RemainingIndices, []int{1}) {
		t.Fatalf("unexpected shuffle remainder: %+v", got)
	}
}

func TestRemainingIndicesAfterCurrentShuffleEntryRemoved(t *testing.T) {
	m := &Manager{queue: []QueueTrack{{QueueID: 1}, {QueueID: 2}, {QueueID: 3}}, currentIndex: -1, shuffleEnabled: true}
	ids := queueIdentities(m.queue)
	m.shuffleHistory = []string{ids[0], "removed-current", ids[1]}
	m.shufflePos = 1
	m.shuffleUnseen = []string{ids[2]}
	got := m.QueueTraversal()
	if !slices.Equal(got.RemainingIndices, []int{1, 2}) {
		t.Fatalf("played history leaked into remaining rows: %+v", got)
	}
}
