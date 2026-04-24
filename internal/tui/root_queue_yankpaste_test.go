package tui

import (
	"context"
	"strings"
	"testing"
)

// sampleQueueEntries returns a slice of queue entries for yank/paste tests.
func sampleQueueEntries() []QueueEntry {
	return []QueueEntry{
		{ID: 101, Path: "/a.mp3", Hash: "h1", Display: "Artist – A", Artist: "Artist", Title: "A", Album: "Album", DurationMs: 180000},
		{ID: 102, Path: "/b.mp3", Hash: "h2", Display: "Artist – B", Artist: "Artist", Title: "B", Album: "Album", DurationMs: 200000},
		{ID: 103, Path: "/c.mp3", Hash: "h3", Display: "Artist – C", Artist: "Artist", Title: "C", Album: "Album", DurationMs: 220000},
		{ID: 104, Path: "/d.mp3", Hash: "h4", Display: "Artist – D", Artist: "Artist", Title: "D", Album: "Album", DurationMs: 240000},
	}
}

// TestYankSingleTrack verifies that ctrl+y with no selection yanks the focused track.
func TestYankSingleTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 1

	rm, _, handled := m.handleQueueKeyAction(actionYank)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionYank should be handled")
	}
	if len(m.yankBuffer) != 1 {
		t.Fatalf("expected 1 yanked entry, got %d", len(m.yankBuffer))
	}
	if m.yankBuffer[0].Hash != "h2" {
		t.Errorf("expected yanked hash 'h2', got %q", m.yankBuffer[0].Hash)
	}
	if !strings.Contains(m.statusBar.notification.msg, "1 track") {
		t.Errorf("expected notification about 1 track, got %q", m.statusBar.notification.msg)
	}
}

// TestYankBulkSelection verifies that ctrl+y with selected entries yanks all
// selected entries in queue order.
func TestYankBulkSelection(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.entries[0].Selected = true
	m.queue.entries[2].Selected = true
	m.queue.cursor = 0

	rm, _, handled := m.handleQueueKeyAction(actionYank)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionYank should be handled")
	}
	if len(m.yankBuffer) != 2 {
		t.Fatalf("expected 2 yanked entries, got %d", len(m.yankBuffer))
	}
	if m.yankBuffer[0].Hash != "h1" || m.yankBuffer[1].Hash != "h3" {
		t.Errorf("yanked entries out of order: %q, %q", m.yankBuffer[0].Hash, m.yankBuffer[1].Hash)
	}
	if !strings.Contains(m.statusBar.notification.msg, "2 track") {
		t.Errorf("expected notification about 2 tracks, got %q", m.statusBar.notification.msg)
	}
}

// TestYankReplacesBuffer verifies that yanking again replaces the previous buffer.
func TestYankReplacesBuffer(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0

	rm, _, _ := m.handleQueueKeyAction(actionYank)
	m = rm.(Model)
	if m.yankBuffer[0].Hash != "h1" {
		t.Fatal("first yank should capture h1")
	}

	m.queue.cursor = 3
	rm, _, _ = m.handleQueueKeyAction(actionYank)
	m = rm.(Model)
	if len(m.yankBuffer) != 1 || m.yankBuffer[0].Hash != "h4" {
		t.Errorf("second yank should replace buffer with h4, got %v", m.yankBuffer)
	}
}

// TestPasteSingleTrack verifies that ctrl+p inserts a copy below the cursor.
func TestPasteSingleTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 1
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	rm, cmd, handled := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for paste")
	}

	// Execute the command to get the QueuePastedMsg.
	msg := cmd().(QueuePastedMsg)
	if msg.Err != nil {
		t.Fatalf("paste cmd returned error: %v", msg.Err)
	}
	if len(msg.Entries) != 1 {
		t.Fatalf("expected 1 pasted entry, got %d", len(msg.Entries))
	}
	if msg.InsertPos != 2 {
		t.Errorf("expected InsertPos 2, got %d", msg.InsertPos)
	}
	// The pasted entry should have a non-zero DB ID assigned by the writer.
	if msg.Entries[0].ID == 0 {
		t.Error("pasted entry should have a non-zero DB ID from the writer")
	}

	// Feed the message through the handler.
	m = m.handleQueuePastedMsg(msg)
	if len(m.queue.entries) != 5 {
		t.Fatalf("expected 5 entries after paste, got %d", len(m.queue.entries))
	}
	// Pasted entry should be at position 2 (after cursor=1).
	if m.queue.entries[2].Hash != "h1" {
		t.Errorf("expected pasted entry at index 2 with hash h1, got %q", m.queue.entries[2].Hash)
	}
	// Cursor should move to the last pasted entry.
	if m.queue.cursor != 2 {
		t.Errorf("expected cursor at 2, got %d", m.queue.cursor)
	}
}

// TestPasteMultipleTracks verifies that pasting multiple tracks inserts them
// contiguously below the cursor.
func TestPasteMultipleTracks(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0
	m.yankBuffer = []QueueEntry{m.queue.entries[2], m.queue.entries[3]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 6 {
		t.Fatalf("expected 6 entries after paste, got %d", len(m.queue.entries))
	}
	// Entries at positions 1 and 2 should be the pasted tracks.
	if m.queue.entries[1].Hash != "h3" || m.queue.entries[2].Hash != "h4" {
		t.Errorf("pasted entries at wrong positions: [1]=%q [2]=%q",
			m.queue.entries[1].Hash, m.queue.entries[2].Hash)
	}
	if m.queue.cursor != 2 {
		t.Errorf("cursor should be at last pasted entry (2), got %d", m.queue.cursor)
	}
}

// TestPasteTwice verifies that the yank buffer persists across multiple pastes.
func TestPasteTwice(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	// First paste.
	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)
	if len(m.queue.entries) != 5 {
		t.Fatalf("expected 5 after first paste, got %d", len(m.queue.entries))
	}

	// Second paste (cursor moved to 1 after first paste).
	_, cmd, _ = m.handleQueueKeyAction(actionPaste)
	msg = cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)
	if len(m.queue.entries) != 6 {
		t.Fatalf("expected 6 after second paste, got %d", len(m.queue.entries))
	}
}

// TestPasteEmptyBuffer verifies notification when nothing has been yanked.
func TestPasteEmptyBuffer(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()

	rm, cmd, handled := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when yank buffer is empty")
	}
	if !strings.Contains(m.statusBar.notification.msg, "Nothing to paste") {
		t.Errorf("expected 'Nothing to paste' notification, got %q", m.statusBar.notification.msg)
	}
}

// TestPasteWhileSorted verifies paste is blocked when queue is sorted.
func TestPasteWhileSorted(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}
	m.queue.displayOrder = []int{3, 2, 1, 0} // non-nil = sorted

	rm, cmd, handled := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when sorted")
	}
	if !strings.Contains(m.statusBar.notification.msg, "Cannot paste") {
		t.Errorf("expected 'Cannot paste' notification, got %q", m.statusBar.notification.msg)
	}
}

// TestPasteWhileFiltered verifies paste is blocked when filter is active.
func TestPasteWhileFiltered(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}
	m.queue.filter.Text = "artist"

	rm, cmd, handled := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when filtered")
	}
	if !strings.Contains(m.statusBar.notification.msg, "Cannot paste") {
		t.Errorf("expected 'Cannot paste' notification, got %q", m.statusBar.notification.msg)
	}
}

// TestPasteAdjustsCurrentIndex verifies that pasting before the currently
// playing track shifts currentIndex forward.
func TestPasteAdjustsCurrentIndex(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.playback.currentIndex = 2
	m.queue.cursor = 0
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Insert at position 1 (after cursor 0), which is before currentIndex=2.
	// currentIndex should shift by 1.
	if m.playback.currentIndex != 3 {
		t.Errorf("expected currentIndex 3, got %d", m.playback.currentIndex)
	}
}

// TestPasteAtEndOfQueue verifies paste when cursor is on the last entry.
func TestPasteAtEndOfQueue(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 3 // last entry
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(m.queue.entries))
	}
	// Pasted entry should be at the end.
	if m.queue.entries[4].Hash != "h1" {
		t.Errorf("expected pasted entry at end with hash h1, got %q", m.queue.entries[4].Hash)
	}
	if m.queue.cursor != 4 {
		t.Errorf("expected cursor at 4, got %d", m.queue.cursor)
	}
}

// TestYankEmptyQueue verifies yank is a no-op on an empty queue.
func TestYankEmptyQueue(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue

	rm, _, handled := m.handleQueueKeyAction(actionYank)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionYank should be handled even on empty queue")
	}
	if len(m.yankBuffer) != 0 {
		t.Errorf("expected empty yank buffer, got %d entries", len(m.yankBuffer))
	}
}

// TestPasteSetsUndoSlot verifies that paste sets up an undo slot.
func TestPasteSetsUndoSlot(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 1
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if m.undo.slot.action == nil {
		t.Fatal("paste should set an undo slot")
	}
	if m.undo.slot.action.kind() != undoKindPaste {
		t.Errorf("expected undoKindPaste, got %v", m.undo.slot.action.kind())
	}
	if !strings.Contains(m.undo.slot.action.description(), "1 track") {
		t.Errorf("unexpected undo description: %q", m.undo.slot.action.description())
	}
}

// TestUndoPasteRemovesPastedEntries verifies that undoing a paste removes the
// pasted entries and restores the queue to its previous state.
func TestUndoPasteRemovesPastedEntries(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 1
	m.yankBuffer = []QueueEntry{m.queue.entries[0], m.queue.entries[2]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 6 {
		t.Fatalf("expected 6 entries after paste, got %d", len(m.queue.entries))
	}

	// Undo the paste.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after undo, got %d", len(m.queue.entries))
	}
	// Original entries should be intact.
	for i, want := range []string{"h1", "h2", "h3", "h4"} {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
	// Batch enqueuer should have been called to soft-delete.
	if len(be.dequeueTracksSoftIDs) != 1 {
		t.Fatalf("expected 1 DequeueTracksSoft call, got %d", len(be.dequeueTracksSoftIDs))
	}
	if len(be.dequeueTracksSoftIDs[0]) != 2 {
		t.Errorf("expected 2 IDs soft-deleted, got %d", len(be.dequeueTracksSoftIDs[0]))
	}
}

// TestUndoPasteAdjustsCurrentIndex verifies that undoing a paste that was
// inserted before the playing track shifts currentIndex back.
func TestUndoPasteAdjustsCurrentIndex(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.playback.currentIndex = 2
	m.queue.cursor = 0
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// currentIndex shifted from 2 to 3 by paste.
	if m.playback.currentIndex != 3 {
		t.Fatalf("expected currentIndex 3 after paste, got %d", m.playback.currentIndex)
	}

	// Undo should shift it back.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if m.playback.currentIndex != 2 {
		t.Errorf("expected currentIndex 2 after undo, got %d", m.playback.currentIndex)
	}
}

// TestPasteResumesPlaybackWhenQueueExhausted verifies that pasting tracks after
// the queue was exhausted (PlaybackEndedMsg already processed) auto-resumes
// playback from the first pasted entry and populates all playback bookkeeping
// (currentHash, currentIndex, status bar).
func TestPasteResumesPlaybackWhenQueueExhausted(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithPlayer(p), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()[:2] // [A, B]
	m.queue.cursor = 1                         // cursor on B (last)
	m.playback.state = PlaybackIdle
	m.playback.currentIndex = -1
	m.playback.queueExhausted = true // queue ran out

	m.yankBuffer = []QueueEntry{sampleQueueEntries()[2]} // yank C

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if m.playback.state != PlaybackPlaying {
		t.Fatalf("expected PlaybackPlaying after paste with exhausted queue, got %v", m.playback.state)
	}
	// Should call PlayFromIndex(2) — the insert position.
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 2 {
		t.Fatalf("expected PlayFromIndex(2), got calls=%v", p.playFromIndexCalls)
	}
	if m.playback.queueExhausted {
		t.Fatal("queueExhausted should be cleared after auto-resume")
	}
	// The pasted entry's metadata must be reflected in playback state and
	// status bar so the UI is correct immediately (before TrackChangedMsg).
	pastedEntry := msg.Entries[0]
	if m.playback.currentHash != pastedEntry.Hash {
		t.Errorf("currentHash = %q, want %q", m.playback.currentHash, pastedEntry.Hash)
	}
	if m.playback.currentIndex != 2 {
		t.Errorf("currentIndex = %d, want 2", m.playback.currentIndex)
	}
	if m.statusBar.artist != pastedEntry.Artist {
		t.Errorf("statusBar.artist = %q, want %q", m.statusBar.artist, pastedEntry.Artist)
	}
	if m.statusBar.title != pastedEntry.Title {
		t.Errorf("statusBar.title = %q, want %q", m.statusBar.title, pastedEntry.Title)
	}
}

// TestPasteExhaustedResumeResetsPlaybackFields verifies that pasting after
// queue exhaustion resets stale elapsed/peak/durationCorrected fields so the
// progress bar and duration correction logic start fresh for the new track.
func TestPasteExhaustedResumeResetsPlaybackFields(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithPlayer(p), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()[:2]
	m.queue.cursor = 1
	m.playback.state = PlaybackIdle
	m.playback.currentIndex = -1
	m.playback.queueExhausted = true
	// Simulate stale fields from the previous track.
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.playback.peakElapsedSecs = 100
	m.playback.peakTotalSecs = 200
	m.playback.durationCorrected = true

	m.yankBuffer = []QueueEntry{sampleQueueEntries()[2]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if m.playback.state != PlaybackPlaying {
		t.Fatalf("expected PlaybackPlaying, got %v", m.playback.state)
	}
	if m.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0", m.playback.elapsedSecs)
	}
	if m.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0", m.playback.totalSecs)
	}
	if m.playback.peakElapsedSecs != 0 {
		t.Errorf("peakElapsedSecs = %v, want 0", m.playback.peakElapsedSecs)
	}
	if m.playback.peakTotalSecs != 0 {
		t.Errorf("peakTotalSecs = %v, want 0", m.playback.peakTotalSecs)
	}
	if m.playback.durationCorrected {
		t.Error("durationCorrected should be false after exhaustion resume")
	}
}

// TestPasteDoesNotResumeWhenUserStopped verifies that pasting tracks when
// playback is idle but NOT due to queue exhaustion does not auto-resume.
func TestPasteDoesNotResumeWhenUserStopped(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithPlayer(p), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()[:2]
	m.queue.cursor = 1
	m.playback.state = PlaybackIdle
	m.playback.currentIndex = -1
	m.playback.queueExhausted = false // user stopped, not exhausted

	m.yankBuffer = []QueueEntry{sampleQueueEntries()[2]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if m.playback.state != PlaybackIdle {
		t.Fatalf("expected PlaybackIdle (user-stopped), got %v", m.playback.state)
	}
	if len(p.playFromIndexCalls) != 0 {
		t.Fatalf("expected no PlayFromIndex calls, got %v", p.playFromIndexCalls)
	}
}

// --- Cut tests ---

// TestCutSingleTrack verifies that ctrl+x with no selection cuts the focused
// track and sets yankIsCut.
func TestCutSingleTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 1

	rm, _, handled := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionCut should be handled")
	}
	if len(m.yankBuffer) != 1 {
		t.Fatalf("expected 1 cut entry, got %d", len(m.yankBuffer))
	}
	if m.yankBuffer[0].Hash != "h2" {
		t.Errorf("expected cut hash 'h2', got %q", m.yankBuffer[0].Hash)
	}
	if !m.yankIsCut {
		t.Error("yankIsCut should be true after cut")
	}
	if !strings.Contains(m.statusBar.notification.msg, "Cut") {
		t.Errorf("expected 'Cut' in notification, got %q", m.statusBar.notification.msg)
	}
	// Queue should not be modified yet (deferred to paste).
	if len(m.queue.entries) != 4 {
		t.Errorf("queue should still have 4 entries, got %d", len(m.queue.entries))
	}
}

// TestCutBulkSelection verifies that ctrl+x with selected entries cuts all
// selected entries.
func TestCutBulkSelection(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.entries[0].Selected = true
	m.queue.entries[2].Selected = true
	m.queue.cursor = 0

	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	if len(m.yankBuffer) != 2 {
		t.Fatalf("expected 2 cut entries, got %d", len(m.yankBuffer))
	}
	if m.yankBuffer[0].Hash != "h1" || m.yankBuffer[1].Hash != "h3" {
		t.Errorf("cut entries out of order: %q, %q", m.yankBuffer[0].Hash, m.yankBuffer[1].Hash)
	}
	if !m.yankIsCut {
		t.Error("yankIsCut should be true")
	}
}

// TestCutThenPasteRemovesOriginals verifies that pasting a cut buffer moves
// tracks: inserts copies at new position and removes originals.
func TestCutThenPasteRemovesOriginals(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.queue.cursor = 0

	// Cut track A (ID=101).
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Move cursor to after D and paste.
	m.queue.cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Queue should still have 4 entries (1 removed + 1 added = net 0).
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}
	// Track A should now be at the end.
	wantOrder := []string{"h2", "h3", "h4", "h1"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
	// Original ID 101 should have been soft-deleted.
	if len(be.dequeueTracksSoftIDs) != 1 || len(be.dequeueTracksSoftIDs[0]) != 1 {
		t.Fatalf("expected 1 soft-delete call with 1 ID, got %v", be.dequeueTracksSoftIDs)
	}
	if be.dequeueTracksSoftIDs[0][0] != 101 {
		t.Errorf("expected soft-deleted ID 101, got %d", be.dequeueTracksSoftIDs[0][0])
	}
	// yankIsCut should be cleared after paste.
	if m.yankIsCut {
		t.Error("yankIsCut should be false after cut-paste")
	}
}

// TestCutPasteAdjustsPlaybackIndex verifies that cut-paste correctly adjusts
// the playback index for both the insertion and removal.
func TestCutPasteAdjustsPlaybackIndex(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.playback.currentIndex = 2            // playing C

	// Cut A (index 0, before currently playing).
	m.queue.cursor = 0
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste at end.
	m.queue.cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// After insert at pos 4: [A, B, C, D, A'] -> currentIndex shifts to 3 (insert at 4 > 2, no shift)
	// Wait, insert at cursor+1=4, which is > currentIndex=2, no shift from insert.
	// After removing original A at index 0 (before currentIndex=2): currentIndex = 2-1 = 1.
	// Result: [B, C, D, A'] with currentIndex=1 (still C).
	if m.playback.currentIndex != 1 {
		t.Errorf("expected currentIndex 1 (still playing C), got %d", m.playback.currentIndex)
	}
	if m.queue.entries[m.playback.currentIndex].Hash != "h3" {
		t.Errorf("expected playing track to be h3, got %q", m.queue.entries[m.playback.currentIndex].Hash)
	}
}

// TestYankAfterCutClearsCutFlag verifies that yanking after a cut downgrades
// the buffer to a copy operation.
func TestYankAfterCutClearsCutFlag(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0

	// Cut.
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)
	if !m.yankIsCut {
		t.Fatal("yankIsCut should be true after cut")
	}

	// Now yank a different track.
	m.queue.cursor = 1
	rm, _, _ = m.handleQueueKeyAction(actionYank)
	m = rm.(Model)

	if m.yankIsCut {
		t.Error("yankIsCut should be false after yank")
	}
	if m.yankBuffer[0].Hash != "h2" {
		t.Errorf("yank buffer should have h2, got %q", m.yankBuffer[0].Hash)
	}
}

// TestCutPasteSetsUndoSlot verifies that a cut-paste sets an undoKindCutPaste
// undo slot.
func TestCutPasteSetsUndoSlot(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0

	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	m.queue.cursor = 2
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if m.undo.slot.action == nil {
		t.Fatal("cut-paste should set an undo slot")
	}
	if m.undo.slot.action.kind() != undoKindCutPaste {
		t.Errorf("expected undoKindCutPaste, got %v", m.undo.slot.action.kind())
	}
	if !strings.Contains(m.undo.slot.action.description(), "Moved") {
		t.Errorf("unexpected undo description: %q", m.undo.slot.action.description())
	}
}

// TestUndoCutPasteRestoresOriginals verifies that undoing a cut-paste removes
// the pasted copies and re-inserts the originals.
func TestUndoCutPasteRestoresOriginals(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A, B, C, D]
	m.queue.cursor = 0

	// Cut A.
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste after C (cursor=2 in the original, but we move it).
	m.queue.cursor = 2
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Verify cut-paste worked: A moved from index 0 to after C.
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}

	// Undo the cut-paste.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after undo, got %d", len(m.queue.entries))
	}
	// Original order should be restored: A, B, C, D.
	wantOrder := []string{"h1", "h2", "h3", "h4"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
}

// TestUndoCutPasteMultipleNonAdjacentRestoresOrder verifies that cutting
// multiple non-adjacent tracks, pasting them elsewhere, and then undoing
// restores the original queue order. This catches a bug where the undo used
// post-insert indices instead of pre-paste positions.
func TestUndoCutPasteMultipleNonAdjacentRestoresOrder(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A(101), B(102), C(103), D(104)]

	// Bulk-select B and D (indices 1, 3).
	m.queue.entries[1].Selected = true
	m.queue.entries[3].Selected = true

	// Cut the selection.
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)
	if len(m.yankBuffer) != 2 {
		t.Fatalf("expected 2 cut entries, got %d", len(m.yankBuffer))
	}

	// Paste after A (cursor=0 → insertPos=1). Queue is [A, C] before paste.
	m.queue.cursor = 0
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}

	// Undo the cut-paste.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after undo, got %d", len(m.queue.entries))
	}
	// Original order should be restored: A, B, C, D.
	wantOrder := []string{"h1", "h2", "h3", "h4"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
}

// TestCutPasteSecondPasteDuplicates verifies that after a cut-paste, a second
// paste duplicates instead of moving (cut state is cleared).
func TestCutPasteSecondPasteDuplicates(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A, B, C, D]
	m.queue.cursor = 0

	// Cut A.
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// First paste (moves A).
	m.queue.cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}

	// Second paste should duplicate (yankIsCut is cleared).
	rm, cmd, _ = m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg = cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 5 {
		t.Fatalf("expected 5 entries after second paste (duplicate), got %d", len(m.queue.entries))
	}
}

// TestUndoCutPasteThenPasteMoves verifies that after cut → paste → undo → paste,
// the tracks are moved (not duplicated). The undo must restore yankIsCut and
// update yankBuffer IDs to match the re-inserted originals.
func TestUndoCutPasteThenPasteMoves(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.queue.cursor = 0

	// Cut A.
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste after D.
	m.queue.cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Undo the cut-paste.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if !m.yankIsCut {
		t.Fatal("yankIsCut should be restored after undo")
	}
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after undo, got %d", len(m.queue.entries))
	}

	// Paste again after C — should move, not duplicate.
	m.queue.cursor = 2
	rm, cmd, _ = m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg = cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after re-paste (move), got %d", len(m.queue.entries))
	}
	// A should now be after C: [B, C, A, D].
	wantOrder := []string{"h2", "h3", "h1", "h4"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
}

// TestCutPasteCursorPointsAtPastedEntry verifies that after a cut-paste where
// the cut originals are after the paste position, the cursor lands on the last
// pasted entry (not incorrectly shifted by originals removed after the cursor).
func TestCutPasteCursorPointsAtPastedEntry(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.queue.cursor = 3                     // on D

	// Cut D (last track).
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste after A (cursor=0).
	m.queue.cursor = 0
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Expected queue: [A, D', B, C] (original D removed, copy inserted at 1).
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}
	wantOrder := []string{"h1", "h4", "h2", "h3"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
	// Cursor should point at D' (index 1), the pasted entry.
	if m.queue.cursor != 1 {
		t.Errorf("expected cursor at 1 (pasted entry), got %d", m.queue.cursor)
	}
}

// TestCutPasteMultipleCursorPointsAtLastPasted verifies that cutting multiple
// tracks from after the paste position leaves the cursor on the last pasted
// entry, not incorrectly shifted backward.
func TestCutPasteMultipleCursorPointsAtLastPasted(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]

	// Select and cut C and D (indices 2 and 3).
	m.queue.entries[2].Selected = true
	m.queue.entries[3].Selected = true
	m.queue.cursor = 2
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste after A (cursor=0).
	m.queue.cursor = 0
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Expected queue: [A, C', D', B] — originals at indices 3,4 removed.
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}
	wantOrder := []string{"h1", "h3", "h4", "h2"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
	// Cursor should point at D' (index 2), the last pasted entry.
	if m.queue.cursor != 2 {
		t.Errorf("expected cursor at 2 (last pasted entry), got %d", m.queue.cursor)
	}
}

// TestCutPastePreservesPurgeCancelChannel verifies that a cut-paste preserves the
// purge cancel channel so that the soft-deleted originals are eventually purged.
// Before the fix, setUndoSlot(…, nil) would close the cancel channel created by
// schedulePurge, permanently preventing the purge timer from firing.
func TestCutPastePreservesPurgeCancelChannel(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	dequeuer := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.undoer = dequeuer
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.queue.cursor = 0

	// Cut A.
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste after D.
	m.queue.cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// The undo slot should have a non-nil cancelPurge channel — this is the
	// channel that keeps the purge timer alive for the soft-deleted originals.
	if m.undo.slot.cancelPurge == nil {
		t.Fatal("cut-paste undo slot should have a non-nil cancelPurge channel; " +
			"without it the purge timer for soft-deleted originals is immediately cancelled")
	}
	// The channel should not be closed (select on it should not succeed).
	select {
	case <-m.undo.slot.cancelPurge:
		t.Fatal("cancelPurge channel should not be closed")
	default:
		// expected: channel is open
	}
}
