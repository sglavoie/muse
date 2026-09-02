package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestRootUpdatePlayQueueIndexMsgCallsPlayerPlayFromIndex verifies that root
// handles PlayQueueIndexMsg by calling PlayFromIndex on the player with the
// correct index.
func TestRootUpdatePlayQueueIndexMsgCallsPlayerPlayFromIndex(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	_, cmd := m.Update(playback.PlayQueueIndexMsg{Index: 1})
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
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.Cursor = 0

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter in queue view must return a non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(playback.PlayQueueIndexMsg); !ok {
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

// TestPlayQueueIndexMsgUpdatesCurrentTrackHash verifies that starting playback
// from a specific queue index updates currentTrackHash immediately, so the
// status bar and session persistence use the correct hash without waiting for
// the asynchronous TrackChangedMsg from the event reader.
func TestPlayQueueIndexMsgUpdatesCurrentTrackHash(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.currentHash = "stale-hash"
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "hash-a", Path: "/a.flac", Display: "A", Artist: "A", Title: "One"},
		{ID: 2, Hash: "hash-b", Path: "/b.flac", Display: "B", Artist: "B", Title: "Two"},
	}

	next, _ := m.Update(playback.PlayQueueIndexMsg{Index: 1})
	nm := next.(Model)

	if nm.playback.currentHash != "hash-b" {
		t.Fatalf("PlayQueueIndexMsg: expected currentTrackHash=%q, got %q", "hash-b", nm.playback.currentHash)
	}
}
