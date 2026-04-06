package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	if cmd != nil {
		t.Fatal("PlayQueueIndexMsg must not return a follow-up cmd")
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 1 {
		t.Fatalf("expected PlayFromIndex(1), got calls=%v", p.playFromIndexCalls)
	}
}

// TestRootUpdateEnterInQueueViewDelegatesToQueueUpdate verifies the end-to-end
// flow: pressing Enter while the queue view is active returns a Cmd that emits
// PlayQueueIndexMsg, and a subsequent Update call invokes PlayFromIndex.
func TestRootUpdateEnterInQueueViewDelegatesToQueueUpdate(t *testing.T) {
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
	if cmd2 != nil {
		t.Fatal("PlayQueueIndexMsg must not return a follow-up cmd")
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 0 {
		t.Fatalf("expected PlayFromIndex(0), got calls=%v", p.playFromIndexCalls)
	}
}

// TestSpaceTogglesPlayPauseFromAnyView verifies that pressing space triggers
// a play or pause call on the player regardless of which view is focused.
func TestSpaceTogglesPlayPauseFromAnyView(t *testing.T) {
	views := []struct {
		name   string
		viewID viewID
	}{
		{"Library", viewLibrary},
		{"Playlists", viewPlaylists},
		{"Queue", viewQueue},
	}

	spaceMsg := tea.KeyPressMsg{Code: tea.KeySpace}

	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			p := &stubPlayer{}
			m := New(context.Background(), WithPlayer(p))
			m.activeView = tc.viewID
			m.queue.entries = []QueueEntry{{ID: 1, Display: "Track 1"}}

			// First press: not playing → Play() called.
			next, _ := m.Update(spaceMsg)
			nm := next.(Model)
			if p.playCalls != 1 || p.pauseCalls != 0 {
				t.Fatalf("first space (%s): expected 1 Play call, got play=%d pause=%d", tc.name, p.playCalls, p.pauseCalls)
			}

			// Second press: playing → Pause() called.
			_, _ = nm.Update(spaceMsg)
			if p.playCalls != 1 || p.pauseCalls != 1 {
				t.Fatalf("second space (%s): expected 1 Pause call, got play=%d pause=%d", tc.name, p.playCalls, p.pauseCalls)
			}
		})
	}
}

// TestModelPlaybackStateTransitions verifies that the playbackState field on
// root Model transitions correctly through playing, paused, and idle states.
func TestModelPlaybackStateTransitions(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{Path: "/a.flac", Display: "Artist – Song", Artist: "Artist", Title: "Song"},
	}

	if m.playback.state != PlaybackIdle {
		t.Fatalf("initial: expected PlaybackIdle, got %v", m.playback.state)
	}

	// PlayQueueIndexMsg → playing.
	next, _ := m.Update(PlayQueueIndexMsg{Index: 0})
	nm := next.(Model)
	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("after PlayQueueIndexMsg: expected PlaybackPlaying, got %v", nm.playback.state)
	}

	// Space while playing → paused.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm = next.(Model)
	if nm.playback.state != PlaybackPaused {
		t.Fatalf("space (playing→paused): expected PlaybackPaused, got %v", nm.playback.state)
	}

	// Space while paused → playing.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm = next.(Model)
	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("space (paused→playing): expected PlaybackPlaying, got %v", nm.playback.state)
	}

	// PlaybackEndedMsg → idle.
	next, _ = nm.Update(PlaybackEndedMsg{})
	nm = next.(Model)
	if nm.playback.state != PlaybackIdle {
		t.Fatalf("after PlaybackEndedMsg: expected PlaybackIdle, got %v", nm.playback.state)
	}
}

// TestModelStopKeyTransitionsToStopped verifies that the s key transitions
// the playback state to stopped when the player implements commandPlayer.
func TestModelStopKeyTransitionsToStopped(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying

	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	nm := next.(Model)
	if nm.playback.state != PlaybackStopped {
		t.Fatalf("s key: expected PlaybackStopped, got %v", nm.playback.state)
	}
	if p.stopCalls != 1 {
		t.Fatalf("s key: expected 1 Stop() call, got %d", p.stopCalls)
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

// TestPlaybackEndedMsgRepeatAllRestartsQueue verifies that when repeat mode is
// RepeatAll and the queue is non-empty, receiving PlaybackEndedMsg restarts
// playback from index 0 instead of transitioning to idle. Metadata is NOT
// speculatively set from entries[0] because PlayFromIndex may skip missing
// entries; the incoming TrackChangedMsg provides the correct metadata.
func TestPlaybackEndedMsgRepeatAllRestartsQueue(t *testing.T) {
	p := &stubPlayer{}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatAll
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Display: "A – One", Artist: "A", Title: "One"},
		{ID: 2, Path: "/b.flac", Display: "B – Two", Artist: "B", Title: "Two"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("RepeatAll + PlaybackEndedMsg: expected PlaybackPlaying, got %v", nm.playback.state)
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 0 {
		t.Fatalf("RepeatAll + PlaybackEndedMsg: expected PlayFromIndex(0), got calls=%v", p.playFromIndexCalls)
	}
	if nm.playback.elapsedSecs != 0 || nm.playback.totalSecs != 0 {
		t.Fatalf("RepeatAll + PlaybackEndedMsg: expected elapsed/total reset to 0")
	}
}

// TestPlaybackEndedMsgRepeatAllDoesNotSpekulativelySetHash verifies that when
// RepeatAll wraps back, currentTrackHash is NOT updated from entries[0]
// because PlayFromIndex may skip missing entries. The TrackChangedMsg from
// the event reader will set the correct hash once mpv starts the track.
func TestPlaybackEndedMsgRepeatAllDoesNotSpekulativelySetHash(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatAll
	m.playback.currentHash = "old-hash-last-track"
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "hash-first", Path: "/a.flac", Display: "A", Artist: "A", Title: "One"},
		{ID: 2, Hash: "hash-second", Path: "/b.flac", Display: "B", Artist: "B", Title: "Two"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	// currentHash should NOT be speculatively updated to entries[0].Hash,
	// because PlayFromIndex(0) may have skipped missing entries and
	// actually started a later track.
	if nm.playback.currentHash != "old-hash-last-track" {
		t.Fatalf("RepeatAll wrap: expected currentTrackHash to remain %q, got %q", "old-hash-last-track", nm.playback.currentHash)
	}
	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("RepeatAll wrap: expected PlaybackPlaying, got %v", nm.playback.state)
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

// TestPlaybackEndedMsgRepeatAllDoesNotSpekulativelySetSource verifies that
// when RepeatAll wraps, the status bar source is NOT speculatively updated
// from entries[0] because PlayFromIndex may skip missing entries. The
// TrackChangedMsg from the event reader will set the correct source.
func TestPlaybackEndedMsgRepeatAllDoesNotSpekulativelySetSource(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatAll
	m.statusBar.playlistSource = "Old Source"
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Display: "A – One", Artist: "A", Title: "One", SourcePlaylist: "My Playlist"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Display: "B – Two", Artist: "B", Title: "Two", SourcePlaylist: "My Playlist"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	// Status bar source should remain unchanged; TrackChangedMsg will update it.
	if nm.statusBar.playlistSource != "Old Source" {
		t.Fatalf("RepeatAll wrap: expected playlistSource to remain %q, got %q", "Old Source", nm.statusBar.playlistSource)
	}
}

// TestPlaybackEndedMsgRepeatAllKeepsOldSource verifies that when
// RepeatAll wraps, the status bar keeps its existing source until
// TrackChangedMsg arrives with the correct metadata.
func TestPlaybackEndedMsgRepeatAllKeepsOldSource(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatAll
	m.statusBar.playlistSource = "Queue"
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Display: "A – One", Artist: "A", Title: "One"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	// Source should remain unchanged; TrackChangedMsg will update it.
	if nm.statusBar.playlistSource != "Queue" {
		t.Fatalf("RepeatAll wrap: expected playlistSource to remain %q, got %q", "Queue", nm.statusBar.playlistSource)
	}
}

// TestPlaybackEndedMsgRepeatAllEmptyQueueGoesIdle verifies that RepeatAll with
// an empty queue still transitions to idle (no infinite restart loop).
func TestPlaybackEndedMsgRepeatAllEmptyQueueGoesIdle(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatAll
	m.queue.entries = nil

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Fatalf("RepeatAll + empty queue: expected PlaybackIdle, got %v", nm.playback.state)
	}
	if len(p.playFromIndexCalls) != 0 {
		t.Fatalf("RepeatAll + empty queue: expected no PlayFromIndex calls, got %v", p.playFromIndexCalls)
	}
}

// TestPlaybackEndedMsgRepeatOffShowsNotification verifies that RepeatOff still
// shows the "Queue finished" notification (regression guard for the RepeatAll fix).
func TestPlaybackEndedMsgRepeatOffShowsNotification(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatOff

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Fatalf("RepeatOff + PlaybackEndedMsg: expected PlaybackIdle, got %v", nm.playback.state)
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Queue finished") {
		t.Fatalf("RepeatOff + PlaybackEndedMsg: expected 'Queue finished' notification, got %q", nm.statusBar.notification.msg)
	}
}

// TestTickMsgClearsBulkRemovePendingAfterWindow verifies that the tick handler
// clears the bulkRemovePending flag once the confirmation window has elapsed,
// consistent with how clearQueuePending is cleared on tick.
func TestTickMsgClearsBulkRemovePendingAfterWindow(t *testing.T) {
	m := New(context.Background())
	// Simulate pressing the bulk-remove key once — set the flag and a past timestamp.
	m.undo.bulkRemovePending = true
	m.undo.bulkRemovePendingAt = time.Now().Add(-5 * time.Second) // well past the 2s window

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.undo.bulkRemovePending {
		t.Fatal("expected bulkRemovePending=false after tick past confirmation window")
	}
}

// TestTickMsgKeepsBulkRemovePendingWithinWindow verifies that the tick handler
// does NOT clear bulkRemovePending while the confirmation window is still open.
func TestTickMsgKeepsBulkRemovePendingWithinWindow(t *testing.T) {
	m := New(context.Background())
	m.undo.bulkRemovePending = true
	m.undo.bulkRemovePendingAt = time.Now() // just set, well within 2s window

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if !nm.undo.bulkRemovePending {
		t.Fatal("expected bulkRemovePending=true while within confirmation window")
	}
}

// TestRepeatAll_ResetsMissingTrackCount verifies that when repeat-all wraps
// back to the beginning of the queue, the missingTrackCount is reset so it
// does not accumulate across repeat cycles.
func TestRepeatAll_ResetsMissingTrackCount(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.repeatMode = RepeatAll
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"
	m.playback.elapsedSecs = 60
	m.playback.totalSecs = 120
	m.missingTrackCount = 3 // simulate 3 missing files in this cycle
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "B", Title: "T2"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("expected PlaybackPlaying after repeat-all wrap, got %v", nm.playback.state)
	}
	if nm.missingTrackCount != 0 {
		t.Errorf("missingTrackCount = %d after repeat-all wrap, want 0", nm.missingTrackCount)
	}
}

// TestPlaybackEndedReturnsIncrementCmd verifies that handlePlaybackEndedMsg
// returns a tea.Cmd for persisting the play count instead of using a bare
// goroutine. This ensures the DB write is managed by the Bubble Tea runtime.
func TestPlaybackEndedReturnsIncrementCmd(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := New(context.Background(), WithHistoryRecorder(hr))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"
	m.playback.elapsedSecs = 60
	m.playback.totalSecs = 120

	next, cmd := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Fatalf("expected PlaybackIdle, got %v", nm.playback.state)
	}
	if cmd == nil {
		t.Fatal("handlePlaybackEndedMsg must return a non-nil cmd for play count persistence")
	}
	// Execute the cmd to trigger RecordPlay.
	cmd()
	if len(hr.calls) != 1 {
		t.Fatalf("expected 1 RecordPlay call, got %d", len(hr.calls))
	}
	if hr.calls[0].hash != "h1" {
		t.Errorf("RecordPlay hash = %q, want %q", hr.calls[0].hash, "h1")
	}
}

// TestPlaybackEndedNoCmdWhenBelowThreshold verifies that no increment command
// is returned when the listen duration is below the threshold.
func TestPlaybackEndedNoCmdWhenBelowThreshold(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := New(context.Background(), WithHistoryRecorder(hr))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"
	m.playback.elapsedSecs = 5
	m.playback.totalSecs = 300 // 5 out of 300 = 1.7%, well below 50% and < 30s

	next, cmd := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Fatalf("expected PlaybackIdle, got %v", nm.playback.state)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when listen duration is below threshold")
	}
}

// TestDismissPreviewDoesNotResumeOnPlayError verifies that when the player's
// Play() returns an error after preview dismissal, the playback state stays
// paused instead of incorrectly transitioning to PlaybackPlaying.
func TestDismissPreviewDoesNotResumeOnPlayError(t *testing.T) {
	p := &stubFailingPlayer{playErr: errors.New("mpv disconnected")}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPaused
	m.overlay.previewWasPlaying = true
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}

	nm := m.dismissPreview()

	if nm.playback.state != PlaybackPaused {
		t.Fatalf("expected PlaybackPaused when resume fails, got %v", nm.playback.state)
	}
	if nm.overlay.preview != nil {
		t.Fatal("preview overlay should be cleared after dismiss")
	}
}

// TestVolumeUpNoUpdateOnError verifies that the TUI volume and session are not
// updated when the mpv SetVolumeRelative IPC call fails, preventing drift
// between the displayed volume and mpv's actual volume.
func TestVolumeUpNoUpdateOnError(t *testing.T) {
	p := &stubFailingVolumePlayer{volumeErr: errors.New("mpv disconnected")}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))
	m.playback.volume = 50

	next, _, _ := m.handlePlaybackKeyAction(actionVolumeUp)
	nm := next.(Model)

	if nm.playback.volume != 50 {
		t.Fatalf("volume should stay at 50 on error, got %d", nm.playback.volume)
	}
	if len(sw.volumeCalls) != 0 {
		t.Fatalf("session volume should not be written on error, got %d calls", len(sw.volumeCalls))
	}
}

// TestVolumeDownNoUpdateOnError verifies that a failed SetVolumeRelative for
// volume-down does not update the TUI volume or persist to the session.
func TestVolumeDownNoUpdateOnError(t *testing.T) {
	p := &stubFailingVolumePlayer{volumeErr: errors.New("mpv disconnected")}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))
	m.playback.volume = 50

	next, _, _ := m.handlePlaybackKeyAction(actionVolumeDown)
	nm := next.(Model)

	if nm.playback.volume != 50 {
		t.Fatalf("volume should stay at 50 on error, got %d", nm.playback.volume)
	}
	if len(sw.volumeCalls) != 0 {
		t.Fatalf("session volume should not be written on error, got %d calls", len(sw.volumeCalls))
	}
}

// TestMainPlaybackTickCorrectsDuration verifies that when mpv reports a
// duration that significantly differs from the stored value, the library
// index, queue entries, and DB are corrected.
func TestMainPlaybackTickCorrectsDuration(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 10.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 360.0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 360000},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, cmd := m.handleTickMsg()

	// Library index should be corrected to mpv value.
	if t2, ok := nm.library.index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
		t.Fatalf("expected library duration 180.0, got %v", t2.Duration)
	}
	// Queue entry should be corrected.
	if nm.queue.entries[0].DurationMs != 180000 {
		t.Fatalf("expected queue DurationMs 180000, got %d", nm.queue.entries[0].DurationMs)
	}
	// Flag should be set.
	if !nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected to be set")
	}
	// DB write should be scheduled.
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("expected BatchMsg")
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		msg := make(chan tea.Msg, 1)
		go func(fn tea.Cmd) { msg <- fn() }(c)
		select {
		case <-msg:
		case <-time.After(50 * time.Millisecond):
		}
	}
	if len(dw.calls) != 1 {
		t.Fatalf("expected 1 DB write call, got %d", len(dw.calls))
	}
	if dw.calls[0].hash != "h1" || dw.calls[0].durationMs != 180000 {
		t.Fatalf("unexpected DB write: %+v", dw.calls[0])
	}
}

// TestMainPlaybackTickSkipsSmallDifference verifies that when the stored
// duration is within 2 seconds of mpv's total, no correction occurs.
func TestMainPlaybackTickSkipsSmallDifference(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 10.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 180.5,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 180500},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, _ := m.handleTickMsg()

	// Library index should NOT be changed.
	if t2, ok := nm.library.index.FindByHash("h1"); !ok || t2.Duration != 180.5 {
		t.Fatalf("expected library duration to stay 180.5, got %v", t2.Duration)
	}
	if nm.queue.entries[0].DurationMs != 180500 {
		t.Fatalf("expected queue DurationMs to stay 180500, got %d", nm.queue.entries[0].DurationMs)
	}
	if !nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected to be set (to prevent re-checking)")
	}
	if len(dw.calls) != 0 {
		t.Fatalf("expected 0 DB write calls, got %d", len(dw.calls))
	}
}

// TestMainPlaybackTickFillsZeroDuration verifies that when a track has
// Duration=0, the main playback tick fills it from mpv.
func TestMainPlaybackTickFillsZeroDuration(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 5.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 0},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, _ := m.handleTickMsg()

	if t2, ok := nm.library.index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
		t.Fatalf("expected library duration 180.0, got %v", t2.Duration)
	}
	if nm.queue.entries[0].DurationMs != 180000 {
		t.Fatalf("expected queue DurationMs 180000, got %d", nm.queue.entries[0].DurationMs)
	}
}

// TestMainPlaybackTickCorrectOnceOnly verifies that after the flag is set,
// subsequent ticks do not trigger additional corrections.
func TestMainPlaybackTickCorrectOnceOnly(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 10.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 360.0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 360000},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	// First tick corrects.
	nm, _ := m.handleTickMsg()
	if !nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected after first tick")
	}

	// Change the stub to report a different total.
	sp.total = 999.0
	nm2, _ := nm.handleTickMsg()

	// Duration should remain at the first correction (180), not 999.
	if t2, ok := nm2.library.index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
		t.Fatalf("expected duration to remain 180.0, got %v", t2.Duration)
	}
}

// TestTrackChangedResetsCorrectFlag verifies that the durationCorrected flag
// is reset when a new track starts.
func TestTrackChangedResetsCorrectFlag(t *testing.T) {
	m := New(context.Background())
	m.playback.durationCorrected = true

	nm, _ := m.handleTrackChangedMsg(TrackChangedMsg{
		Hash: "h2", Index: 0, Artist: "B", Title: "T2",
	})

	if nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected to be reset after track change")
	}
}
