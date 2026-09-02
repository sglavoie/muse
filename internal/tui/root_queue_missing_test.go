package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestUpdateMissingTrackMsgMarksQueueEntryAndSetsError verifies that
// MissingTrackMsg marks the matching queue entry as missing and sets a
// transient error message in the status bar.
func TestUpdateMissingTrackMsgMarksQueueEntryAndSetsError(t *testing.T) {
	m := New(context.Background())
	m.queue.Entries = []queue.Entry{
		{Path: "/music/song.flac", Display: "Artist - Song"},
	}

	next, _ := m.Update(playback.MissingTrackMsg{Path: "/music/song.flac"})
	nm := next.(Model)

	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("MissingTrackMsg must set a transient status bar error message")
	}
	if !nm.queue.Entries[0].Missing {
		t.Fatal("MissingTrackMsg must mark the matching queue entry as missing")
	}
}

// TestSelectMissingTrackRemovesEntry verifies that pressing Enter on a
// track already flagged .Missing emits MissingTrackSelectedMsg with the
// entry's fields populated, and that root handles it by removing the
// entry from the queue, issuing a warning, and scheduling a
// soft-delete + immediate purge via the injected queueDequeuer.
func TestSelectMissingTrackRemovesEntry(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 42, Path: "/music/gone.flac", Display: "Artist - Gone", Missing: true, Hash: "abc123"},
	}
	m.queue.Cursor = 0
	dequeuer := &stubQueueDequeuer{}
	m.undoer = dequeuer

	v, cmd := m.queue.HandleAction(keys.ActionSelect)
	m.queue = v
	if cmd == nil {
		t.Fatal("expected a command from selecting a missing track")
	}
	msg, ok := cmd().(playback.MissingTrackSelectedMsg)
	if !ok {
		t.Fatalf("expected MissingTrackSelectedMsg, got %T", cmd())
	}
	if msg.Index != 0 || msg.Path != "/music/gone.flac" || msg.Hash != "abc123" {
		t.Fatalf("unexpected MissingTrackSelectedMsg fields: %+v", msg)
	}

	next, resultCmd := m.Update(msg)
	nm := next.(Model)
	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("MissingTrackSelectedMsg must set a transient notification")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "missing") {
		t.Errorf("unexpected notification: %s", nm.statusBar.TopNotification().Msg)
	}
	if len(nm.queue.Entries) != 0 {
		t.Fatalf("queue must be empty after removal, got %d entries", len(nm.queue.Entries))
	}
	if resultCmd == nil {
		t.Fatal("expected a DB cleanup Cmd")
	}
	resultCmd()
	if len(dequeuer.dequeueTrackSoftCalls) != 1 || dequeuer.dequeueTrackSoftCalls[0] != 42 {
		t.Errorf("DequeueTrackSoft calls = %v, want [42]", dequeuer.dequeueTrackSoftCalls)
	}
	if len(dequeuer.purgeBeforeCalls) != 1 {
		t.Errorf("PurgeDeletedQueueEntries calls = %d, want 1", len(dequeuer.purgeBeforeCalls))
	}
}

// TestSelectDeletedFilePreflightStatDispatchesMissing verifies that
// pressing Enter on a queue entry whose file has been deleted from disk
// — but that has not yet been flagged .Missing by the debounced
// watcher — triggers the pre-flight stat check and emits
// MissingTrackSelectedMsg instead of PlayQueueIndexMsg. This prevents
// the synchronous player.PlayFromIndex call that caused the UI freeze.
func TestSelectDeletedFilePreflightStatDispatchesMissing(t *testing.T) {
	tmpDir := t.TempDir()
	deletedPath := filepath.Join(tmpDir, "gone.flac")
	if err := os.WriteFile(deletedPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := os.Remove(deletedPath); err != nil {
		t.Fatalf("remove seed file: %v", err)
	}

	m := New(context.Background())
	m.activeView = viewQueue
	// Missing=false — the watcher hasn't flagged it yet (2.5s debounce).
	m.queue.Entries = []queue.Entry{
		{ID: 7, Path: deletedPath, Display: "Artist - Gone", Hash: "zzz"},
	}
	m.queue.Cursor = 0

	v, cmd := m.queue.HandleAction(keys.ActionSelect)
	m.queue = v
	if cmd == nil {
		t.Fatal("expected a command from actionSelect")
	}
	switch msg := cmd().(type) {
	case playback.MissingTrackSelectedMsg:
		if msg.Index != 0 || msg.Path != deletedPath || msg.Hash != "zzz" {
			t.Fatalf("unexpected MissingTrackSelectedMsg fields: %+v", msg)
		}
	case playback.PlayQueueIndexMsg:
		t.Fatalf("pre-flight stat must route deleted file to removal; got PlayQueueIndexMsg")
	default:
		t.Fatalf("unexpected message type %T", msg)
	}
}

// TestSelectMissingCurrentTrackResetsPlayback verifies that removing a
// missing track that is the currently-playing entry fully resets the
// playback state. Before the fix, stopPlaybackIfCurrentMissing was
// called after currentHash was already cleared, causing it to return
// early and leave playback.state stuck at PlaybackPlaying with no
// valid current track.
func TestSelectMissingCurrentTrackResetsPlayback(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 0
	m.playback.currentHash = "abc123"
	m.statusBar.Artist = "Artist"
	m.statusBar.Title = "Title"
	m.statusBar.PlaylistSource = "Queue"
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/music/gone.flac", Display: "Gone", Hash: "abc123", Missing: true},
		{ID: 20, Path: "/music/ok.flac", Display: "OK", Hash: "other"},
	}

	next, _ := m.Update(playback.MissingTrackSelectedMsg{Index: 0, Path: "/music/gone.flac", Hash: "abc123"})
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Errorf("playback.state = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.playback.currentIndex != -1 {
		t.Errorf("playback.currentIndex = %d, want -1", nm.playback.currentIndex)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("playback.currentHash = %q, want empty", nm.playback.currentHash)
	}
	if nm.statusBar.Artist != "" {
		t.Errorf("statusBar.artist = %q, want empty", nm.statusBar.Artist)
	}
	if nm.statusBar.Title != "" {
		t.Errorf("statusBar.title = %q, want empty", nm.statusBar.Title)
	}
	if len(nm.queue.Entries) != 1 {
		t.Fatalf("queue should have 1 entry, got %d", len(nm.queue.Entries))
	}
	if p.stopCalls != 1 {
		t.Errorf("Stop must be called once to halt playback, got %d calls", p.stopCalls)
	}
}

// TestPlayFromIndexResultMsgErrorResetsPlayback verifies that a
// playFromIndexResultMsg carrying an error (e.g. the race where the file
// vanished between the pre-flight stat and mpv loadfile) resets
// playback state and surfaces a notification instead of leaving the UI
// stuck in the optimistic "playing" state.
func TestPlayFromIndexResultMsgErrorResetsPlayback(t *testing.T) {
	m := New(context.Background())
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 3
	m.playback.currentHash = "abc"
	m.statusBar.Artist = "Artist"
	m.statusBar.Title = "Title"

	next, _ := m.Update(playback.PlayFromIndexResultMsg{Index: 3, Hash: "abc", Err: errors.New("boom")})
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Errorf("playback state = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.playback.currentIndex != -1 {
		t.Errorf("currentIndex = %d, want -1", nm.playback.currentIndex)
	}
	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("expected a notification on play failure")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "Play failed") {
		t.Errorf("unexpected notification: %s", nm.statusBar.TopNotification().Msg)
	}
}
