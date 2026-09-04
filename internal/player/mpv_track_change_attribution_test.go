package player

import (
	"testing"
	"time"
)

// This file covers one question: which queue row does a start-file report?
//
// The event reader syncs mpv's position between reading a start-file and
// reporting it, and that sync is an IPC round trip on the same socket every
// playlist replacement uses. A selection racing it publishes the row it is making
// current *before* it sends its own load, so anything the reader re-reads after
// the sync describes the newer selection rather than the entry mpv actually
// started. The report has to come from state captured before the sync.

// trackChange is one OnTrackChanged callback, reduced to the fields that name the
// occurrence it describes.
type trackChange struct {
	hash    string
	index   int
	queueID int64
}

// recordTrackChanges wires the manager's callbacks so every track change is
// recorded in arrival order. The channel is buffered, so a callback nothing is
// waiting on is still captured.
func recordTrackChanges(m *Manager) <-chan trackChange {
	changes := make(chan trackChange, 8)
	m.SetEventCallbacks(EventCallbacks{
		OnStatus:        func(string) {},
		OnMissingFile:   func(string) {},
		OnPlaybackEnded: func() {},
		OnTrackChanged: func(hash, _, _, _ string, index int, queueID int64) {
			select {
			case changes <- trackChange{hash: hash, index: index, queueID: queueID}:
			default:
			}
		},
		OnPauseChanged: func(bool) {},
	})
	return changes
}

// wantTrackChange takes the next track change and checks it names the queue row
// at wantIndex.
func wantTrackChange(t *testing.T, changes <-chan trackChange, queue []QueueTrack, wantIndex int, desc string) {
	t.Helper()
	select {
	case got := <-changes:
		want := trackChange{
			hash:    queue[wantIndex].Hash,
			index:   wantIndex,
			queueID: queue[wantIndex].QueueID,
		}
		if got != want {
			t.Fatalf("%s: track change = %+v, want %+v", desc, got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: timed out waiting for a track change", desc)
	}
}

// waitCurrentIndex blocks until the manager's current index reaches want, which
// is what a racing selection publishes before its load is ever sent.
func waitCurrentIndex(t *testing.T, m *Manager, f *fakePlaylist, want int) {
	t.Helper()
	f.waitFor(t, "the newer selection to publish its row", func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.currentIndex == want
	})
}

// TestTrackChangedDelayedStartKeepsItsOccurrenceAcrossNewerSelection parks a
// start-file's report inside the position sync it performs, commits a newer
// selection while it is parked, and requires the parked report to still name the
// row that started.
//
// Both subtests park the same way — the sync's `get_property` is held inside the
// fake — because that is the real window: the reader has read the event and is
// blocked on the command socket, which is exactly where a replacement can publish
// its row while the report is pending.
func TestTrackChangedDelayedStartKeepsItsOccurrenceAcrossNewerSelection(t *testing.T) {
	t.Run("load-induced start", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "tc-attr-load")
		queue := queueOf(t, 3)
		installQueue(m, queue, -1, false)
		changes := recordTrackChanges(m)
		runEventReader(t, m, f)
		f.waitForEventStream(t)

		// Steady state: one selection reported normally, so the reader has
		// correlated a load and the queue that follows is empty.
		if err := m.PlayFromIndex(0); err != nil {
			t.Fatalf("PlayFromIndex(0): %v", err)
		}
		wantTrackChange(t, changes, queue, 0, "first selection")

		// The older selection. Its loadfile is answered, so mpv has emitted the
		// start-file; the reader reads it and parks in the position sync.
		gate := f.block(t, cmdGetProperty)
		if err := m.PlayFromIndex(1); err != nil {
			t.Fatalf("PlayFromIndex(1): %v", err)
		}
		gate.waitArrived(t, "the position sync of the older start-file")

		// The newer selection, racing that parked report. It publishes row 2 as
		// current under m.mu and only then blocks sending its own loadfile behind
		// the parked sync, which is the interleaving the finding describes.
		selected := make(chan error, 1)
		go func() { selected <- m.PlayFromIndex(2) }()
		waitCurrentIndex(t, m, f, 2)

		f.unblock(cmdGetProperty)

		// The parked report belongs to row 1 and must say so even though row 2 is
		// the row the Manager now considers current. Only row 2's own start-file
		// may confirm row 2.
		wantTrackChange(t, changes, queue, 1, "the parked older start-file")
		if err := <-selected; err != nil {
			t.Fatalf("PlayFromIndex(2): %v", err)
		}
		wantTrackChange(t, changes, queue, 2, "the newer selection's start-file")
	})

	t.Run("gapless start", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "tc-attr-gap")
		queue := queueOf(t, 4)
		installQueue(m, queue, -1, true)
		changes := recordTrackChanges(m)
		runEventReader(t, m, f)
		f.waitForEventStream(t)

		if err := m.PlayFromIndex(0); err != nil {
			t.Fatalf("PlayFromIndex(0): %v", err)
		}
		wantTrackChange(t, changes, queue, 0, "first selection")
		f.waitFor(t, "the successor to be prefetched", func() bool {
			tracked, _ := prefetchTracking(m)
			return tracked.index == 1
		})

		// mpv plays row 0 out and continues into the successor already in its
		// playlist. The reader promotes it, then parks reporting it.
		gate := f.block(t, cmdGetProperty)
		f.finishCurrent()
		gate.waitArrived(t, "the position sync of the gapless start-file")

		// Publish a newer row as current the way a replacement's commit does,
		// directly rather than through PlayFromIndex: the EOF maintenance holds
		// the playlist transaction lock while it waits behind the same parked
		// command, so a real selection could not reach its commit here.
		m.mu.Lock()
		m.setCurrentIndexLocked(3)
		m.mu.Unlock()

		f.unblock(cmdGetProperty)

		wantTrackChange(t, changes, queue, 1, "the parked gapless start-file")
	})
}
