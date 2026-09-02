package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestTrackEnqueuedMsgStartsPlaybackWhenQueueWasEmpty verifies that the root
// model transitions from PlaybackIdle to PlaybackPlaying and calls
// PlayFromIndex(0) when a TrackEnqueuedMsg arrives with WasEmpty=true.
func TestTrackEnqueuedMsgStartsPlaybackWhenQueueWasEmpty(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle
	// Pre-seed one queue entry so the model knows what to display.
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Display: "Alice - Song One", Artist: "Alice", Title: "Song One"},
	}

	track := library.Track{Artist: "Alice", Title: "Song One", Path: "/music/a.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 1, WasEmpty: true}
	next, cmd := m.Update(msg)
	nm := next.(Model)

	if cmd != nil {
		t.Fatalf("TrackEnqueuedMsg must not return a follow-up cmd, got %T", cmd)
	}
	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("expected playbackState PlaybackPlaying, got %v", nm.playback.state)
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 0 {
		t.Fatalf("expected PlayFromIndex(0), got calls=%v", p.playFromIndexCalls)
	}
}

// TestTrackEnqueuedMsgDoesNotStartPlaybackWhenQueueWasNonEmpty verifies that
// the root model does NOT call PlayFromIndex when WasEmpty is false, leaving
// current playback uninterrupted.
func TestTrackEnqueuedMsgDoesNotStartPlaybackWhenQueueWasNonEmpty(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle

	track := library.Track{Artist: "Bob", Title: "Track Two", Path: "/music/b.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, WasEmpty: false}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if len(p.playFromIndexCalls) != 0 {
		t.Fatalf("expected no PlayFromIndex call when WasEmpty=false, got calls=%v", p.playFromIndexCalls)
	}
	if nm.playback.state != playback.StateIdle {
		t.Fatalf("expected playbackState PlaybackIdle (unchanged), got %v", nm.playback.state)
	}
}

// TestTrackEnqueuedMsgWithErrorShowsNotification verifies that a
// TrackEnqueuedMsg with a non-nil Err sets a transient error notification.
func TestTrackEnqueuedMsgWithErrorShowsNotification(t *testing.T) {
	m := New(context.Background())

	msg := libtui.TrackEnqueuedMsg{
		Track: library.Track{Artist: "X", Title: "Y", Path: "/music/x.flac"},
		Err:   &errPlaylistCreate{msg: "db locked"},
	}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("TrackEnqueuedMsg with error must set a notification")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "db locked") {
		t.Fatalf("notification must contain the error message, got %q", nm.statusBar.TopNotification().Msg)
	}
	// Queue must not have grown.
	if len(nm.queue.Entries) != 0 {
		t.Fatalf("queue must remain empty on enqueue error, got %d entries", len(nm.queue.Entries))
	}
}

// TestTrackEnqueuedAutoPlayUpdatesCurrentTrackHash verifies that when a track
// is enqueued into an empty queue and triggers auto-play, currentTrackHash is
// set to the newly enqueued track's hash.
func TestTrackEnqueuedAutoPlayUpdatesCurrentTrackHash(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "hash-x", Path: "/x.flac", Display: "X", Artist: "X", Title: "Song"},
	}

	track := library.Track{Hash: "hash-x", Artist: "X", Title: "Song", Path: "/x.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 1, WasEmpty: true}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.currentHash != "hash-x" {
		t.Fatalf("auto-play enqueue: expected currentTrackHash=%q, got %q", "hash-x", nm.playback.currentHash)
	}
}

// TestTrackEnqueuedResumesPlaybackOnQueueExhausted verifies that when the
// queue has been exhausted (finished playing all tracks) and a new track is
// enqueued, playback auto-resumes from the new track even though WasEmpty is
// false (old entries are still present).
func TestTrackEnqueuedResumesPlaybackOnQueueExhausted(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	// Simulate exhausted queue: playback stopped, old entries remain.
	m.playback.state = playback.StateIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.playback.currentHash = ""
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "old-hash", Path: "/music/old.flac", Display: "Old"},
	}

	track := library.Track{Hash: "new-hash", Artist: "New", Title: "Track", Path: "/music/new.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, WasEmpty: false, InsertAfter: -1}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("expected PlaybackPlaying after exhausted-queue enqueue, got %v", nm.playback.state)
	}
	if nm.playback.queueExhausted {
		t.Fatal("queueExhausted should be cleared after auto-resume")
	}
	// Should play from the newly enqueued track's index (appended at end).
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 1 {
		t.Fatalf("expected PlayFromIndex(1), got calls=%v", p.playFromIndexCalls)
	}
	if nm.playback.currentHash != "new-hash" {
		t.Fatalf("expected currentHash=%q, got %q", "new-hash", nm.playback.currentHash)
	}
}

// TestBatchEnqueuedResumesPlaybackOnQueueExhausted verifies that when the
// queue has been exhausted and a batch of tracks is enqueued, playback
// auto-resumes from the first new track.
func TestBatchEnqueuedResumesPlaybackOnQueueExhausted(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	// Simulate exhausted queue.
	m.playback.state = playback.StateIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.playback.currentHash = ""
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "old-hash", Path: "/music/old.flac", Display: "Old"},
	}

	entries := []queue.Entry{
		{ID: 10, Hash: "batch-a", Path: "/a.flac", Display: "A", Artist: "A", Title: "Song A"},
		{ID: 11, Hash: "batch-b", Path: "/b.flac", Display: "B", Artist: "B", Title: "Song B"},
	}
	msg := BatchEnqueuedMsg{
		Entries:      entries,
		QueueIDs:     []int64{10, 11},
		WasEmpty:     false,
		PlaylistName: "Test",
	}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("expected PlaybackPlaying after exhausted-queue batch enqueue, got %v", nm.playback.state)
	}
	if nm.playback.queueExhausted {
		t.Fatal("queueExhausted should be cleared after auto-resume")
	}
	// Should play from index 1 (first newly appended entry).
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 1 {
		t.Fatalf("expected PlayFromIndex(1), got calls=%v", p.playFromIndexCalls)
	}
	if nm.playback.currentHash != "batch-a" {
		t.Fatalf("expected currentHash=%q, got %q", "batch-a", nm.playback.currentHash)
	}
}

// TestTrackEnqueuedDoesNotResumeWhenNotExhausted verifies that a regular
// enqueue into a non-empty, non-exhausted queue does not trigger auto-play.
func TestTrackEnqueuedDoesNotResumeWhenNotExhausted(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle
	m.playback.queueExhausted = false
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "old-hash", Path: "/music/old.flac", Display: "Old"},
	}

	track := library.Track{Hash: "new-hash", Artist: "New", Title: "Track", Path: "/music/new.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, WasEmpty: false, InsertAfter: -1}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Fatalf("expected PlaybackIdle (no auto-resume), got %v", nm.playback.state)
	}
	if len(p.playFromIndexCalls) != 0 {
		t.Fatalf("expected no PlayFromIndex call, got calls=%v", p.playFromIndexCalls)
	}
}

// TestBatchEnqueuedAutoPlayUpdatesCurrentTrackHash verifies that when a batch
// enqueue into an empty queue triggers auto-play, currentTrackHash is set to
// the first entry's hash.
func TestBatchEnqueuedAutoPlayUpdatesCurrentTrackHash(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle

	entries := []queue.Entry{
		{ID: 10, Hash: "hash-p", Path: "/p.flac", Display: "P", Artist: "P", Title: "Song"},
		{ID: 11, Hash: "hash-q", Path: "/q.flac", Display: "Q", Artist: "Q", Title: "Song"},
	}
	msg := BatchEnqueuedMsg{
		Entries:      entries,
		QueueIDs:     []int64{10, 11},
		WasEmpty:     true,
		PlaylistName: "Test",
	}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.currentHash != "hash-p" {
		t.Fatalf("batch auto-play: expected currentTrackHash=%q, got %q", "hash-p", nm.playback.currentHash)
	}
}

// TestClearQueueThenBatchEnqueuedRebuildsPlayerQueueAndStartsPlayback proves
// the clear-then-playlist enqueue flow rebuilds the player queue and starts the
// freshly enqueued batch instead of only updating TUI state.
func TestClearQueueThenBatchEnqueuedRebuildsPlayerQueueAndStartsPlayback(t *testing.T) {
	p := &stubQueueLifecyclePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 0
	m.playback.currentHash = "old-hash"
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "old-hash", Path: "/old.flac", Display: "Old", Artist: "Old", Title: "Track"},
	}

	next, cmd := m.Update(queue.ClearQueueMsg{})
	if cmd != nil {
		t.Fatalf("ClearQueueMsg must not return a follow-up cmd, got %T", cmd)
	}
	cleared := next.(Model)
	if len(cleared.queue.Entries) != 0 {
		t.Fatalf("ClearQueueMsg: expected empty TUI queue, got %d entries", len(cleared.queue.Entries))
	}
	if p.stopCalls != 1 {
		t.Fatalf("ClearQueueMsg: expected 1 Stop() call, got %d", p.stopCalls)
	}
	if len(p.setQueueCalls) != 1 {
		t.Fatalf("ClearQueueMsg: expected one SetQueue call, got %d", len(p.setQueueCalls))
	}
	if len(p.setQueueCalls[0]) != 0 {
		t.Fatalf("ClearQueueMsg: expected player queue clear, got %+v", p.setQueueCalls[0])
	}

	entries := []queue.Entry{
		{ID: 10, Hash: "new-a", Path: "/new-a.flac", Display: "New A", Artist: "Artist A", Title: "Title A", SourcePlaylist: "Batch"},
		{ID: 11, Hash: "new-b", Path: "/new-b.flac", Display: "New B", Artist: "Artist B", Title: "Title B", SourcePlaylist: "Batch"},
	}
	next, cmd = cleared.Update(BatchEnqueuedMsg{
		Entries:      entries,
		QueueIDs:     []int64{10, 11},
		WasEmpty:     true,
		PlaylistName: "Batch",
	})
	if cmd != nil {
		t.Fatalf("BatchEnqueuedMsg must not return a follow-up cmd, got %T", cmd)
	}
	nm := next.(Model)

	if len(nm.queue.Entries) != len(entries) {
		t.Fatalf("BatchEnqueuedMsg: expected %d queue entries, got %d", len(entries), len(nm.queue.Entries))
	}
	if len(p.appendToQueueCalls) != len(entries) {
		t.Fatalf("BatchEnqueuedMsg: expected %d AppendToQueue calls, got %d", len(entries), len(p.appendToQueueCalls))
	}
	for i, want := range entries {
		if got := nm.queue.Entries[i]; got != want {
			t.Fatalf("BatchEnqueuedMsg: entry %d = %+v, want %+v", i, got, want)
		}
		if got, wantTrack := p.appendToQueueCalls[i], queueTrackFromEntry(want); got != wantTrack {
			t.Fatalf("AppendToQueue call %d = %+v, want %+v", i, got, wantTrack)
		}
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 0 {
		t.Fatalf("BatchEnqueuedMsg: expected PlayFromIndex(0), got calls=%v", p.playFromIndexCalls)
	}
	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("BatchEnqueuedMsg: expected PlaybackPlaying, got %v", nm.playback.state)
	}
	if nm.playback.currentHash != entries[0].Hash {
		t.Fatalf("BatchEnqueuedMsg: expected currentHash=%q, got %q", entries[0].Hash, nm.playback.currentHash)
	}
	if nm.playback.currentIndex != 0 {
		t.Fatalf("BatchEnqueuedMsg: expected currentIndex=0, got %d", nm.playback.currentIndex)
	}

	wantCalls := []string{"Stop", "SetQueue", "AppendToQueue", "AppendToQueue", "PlayFromIndex"}
	if len(p.calls) != len(wantCalls) {
		t.Fatalf("player calls = %v, want %v", p.calls, wantCalls)
	}
	for i, want := range wantCalls {
		if p.calls[i] != want {
			t.Fatalf("player call %d = %q, want %q (all calls: %v)", i, p.calls[i], want, p.calls)
		}
	}
}
