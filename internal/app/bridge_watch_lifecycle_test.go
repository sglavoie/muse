package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// TestBridgeWatchChannel_IdleFlushPersistsWithoutClose verifies that a single
// changed track is persisted after the idle window elapses, even though the
// input channel stays open and the size cap is never reached.
func TestBridgeWatchChannel_IdleFlushPersistsWithoutClose(t *testing.T) {
	raw := make(chan library.WatchEvent, 1)
	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}

	// A large size cap ensures only the idle window can trigger the flush.
	ch := BridgeWatchChannel(raw, writer, marker, nil, 500)

	raw <- library.WatchEvent{Track: library.Track{Hash: "a", Path: "/a.mp3"}}
	<-ch // consume the forwarded event

	if !waitForCondition(2*time.Second, func() bool {
		return len(writer.allTracks()) == 1
	}) {
		t.Fatalf("expected 1 persisted track after idle window, got %d", len(writer.allTracks()))
	}

	close(raw)
	for range ch {
	}
}

// TestBridgeWatchChannel_IdleRetryAfterTransientFailure verifies that a batch
// whose first flush fails is retained and retried on the backoff timer until it
// persists, all without closing the input channel.
func TestBridgeWatchChannel_IdleRetryAfterTransientFailure(t *testing.T) {
	raw := make(chan library.WatchEvent, 1)
	writer := &flakyUpsertBatcher{failures: 1, err: errors.New("transient upsert failed")}
	marker := &stubMissingMarker{}

	ch := BridgeWatchChannel(raw, writer, marker, nil, 500)

	raw <- library.WatchEvent{Track: library.Track{Hash: "a", Path: "/a.mp3"}}
	<-ch

	if !waitForCondition(3*time.Second, func() bool {
		writer.mu.Lock()
		defer writer.mu.Unlock()
		return len(writer.persisted) == 1
	}) {
		writer.mu.Lock()
		attempts := len(writer.attempts)
		persisted := len(writer.persisted)
		writer.mu.Unlock()
		t.Fatalf("expected retained batch to persist after retry; attempts=%d persisted=%d", attempts, persisted)
	}

	writer.mu.Lock()
	attempts := len(writer.attempts)
	writer.mu.Unlock()
	if attempts < 2 {
		t.Fatalf("expected at least 2 upsert attempts (1 fail + retry), got %d", attempts)
	}

	close(raw)
	for range ch {
	}
}

// TestBridgeWatchChannel_ContextCancelSignalsCompletion verifies that
// cancelling the context stops the bridge, closes the forwarded event channel,
// and closes the completion signal even though the input channel never closes.
func TestBridgeWatchChannel_ContextCancelSignalsCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	raw := make(chan library.WatchEvent) // open, never closed
	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}

	ch, done := bridgeWatchChannel(ctx, raw, writer, marker, nil, 500)

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not signal completion after context cancellation")
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected event channel to be closed after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("event channel not closed after cancellation")
	}
}

// TestBridgeWatchChannel_CancelPersistsInFlightChange verifies that a changed
// track already dequeued from raw has its upsert attempted before the bridge
// completes, even when the TUI channel is saturated and the context is
// cancelled while the outward forward is blocked (FR-006).
func TestBridgeWatchChannel_CancelPersistsInFlightChange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	raw := make(chan library.WatchEvent)
	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}

	// A tiny batch size forces an immediate flush once the in-flight track is
	// staged, so a successful persist is observable via the writer.
	ch, done := bridgeWatchChannel(ctx, raw, writer, marker, nil, 1)

	// Saturate the outward channel so the bridge's forward blocks: fill the
	// buffer, then feed the in-flight event that cannot be forwarded.
	for i := 0; i < watchChannelBuffer; i++ {
		raw <- library.WatchEvent{Track: library.Track{Hash: "seed", Path: "/seed.mp3"}}
	}
	raw <- library.WatchEvent{Track: library.Track{Hash: "inflight", Path: "/inflight.mp3"}}

	// The bridge is now blocked forwarding the in-flight event. Cancelling must
	// still persist it before completion.
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not complete after cancellation")
	}
	for range ch {
	}

	found := false
	for _, tr := range writer.allTracks() {
		if tr.Hash == "inflight" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("in-flight changed track was not persisted before completion; got %v", writer.allTracks())
	}
}

// TestBridgeWatchChannel_CancelMarksInFlightDeletion verifies that a deletion
// event already dequeued from raw has its MarkTrackMissing attempted before the
// bridge completes, even when the TUI channel is saturated and the context is
// cancelled while the outward forward is blocked (FR-006).
func TestBridgeWatchChannel_CancelMarksInFlightDeletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	raw := make(chan library.WatchEvent)
	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}

	// Seed the registry so the in-flight deletion resolves to a hash.
	seed := map[string]string{"/gone.mp3": "gone"}
	ch, done := bridgeWatchChannel(ctx, raw, writer, marker, newPathHashRegistry(seed), 500)

	// Saturate the outward channel, then feed the in-flight deletion.
	for i := 0; i < watchChannelBuffer; i++ {
		raw <- library.WatchEvent{Track: library.Track{Hash: "seed", Path: "/seed.mp3"}}
	}
	raw <- library.WatchEvent{Deleted: true, Path: "/gone.mp3"}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not complete after cancellation")
	}
	for range ch {
	}

	marker.mu.Lock()
	hashes := append([]string(nil), marker.hashes...)
	marker.mu.Unlock()
	found := false
	for _, h := range hashes {
		if h == "gone" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("in-flight deletion was not marked missing before completion; got %v", hashes)
	}
}

func waitForCondition(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}
