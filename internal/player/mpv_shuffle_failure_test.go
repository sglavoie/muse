package player

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
)

// newFailableManager creates a connected Manager whose fake mpv server fails
// every `loadfile ... replace` command (the PlayQueueEntry load) with an mpv
// error while failLoad is set, and succeeds otherwise. Prefetch appends and all
// other commands always succeed, so only the transport load can be made to
// fail. Returns the manager and the toggle.
func newFailableManager(t *testing.T, name string) (*Manager, *atomic.Bool) {
	t.Helper()
	sock := shortSockPath(t, name)
	var failLoad atomic.Bool
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		if failLoad.Load() && req.Command.stringIs(0, cmdLoadfile) && req.Command.stringIs(2, "replace") {
			return errorResponse(req, "device error")
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		prefetchedIndex: -1,
	}
	t.Cleanup(func() { m.Close() })

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()
	return m, &failLoad
}

// shuffleState is a snapshot of the traversal fields plus currentIndex, used to
// assert that a failed load leaves every one of them unchanged.
type shuffleState struct {
	history []string
	pos     int
	unseen  []string
	current int
}

func captureShuffleState(m *Manager) shuffleState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return shuffleState{
		history: append([]string(nil), m.shuffleHistory...),
		pos:     m.shufflePos,
		unseen:  append([]string(nil), m.shuffleUnseen...),
		current: m.currentIndex,
	}
}

func assertShuffleStateEqual(t *testing.T, got, want shuffleState, label string) {
	t.Helper()
	if !slices.Equal(got.history, want.history) {
		t.Errorf("%s: history = %v, want %v", label, got.history, want.history)
	}
	if got.pos != want.pos {
		t.Errorf("%s: pos = %d, want %d", label, got.pos, want.pos)
	}
	if !slices.Equal(got.unseen, want.unseen) {
		t.Errorf("%s: unseen = %v, want %v", label, got.unseen, want.unseen)
	}
	if got.current != want.current {
		t.Errorf("%s: currentIndex = %d, want %d", label, got.current, want.current)
	}
}

// assertNonMissingFailure fails the test unless err is a real load failure (not
// nil, not a missing-file skip).
func assertNonMissingFailure(t *testing.T, err error, op string) {
	t.Helper()
	if err == nil || errors.Is(err, ErrMissingFile) {
		t.Fatalf("%s under injected failure: want non-missing error, got %v", op, err)
	}
}

// TestShuffleFailure_ForwardLoadIsTransactional verifies that a non-missing load
// failure during forward traversal (Next) does not consume the intended
// identity: traversal state and currentIndex are unchanged, and a retry attempts
// the same identity rather than skipping it.
func TestShuffleFailure_ForwardLoadIsTransactional(t *testing.T) {
	m, failLoad := newFailableManager(t, "shuf-fail-fwd")

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = 0
	m.mu.Unlock()

	m.SetPermuteFunc(reversePermute) // unseen over [1,2,3] -> 3,2,1
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	before := captureShuffleState(m)

	failLoad.Store(true)
	assertNonMissingFailure(t, m.Next(), "Next")
	assertShuffleStateEqual(t, captureShuffleState(m), before, "after failed Next")

	// Retry once the failure clears: the same identity (index 3) must play, not
	// the one after it.
	failLoad.Store(false)
	if err := m.Next(); err != nil {
		t.Fatalf("Next retry: %v", err)
	}
	if got := currentIdx(m); got != 3 {
		t.Fatalf("retry played index %d, want 3 (intended identity, not skipped)", got)
	}
}

// TestShuffleFailure_BackwardLoadIsTransactional verifies that a non-missing load
// failure during backward traversal (Previous) does not consume the previous
// identity: the cursor is unchanged and a retry re-attempts the same history
// entry.
func TestShuffleFailure_BackwardLoadIsTransactional(t *testing.T) {
	m, failLoad := newFailableManager(t, "shuf-fail-bwd")

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = 0
	m.mu.Unlock()

	m.SetPermuteFunc(reversePermute)
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	// Build history 0 -> 3 -> 2 so Previous has somewhere to walk back to.
	if err := m.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if err := m.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got := currentIdx(m); got != 2 {
		t.Fatalf("setup currentIndex = %d, want 2", got)
	}

	before := captureShuffleState(m)

	failLoad.Store(true)
	assertNonMissingFailure(t, m.Previous(), "Previous")
	assertShuffleStateEqual(t, captureShuffleState(m), before, "after failed Previous")

	failLoad.Store(false)
	if err := m.Previous(); err != nil {
		t.Fatalf("Previous retry: %v", err)
	}
	if got := currentIdx(m); got != 3 {
		t.Fatalf("retry played index %d, want 3 (intended previous identity)", got)
	}
}

// TestShuffleFailure_ExplicitAnchorIsTransactional verifies that a non-missing
// load failure while anchoring an explicit selection does not discard the prior
// cycle: traversal state and currentIndex are unchanged, and a retry anchors the
// same identity.
func TestShuffleFailure_ExplicitAnchorIsTransactional(t *testing.T) {
	m, failLoad := newFailableManager(t, "shuf-fail-anchor")

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = 0
	m.mu.Unlock()

	m.SetPermuteFunc(reversePermute)
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	if err := m.Next(); err != nil { // 0 -> 3, so history is non-trivial
		t.Fatalf("Next: %v", err)
	}

	before := captureShuffleState(m)

	failLoad.Store(true)
	assertNonMissingFailure(t, m.PlayFromIndex(1), "PlayFromIndex")
	assertShuffleStateEqual(t, captureShuffleState(m), before, "after failed PlayFromIndex")

	failLoad.Store(false)
	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex retry: %v", err)
	}
	if got := currentIdx(m); got != 1 {
		t.Fatalf("retry played index %d, want 1 (anchored identity)", got)
	}
	m.mu.RLock()
	histLen := len(m.shuffleHistory)
	pos := m.shufflePos
	m.mu.RUnlock()
	if histLen != 1 || pos != 0 {
		t.Fatalf("after retry anchor: history len = %d, pos = %d; want 1, 0", histLen, pos)
	}
}

// TestShuffleFailure_RestartCycleIsTransactional verifies that a non-missing load
// failure while starting a fresh repeat-all cycle rolls back the regenerated
// permutation: traversal state and currentIndex are unchanged, so a retry starts
// from the same point.
func TestShuffleFailure_RestartCycleIsTransactional(t *testing.T) {
	m, failLoad := newFailableManager(t, "shuf-fail-restart")

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = 0
	m.mu.Unlock()

	m.SetPermuteFunc(reversePermute)
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	if err := m.Next(); err != nil { // 0 -> 3
		t.Fatalf("Next: %v", err)
	}

	before := captureShuffleState(m)

	failLoad.Store(true)
	assertNonMissingFailure(t, m.RestartQueueCycle(), "RestartQueueCycle")
	assertShuffleStateEqual(t, captureShuffleState(m), before, "after failed RestartQueueCycle")

	failLoad.Store(false)
	if err := m.RestartQueueCycle(); err != nil {
		t.Fatalf("RestartQueueCycle retry: %v", err)
	}
}
