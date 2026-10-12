package tui

import (
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
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

// exhaustedSequentialModel returns the shuffle-off counterpart of
// exhaustedShuffleModel: a four-entry queue played straight through in visible
// order, so the cycle ended on the last row. The player is seeded with the queue
// so it can reconcile a later insertion by identity rather than by position.
func exhaustedSequentialModel() (Model, *fakeTraversalPlayer) {
	p := newFakeTraversalPlayer([]int{0, 1, 2, 3}, 3)
	p.shuffled = false
	m := newModelWithTracks()
	m.player = p
	m.queue.Entries = sampleQueueEntries()
	tracks := make([]player.QueueTrack, len(m.queue.Entries))
	for i, entry := range m.queue.Entries {
		tracks[i] = queueTrackFromEntry(entry)
	}
	p.SetQueue(tracks)
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
		QueueIDs:    []int64{70},
		WasEmpty:    false,
		fingerprint: m.djLiveFingerprint(time.Now()),
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

// finishTrack models the natural end of the row that is playing: the player
// promotes the gapless successor it had lined up and reports the row that
// started, or — when it had none — reports the queue exhausted. It returns the
// carried model and the queue row that started, -1 when the queue ran out.
func finishTrack(m Model, p *fakeTraversalPlayer) (Model, int) {
	started, ok := p.finishCurrent()
	if !ok {
		m, _ = m.handlePlaybackEndedMsg()
		return m, -1
	}
	entry := m.queue.Entries[started]
	m, _ = m.handleTrackChangedMsg(playback.TrackChangedMsg{
		Hash: entry.Hash, Index: started, Artist: entry.Artist, Title: entry.Title,
	})
	return m, started
}

// assertQueueRanOut checks the model landed back on natural exhaustion rather
// than on a row it already played.
func assertQueueRanOut(t *testing.T, m Model, p *fakeTraversalPlayer, wantLoaded []int) {
	t.Helper()
	if m.playback.state != playback.StateIdle || !m.playback.queueExhausted {
		t.Errorf("state = %v exhausted = %v; want idle and exhausted",
			m.playback.state, m.playback.queueExhausted)
	}
	if !slices.Equal(p.loaded, wantLoaded) {
		t.Errorf("loaded rows = %v, want only the insertions %v", p.loaded, wantLoaded)
	}
	if p.resumeCalls != 1 {
		t.Errorf("ResumeAfterQueueExtension calls = %d, want 1; the exhaustion must not resume again",
			p.resumeCalls)
	}
}

// TestPasteBeforeCompletedRowResumesNewAddition covers the insertions that land
// at or before the row a shuffle-off traversal finished on. Both queue paste and
// "play next" insert at the cursor, and on an exhausted queue that is index 0,
// so plain currentIndex+1 arithmetic would step past the new row entirely — it
// either replays a completed row or reports the queue exhausted again. The row
// that was inserted is the one that must play, and nothing already heard may be
// loaded a second time.
func TestPasteBeforeCompletedRowResumesNewAddition(t *testing.T) {
	t.Run("a paste before the completed row plays the pasted row", func(t *testing.T) {
		m, p := exhaustedSequentialModel()

		m = m.handleQueuePastedMsg(QueuePastedMsg{
			Entries:   []queue.Entry{{ID: 12, Hash: "pasted", Path: "/p.flac", Artist: "P", Title: "Pasted"}},
			InsertPos: 0,
		})

		assertResumedIntoAddition(t, m, p, 0)
		if len(m.queue.Entries) != 5 {
			t.Errorf("queue length = %d, want 5", len(m.queue.Entries))
		}
	})

	t.Run("play next on an exhausted queue plays the inserted row", func(t *testing.T) {
		m, p := exhaustedSequentialModel()

		// With nothing playing, "play next" inserts at the front — before every
		// row the traversal already consumed.
		insertAt := m.playNextIndex()
		if insertAt != 0 {
			t.Fatalf("playNextIndex() = %d, want 0 on an exhausted queue", insertAt)
		}
		track := library.Track{Hash: "next-hash", Artist: "Next", Title: "Up", Path: "/next.flac"}
		m = m.handleTrackEnqueuedMsg(libtui.TrackEnqueuedMsg{
			Track: track, QueueID: 9, WasEmpty: false, InsertAfter: insertAt,
		})

		assertResumedIntoAddition(t, m, p, 0)
	})

	t.Run("two pasted rows play in order and then the queue ends", func(t *testing.T) {
		m, p := exhaustedSequentialModel()

		m = m.handleQueuePastedMsg(QueuePastedMsg{
			Entries: []queue.Entry{
				{ID: 12, Hash: "pasted-a", Path: "/pa.flac", Artist: "P", Title: "Pasted A"},
				{ID: 13, Hash: "pasted-b", Path: "/pb.flac", Artist: "P", Title: "Pasted B"},
			},
			InsertPos: 0,
		})

		assertResumedIntoAddition(t, m, p, 0)

		// The second pasted row is what the player had lined up: the rows between
		// it and the cursor are the ones the traversal already finished.
		m, started := finishTrack(m, p)
		if started != 1 {
			t.Fatalf("EOF started row %d, want the second pasted row 1", started)
		}
		m, started = finishTrack(m, p)
		if started != -1 {
			t.Fatalf("EOF started row %d, want the queue to run out", started)
		}
		assertQueueRanOut(t, m, p, []int{0, 1})
	})

	t.Run("a paste in the middle plays only the pasted rows", func(t *testing.T) {
		m, p := exhaustedSequentialModel()

		// Pasting between two completed rows: positional continuation from the
		// pasted row would step onto the row it was inserted above.
		m = m.handleQueuePastedMsg(QueuePastedMsg{
			Entries: []queue.Entry{
				{ID: 12, Hash: "mid-a", Path: "/ma.flac", Artist: "M", Title: "Mid A"},
				{ID: 13, Hash: "mid-b", Path: "/mb.flac", Artist: "M", Title: "Mid B"},
			},
			InsertPos: 2,
		})

		assertResumedIntoAddition(t, m, p, 2)

		m, started := finishTrack(m, p)
		if started != 3 {
			t.Fatalf("EOF started row %d, want the second pasted row 3", started)
		}
		m, started = finishTrack(m, p)
		if started != -1 {
			t.Fatalf("EOF started row %d, want the queue to run out", started)
		}
		assertQueueRanOut(t, m, p, []int{2, 3})
	})

	t.Run("play next then the natural EOF ends the queue", func(t *testing.T) {
		m, p := exhaustedSequentialModel()

		track := library.Track{Hash: "next-hash", Artist: "Next", Title: "Up", Path: "/next.flac"}
		m = m.handleTrackEnqueuedMsg(libtui.TrackEnqueuedMsg{
			Track: track, QueueID: 9, WasEmpty: false, InsertAfter: m.playNextIndex(),
		})

		assertResumedIntoAddition(t, m, p, 0)

		// One insertion, so there is no successor to promote: the transition after
		// it must end the queue instead of replaying row 1.
		m, started := finishTrack(m, p)
		if started != -1 {
			t.Fatalf("EOF started row %d, want the queue to run out", started)
		}
		assertQueueRanOut(t, m, p, []int{0})
	})

	t.Run("a paste at the tail still resumes positionally", func(t *testing.T) {
		m, p := exhaustedSequentialModel()

		m = m.handleQueuePastedMsg(QueuePastedMsg{
			Entries:   []queue.Entry{{ID: 12, Hash: "appended", Path: "/tail.flac", Artist: "T", Title: "Tail"}},
			InsertPos: len(m.queue.Entries),
		})

		assertResumedIntoAddition(t, m, p, 4)
	})

	t.Run("a pasted row missing from disk leaves the queue exhausted", func(t *testing.T) {
		m, p := exhaustedSequentialModel()
		// missing is keyed by queue index, and the paste below lands at 0. The
		// row sitting there now is behind the cursor, so the flag only takes
		// effect once the pasted row occupies that index.
		p.missing[0] = true

		m = m.handleQueuePastedMsg(QueuePastedMsg{
			Entries:   []queue.Entry{{ID: 12, Hash: "gone", Path: "/gone.flac", Artist: "G", Title: "Gone"}},
			InsertPos: 0,
		})

		if p.resumeCalls != 1 {
			t.Errorf("ResumeAfterQueueExtension calls = %d, want 1", p.resumeCalls)
		}
		if len(p.loaded) != 0 {
			t.Errorf("loaded %v; an unplayable insertion must not fall back onto a completed row", p.loaded)
		}
		if len(p.playFromIndexCalls) != 0 {
			t.Errorf("PlayFromIndex called %v; a failed resume must not reanchor the cycle", p.playFromIndexCalls)
		}
		if m.playback.state != playback.StateIdle {
			t.Errorf("state = %v, want StateIdle", m.playback.state)
		}
		if !m.playback.queueExhausted {
			t.Error("queueExhausted must survive an unplayable insertion so the next extension retries")
		}
		if m.playback.currentIndex != -1 {
			t.Errorf("currentIndex = %d, want -1 (nothing started)", m.playback.currentIndex)
		}
	})
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

// assertIdleAwaitingRetry asserts the state a refused resume must leave behind:
// idle and still exhausted with every queue row intact and nothing loaded, so
// the next extension retries rather than starting a fresh cycle.
func assertIdleAwaitingRetry(t *testing.T, m Model, p *fakeTraversalPlayer, wantEntries int) {
	t.Helper()
	if m.playback.state != playback.StateIdle || !m.playback.queueExhausted {
		t.Errorf("state = %v exhausted = %v; want idle and exhausted so the next extension retries",
			m.playback.state, m.playback.queueExhausted)
	}
	if m.playback.currentIndex != -1 {
		t.Errorf("currentIndex = %d, want -1 (nothing started)", m.playback.currentIndex)
	}
	if len(m.queue.Entries) != wantEntries {
		t.Fatalf("queue length = %d, want the extension rows retained (%d)", len(m.queue.Entries), wantEntries)
	}
	if len(p.loaded) != 0 {
		t.Errorf("loaded %v; a refused resume must not have started a row", p.loaded)
	}
}
