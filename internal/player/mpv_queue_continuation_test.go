package player

import (
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

// countCommands returns how many recorded commands match.
func countCommands(mu *sync.Mutex, received *[]ipcTestCommand, match func(ipcTestCommand) bool) int {
	mu.Lock()
	defer mu.Unlock()
	n := 0
	for _, cmd := range *received {
		if match(cmd) {
			n++
		}
	}
	return n
}

// loadfileFor matches a loadfile of path with the given mode.
func loadfileFor(path, mode string) func(ipcTestCommand) bool {
	return func(cmd ipcTestCommand) bool {
		return len(cmd) >= 3 && cmd.stringIs(0, "loadfile") &&
			cmd.stringIs(1, path) && cmd.stringIs(2, mode)
	}
}

// traversalState is the raw shuffle traversal state, copied for before/after
// comparison in tests that must prove an operation left it untouched.
type traversalState struct {
	history  []string
	pos      int
	unseen   []string
	excluded []string
	current  int
}

func readTraversalState(m *Manager) traversalState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	excluded := make([]string, 0, len(m.shuffleExcluded))
	for id := range m.shuffleExcluded {
		excluded = append(excluded, id)
	}
	slices.Sort(excluded)
	return traversalState{
		history:  append([]string(nil), m.shuffleHistory...),
		pos:      m.shufflePos,
		unseen:   append([]string(nil), m.shuffleUnseen...),
		excluded: excluded,
		current:  m.currentIndex,
	}
}

func (s traversalState) equal(o traversalState) bool {
	return slices.Equal(s.history, o.history) && s.pos == o.pos &&
		slices.Equal(s.unseen, o.unseen) && slices.Equal(s.excluded, o.excluded) &&
		s.current == o.current
}

// TestReplayCurrent_PreservesTraversalAndRefreshesSuccessor proves that a replay
// replaces playback with the current entry without moving the timeline: history,
// cursor, unseen pool, exclusions, and current index all survive, the snapshot is
// identical, and the same successor is prefetched again after the replacement
// wiped mpv's playlist.
func TestReplayCurrent_PreservesTraversalAndRefreshesSuccessor(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "replay-cur", &mu, &received)

	dir := t.TempDir()
	anchor := dir + "/anchor.flac"
	playable := dir + "/playable.flac"
	successor := dir + "/successor.flac"
	for _, p := range []string{anchor, playable, successor} {
		if err := os.WriteFile(p, []byte{}, 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
	}

	m.mu.Lock()
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: anchor, Hash: "h0", QueueID: 1},
		{Path: dir + "/gone.flac", Hash: "h1", QueueID: 2},
		{Path: playable, Hash: "h2", QueueID: 3},
		{Path: successor, Hash: "h3", QueueID: 4},
	}
	m.currentIndex = 0
	m.mu.Unlock()
	m.SetMissingMarker(&countingMarker{})

	if err := m.SetShuffle(true); err != nil { // identity order: unseen [1,2,3]
		t.Fatalf("SetShuffle(true): %v", err)
	}
	if err := m.Next(); err != nil { // skips the missing row 1, plays 2
		t.Fatalf("Next: %v", err)
	}
	if got := currentIdx(m); got != 2 {
		t.Fatalf("after Next, currentIndex = %d, want 2", got)
	}
	// The gapless successor is index 3.
	waitForCommand(t, &mu, &received, loadfileFor(successor, "append-play"),
		2*time.Second, "prefetch of the successor")

	before := readTraversalState(m)
	beforeSnapshot := m.QueueTraversal()
	// Everything recorded from here on belongs to the replay.
	mu.Lock()
	received = nil
	mu.Unlock()

	if err := m.ReplayCurrent(); err != nil {
		t.Fatalf("ReplayCurrent: %v", err)
	}

	if got := countCommands(&mu, &received, loadfileFor(playable, "replace")); got != 1 {
		t.Fatalf("loadfile replace of the current entry: got %d, want 1", got)
	}
	if after := readTraversalState(m); !after.equal(before) {
		t.Fatalf("ReplayCurrent moved the traversal: %+v, want %+v", after, before)
	}
	assertTraversalEqual(t, m.QueueTraversal(), beforeSnapshot)

	// The replacement reset mpv's playlist, so the same successor must be
	// appended again rather than left behind.
	waitForCommand(t, &mu, &received, loadfileFor(successor, "append-play"),
		2*time.Second, "the successor to be prefetched again")
}

// TestReplayCurrent_NoCurrentEntry verifies that a replay with nothing playing
// fails without touching mpv.
func TestReplayCurrent_NoCurrentEntry(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "replay-none", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 2)
	m.currentIndex = -1
	m.mu.Unlock()

	if err := m.ReplayCurrent(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("ReplayCurrent with no current entry: want ErrPlaybackFailed, got %v", err)
	}
	if got := countCommands(&mu, &received, isLoadfileReplace); got != 0 {
		t.Fatalf("ReplayCurrent sent %d replacements, want 0", got)
	}
}
