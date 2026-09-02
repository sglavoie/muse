package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestPlaybackEndedMsgRepeatAllRestartsQueue verifies that when repeat mode is
// RepeatAll and the queue is non-empty, receiving PlaybackEndedMsg restarts
// playback from index 0 instead of transitioning to idle. Metadata is NOT
// speculatively set from entries[0] because PlayFromIndex may skip missing
// entries; the incoming TrackChangedMsg provides the correct metadata.
func TestPlaybackEndedMsgRepeatAllRestartsQueue(t *testing.T) {
	p := &stubPlayer{}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))
	m.playback.state = playback.StatePlaying
	m.playback.repeatMode = playback.RepeatAll
	m.queue.Entries = []queue.Entry{
		{ID: 1, Path: "/a.flac", Display: "A – One", Artist: "A", Title: "One"},
		{ID: 2, Path: "/b.flac", Display: "B – Two", Artist: "B", Title: "Two"},
	}

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != playback.StatePlaying {
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
	m.playback.state = playback.StatePlaying
	m.playback.repeatMode = playback.RepeatAll
	m.playback.currentHash = "old-hash-last-track"
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "hash-first", Path: "/a.flac", Display: "A", Artist: "A", Title: "One"},
		{ID: 2, Hash: "hash-second", Path: "/b.flac", Display: "B", Artist: "B", Title: "Two"},
	}

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	// currentHash should NOT be speculatively updated to entries[0].Hash,
	// because PlayFromIndex(0) may have skipped missing entries and
	// actually started a later track.
	if nm.playback.currentHash != "old-hash-last-track" {
		t.Fatalf("RepeatAll wrap: expected currentTrackHash to remain %q, got %q", "old-hash-last-track", nm.playback.currentHash)
	}
	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("RepeatAll wrap: expected PlaybackPlaying, got %v", nm.playback.state)
	}
}

// TestPlaybackEndedMsgRepeatAllDoesNotSpekulativelySetSource verifies that
// when RepeatAll wraps, the status bar source is NOT speculatively updated
// from entries[0] because PlayFromIndex may skip missing entries. The
// TrackChangedMsg from the event reader will set the correct source.
func TestPlaybackEndedMsgRepeatAllDoesNotSpekulativelySetSource(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.playback.repeatMode = playback.RepeatAll
	m.statusBar.PlaylistSource = "Old Source"
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Display: "A – One", Artist: "A", Title: "One", SourcePlaylist: "My Playlist"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Display: "B – Two", Artist: "B", Title: "Two", SourcePlaylist: "My Playlist"},
	}

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	// Status bar source should remain unchanged; TrackChangedMsg will update it.
	if nm.statusBar.PlaylistSource != "Old Source" {
		t.Fatalf("RepeatAll wrap: expected playlistSource to remain %q, got %q", "Old Source", nm.statusBar.PlaylistSource)
	}
}

// TestPlaybackEndedMsgRepeatAllKeepsOldSource verifies that when
// RepeatAll wraps, the status bar keeps its existing source until
// TrackChangedMsg arrives with the correct metadata.
func TestPlaybackEndedMsgRepeatAllKeepsOldSource(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.playback.repeatMode = playback.RepeatAll
	m.statusBar.PlaylistSource = "Queue"
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Display: "A – One", Artist: "A", Title: "One"},
	}

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	// Source should remain unchanged; TrackChangedMsg will update it.
	if nm.statusBar.PlaylistSource != "Queue" {
		t.Fatalf("RepeatAll wrap: expected playlistSource to remain %q, got %q", "Queue", nm.statusBar.PlaylistSource)
	}
}

// TestPlaybackEndedMsgRepeatAllEmptyQueueGoesIdle verifies that RepeatAll with
// an empty queue still transitions to idle (no infinite restart loop).
func TestPlaybackEndedMsgRepeatAllEmptyQueueGoesIdle(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.playback.repeatMode = playback.RepeatAll
	m.queue.Entries = nil

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
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
	m.playback.state = playback.StatePlaying
	m.playback.repeatMode = playback.RepeatOff

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Fatalf("RepeatOff + PlaybackEndedMsg: expected PlaybackIdle, got %v", nm.playback.state)
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "Queue finished") {
		t.Fatalf("RepeatOff + PlaybackEndedMsg: expected 'Queue finished' notification, got %q", nm.statusBar.TopNotification().Msg)
	}
}

// stubCycleRestarter implements queueCycleRestarter (and playPauser via the
// embedded stubPlayer) so repeat-all tests can assert the handler prefers
// RestartQueueCycle over PlayFromIndex(0).
type stubCycleRestarter struct {
	stubPlayer
	restartCalls int
	restartErr   error
}

func (s *stubCycleRestarter) RestartQueueCycle() error {
	s.restartCalls++
	return s.restartErr
}

// TestPlaybackEndedMsgRepeatAllPrefersRestartQueueCycle verifies that when the
// player implements queueCycleRestarter, repeat-all exhaustion routes through
// RestartQueueCycle (fresh shuffle permutation) rather than PlayFromIndex(0).
func TestPlaybackEndedMsgRepeatAllPrefersRestartQueueCycle(t *testing.T) {
	p := &stubCycleRestarter{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.playback.repeatMode = playback.RepeatAll
	m.queue.Entries = []queue.Entry{
		{ID: 1, Path: "/a.flac", Artist: "A", Title: "One"},
		{ID: 2, Path: "/b.flac", Artist: "B", Title: "Two"},
	}

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("expected PlaybackPlaying after repeat-all, got %v", nm.playback.state)
	}
	if p.restartCalls != 1 {
		t.Fatalf("expected RestartQueueCycle called once, got %d", p.restartCalls)
	}
	if len(p.playFromIndexCalls) != 0 {
		t.Fatalf("expected no PlayFromIndex fallback, got %v", p.playFromIndexCalls)
	}
}

// TestPlaybackEndedMsgRepeatAllRestartFallbackOnError verifies that if
// RestartQueueCycle fails, the handler logs and falls through to the idle
// transition (no crash, no PlayFromIndex fallback for the restart path).
func TestPlaybackEndedMsgRepeatAllRestartError(t *testing.T) {
	p := &stubCycleRestarter{restartErr: errStubPlay}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.playback.repeatMode = playback.RepeatAll
	m.queue.Entries = []queue.Entry{
		{ID: 1, Path: "/a.flac", Artist: "A", Title: "One"},
	}

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	if p.restartCalls != 1 {
		t.Fatalf("expected RestartQueueCycle attempted once, got %d", p.restartCalls)
	}
	// On restart failure the queue is treated as finished (idle), matching the
	// prior PlayFromIndex-failure behaviour.
	if nm.playback.state != playback.StateIdle {
		t.Fatalf("expected PlaybackIdle after restart error, got %v", nm.playback.state)
	}
}

// TestRepeatAll_ResetsMissingTrackCount verifies that when repeat-all wraps
// back to the beginning of the queue, the missingTrackCount is reset so it
// does not accumulate across repeat cycles.
func TestRepeatAll_ResetsMissingTrackCount(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.repeatMode = playback.RepeatAll
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "h1"
	m.playback.elapsedSecs = 60
	m.playback.totalSecs = 120
	m.playback.peakElapsedSecs = 60
	m.playback.peakTotalSecs = 120
	m.missingTrackCount = 3 // simulate 3 missing files in this cycle
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "B", Title: "T2"},
	}

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("expected PlaybackPlaying after repeat-all wrap, got %v", nm.playback.state)
	}
	if nm.missingTrackCount != 0 {
		t.Errorf("missingTrackCount = %d after repeat-all wrap, want 0", nm.missingTrackCount)
	}
}
