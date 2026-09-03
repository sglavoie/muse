package tui

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// exhaustedShuffleModel returns a model whose four-entry queue has been played
// through in a shuffled order — [2 0 3 1], so the cycle ended on the row the
// user sees second — together with the TUI state handlePlaybackEndedMsg leaves
// behind on natural exhaustion: idle, exhausted, no visible current index.
func exhaustedShuffleModel() (Model, *fakeTraversalPlayer) {
	p := newFakeTraversalPlayer([]int{2, 0, 3, 1}, 3)
	m := newModelWithTracks()
	m.player = p
	m.queue.Entries = sampleQueueEntries()
	m.playback.state = playback.StateIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.playback.currentHash = ""
	return m, p
}

// assertResumedIntoAddition asserts that the model resumed onto the queue row at
// wantIdx through the traversal extension, without reanchoring and without
// replaying any row the completed cycle already visited.
func assertResumedIntoAddition(t *testing.T, m Model, p *fakeTraversalPlayer, wantIdx int) {
	t.Helper()
	if p.resumeCalls != 1 {
		t.Errorf("ResumeAfterQueueExtension calls = %d, want 1", p.resumeCalls)
	}
	if len(p.playFromIndexCalls) != 0 {
		t.Errorf("PlayFromIndex called %v; resume must not reanchor the cycle", p.playFromIndexCalls)
	}
	if !slices.Equal(p.loaded, []int{wantIdx}) {
		t.Errorf("loaded rows = %v, want only the addition [%d]", p.loaded, wantIdx)
	}
	if m.playback.state != playback.StatePlaying {
		t.Errorf("state = %v, want StatePlaying", m.playback.state)
	}
	if m.playback.queueExhausted {
		t.Error("queueExhausted should be cleared after a successful resume")
	}
	if m.playback.currentIndex != wantIdx {
		t.Errorf("currentIndex = %d, want %d", m.playback.currentIndex, wantIdx)
	}
	if wantIdx < 0 || wantIdx >= len(m.queue.Entries) {
		t.Fatalf("wantIdx %d out of queue range", wantIdx)
	}
	entry := m.queue.Entries[wantIdx]
	if m.playback.currentHash != entry.Hash {
		t.Errorf("currentHash = %q, want %q (the row that actually played)", m.playback.currentHash, entry.Hash)
	}
	if m.statusBar.Artist != entry.Artist || m.statusBar.Title != entry.Title {
		t.Errorf("status bar = %q/%q, want %q/%q", m.statusBar.Artist, m.statusBar.Title, entry.Artist, entry.Title)
	}
}

// TestSingleEnqueueResumesIntoAddition covers the single-enqueue source: a track
// appended onto an exhausted shuffle cycle resumes into that new identity.
func TestSingleEnqueueResumesIntoAddition(t *testing.T) {
	m, p := exhaustedShuffleModel()

	track := library.Track{Hash: "new-hash", Artist: "New", Title: "Track", Path: "/new.flac"}
	m = m.handleTrackEnqueuedMsg(libtui.TrackEnqueuedMsg{
		Track: track, QueueID: 9, WasEmpty: false, InsertAfter: -1,
	})

	assertResumedIntoAddition(t, m, p, 4)
	if m.statusBar.PlaylistSource != "Queue" {
		t.Errorf("PlaylistSource = %q, want %q", m.statusBar.PlaylistSource, "Queue")
	}
}

// TestBatchEnqueueResumesIntoAddition covers the batch-enqueue source.
func TestBatchEnqueueResumesIntoAddition(t *testing.T) {
	m, p := exhaustedShuffleModel()

	m = m.handleBatchEnqueuedMsg(BatchEnqueuedMsg{
		Entries: []queue.Entry{
			{ID: 10, Hash: "batch-a", Path: "/a.flac", Artist: "A", Title: "Song A", SourcePlaylist: "Mix"},
			{ID: 11, Hash: "batch-b", Path: "/b.flac", Artist: "B", Title: "Song B", SourcePlaylist: "Mix"},
		},
		QueueIDs:     []int64{10, 11},
		WasEmpty:     false,
		PlaylistName: "Mix",
	})

	assertResumedIntoAddition(t, m, p, 4)
	if m.statusBar.PlaylistSource != "Mix" {
		t.Errorf("PlaylistSource = %q, want %q", m.statusBar.PlaylistSource, "Mix")
	}
}

// TestRadioBatchResumesIntoAddition covers the Auto-DJ refill source.
func TestRadioBatchResumesIntoAddition(t *testing.T) {
	m, p := exhaustedShuffleModel()
	m.radio.enabled = true
	m.radio.enqueuePending = true

	m, cmd := m.handleRadioBatchEnqueuedMsg(RadioBatchEnqueuedMsg{
		Entries: []queue.Entry{
			{ID: 70, Hash: "radio-a", Path: "/r.flac", Artist: "R", Title: "Radio A", SourcePlaylist: "Radio"},
		},
		QueueIDs: []int64{70},
		WasEmpty: false,
	})

	assertResumedIntoAddition(t, m, p, 4)
	if cmd != nil {
		t.Error("a successful radio resume must not schedule another enqueue")
	}
	if m.radio.enqueuePending {
		t.Error("enqueuePending should be cleared once the batch is applied")
	}
	if m.statusBar.PlaylistSource != "Radio" {
		t.Errorf("PlaylistSource = %q, want %q", m.statusBar.PlaylistSource, "Radio")
	}
}

// TestPasteResumesIntoAddition covers the queue-paste source, which reaches the
// player through SetQueue rather than an append.
func TestPasteResumesIntoAddition(t *testing.T) {
	m, p := exhaustedShuffleModel()

	m = m.handleQueuePastedMsg(QueuePastedMsg{
		Entries:   []queue.Entry{{ID: 12, Hash: "pasted", Path: "/p.flac", Artist: "P", Title: "Pasted"}},
		InsertPos: len(m.queue.Entries),
	})

	assertResumedIntoAddition(t, m, p, 4)
}

// TestPlaybackEndedResumesIntoConcurrentExtension covers the fifth source: rows
// that landed while PlaybackEndedMsg was in flight. The visible index says
// nothing here — it is the traversal's remaining count that reveals the
// extension — and the resume must continue the cycle rather than reanchor it.
func TestPlaybackEndedResumesIntoConcurrentExtension(t *testing.T) {
	p := newFakeTraversalPlayer([]int{2, 0, 3, 1}, 3)
	m := newModelWithTracks()
	m.player = p
	m.queue.Entries = sampleQueueEntries()
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 1 // the row the shuffle cycle ended on

	// A paste landed first: the row is in the queue and in the traversal.
	m.queue.Entries = append(m.queue.Entries, queue.Entry{
		ID: 12, Hash: "late", Path: "/late.flac", Artist: "L", Title: "Late",
	})
	p.AppendToQueue(queueTrackFromEntry(m.queue.Entries[4]))

	m, cmd := m.handlePlaybackEndedMsg()

	assertResumedIntoAddition(t, m, p, 4)
	if cmd != nil {
		t.Error("a resumed queue must not also schedule a radio refill")
	}
}

// TestPlaybackEndedDoesNotResumeWhenTraversalIsDone proves the other direction:
// a shuffle cursor parked near the persisted head does not look like an
// extension. Visible-index arithmetic would see three rows after index 0 and
// wrongly resume onto an already-played row.
func TestPlaybackEndedDoesNotResumeWhenTraversalIsDone(t *testing.T) {
	p := newFakeTraversalPlayer([]int{1, 2, 3, 0}, 3)
	m := newModelWithTracks()
	m.player = p
	m.queue.Entries = sampleQueueEntries()
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 0 // persisted head, but the last row of the cycle

	m, _ = m.handlePlaybackEndedMsg()

	if len(p.loaded) != 0 {
		t.Errorf("loaded %v; an exhausted traversal must not replay anything", p.loaded)
	}
	if m.playback.state != playback.StateIdle {
		t.Errorf("state = %v, want StateIdle", m.playback.state)
	}
	if !m.playback.queueExhausted {
		t.Error("expected queueExhausted=true when the traversal really ran out")
	}
}

// TestResumeSkipsAdditionMissingFromDisk proves the resumed row — not the index
// the caller guessed — drives playback state: when the first appended file is
// missing the player lands on the second one, and the status bar must describe
// that row.
func TestResumeSkipsAdditionMissingFromDisk(t *testing.T) {
	m, p := exhaustedShuffleModel()
	p.missing[4] = true // the first appended row is gone from disk

	m = m.handleBatchEnqueuedMsg(BatchEnqueuedMsg{
		Entries: []queue.Entry{
			{ID: 10, Hash: "gone", Path: "/gone.flac", Artist: "G", Title: "Gone"},
			{ID: 11, Hash: "playable", Path: "/ok.flac", Artist: "O", Title: "Playable"},
		},
		QueueIDs:     []int64{10, 11},
		WasEmpty:     false,
		PlaylistName: "Mix",
	})

	assertResumedIntoAddition(t, m, p, 5)
}

// TestResumeFailureKeepsQueueExhausted proves a failed resume is not treated as
// a resume: the appended rows stay, the model stays idle and still exhausted so
// the next extension can retry, and the user is told.
func TestResumeFailureKeepsQueueExhausted(t *testing.T) {
	m, p := exhaustedShuffleModel()
	p.resumeErr = errFakeTraversalExhausted

	m = m.handleBatchEnqueuedMsg(BatchEnqueuedMsg{
		Entries: []queue.Entry{
			{ID: 10, Hash: "batch-a", Path: "/a.flac", Artist: "A", Title: "Song A"},
		},
		QueueIDs:     []int64{10},
		WasEmpty:     false,
		PlaylistName: "Mix",
	})

	if len(m.queue.Entries) != 5 {
		t.Fatalf("queue length = %d, want the appended row retained (5)", len(m.queue.Entries))
	}
	if m.playback.state != playback.StateIdle {
		t.Errorf("state = %v, want StateIdle after a failed resume", m.playback.state)
	}
	if !m.playback.queueExhausted {
		t.Error("queueExhausted must survive a failed resume so the next extension retries")
	}
	if m.playback.currentIndex != -1 {
		t.Errorf("currentIndex = %d, want -1 (nothing started)", m.playback.currentIndex)
	}
	if got := m.statusBar.TopNotification().Msg; got == "" {
		t.Error("expected a notification describing the failed resume")
	}
}

// TestRadioResumeFailureDoesNotLoop proves a failed Auto-DJ resume leaves the
// batch in place without scheduling another one: a refill loop on an unplayable
// player would append batch after batch.
func TestRadioResumeFailureDoesNotLoop(t *testing.T) {
	m, p := exhaustedShuffleModel()
	p.resumeErr = errFakeTraversalExhausted
	m.radio.enabled = true
	m.radio.enqueuePending = true

	m, cmd := m.handleRadioBatchEnqueuedMsg(RadioBatchEnqueuedMsg{
		Entries:  []queue.Entry{{ID: 70, Hash: "radio-a", Path: "/r.flac", SourcePlaylist: "Radio"}},
		QueueIDs: []int64{70},
		WasEmpty: false,
	})

	if cmd != nil {
		t.Error("a failed radio resume must not schedule another enqueue")
	}
	if !m.playback.queueExhausted {
		t.Error("queueExhausted must survive a failed radio resume")
	}
	if len(m.queue.Entries) != 5 {
		t.Errorf("queue length = %d, want the radio batch retained (5)", len(m.queue.Entries))
	}
}

// TestResumeFallsBackToPlayFromIndex proves players without the optional
// extension keep the old anchoring behaviour, driven by the index the caller
// computed from the visible queue.
func TestResumeFallsBackToPlayFromIndex(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "old", Path: "/old.flac"}}

	m = m.handleBatchEnqueuedMsg(BatchEnqueuedMsg{
		Entries:      []queue.Entry{{ID: 10, Hash: "batch-a", Path: "/a.flac", Artist: "A", Title: "Song A"}},
		QueueIDs:     []int64{10},
		WasEmpty:     false,
		PlaylistName: "Mix",
	})

	if !slices.Equal(p.playFromIndexCalls, []int{1}) {
		t.Fatalf("PlayFromIndex calls = %v, want [1]", p.playFromIndexCalls)
	}
	if m.playback.state != playback.StatePlaying || m.playback.currentIndex != 1 {
		t.Fatalf("state = %v, currentIndex = %d; want playing at 1", m.playback.state, m.playback.currentIndex)
	}
	if m.playback.queueExhausted {
		t.Error("queueExhausted should be cleared after a successful fallback resume")
	}
}

// TestResumeFallbackFailureKeepsQueueExhausted proves the fallback path shares
// the failure contract: a rejected PlayFromIndex leaves the model exhausted.
func TestResumeFallbackFailureKeepsQueueExhausted(t *testing.T) {
	p := &stubPlayer{playFromIndexErr: errStubPlay}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "old", Path: "/old.flac"}}

	track := library.Track{Hash: "new-hash", Artist: "New", Title: "Track", Path: "/new.flac"}
	m = m.handleTrackEnqueuedMsg(libtui.TrackEnqueuedMsg{
		Track: track, QueueID: 2, WasEmpty: false, InsertAfter: -1,
	})

	if m.playback.state != playback.StateIdle {
		t.Errorf("state = %v, want StateIdle after a failed resume", m.playback.state)
	}
	if !m.playback.queueExhausted {
		t.Error("queueExhausted must survive a failed fallback resume")
	}
	if m.playback.currentHash != "" {
		t.Errorf("currentHash = %q, want empty (nothing started)", m.playback.currentHash)
	}
}

// TestExhaustedSyncDoesNotResetPlayerPosition proves the TUI never pushes its
// cleared current index onto the player: the Manager clamps a negative index to
// the head of the queue, which would make the next resume replay the cycle from
// the start. A paste is the path that syncs the queue while exhausted.
func TestExhaustedSyncDoesNotResetPlayerPosition(t *testing.T) {
	m, p := exhaustedShuffleModel()

	m = m.handleQueuePastedMsg(QueuePastedMsg{
		Entries:   []queue.Entry{{ID: 12, Hash: "pasted", Path: "/p.flac", Artist: "P", Title: "Pasted"}},
		InsertPos: len(m.queue.Entries),
	})

	for _, idx := range p.setCurrentIndexCalls {
		if idx < 0 {
			t.Fatalf("SetCurrentIndex calls = %v; a cleared index must not be pushed to the player",
				p.setCurrentIndexCalls)
		}
	}
	if p.resumeCalls != 1 {
		t.Errorf("ResumeAfterQueueExtension calls = %d, want 1", p.resumeCalls)
	}
}

// TestSyncPlayerQueuePushesRealIndex proves the guard above is narrow: a genuine
// position is still synced, so reorder and dedup keep the player in step.
func TestSyncPlayerQueuePushesRealIndex(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.Entries = sampleQueueEntries()
	m.playback.currentIndex = 2

	m.syncPlayerQueue()

	if !slices.Equal(p.setCurrentIndexCalls, []int{2}) {
		t.Fatalf("SetCurrentIndex calls = %v, want [2]", p.setCurrentIndexCalls)
	}
}

// TestUserClearDuringScoringStillDiscardsBatch proves the user-clear guard is
// untouched by the traversal wiring: a batch that lands on a queue the user
// emptied is dropped, and nothing resumes.
func TestUserClearDuringScoringStillDiscardsBatch(t *testing.T) {
	p := newFakeTraversalPlayer(nil, -1)
	m := newModelWithTracks()
	m.player = p
	m.radio.enabled = true
	m.radio.enqueuePending = true
	m.queue.Entries = nil
	m.playback.state = playback.StateIdle
	m.playback.queueExhausted = false // reset by resetPlaybackState on the clear

	m, cmd := m.handleRadioBatchEnqueuedMsg(RadioBatchEnqueuedMsg{
		Entries:  []queue.Entry{{ID: 70, Hash: "radio-a", Path: "/r.flac"}},
		QueueIDs: []int64{70},
		WasEmpty: false,
	})

	if len(m.queue.Entries) != 0 {
		t.Errorf("queue length = %d, want 0 after a user clear", len(m.queue.Entries))
	}
	if p.resumeCalls != 0 || len(p.playFromIndexCalls) != 0 {
		t.Errorf("resume=%d playFromIndex=%v; a discarded batch must not start playback",
			p.resumeCalls, p.playFromIndexCalls)
	}
	if cmd != nil {
		t.Error("a discarded batch must not schedule another enqueue")
	}
}

// TestPasteStillDoesNotResumeWhenUserStopped guards the other exhaustion signal:
// an idle model that is idle because the user stopped is not resumed, however
// much traversal is left.
func TestPasteStillDoesNotResumeWhenUserStopped(t *testing.T) {
	m, p := exhaustedShuffleModel()
	m.playback.queueExhausted = false // user stopped, not exhausted

	m = m.handleQueuePastedMsg(QueuePastedMsg{
		Entries:   []queue.Entry{{ID: 12, Hash: "pasted", Path: "/p.flac"}},
		InsertPos: len(m.queue.Entries),
	})

	if p.resumeCalls != 0 {
		t.Errorf("ResumeAfterQueueExtension calls = %d, want 0 when the user stopped", p.resumeCalls)
	}
	if m.playback.state != playback.StateIdle {
		t.Errorf("state = %v, want StateIdle", m.playback.state)
	}
}

// TestResumeReportsErrorFromResumer keeps the failure surface honest: the error
// the player returned is what the model reports, not a generic message.
func TestResumeReportsErrorFromResumer(t *testing.T) {
	m, p := exhaustedShuffleModel()
	p.resumeErr = errors.New("ipc gone")

	m, resumed := m.resumeExtendedQueue(0)

	if resumed {
		t.Fatal("resumeExtendedQueue reported success on a failing player")
	}
	if got := m.statusBar.TopNotification().Msg; got != "Resume failed: ipc gone" {
		t.Errorf("notification = %q, want %q", got, "Resume failed: ipc gone")
	}
}
