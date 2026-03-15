package tui

import (
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
	m := New("", false, nil, nil, "", nil, p)
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
	m := New("", false, nil, nil, "", nil, p)
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
			m := New("", false, nil, nil, "", nil, p)
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
	m := New("", false, nil, nil, "", nil, p)
	m.queue.entries = []QueueEntry{
		{Path: "/a.flac", Display: "Artist – Song", Artist: "Artist", Title: "Song"},
	}

	if m.playbackState != PlaybackIdle {
		t.Fatalf("initial: expected PlaybackIdle, got %v", m.playbackState)
	}

	// PlayQueueIndexMsg → playing.
	next, _ := m.Update(PlayQueueIndexMsg{Index: 0})
	nm := next.(Model)
	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("after PlayQueueIndexMsg: expected PlaybackPlaying, got %v", nm.playbackState)
	}

	// Space while playing → paused.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm = next.(Model)
	if nm.playbackState != PlaybackPaused {
		t.Fatalf("space (playing→paused): expected PlaybackPaused, got %v", nm.playbackState)
	}

	// Space while paused → playing.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm = next.(Model)
	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("space (paused→playing): expected PlaybackPlaying, got %v", nm.playbackState)
	}

	// PlaybackEndedMsg → idle.
	next, _ = nm.Update(PlaybackEndedMsg{})
	nm = next.(Model)
	if nm.playbackState != PlaybackIdle {
		t.Fatalf("after PlaybackEndedMsg: expected PlaybackIdle, got %v", nm.playbackState)
	}
}

// TestModelStopKeyTransitionsToStopped verifies that the s key transitions
// the playback state to stopped when the player implements commandPlayer.
func TestModelStopKeyTransitionsToStopped(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New("", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackPlaying

	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	nm := next.(Model)
	if nm.playbackState != PlaybackStopped {
		t.Fatalf("s key: expected PlaybackStopped, got %v", nm.playbackState)
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
	m := New("", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackIdle
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
	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("expected playbackState PlaybackPlaying, got %v", nm.playbackState)
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
	m := New("", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackIdle

	track := library.Track{Artist: "Bob", Title: "Track Two", Path: "/music/b.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, WasEmpty: false}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if len(p.playFromIndexCalls) != 0 {
		t.Fatalf("expected no PlayFromIndex call when WasEmpty=false, got calls=%v", p.playFromIndexCalls)
	}
	if nm.playbackState != PlaybackIdle {
		t.Fatalf("expected playbackState PlaybackIdle (unchanged), got %v", nm.playbackState)
	}
}

// TestClearQueueMsgStopsPlaybackAndTransitionsToIdle verifies that handling
// ClearQueueMsg calls Stop() on the player exactly once and transitions
// playbackState to PlaybackIdle (not PlaybackStopped).
func TestClearQueueMsgStopsPlaybackAndTransitionsToIdle(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New("", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackPlaying
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/music/a.flac", Display: "A"},
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.playbackState != PlaybackIdle {
		t.Fatalf("ClearQueueMsg: expected PlaybackIdle, got %v", nm.playbackState)
	}
	if p.stopCalls != 1 {
		t.Fatalf("ClearQueueMsg: expected 1 Stop() call, got %d", p.stopCalls)
	}
}
