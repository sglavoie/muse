package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestPauseEmitsSessionPositionWriteCmd verifies that transitioning to paused
// returns a Cmd that calls UpdateSessionPosition on the session writer with
// the current track hash and elapsed position in milliseconds.
func TestPauseEmitsSessionPositionWriteCmd(t *testing.T) {
	p := &stubPlayer{}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "abc123"
	m.playback.elapsedSecs = 42.5 // 42500 ms

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd == nil {
		t.Fatal("pause: expected non-nil player command")
	}
	_, cmd = next.(Model).Update(cmd())
	if cmd == nil {
		t.Fatal("pause: expected non-nil cmd for session position write")
	}
	cmd()
	if len(sw.positionCalls) != 1 {
		t.Fatalf("pause: expected 1 UpdateSessionPosition call, got %d", len(sw.positionCalls))
	}
	if sw.positionCalls[0].hash != "abc123" {
		t.Fatalf("pause: expected hash %q, got %q", "abc123", sw.positionCalls[0].hash)
	}
	if sw.positionCalls[0].positionMs != 42500 {
		t.Fatalf("pause: expected positionMs 42500, got %d", sw.positionCalls[0].positionMs)
	}
}

// TestPauseWithoutSessionWriterReturnsNilCmd verifies that after the ordered
// pause call finishes, its follow-up session write is nil without a writer.
func TestPauseWithoutSessionWriterReturnsNilCmd(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.queue.Entries = []queue.Entry{{ID: 1, Display: "Track"}}

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd == nil {
		t.Fatal("pause without session writer: expected a player command")
	}
	_, sessionCmd := next.(Model).Update(cmd())
	if sessionCmd != nil {
		t.Fatal("pause without session writer: expected nil session cmd")
	}
}

// TestSeekEmitsSessionPositionWriteCmd verifies that each seek action (fine and
// coarse, forward and backward) returns a Cmd that calls UpdateSessionPosition
// on the session writer.
func TestSeekEmitsSessionPositionWriteCmd(t *testing.T) {
	seekKeys := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"seek-back-fine", tea.KeyPressMsg{Code: tea.KeyLeft}},
		{"seek-forward-fine", tea.KeyPressMsg{Code: tea.KeyRight}},
		{"seek-back-coarse", tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift}},
		{"seek-forward-coarse", tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}},
	}

	for _, tc := range seekKeys {
		t.Run(tc.name, func(t *testing.T) {
			p := &stubSeekPlayer{}
			sw := &stubSessionWriter{}
			m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))
			m.playback.currentHash = "seekhash"
			m.playback.elapsedSecs = 10.0 // 10000 ms

			_, cmd := m.Update(tc.msg)
			if cmd == nil {
				t.Fatalf("%s: expected non-nil cmd for session position write", tc.name)
			}
			cmd()
			if len(sw.positionCalls) != 1 {
				t.Fatalf("%s: expected 1 UpdateSessionPosition call, got %d", tc.name, len(sw.positionCalls))
			}
			if sw.positionCalls[0].hash != "seekhash" {
				t.Fatalf("%s: expected hash %q, got %q", tc.name, "seekhash", sw.positionCalls[0].hash)
			}
			if sw.positionCalls[0].positionMs != 10000 {
				t.Fatalf("%s: expected positionMs 10000, got %d", tc.name, sw.positionCalls[0].positionMs)
			}
		})
	}
}

// TestTrackChangedMsgEmitsSessionPositionWriteCmd verifies that receiving a
// TrackChangedMsg returns a Cmd that calls UpdateSessionPosition with the new
// track's hash and position 0 (start of track).
func TestTrackChangedMsgEmitsSessionPositionWriteCmd(t *testing.T) {
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithSessionWriter(sw))
	m.playback.elapsedSecs = 99.0 // must be reset to 0 before the cmd captures position
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "newhash", Path: "/a.flac"}}

	_, cmd := m.Update(playback.TrackChangedMsg{
		Artist: "Artist",
		Title:  "Title",
		Hash:   "newhash",
	})
	if cmd == nil {
		t.Fatal("TrackChangedMsg: expected non-nil cmd for session position write")
	}
	drainCmd(cmd)
	if len(sw.positionCalls) != 1 {
		t.Fatalf("TrackChangedMsg: expected 1 UpdateSessionPosition call, got %d", len(sw.positionCalls))
	}
	if sw.positionCalls[0].hash != "newhash" {
		t.Fatalf("TrackChangedMsg: expected hash %q, got %q", "newhash", sw.positionCalls[0].hash)
	}
	if sw.positionCalls[0].positionMs != 0 {
		t.Fatalf("TrackChangedMsg: expected positionMs 0, got %d", sw.positionCalls[0].positionMs)
	}
}

// TestTrackChangedMsgStoresCurrentTrackHash verifies that TrackChangedMsg
// updates m.currentTrackHash so that subsequent pause/seek writes use the
// correct hash.
func TestTrackChangedMsgStoresCurrentTrackHash(t *testing.T) {
	m := New(context.Background())
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "testhash", Path: "/a.flac"}}

	next, _ := m.Update(playback.TrackChangedMsg{Artist: "A", Title: "T", Hash: "testhash"})
	nm := next.(Model)

	if nm.playback.currentHash != "testhash" {
		t.Fatalf("expected currentTrackHash %q, got %q", "testhash", nm.playback.currentHash)
	}
}

// TestTrackChangedMsgWithoutSessionWriterReturnsNonNilPulseCmd verifies that
// even without a session writer, TrackChangedMsg returns a non-nil cmd because
// the now-playing pulse tick is always scheduled. The session-position write
// portion of the cmd batch remains a no-op.
func TestTrackChangedMsgWithoutSessionWriterReturnsNonNilPulseCmd(t *testing.T) {
	m := New(context.Background())
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "h", Path: "/a.flac"}}

	_, cmd := m.Update(playback.TrackChangedMsg{Artist: "A", Title: "T", Hash: "h"})
	if cmd == nil {
		t.Fatal("TrackChangedMsg: expected pulse cmd even without session writer")
	}
}

// TestSessionTickMsgReschedulesCmd verifies that sessionTickMsg always returns
// a non-nil follow-up command so the 10-second periodic ticker keeps firing.
func TestSessionTickMsgReschedulesCmd(t *testing.T) {
	m := New(context.Background())
	_, cmd := m.Update(sessionTickMsg{})
	if cmd == nil {
		t.Fatal("sessionTickMsg must return a follow-up cmd to reschedule the ticker")
	}
}

// TestSessionTickMsgWritesPositionWhenPlaying verifies that when playback is
// active, sessionTickMsg triggers an UpdateSessionPosition write through the
// session writer with the current track hash and elapsed position in ms.
//
// tea.Batch returns a BatchMsg (a []tea.Cmd) when invoked — the runtime then
// executes each sub-cmd concurrently. The test extracts and runs only the
// write sub-cmd to confirm the correct value is captured.
func TestSessionTickMsgWritesPositionWhenPlaying(t *testing.T) {
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithSessionWriter(sw))
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "abc123"
	m.playback.elapsedSecs = 45.5 // 45500 ms

	_, cmd := m.Update(sessionTickMsg{})
	if cmd == nil {
		t.Fatal("sessionTickMsg while playing: expected non-nil cmd")
	}

	// cmd() returns a tea.BatchMsg containing [sessionTickCmd, writeCmd].
	// Execute each sub-cmd in a goroutine with a short timeout so the
	// blocking timer cmd is skipped while the write cmd runs to completion.
	batchMsg := cmd()
	batch, ok := batchMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", batchMsg)
	}
	for _, subcmd := range batch {
		if subcmd == nil {
			continue
		}
		done := make(chan struct{})
		go func(c tea.Cmd) {
			c()
			close(done)
		}(subcmd)
		select {
		case <-done:
			// completed quickly — this is the write cmd
		case <-time.After(2 * time.Second):
			// timed out — this is the blocking tick timer; skip it
		}
	}

	if len(sw.positionCalls) != 1 {
		t.Fatalf("expected 1 UpdateSessionPosition call, got %d", len(sw.positionCalls))
	}
	got := sw.positionCalls[0]
	if got.hash != "abc123" {
		t.Fatalf("expected hash %q, got %q", "abc123", got.hash)
	}
	if got.positionMs != 45500 {
		t.Fatalf("expected positionMs 45500, got %d", got.positionMs)
	}
}

// TestSessionTickMsgSkipsWriteWhenNotPlaying verifies that sessionTickMsg does
// not call UpdateSessionPosition when playback is paused, stopped, or idle.
// The tick cmd itself is a timer and is not executed (it would block); only the
// absence of write side-effects is asserted.
func TestSessionTickMsgSkipsWriteWhenNotPlaying(t *testing.T) {
	states := []playback.State{playback.StateIdle, playback.StatePaused, playback.StateStopped}
	for _, state := range states {
		sw := &stubSessionWriter{}
		m := New(context.Background(), WithSessionWriter(sw))
		m.playback.state = state
		m.playback.currentHash = "abc123"
		m.playback.elapsedSecs = 30.0

		_, cmd := m.Update(sessionTickMsg{})
		if cmd == nil {
			t.Fatalf("state %v: sessionTickMsg must return a reschedule cmd even when not playing", state)
		}
		// When not playing, tea.Batch(sessionTickCmd(), nil) compacts to just
		// sessionTickCmd() — a timer that must NOT be executed here (it blocks).
		// Asserting positionCalls is empty confirms no write was scheduled.
		if len(sw.positionCalls) != 0 {
			t.Fatalf("state %v: expected 0 UpdateSessionPosition calls, got %d", state, len(sw.positionCalls))
		}
	}
}

// TestInitSchedulesSessionTicker verifies that Init starts the 10-second
// periodic session ticker in addition to the 1-second display tick.
func TestInitSchedulesSessionTicker(t *testing.T) {
	m := New(context.Background())
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init must return a non-nil cmd (includes sessionTickCmd)")
	}
}
