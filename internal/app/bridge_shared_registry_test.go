package app

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestSharedRegistry_ScanDiscoveredPathDeletableByWatcher is the FR-005
// regression: a track that exists only because the startup scan discovered it
// (never cached in the DB, so the registry starts empty) must still resolve to
// its content hash when a later live deletion reaches the watch bridge. Before
// the registry was shared, the watch bridge owned a private map the scan never
// touched, so MarkTrackMissing was never called for scan-only tracks.
func TestSharedRegistry_ScanDiscoveredPathDeletableByWatcher(t *testing.T) {
	// Empty seed: the track is NOT in the DB, it is discovered solely by scan.
	registry := newPathHashRegistry(nil)

	// 1. Scan bridge discovers a brand-new track and registers it.
	rawScan := make(chan library.Track, 1)
	rawScan <- library.Track{Hash: "scan-hash", Path: "/discovered.mp3"}
	close(rawScan)

	scanCh, scanDone := bridgeScanChannel(context.Background(), rawScan, &stubUpsertBatcher{}, registry, 10)
	for range scanCh {
	}
	<-scanDone

	// 2. A live deletion of the same path arrives at the watch bridge, which
	//    shares the registry.
	rawWatch := make(chan library.WatchEvent, 1)
	rawWatch <- library.WatchEvent{Deleted: true, Path: "/discovered.mp3"}
	close(rawWatch)

	marker := &stubMissingMarker{}
	watchCh, watchDone := bridgeWatchChannel(context.Background(), rawWatch, &stubUpsertBatcher{}, marker, registry, 10)

	// The deletion event is still forwarded to the TUI channel; that forwarded
	// event is how the queue's instances of this track get marked missing
	// downstream, independent of the DB-row MarkTrackMissing below.
	var forwarded []library.WatchEvent
	for ev := range watchCh {
		forwarded = append(forwarded, ev)
	}
	<-watchDone

	if len(marker.hashes) != 1 || marker.hashes[0] != "scan-hash" {
		t.Fatalf("expected scan-discovered track to mark [scan-hash] missing, got %v", marker.hashes)
	}
	if len(forwarded) != 1 || !forwarded[0].Deleted || forwarded[0].Path != "/discovered.mp3" {
		t.Fatalf("expected deletion event forwarded to TUI channel, got %v", forwarded)
	}
}

// TestSharedRegistry_ScanMoveMakesOldPathDeleteNoOp confirms that a scan
// discovery of a moved file (same hash, new path) reconciles the shared state
// so the subsequent deletion of the file's *old* path (fed as an initial
// deletion) is a no-op rather than spuriously marking the still-present track
// missing.
func TestSharedRegistry_ScanMoveMakesOldPathDeleteNoOp(t *testing.T) {
	// The DB knew the file at its old path.
	registry := newPathHashRegistry(map[string]string{"/old.mp3": "same-hash"})

	// Scan discovers the same content at a new path (a move).
	rawScan := make(chan library.Track, 1)
	rawScan <- library.Track{Hash: "same-hash", Path: "/new.mp3"}
	close(rawScan)
	scanCh, scanDone := bridgeScanChannel(context.Background(), rawScan, &stubUpsertBatcher{}, registry, 10)
	for range scanCh {
	}
	<-scanDone

	// The diff walk emits the old path as a deletion. It must resolve to
	// nothing now that the move has re-pointed the hash at /new.mp3.
	rawWatch := make(chan library.WatchEvent, 1)
	rawWatch <- library.WatchEvent{Deleted: true, Path: "/old.mp3"}
	close(rawWatch)
	marker := &stubMissingMarker{}
	watchCh, watchDone := bridgeWatchChannel(context.Background(), rawWatch, &stubUpsertBatcher{}, marker, registry, 10)
	for range watchCh {
	}
	<-watchDone

	if len(marker.hashes) != 0 {
		t.Fatalf("expected moved-away old-path delete to be a no-op, got missing hashes %v", marker.hashes)
	}
	// The moved file is still resolvable at its new path.
	if hash, ok := registry.remove("/new.mp3"); !ok || hash != "same-hash" {
		t.Fatalf("expected /new.mp3 to resolve to same-hash, got (%q, %v)", hash, ok)
	}
}

// TestSharedRegistry_ConcurrentScanAndWatchRaceFree runs both bridges against
// one registry concurrently, registering the same identities from both sides,
// to exercise the registry's mutex under -race and confirm the end state is
// deterministic regardless of interleaving (idempotent registration).
func TestSharedRegistry_ConcurrentScanAndWatchRaceFree(t *testing.T) {
	registry := newPathHashRegistry(nil)

	const n = 200
	rawScan := make(chan library.Track, n)
	rawWatch := make(chan library.WatchEvent, n)
	for i := 0; i < n; i++ {
		p := fmt.Sprintf("/track-%d.mp3", i)
		h := fmt.Sprintf("hash-%d", i)
		rawScan <- library.Track{Hash: h, Path: p}
		rawWatch <- library.WatchEvent{Track: library.Track{Hash: h, Path: p}}
	}
	close(rawScan)
	close(rawWatch)

	scanCh, scanDone := bridgeScanChannel(context.Background(), rawScan, &stubUpsertBatcher{}, registry, 32)
	watchCh, watchDone := bridgeWatchChannel(context.Background(), rawWatch, &stubUpsertBatcher{}, &stubMissingMarker{}, registry, 32)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range scanCh {
		}
	}()
	go func() {
		defer wg.Done()
		for range watchCh {
		}
	}()
	<-scanDone
	<-watchDone
	wg.Wait()

	// After both bridges settle, every path resolves to its hash regardless of
	// which bridge registered it last.
	for i := 0; i < n; i++ {
		p := fmt.Sprintf("/track-%d.mp3", i)
		h := fmt.Sprintf("hash-%d", i)
		if got, ok := registry.remove(p); !ok || got != h {
			t.Fatalf("path %s resolved to (%q, %v), want (%q, true)", p, got, ok, h)
		}
	}
}
