package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// TestRootUpdatePlayQueueIndexMsgCallsPlayerPlayFromIndex verifies that root
// handles PlayQueueIndexMsg by calling PlayFromIndex on the player with the
// correct index.
func TestRootUpdatePlayQueueIndexMsgCallsPlayerPlayFromIndex(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	_, cmd := m.Update(PlayQueueIndexMsg{Index: 1})
	if cmd == nil {
		t.Fatal("PlayQueueIndexMsg must dispatch PlayFromIndex asynchronously via a Cmd")
	}
	// Execute the async Cmd to drive the player.
	if result := cmd(); result == nil {
		t.Fatal("async play Cmd must return a playFromIndexResultMsg")
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 1 {
		t.Fatalf("expected PlayFromIndex(1), got calls=%v", p.playFromIndexCalls)
	}
}

// TestRootUpdateEnterInQueueViewDelegatesToQueueUpdate verifies the end-to-end
// flow: pressing Enter while the queue view is active returns a Cmd that emits
// PlayQueueIndexMsg, and a subsequent Update call invokes PlayFromIndex.
func TestRootUpdateEnterInQueueViewDelegatesToQueueUpdate(t *testing.T) {
	stubFileExistsTrue(t)
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 0

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter in queue view must return a non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(PlayQueueIndexMsg); !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
	// Feed the message back to root to trigger playback.
	_, cmd2 := m.Update(msg)
	if cmd2 == nil {
		t.Fatal("PlayQueueIndexMsg must dispatch PlayFromIndex asynchronously via a Cmd")
	}
	cmd2() // execute the async Cmd
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 0 {
		t.Fatalf("expected PlayFromIndex(0), got calls=%v", p.playFromIndexCalls)
	}
}

// TestTrackEnqueuedMsgStartsPlaybackWhenQueueWasEmpty verifies that the root
// model transitions from PlaybackIdle to PlaybackPlaying and calls
// PlayFromIndex(0) when a TrackEnqueuedMsg arrives with WasEmpty=true.
func TestTrackEnqueuedMsgStartsPlaybackWhenQueueWasEmpty(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackIdle
	// Pre-seed one queue entry so the model knows what to display.
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "Alice – Song One", Artist: "Alice", Title: "Song One"},
	}

	track := library.Track{Artist: "Alice", Title: "Song One", Path: "/music/a.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 1, WasEmpty: true}
	next, cmd := m.Update(msg)
	nm := next.(Model)

	if cmd != nil {
		t.Fatalf("TrackEnqueuedMsg must not return a follow-up cmd, got %T", cmd)
	}
	if nm.playback.state != PlaybackPlaying {
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
	m.playback.state = PlaybackIdle

	track := library.Track{Artist: "Bob", Title: "Track Two", Path: "/music/b.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, WasEmpty: false}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if len(p.playFromIndexCalls) != 0 {
		t.Fatalf("expected no PlayFromIndex call when WasEmpty=false, got calls=%v", p.playFromIndexCalls)
	}
	if nm.playback.state != PlaybackIdle {
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

	if nm.statusBar.notification.msg == "" {
		t.Fatal("TrackEnqueuedMsg with error must set a notification")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "db locked") {
		t.Fatalf("notification must contain the error message, got %q", nm.statusBar.notification.msg)
	}
	// Queue must not have grown.
	if len(nm.queue.entries) != 0 {
		t.Fatalf("queue must remain empty on enqueue error, got %d entries", len(nm.queue.entries))
	}
}

// TestClearQueueMsgStopsPlaybackAndTransitionsToIdle verifies that handling
// ClearQueueMsg calls Stop() on the player exactly once and transitions
// playbackState to PlaybackIdle (not PlaybackStopped).
func TestClearQueueMsgStopsPlaybackAndTransitionsToIdle(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/music/a.flac", Display: "A"},
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Fatalf("ClearQueueMsg: expected PlaybackIdle, got %v", nm.playback.state)
	}
	if p.stopCalls != 1 {
		t.Fatalf("ClearQueueMsg: expected 1 Stop() call, got %d", p.stopCalls)
	}
}

// TestClearQueueMsgResetsScrollOffset verifies that ClearQueueMsg resets the
// queue scrollOffset to 0 so that entries added after a clear are visible
// from the first row instead of being off-screen at a stale scroll position.
func TestClearQueueMsgResetsScrollOffset(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.queue.entries = make([]QueueEntry, 50)
	for i := range m.queue.entries {
		m.queue.entries[i] = QueueEntry{ID: int64(i + 1), Display: "Track"}
	}
	m.queue.scrollOffset = 30

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.queue.scrollOffset != 0 {
		t.Fatalf("ClearQueueMsg: expected scrollOffset=0, got %d", nm.queue.scrollOffset)
	}
}

// TestPlayQueueIndexMsgUpdatesCurrentTrackHash verifies that starting playback
// from a specific queue index updates currentTrackHash immediately, so the
// status bar and session persistence use the correct hash without waiting for
// the asynchronous TrackChangedMsg from the event reader.
func TestPlayQueueIndexMsgUpdatesCurrentTrackHash(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.currentHash = "stale-hash"
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "hash-a", Path: "/a.flac", Display: "A", Artist: "A", Title: "One"},
		{ID: 2, Hash: "hash-b", Path: "/b.flac", Display: "B", Artist: "B", Title: "Two"},
	}

	next, _ := m.Update(PlayQueueIndexMsg{Index: 1})
	nm := next.(Model)

	if nm.playback.currentHash != "hash-b" {
		t.Fatalf("PlayQueueIndexMsg: expected currentTrackHash=%q, got %q", "hash-b", nm.playback.currentHash)
	}
}

// TestTrackEnqueuedAutoPlayUpdatesCurrentTrackHash verifies that when a track
// is enqueued into an empty queue and triggers auto-play, currentTrackHash is
// set to the newly enqueued track's hash.
func TestTrackEnqueuedAutoPlayUpdatesCurrentTrackHash(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackIdle
	m.queue.entries = []QueueEntry{
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
	m.playback.state = PlaybackIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.playback.currentHash = ""
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "old-hash", Path: "/music/old.flac", Display: "Old"},
	}

	track := library.Track{Hash: "new-hash", Artist: "New", Title: "Track", Path: "/music/new.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, WasEmpty: false, InsertAfter: -1}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
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
	m.playback.state = PlaybackIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.playback.currentHash = ""
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "old-hash", Path: "/music/old.flac", Display: "Old"},
	}

	entries := []QueueEntry{
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

	if nm.playback.state != PlaybackPlaying {
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
	m.playback.state = PlaybackIdle
	m.playback.queueExhausted = false
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "old-hash", Path: "/music/old.flac", Display: "Old"},
	}

	track := library.Track{Hash: "new-hash", Artist: "New", Title: "Track", Path: "/music/new.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, WasEmpty: false, InsertAfter: -1}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Fatalf("expected PlaybackIdle (no auto-resume), got %v", nm.playback.state)
	}
	if len(p.playFromIndexCalls) != 0 {
		t.Fatalf("expected no PlayFromIndex call, got calls=%v", p.playFromIndexCalls)
	}
}

// TestTrackEnqueuedExhaustedResumeResetsPlaybackFields verifies that when a
// track is enqueued after queue exhaustion and playback resumes, the stale
// elapsed/peak/durationCorrected fields from the previous track are cleared
// so the progress bar and duration correction logic start fresh.
func TestTrackEnqueuedExhaustedResumeResetsPlaybackFields(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.playback.currentHash = ""
	// Simulate stale fields from the previous track that ended.
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.playback.peakElapsedSecs = 100
	m.playback.peakTotalSecs = 200
	m.playback.durationCorrected = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "old-hash", Path: "/music/old.flac", Display: "Old"},
	}

	track := library.Track{Hash: "new-hash", Artist: "New", Title: "Track", Path: "/music/new.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, WasEmpty: false, InsertAfter: -1}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("expected PlaybackPlaying, got %v", nm.playback.state)
	}
	if nm.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0", nm.playback.totalSecs)
	}
	if nm.playback.peakElapsedSecs != 0 {
		t.Errorf("peakElapsedSecs = %v, want 0", nm.playback.peakElapsedSecs)
	}
	if nm.playback.peakTotalSecs != 0 {
		t.Errorf("peakTotalSecs = %v, want 0", nm.playback.peakTotalSecs)
	}
	if nm.playback.durationCorrected {
		t.Error("durationCorrected should be false after exhaustion resume")
	}
}

// TestBatchEnqueuedExhaustedResumeResetsPlaybackFields verifies the same reset
// for batch enqueue after queue exhaustion.
func TestBatchEnqueuedExhaustedResumeResetsPlaybackFields(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.playback.currentHash = ""
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.playback.peakElapsedSecs = 100
	m.playback.peakTotalSecs = 200
	m.playback.durationCorrected = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "old-hash", Path: "/music/old.flac", Display: "Old"},
	}

	entries := []QueueEntry{
		{ID: 10, Hash: "batch-a", Path: "/a.flac", Display: "A", Artist: "A", Title: "Song A"},
	}
	msg := BatchEnqueuedMsg{
		Entries:      entries,
		QueueIDs:     []int64{10},
		WasEmpty:     false,
		PlaylistName: "Test",
	}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("expected PlaybackPlaying, got %v", nm.playback.state)
	}
	if nm.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0", nm.playback.totalSecs)
	}
	if nm.playback.peakElapsedSecs != 0 {
		t.Errorf("peakElapsedSecs = %v, want 0", nm.playback.peakElapsedSecs)
	}
	if nm.playback.peakTotalSecs != 0 {
		t.Errorf("peakTotalSecs = %v, want 0", nm.playback.peakTotalSecs)
	}
	if nm.playback.durationCorrected {
		t.Error("durationCorrected should be false after exhaustion resume")
	}
}

// TestBatchEnqueuedAutoPlayUpdatesCurrentTrackHash verifies that when a batch
// enqueue into an empty queue triggers auto-play, currentTrackHash is set to
// the first entry's hash.
func TestBatchEnqueuedAutoPlayUpdatesCurrentTrackHash(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackIdle

	entries := []QueueEntry{
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
