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

// exhaustedShuffleCycle returns a connected manager whose four-entry shuffle
// cycle has been played to exhaustion, together with two further playable tracks
// a test can append to it.
func exhaustedShuffleCycle(t *testing.T, name string, mu *sync.Mutex, received *[]ipcTestCommand) (*Manager, []QueueTrack) {
	t.Helper()
	m := newConnectedManager(t, name, mu, received)

	q := shuffleTestQueue(t, 6)
	m.mu.Lock()
	m.queue = append([]QueueTrack(nil), q[:4]...)
	m.currentIndex = 0
	m.mu.Unlock()
	m.SetMissingMarker(&countingMarker{})

	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	for range 3 {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
	}
	if err := m.Next(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Next after a full cycle: want ErrPlaybackFailed, got %v", err)
	}
	return m, q[4:]
}

// missingTrack returns a queue entry whose file does not exist.
func missingTrack(t *testing.T, queueID int64) QueueTrack {
	t.Helper()
	return QueueTrack{Path: t.TempDir() + "/gone.flac", Hash: "hx", QueueID: queueID}
}

// TestResumeAfterQueueExtension_ShufflePlaysNewEntries proves that resuming an
// exhausted shuffle cycle plays the newly eligible row, skips additions missing
// on disk, returns the row it actually played, and leaves the completed cycle in
// place instead of reanchoring.
func TestResumeAfterQueueExtension_ShufflePlaysNewEntries(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m, extra := exhaustedShuffleCycle(t, "resume-shuf", &mu, &received)

	// Nothing to resume into yet.
	if _, err := m.ResumeAfterQueueExtension(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("resume without an extension: want ErrPlaybackFailed, got %v", err)
	}
	completed := readTraversalState(m)
	if len(completed.history) != 4 {
		t.Fatalf("completed cycle history = %v, want 4 entries", completed.history)
	}

	// Extend the queue with a row missing on disk followed by a playable one.
	m.AppendToQueue(missingTrack(t, 99))
	m.AppendToQueue(extra[0])

	idx, err := m.ResumeAfterQueueExtension()
	if err != nil {
		t.Fatalf("ResumeAfterQueueExtension: %v", err)
	}
	if idx != 5 || currentIdx(m) != 5 {
		t.Fatalf("resume returned %d (currentIndex %d), want 5 (the playable addition)", idx, currentIdx(m))
	}

	// The completed cycle is preserved: the played history still leads, with the
	// resumed entry appended after it. The missing addition was dropped.
	resumed := readTraversalState(m)
	if len(resumed.history) != 5 || !slices.Equal(resumed.history[:4], completed.history) {
		t.Fatalf("resume rewrote the completed cycle: %v, want %v plus one entry", resumed.history, completed.history)
	}
	// Order: the completed cycle, the resumed entry, then the skipped addition
	// parked at the end with nothing remaining.
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3, 5, 4}, 4, 0, true)
}

// TestResumeAfterQueueExtension_DoesNotRescheduleSkippedAdditions proves that a
// later extension makes only the new row eligible: reconciling it must not
// resurrect an addition an earlier resume skipped as missing, and once the new
// row is played the cycle is exhausted again.
func TestResumeAfterQueueExtension_DoesNotRescheduleSkippedAdditions(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m, extra := exhaustedShuffleCycle(t, "resume-reschedule", &mu, &received)

	m.AppendToQueue(missingTrack(t, 99))
	m.AppendToQueue(extra[0])
	if _, err := m.ResumeAfterQueueExtension(); err != nil {
		t.Fatalf("ResumeAfterQueueExtension: %v", err)
	}

	m.AppendToQueue(extra[1])
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3, 5, 6, 4}, 4, 1, true)

	idx, err := m.ResumeAfterQueueExtension()
	if err != nil {
		t.Fatalf("second ResumeAfterQueueExtension: %v", err)
	}
	if idx != 6 {
		t.Fatalf("second resume returned index %d, want 6", idx)
	}
	if _, err := m.ResumeAfterQueueExtension(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("resume with nothing new: want ErrPlaybackFailed, got %v", err)
	}

	// No row played twice across the whole run.
	final := readTraversalState(m)
	if len(final.history) != 6 {
		t.Fatalf("final history = %v, want 6 entries", final.history)
	}
	seen := map[string]bool{}
	for _, id := range final.history {
		if seen[id] {
			t.Fatalf("identity %q played twice: %v", id, final.history)
		}
		seen[id] = true
	}
}

// TestResumeAfterQueueExtension_SequentialPlaysAppendedRows verifies the
// shuffle-off path: resuming plays the entry after the current index, skipping
// additions missing on disk, and reports the index it landed on.
func TestResumeAfterQueueExtension_SequentialPlaysAppendedRows(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "resume-seq", &mu, &received)

	q := shuffleTestQueue(t, 6)
	m.mu.Lock()
	m.queue = append([]QueueTrack(nil), q[:4]...)
	m.currentIndex = 3 // played through to the end of the queue
	m.mu.Unlock()
	m.SetMissingMarker(&countingMarker{})

	if _, err := m.ResumeAfterQueueExtension(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("resume without an extension: want ErrPlaybackFailed, got %v", err)
	}

	m.AppendToQueue(q[4])
	idx, err := m.ResumeAfterQueueExtension()
	if err != nil {
		t.Fatalf("ResumeAfterQueueExtension: %v", err)
	}
	if idx != 4 || currentIdx(m) != 4 {
		t.Fatalf("resume returned %d (currentIndex %d), want 4", idx, currentIdx(m))
	}

	// A missing addition is skipped in favour of the next playable one, and the
	// returned index is the row that actually played.
	m.AppendToQueue(QueueTrack{Path: t.TempDir() + "/gone.flac", Hash: "hx", QueueID: 99})
	m.AppendToQueue(q[5])
	idx, err = m.ResumeAfterQueueExtension()
	if err != nil {
		t.Fatalf("ResumeAfterQueueExtension over a missing addition: %v", err)
	}
	if idx != 6 || currentIdx(m) != 6 {
		t.Fatalf("resume returned %d (currentIndex %d), want 6", idx, currentIdx(m))
	}
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3, 4, 5, 6}, 6, 0, false)
}
