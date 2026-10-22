package tui

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

func TestUndoTombstoneSurvivesOldTimeoutAndBackgroundRemoval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, d := newRadioBatchDBModel(t)
		w := m.undoer.(*db.Writer)
		tracks := sampleLibraryTracks()
		ids, err := w.AppendTracks(tracks[1:2], "")
		if err != nil {
			t.Fatal(err)
		}
		m.queue.Entries = append(m.queue.Entries, newQueueEntries(tracks[1:2], ids, "")...)
		removedID := m.queue.Entries[0].ID
		m = m.handleDequeueTrackMsg(queue.DequeueTrackMsg{Index: 0})
		// Advance past the former timer interval so any scheduled cleanup runs,
		// without waiting ten wall-clock seconds or merely changing the banner.
		time.Sleep(undoNotificationDuration + 50*time.Millisecond)
		synctest.Wait()
		if !m.canUndo() {
			t.Fatal("undo expired with its banner")
		}
		m, cmd := m.handleMissingTrackSelectedMsg(playback.MissingTrackSelectedMsg{Index: 0, Path: m.queue.Entries[0].Path})
		if cmd != nil {
			cmd()
		}
		m = m.handleUndo()
		rows := assertActiveQueueLen(t, d, 1)
		if rows[0].ID != removedID {
			t.Fatalf("restored ID %d, want %d", rows[0].ID, removedID)
		}
	})
}

func TestReplacingUndoPurgesOldRowsBeforeNewDelete(t *testing.T) {
	m, d := newRadioBatchDBModel(t)
	w := m.undoer.(*db.Writer)
	tracks := sampleLibraryTracks()
	ids, err := w.AppendTracks(tracks[1:2], "")
	if err != nil {
		t.Fatal(err)
	}
	m.queue.Entries = append(m.queue.Entries, newQueueEntries(tracks[1:2], ids, "")...)
	oldID, newID := m.queue.Entries[0].ID, m.queue.Entries[1].ID
	m = m.handleDequeueTrackMsg(queue.DequeueTrackMsg{Index: 0})
	m = m.handleDequeueTrackMsg(queue.DequeueTrackMsg{Index: 0})
	var count int
	if err := d.QueryRow("SELECT count(*) FROM queue WHERE id = ?", oldID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("superseded undo tombstone was not reclaimed")
	}
	m = m.handleUndo()
	rows := assertActiveQueueLen(t, d, 1)
	if rows[0].ID != newID {
		t.Fatal("cleanup removed the new undo's row")
	}
}
