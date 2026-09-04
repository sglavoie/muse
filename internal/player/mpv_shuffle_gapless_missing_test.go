package player

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

// recordingMissingMarker captures the track hashes and queue-entry IDs marked
// missing so a test can assert gapless traversal produces the same durable
// missing markers as non-gapless traversal.
type recordingMissingMarker struct {
	mu       sync.Mutex
	tracks   []string
	queueIDs []int64
}

func (r *recordingMissingMarker) MarkTrackMissing(hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tracks = append(r.tracks, hash)
	return nil
}

func (r *recordingMissingMarker) MarkQueueEntryMissing(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queueIDs = append(r.queueIDs, id)
	return nil
}

func (r *recordingMissingMarker) snapshot() ([]string, []int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.tracks...), append([]int64(nil), r.queueIDs...)
}

// TestShuffleGaplessCommitMarksSkippedMissing verifies that a live queue entry
// which gapless shuffle skips because its file is missing on disk is marked
// through both the track and queue-entry missing APIs and emits the missing-file
// callbacks — the same durable side effects non-gapless traversal produces — and
// that the equivalent non-gapless (Next-driven) traversal marks the same entry.
func TestShuffleGaplessCommitMarksSkippedMissing(t *testing.T) {
	// Non-gapless reference: drive Next() over the same queue with one missing
	// entry and record what gets marked.
	nonGapless := &recordingMissingMarker{}
	var status, missing []string
	{
		var mu sync.Mutex
		var received []ipcTestCommand
		m := newConnectedManager(t, "shuf-miss-nogap", &mu, &received)
		m.SetMissingMarker(nonGapless)
		m.mu.Lock()
		m.callbacks = EventCallbacks{
			OnStatus:      func(s string) { status = append(status, s) },
			OnMissingFile: func(p string) { missing = append(missing, p) },
		}
		m.mu.Unlock()

		queue := shuffleTestQueue(t, 3)
		// Delete the file that reverse-permute visits first after the anchor so
		// traversal must skip it. Reverse permute anchored at 0 visits queue
		// index 2 first, then index 1.
		if err := os.Remove(queue[2].Path); err != nil {
			t.Fatalf("remove queue[2] file: %v", err)
		}
		buildShuffleState(t, m, queue, 0, reversePermute)

		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
		if got := currentIdx(m); got != 1 {
			t.Fatalf("non-gapless: currentIndex = %d, want 1 (skipped missing index 2)", got)
		}
	}
	refTracks, refQueueIDs := nonGapless.snapshot()
	if !slices.Equal(refTracks, []string{"h2"}) || !slices.Equal(refQueueIDs, []int64{3}) {
		t.Fatalf("non-gapless reference marked tracks=%v queueIDs=%v, want [h2] [3]", refTracks, refQueueIDs)
	}
	if len(missing) != 1 || len(status) != 1 {
		t.Fatalf("non-gapless callbacks: status=%v missing=%v, want one of each", status, missing)
	}

	// Gapless: drive the same scenario through an EOF event so the skipped
	// missing entry is committed via commitShufflePrefetchLocked.
	sock := shortSockPath(t, "shuf-miss-gap")
	var mu sync.Mutex
	var received []ipcTestCommand
	events := fakeEventCommandServer(t, sock, &mu, &received)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	marker := &recordingMissingMarker{}
	var gapStatus, gapMissing []string
	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		marker:          marker,
		callbacksReady:  make(chan struct{}),
		prefetchedIndex: -1,
		shufflePos:      -1,
		callbacks: EventCallbacks{
			OnTrackChanged:  func(_, _, _, _ string, _ int, _ int64) {},
			OnPlaybackEnded: func() {},
			OnStatus:        func(s string) { mu.Lock(); gapStatus = append(gapStatus, s); mu.Unlock() },
			OnMissingFile:   func(p string) { mu.Lock(); gapMissing = append(gapMissing, p); mu.Unlock() },
		},
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.gaplessEnabled = true
	m.mu.Unlock()
	close(m.callbacksReady)

	go m.startEventReader(ctx, nil)
	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		return cmd.stringIs(0, cmdObserveProperty) && cmd.stringIs(2, propPause)
	}, time.Second, "pause property observation")

	queue := shuffleTestQueue(t, 3)
	missingPath := queue[2].Path
	if err := os.Remove(missingPath); err != nil {
		t.Fatalf("remove queue[2] file: %v", err)
	}

	m.SetPermuteFunc(reversePermute)
	m.mu.Lock()
	m.queue = queue
	m.currentIndex = 0
	m.shuffleEnabled = true
	m.mu.Unlock()
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}

	// Prefetch skips the missing index 2 and appends index 1.
	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		path, _ := cmd.stringAt(1)
		return isLoadfileAppend(cmd) && path == queue[1].Path
	}, 2*time.Second, "prefetch of index 1 (missing index 2 skipped)")

	events <- `{"event":"end-file","reason":"eof"}`
	waitForCurrentIndex(t, m, 1)

	// The skipped missing entry must be marked with the same track/queue APIs and
	// callbacks as the non-gapless reference.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		tr, _ := marker.snapshot()
		if len(tr) >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	gapTracks, gapQueueIDs := marker.snapshot()
	if !slices.Equal(gapTracks, refTracks) {
		t.Errorf("gapless marked tracks=%v, want %v (equivalent to non-gapless)", gapTracks, refTracks)
	}
	if !slices.Equal(gapQueueIDs, refQueueIDs) {
		t.Errorf("gapless marked queueIDs=%v, want %v (equivalent to non-gapless)", gapQueueIDs, refQueueIDs)
	}
	mu.Lock()
	gs, gm := append([]string(nil), gapStatus...), append([]string(nil), gapMissing...)
	mu.Unlock()
	if len(gm) != 1 || gm[0] != missingPath {
		t.Errorf("gapless OnMissingFile = %v, want [%s]", gm, missingPath)
	}
	if len(gs) != 1 {
		t.Errorf("gapless OnStatus = %v, want exactly one missing-file status", gs)
	}
}

// TestCommitShufflePrefetch_ReportsLiveSkippedNotRemoved verifies the commit
// distinguishes the two skip reasons: an intervening identity that is still a
// live queue row (skipped by the prefetch peek because its file was missing) is
// reported for missing-marking, while an identity whose row was removed is
// dropped silently and never falsely marked missing.
func TestCommitShufflePrefetch_ReportsLiveSkippedNotRemoved(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, prefetchedIndex: -1, shufflePos: -1}
	defer m.Close()

	// Live queue holds q1 (current), q4, q3. The unseen pool additionally lists
	// q2, whose row is no longer present (removed), ahead of the live-but-missing
	// q4 and the target q3.
	queue := []QueueTrack{
		{Path: "/a.flac", Hash: "h1", QueueID: 1},
		{Path: "/d.flac", Hash: "h4", QueueID: 4},
		{Path: "/c.flac", Hash: "h3", QueueID: 3},
	}
	m.mu.Lock()
	m.queue = queue
	m.shuffleEnabled = true
	m.shuffleHistory = []string{"q:1"}
	m.shufflePos = 0
	m.shuffleUnseen = []string{"q:2", "q:4", "q:3"}
	committed, skipped := m.commitShufflePrefetchLocked("q:3")
	m.mu.Unlock()

	if committed != 2 {
		t.Fatalf("committed = %d, want 2 (q:3 at queue index 2)", committed)
	}
	// Exactly q:4 (live row) reported; q:2 (removed) omitted.
	if len(skipped) != 1 || skipped[0].QueueID != 4 {
		t.Fatalf("skipped = %+v, want exactly the live q:4 entry", skipped)
	}
}

// TestResolveNextShufflePrefetch_SkipsNotExistStopsOnOtherError verifies the
// prefetch resolver skips a file that is genuinely absent (os.ErrNotExist) and
// resolves the next playable identity, but stops resolving on a non-not-exist
// stat error so the EOF fallback surfaces that error through PlayQueueEntry
// instead of the resolver falsely treating it as a skippable missing file.
func TestResolveNextShufflePrefetch_SkipsNotExistStopsOnOtherError(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present.flac")
	if err := os.WriteFile(present, nil, 0o600); err != nil {
		t.Fatalf("write present: %v", err)
	}

	t.Run("skips not-exist to the next playable", func(t *testing.T) {
		queue := []QueueTrack{
			{Path: filepath.Join(dir, "gone.flac"), Hash: "h1", QueueID: 1},
			{Path: present, Hash: "h2", QueueID: 2},
		}
		idx, path, id := resolveNextShufflePrefetch(queue, []string{"q:1", "q:2"})
		if idx != 1 || path != present || id != "q:2" {
			t.Fatalf("resolveNextShufflePrefetch = (%d,%q,%q), want (1,%q,q:2)", idx, path, id, present)
		}
	})

	t.Run("stops on non-not-exist stat error", func(t *testing.T) {
		base := filepath.Join(dir, "regular")
		if err := os.WriteFile(base, nil, 0o600); err != nil {
			t.Fatalf("write regular file: %v", err)
		}
		// Using a regular file as a path component yields ENOTDIR, which is not
		// os.ErrNotExist, so the file after it must NOT be resolved.
		notDir := filepath.Join(base, "child.flac")
		queue := []QueueTrack{
			{Path: notDir, Hash: "h1", QueueID: 1},
			{Path: present, Hash: "h2", QueueID: 2},
		}
		idx, path, id := resolveNextShufflePrefetch(queue, []string{"q:1", "q:2"})
		if idx != -1 || path != "" || id != "" {
			t.Fatalf("resolveNextShufflePrefetch = (%d,%q,%q), want stop (-1,\"\",\"\") on non-not-exist stat error", idx, path, id)
		}
	})
}
