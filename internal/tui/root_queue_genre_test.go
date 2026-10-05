package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/metadata"
)

func TestQueueGenreSurvivesEnqueueEditUndoAndWatch(t *testing.T) {
	m := New(context.Background())
	track := library.Track{Hash: "h", Path: "/song.flac", Title: "Song", Genre: "Jazz", Duration: 183.25}
	m.library.Index.Insert(track)
	m.queue.Entries = newQueueEntries([]library.Track{track}, []int64{1}, "")
	m = m.handleTrackEnqueuedMsg(libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, InsertAfter: -1})
	m.queue.Filter.Text = "g:Jazz"
	if len(m.queue.FilteredEntries()) != 2 {
		t.Fatal("single and batch enqueue must both carry genre")
	}
	m.queue.Cursor = 1
	ed := metadata.NewEditor(track.Hash, track.Title, "", "", "Rock", track.Duration)
	m.overlay.metadataEditor = &ed
	next, _ := m.handleMetadataSave()
	m = next.(Model)
	if len(m.queue.FilteredEntries()) != 0 || m.queue.Cursor != 0 {
		t.Fatal("genre edit did not update filtered queue and clamp cursor")
	}
	m = m.handleUndo()
	if len(m.queue.FilteredEntries()) != 2 {
		t.Fatal("undo did not restore genre matches")
	}
	track.Genre = "Blues"
	m = m.watchSyncQueueMetadata([]library.Track{track})
	if len(m.queue.FilteredEntries()) != 0 {
		t.Fatal("watch update left stale genre matches")
	}
	if m.queue.Entries[0].ID != 1 || m.queue.Entries[1].ID != 2 {
		t.Fatal("metadata filtering changed queue identity")
	}
}

func TestSingleTrackResetThenSavePreservesPreciseDuration(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h", Title: "Song", Duration: 183.25})
	ed := metadata.NewEditor("h", "Song", "", "", "", 183.25)
	// Keep a title edit, but undo an accidental duration change within the form.
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	for range 4 {
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	}
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: '9', Text: "9"})
	m.overlay.metadataEditor = &ed
	next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(Model)
	next, _ = m.handleMetadataSave()
	m = next.(Model)
	track, _ := m.library.Index.FindByHash("h")
	if track.Title != "Songx" || track.Duration != 183.25 {
		t.Fatalf("save lost other edits or duration precision: %+v", track)
	}
}
