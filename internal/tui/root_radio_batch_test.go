package tui

import (
	"testing"
)

// TestRadioBatchDiscardedWhenDisabled verifies that a RadioBatchEnqueuedMsg
// arriving after the user has toggled radio off does not resurrect playback by
// appending tracks to a queue the user just cleared/disabled.
func TestRadioBatchDiscardedWhenDisabled(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = false
	m.radio.enqueuePending = true
	beforeLen := len(m.queue.entries)

	msg := RadioBatchEnqueuedMsg{
		Entries: []QueueEntry{
			{ID: 99, Hash: "hx", Path: "/x.flac", Artist: "X", Title: "Y"},
		},
		QueueIDs: []int64{99},
		WasEmpty: true,
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset even when batch is discarded")
	}
	if len(nm.queue.entries) != beforeLen {
		t.Errorf("queue must not grow when radio is disabled; before=%d after=%d",
			beforeLen, len(nm.queue.entries))
	}
	if nm.playback.state == PlaybackPlaying {
		t.Error("playback must not start when radio is disabled")
	}
}

// TestRadioSeedTagsSnapshotted verifies that the radio enqueue closure copies
// the seed tag set instead of holding a live reference to m.trackTags[seedHash].
// We exercise the race by mutating the live map concurrently with the closure
// running: the snapshot must isolate the closure from those mutations.
// Run with `go test -race` for the strongest signal.
func TestRadioSeedTagsSnapshotted(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newModelWithTracks()
	m.batchEnqueuer = be
	m.radio.enabled = true
	m.radio.seedHash = "h1"
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a1.flac"}}
	m.playback.currentIndex = 0
	m.trackTags = map[string]map[int64]struct{}{
		"h1": {7: {}, 8: {}},
		// Populate other tracks so ScoreAll enters the shared-tag inner loop
		// that ranges over Seed.TagIDs for each candidate.
		"h2": {7: {}},
		"h3": {8: {}},
		"h4": {7: {}, 8: {}},
		"h5": {7: {}},
		"h6": {8: {}},
	}

	cmd := m.radioEnqueueCmd()
	if cmd == nil {
		t.Fatal("expected non-nil radio enqueue cmd")
	}

	// Concurrently mutate the live tag set while the closure runs. Without
	// the snapshot the closure's range loop over seed.TagIDs would race
	// with these writes and `go test -race` would flag it.
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				for i := 0; i < 50; i++ {
					m.trackTags["h1"][int64(i)] = struct{}{}
					delete(m.trackTags["h1"], int64(i))
				}
			}
		}
	}()

	// Drive the closure many times so its read window overlaps the mutator.
	for i := 0; i < 50; i++ {
		_ = cmd()
	}
	close(stop)
	<-done
}

// TestRadioBatchDiscardedWhenQueueClearedInFlight verifies that a
// RadioBatchEnqueuedMsg arriving with WasEmpty=false (radio triggered at
// low-water-mark with a non-empty queue) is discarded when the queue is empty
// by the time the batch arrives. Without this guard a Shift+C clear that
// races a radio enqueue would silently re-populate the cleared queue with
// surprise tracks the user did not request.
func TestRadioBatchDiscardedWhenQueueClearedInFlight(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.radio.enqueuePending = true
	// Simulate post-clear state: queue empty, queueExhausted=false (cleared
	// by resetPlaybackState), state Idle.
	m.queue.entries = nil
	m.playback.queueExhausted = false
	m.playback.state = PlaybackIdle

	msg := RadioBatchEnqueuedMsg{
		Entries: []QueueEntry{
			{ID: 50, Hash: "hr", Path: "/r.flac", Artist: "R", Title: "S"},
		},
		QueueIDs: []int64{50},
		WasEmpty: false, // captured when queue still had entries
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset even when batch is discarded")
	}
	if len(nm.queue.entries) != 0 {
		t.Errorf("cleared queue must stay empty; got %d entries", len(nm.queue.entries))
	}
	if nm.playback.state == PlaybackPlaying {
		t.Error("playback must not start when batch is discarded after queue clear")
	}
}

// TestRadioBatchAppliedWhenQueueWasEmptyAtEnqueueStart verifies the
// queue-cleared-in-flight guard does not over-fire: a batch whose closure was
// launched with WasEmpty=true (e.g. after PlaybackEndedMsg with no remaining
// queue) must still apply and auto-start playback on arrival.
func TestRadioBatchAppliedWhenQueueWasEmptyAtEnqueueStart(t *testing.T) {
	p := &stubCommandPlayer{}
	m := newModelWithTracks()
	m.player = p
	m.radio.enabled = true
	m.radio.enqueuePending = true
	m.queue.entries = nil
	m.playback.state = PlaybackIdle

	msg := RadioBatchEnqueuedMsg{
		Entries: []QueueEntry{
			{ID: 60, Hash: "hq", Path: "/q.flac", Artist: "Q", Title: "T"},
		},
		QueueIDs: []int64{60},
		WasEmpty: true, // queue truly was empty when enqueue fired
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset after applying batch")
	}
	if len(nm.queue.entries) != 1 {
		t.Errorf("expected 1 entry appended, got %d", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackPlaying {
		t.Errorf("expected PlaybackPlaying after WasEmpty auto-start, got %v", nm.playback.state)
	}
}

// TestRadioBatchAppliedOnNaturalExhaustion verifies that a RadioBatchEnqueuedMsg
// with WasEmpty=false (launched when the queue still held tracks) is NOT
// discarded just because the queue finished playing in flight. The
// distinguishing signal is playback.queueExhausted, which handlePlaybackEndedMsg
// sets when the last track ends naturally (vs. resetPlaybackState clearing it
// after a user Shift+C clear). Without this, the user is left with an empty
// queue and a dead radio session whenever a batch races a natural exhaustion.
func TestRadioBatchAppliedOnNaturalExhaustion(t *testing.T) {
	p := &stubCommandPlayer{}
	m := newModelWithTracks()
	m.player = p
	m.radio.enabled = true
	m.radio.enqueuePending = true
	// Natural exhaustion path: queue is empty but queueExhausted=true (set by
	// handlePlaybackEndedMsg) so auto-resume should fire when the batch lands.
	m.queue.entries = nil
	m.playback.queueExhausted = true
	m.playback.state = PlaybackIdle

	msg := RadioBatchEnqueuedMsg{
		Entries: []QueueEntry{
			{ID: 70, Hash: "hx", Path: "/x.flac", Artist: "X", Title: "T"},
		},
		QueueIDs: []int64{70},
		WasEmpty: false, // captured at low-water-mark before the last track ended
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset after applying batch")
	}
	if len(nm.queue.entries) != 1 {
		t.Fatalf("expected 1 entry appended on natural exhaustion, got %d", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackPlaying {
		t.Errorf("expected PlaybackPlaying after natural-exhaust auto-resume, got %v", nm.playback.state)
	}
	if nm.playback.queueExhausted {
		t.Error("queueExhausted should be cleared after auto-resume")
	}
}
