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
// playback from index 0 instead of transitioning to idle.
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

	next, cmd := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("RepeatAll + PlaybackEndedMsg: expected PlaybackPlaying, got %v", nm.playback.state)
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 0 {
		t.Fatalf("RepeatAll + PlaybackEndedMsg: expected PlayFromIndex(0), got calls=%v", p.playFromIndexCalls)
	}
	if nm.statusBar.artist != "A" || nm.statusBar.title != "One" {
		t.Fatalf("RepeatAll + PlaybackEndedMsg: status bar should show first track, got artist=%q title=%q", nm.statusBar.artist, nm.statusBar.title)
	}
	if cmd == nil {
		t.Fatal("RepeatAll + PlaybackEndedMsg: expected a session-position cmd")
	}
}

// TestPlaybackEndedMsgRepeatAllUpdatesCurrentTrackHash verifies that when
// RepeatAll wraps back to track 0, currentTrackHash is updated to the first
// track's hash so that the session-position write persists the correct hash.
func TestPlaybackEndedMsgRepeatAllUpdatesCurrentTrackHash(t *testing.T) {
	p := &stubPlayer{}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatAll
	m.playback.currentHash = "old-hash-last-track"
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "hash-first", Path: "/a.flac", Display: "A", Artist: "A", Title: "One"},
		{ID: 2, Hash: "hash-second", Path: "/b.flac", Display: "B", Artist: "B", Title: "Two"},
	}

	next, cmd := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.currentHash != "hash-first" {
		t.Fatalf("RepeatAll wrap: expected currentTrackHash=%q, got %q", "hash-first", nm.playback.currentHash)
	}
	// Execute the session-position cmd and verify the hash it persists.
	if cmd == nil {
		t.Fatal("RepeatAll wrap: expected a session-position cmd")
	}
	cmd()
	if len(sw.positionCalls) != 1 {
		t.Fatalf("expected 1 position call, got %d", len(sw.positionCalls))
	}
	if sw.positionCalls[0].hash != "hash-first" {
		t.Fatalf("session position persisted wrong hash: got %q, want %q", sw.positionCalls[0].hash, "hash-first")
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

// TestPlaybackEndedMsgRepeatAllPreservesSourcePlaylist verifies that when
// RepeatAll wraps to the first track, the status bar uses the track's
// SourcePlaylist (e.g. "My Playlist") instead of hard-coding "Queue".
func TestPlaybackEndedMsgRepeatAllPreservesSourcePlaylist(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatAll
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Display: "A – One", Artist: "A", Title: "One", SourcePlaylist: "My Playlist"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Display: "B – Two", Artist: "B", Title: "Two", SourcePlaylist: "My Playlist"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.statusBar.playlistSource != "My Playlist" {
		t.Fatalf("RepeatAll wrap: expected playlistSource=%q, got %q", "My Playlist", nm.statusBar.playlistSource)
	}
}

// TestPlaybackEndedMsgRepeatAllFallsBackToQueue verifies that when
// RepeatAll wraps to a track with no SourcePlaylist, the status bar shows "Queue".
func TestPlaybackEndedMsgRepeatAllFallsBackToQueue(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatAll
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Display: "A – One", Artist: "A", Title: "One"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.statusBar.playlistSource != "Queue" {
		t.Fatalf("RepeatAll wrap (no source): expected playlistSource=%q, got %q", "Queue", nm.statusBar.playlistSource)
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
