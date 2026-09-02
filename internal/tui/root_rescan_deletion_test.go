package tui

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// recordingScanWriter is a scanWriter stub that both satisfies scanUpsertBatcher
// and records durable MarkTrackMissing calls, so a test can assert what the
// manual-rescan deletion path persists for the next cached startup. It is
// mutex-guarded because the real library.Scan launched by actionRescan spins up
// a background bridge goroutine that may also touch the writer under -race.
type recordingScanWriter struct {
	mu       sync.Mutex
	upserted [][]library.Track
	missing  map[string]bool
}

func newRecordingScanWriter() *recordingScanWriter {
	return &recordingScanWriter{missing: map[string]bool{}}
}

func (w *recordingScanWriter) UpsertBatch(tracks []library.Track) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]library.Track, len(tracks))
	copy(cp, tracks)
	w.upserted = append(w.upserted, cp)
	return nil
}

func (w *recordingScanWriter) MarkTrackMissing(hash string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.missing[hash] = true
	return nil
}

func (w *recordingScanWriter) missingSnapshot() map[string]bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string]bool, len(w.missing))
	for k, v := range w.missing {
		out[k] = v
	}
	return out
}

// TestRescanReconcilesDeletionAfterWatcherDegraded pins FR-010: when the live
// watcher is unavailable, the advertised manual-rescan fallback must reconcile
// files that vanished from disk. It exercises the full contract end to end —
// watcher degradation, a deletion discovered only by rescan, the rescan itself,
// and the durable missing state a subsequent cached startup would read back.
func TestRescanReconcilesDeletionAfterWatcherDegraded(t *testing.T) {
	root := t.TempDir()
	keptPath := filepath.Join(root, "keep.mp3")
	deletedPath := filepath.Join(root, "gone.mp3")
	const keptHash = "hkeep"
	const deletedHash = "hdel"

	writer := newRecordingScanWriter()
	m := New(context.Background(),
		WithLibraryRoots([]string{root}),
		WithExtractor(stubExtractor{}),
		WithScanWriter(writer, 500),
	)

	// Seed the pre-rescan library index with a track that will survive and one
	// that will be deleted off disk.
	m.library.Index.Insert(library.Track{Hash: keptHash, Path: keptPath, Title: "Keep", Artist: "A"})
	m.library.Index.Insert(library.Track{Hash: deletedHash, Path: deletedPath, Title: "Gone", Artist: "B"})

	// Queue holds the kept track once and the doomed track twice, so we can prove
	// EVERY in-memory instance of the deletion becomes unavailable.
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: keptHash, Path: keptPath, Title: "Keep", Artist: "A"},
		{ID: 2, Hash: deletedHash, Path: deletedPath, Title: "Gone", Artist: "B"},
		{ID: 3, Hash: deletedHash, Path: deletedPath, Title: "Gone", Artist: "B"},
	}

	// 1. The background pipeline reports the watcher unavailable. This is the
	// condition that makes manual rescan the only deletion-reconciliation path.
	next, _ := m.Update(library.WatchStatusMsg{Available: false})
	m = next.(Model)
	if !m.watchDegraded {
		t.Fatal("watcher-unavailable status should mark the model degraded")
	}

	// 2. Trigger the manual rescan (Ctrl+R). It snapshots the prior paths and
	// clears the index; we drive the batches by hand rather than the real walk.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("rescan: expected a non-nil scan cmd")
	}
	if got := len(m.rescanPriorPaths); got != 2 {
		t.Fatalf("rescan: expected 2 prior paths captured, got %d", got)
	}
	if m.rescanPriorPaths[keptPath] != keptHash || m.rescanPriorPaths[deletedPath] != deletedHash {
		t.Fatalf("rescan: prior-path snapshot mismatch: %v", m.rescanPriorPaths)
	}
	if m.library.Index.Len() != 0 {
		t.Fatalf("rescan: index should be cleared, got %d entries", m.library.Index.Len())
	}

	// 3. Only the surviving file is rediscovered by the scan; the deleted file
	// never reappears.
	next, _ = m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Hash: keptHash, Path: keptPath, Title: "Keep", Artist: "A"},
	}})
	m = next.(Model)

	// 4. Scan completion reconciles the deletion.
	next, doneCmd := m.Update(library.ScanDoneMsg{})
	m = next.(Model)

	// In-memory: the kept instance stays available; both deleted instances are
	// flagged missing.
	if m.queue.entries[0].Missing {
		t.Error("kept queue instance must not be marked missing")
	}
	if !m.queue.entries[1].Missing || !m.queue.entries[2].Missing {
		t.Errorf("every deleted queue instance must be missing, got %v / %v",
			m.queue.entries[1].Missing, m.queue.entries[2].Missing)
	}
	if m.rescanPriorPaths != nil {
		t.Error("rescan: prior paths should be cleared after completion")
	}
	if m.scanning {
		t.Error("rescan: scanning flag should be cleared after completion")
	}

	// Durable: running the returned cmd persists the missing mark by hash.
	if doneCmd == nil {
		t.Fatal("scan-done: expected a cmd persisting the deletion")
	}
	doneCmd()
	persisted := writer.missingSnapshot()
	if !persisted[deletedHash] {
		t.Errorf("deleted track %q must be marked missing durably", deletedHash)
	}
	if persisted[keptHash] {
		t.Errorf("kept track %q must not be marked missing", keptHash)
	}

	// 5. Cached startup: a later launch loads queue rows with the persisted
	// missing state. The deleted track must remain unavailable; the kept one must
	// not. Model the DB-backed load and assert the durable mark drives it.
	startup := []QueueEntry{
		{ID: 1, Hash: keptHash, Path: keptPath, Missing: persisted[keptHash]},
		{ID: 2, Hash: deletedHash, Path: deletedPath, Missing: persisted[deletedHash]},
	}
	if startup[0].Missing {
		t.Error("cached startup: kept track should load available")
	}
	if !startup[1].Missing {
		t.Error("cached startup: deleted track should load unavailable")
	}
}

// TestRescanCanceledSkipsDeletionReconciliation verifies that a canceled rescan
// does not treat the partially-rebuilt index as authoritative: the vanished
// paths it would otherwise compute are meaningless, so no deletion is persisted.
func TestRescanCanceledSkipsDeletionReconciliation(t *testing.T) {
	root := t.TempDir()
	writer := newRecordingScanWriter()
	m := New(context.Background(),
		WithLibraryRoots([]string{root}),
		WithExtractor(stubExtractor{}),
		WithScanWriter(writer, 500),
	)
	m.library.Index.Insert(library.Track{Hash: "h1", Path: filepath.Join(root, "a.mp3")})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: filepath.Join(root, "b.mp3")})

	// Start the rescan, then cancel it via esc before any batch arrives.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(Model)
	m, _, _ = m.handleScanCancelKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.rescanPriorPaths != nil {
		t.Fatal("cancel: prior paths should be dropped so no deletion is computed")
	}

	// The trailing ScanDoneMsg must not persist any missing marks.
	next2, _ := m.Update(library.ScanDoneMsg{})
	_ = next2.(Model)
	if got := len(writer.missingSnapshot()); got != 0 {
		t.Fatalf("canceled rescan must persist no deletions, got %d", got)
	}
}

// TestRescanTruncatedByHashPanicSkipsDeletionReconciliation verifies that a
// rescan whose hash stage died partway is not treated as authoritative. The
// scan channel closes exactly as it does after a complete run, so ScanDoneMsg
// alone cannot tell the two apart; without the stage outcome the files the
// panicking worker never hashed are absent from the rebuilt index and would be
// persisted as deletions even though they are still on disk.
func TestRescanTruncatedByHashPanicSkipsDeletionReconciliation(t *testing.T) {
	root := t.TempDir()
	stillOnDisk := filepath.Join(root, "unhashed.mp3")
	writer := newRecordingScanWriter()
	m := New(context.Background(),
		WithLibraryRoots([]string{root}),
		WithExtractor(stubExtractor{}),
		WithScanWriter(writer, 500),
	)
	m.library.Index.Insert(library.Track{Hash: "hunhashed", Path: stillOnDisk})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.scanOutcome == nil {
		t.Fatal("rescan must carry the hash-stage outcome alongside the scan channel")
	}
	if len(m.rescanPriorPaths) != 1 {
		t.Fatalf("rescan: expected the prior path to be snapshotted, got %v", m.rescanPriorPaths)
	}

	// Stand in for the hash worker that panicked: the stream still ends, but the
	// stage reports that it ended short.
	outcome := &library.StageOutcome{}
	outcome.Fail(library.ErrHashWorkerPanicked)
	m.scanOutcome = outcome

	next, doneCmd := m.Update(library.ScanDoneMsg{})
	m = next.(Model)

	if doneCmd != nil {
		t.Error("a truncated rescan must not return a cmd persisting deletions")
	}
	if got := len(writer.missingSnapshot()); got != 0 {
		t.Errorf("truncated rescan must persist no deletions, got %d", got)
	}
	if m.scanOutcome != nil {
		t.Error("scan outcome should be cleared once the scan is done")
	}
}
