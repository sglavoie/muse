package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/queue"
)

func precisionMetadataModel() Model {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Title: "Song", Artist: "Artist", Duration: 183.7})
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "h1", Title: "Song", Artist: "Artist", DurationMs: 183700}}
	ed := metadata.NewEditor("h1", "Song", "Artist", "", "", 183.7)
	m.overlay.metadataEditor = &ed
	return m
}

func TestBulkMixedMetadataOnlySavesEditedFields(t *testing.T) {
	m := precisionMetadataModel()
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Title: "Other", Artist: "Artist", Duration: 200.9})
	ed := m.bulkEditorForHashes([]string{"h1", "h2"})
	m.overlay.metadataEditor = &ed
	result, cmd := m.handleMetadataSave()
	if cmd != nil || result.(Model).overlay.metadataEditor != nil {
		t.Fatal("untouched mixed fields must close without saving")
	}
	// Change only the shared artist; mixed titles and durations stay intact.
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m.overlay.metadataEditor = &ed
	m = typeIntoEditor(m, "!")
	result, cmd = m.handleMetadataSave()
	nm := result.(Model)
	if cmd == nil {
		t.Fatal("bulk artist edit should save")
	}
	if msg := cmd().(metadataEditedMsg); msg.err != nil {
		t.Fatal(msg.err)
	}
	for hash, title := range map[string]string{"h1": "Song", "h2": "Other"} {
		track, _ := nm.library.Index.FindByHash(hash)
		if track.Title != title || track.Artist != "Artist!" || track.Album != "" {
			t.Fatalf("unexpected bulk edit for %s: %+v", hash, track)
		}
	}
	nm = nm.handleUndo()
	for hash, duration := range map[string]float64{"h1": 183.7, "h2": 200.9} {
		track, _ := nm.library.Index.FindByHash(hash)
		if track.Artist != "Artist" || track.Duration != duration {
			t.Fatalf("bulk undo did not restore %s precisely: %+v", hash, track)
		}
	}
}

func TestUnchangedMetadataSavePreservesUndoAndDuration(t *testing.T) {
	for _, revert := range []string{"none", "backspace", "reset"} {
		m := precisionMetadataModel()
		writer := newImportWriterSpy()
		durations := &stubDurationWriter{}
		m.metadataWriter, m.durationWriter = writer, durations
		previous := &undoBulkEditMetadataAction{originals: map[string][5]string{"prior": {"Old"}}}
		m.setUndoSlot(previous, nil)
		expires := m.undo.slot.expiresAt
		if revert != "none" {
			m = typeIntoEditor(m, "x")
			if revert == "reset" {
				next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
				m = next.(Model)
			} else {
				m = backspaceInEditor(m, 1)
			}
		}
		result, cmd := m.handleMetadataSave()
		nm := result.(Model)
		if cmd != nil || nm.overlay.metadataEditor != nil {
			t.Fatal("unchanged save should close the editor without a write command")
		}
		if nm.undo.slot.action != previous || nm.undo.slot.expiresAt != expires {
			t.Fatal("unchanged save replaced or extended the previous undo")
		}
		track, _ := nm.library.Index.FindByHash("h1")
		if track.Duration != 183.7 || nm.queue.Entries[0].DurationMs != 183700 {
			t.Fatal("unchanged save lost duration precision")
		}
		if nm.statusBar.TopNotification().Msg != "No changes" {
			t.Fatalf("notification = %q", nm.statusBar.TopNotification().Msg)
		}
		meta, _ := writer.snapshot()
		if len(meta) != 0 || len(durations.calls) != 0 {
			t.Fatal("unchanged save wrote to persistence")
		}
	}
}

func TestMetadataTitleEditPreservesDuration(t *testing.T) {
	m := precisionMetadataModel()
	writer := newImportWriterSpy()
	durations := &stubDurationWriter{}
	m.metadataWriter, m.durationWriter = writer, durations
	m = typeIntoEditor(m, "!")
	result, cmd := m.handleMetadataSave()
	nm := result.(Model)
	if cmd == nil {
		t.Fatal("title edit should persist")
	}
	if msg := cmd().(metadataEditedMsg); msg.err != nil {
		t.Fatal(msg.err)
	}
	track, _ := nm.library.Index.FindByHash("h1")
	meta, _ := writer.snapshot()
	if track.Title != "Song!" || meta["h1"][0] != "Song!" {
		t.Fatal("title edit was not applied and persisted")
	}
	if track.Duration != 183.7 || nm.queue.Entries[0].DurationMs != 183700 || len(durations.calls) != 0 {
		t.Fatal("title-only edit changed or persisted duration")
	}
	// No background writes are needed to verify the undo's in-memory precision.
	nm.metadataWriter, nm.durationWriter = nil, nil
	nm = nm.handleUndo()
	track, _ = nm.library.Index.FindByHash("h1")
	if track.Title != "Song" || track.Duration != 183.7 || nm.queue.Entries[0].DurationMs != 183700 {
		t.Fatal("undo failed to restore the precise original track")
	}
}

func TestMetadataExplicitDurationEdits(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int64
	}{{"4:00", 240000}, {"90", 90000}, {"", 0}, {"1:99", -1}} {
		t.Run(tc.input, func(t *testing.T) {
			m := precisionMetadataModel()
			durations := &stubDurationWriter{}
			m.durationWriter = durations
			m.overlay.metadataEditor.Fields[metadata.FieldDuration].SetValue(tc.input)
			result, cmd := m.handleMetadataSave()
			nm := result.(Model)
			track, _ := nm.library.Index.FindByHash("h1")
			if tc.want < 0 {
				if cmd != nil || nm.overlay.metadataEditor == nil || track.Duration != 183.7 {
					t.Fatal("invalid duration must keep the editor open and track unchanged")
				}
				return
			}
			if cmd == nil {
				t.Fatal("duration edit should persist")
			}
			if msg := cmd().(metadataEditedMsg); msg.err != nil {
				t.Fatal(msg.err)
			}
			if int64(track.Duration*1000) != tc.want || nm.queue.Entries[0].DurationMs != tc.want || len(durations.calls) != 1 || durations.calls[0].durationMs != tc.want {
				t.Fatal("duration edit did not update library, queue, and persistence")
			}
			nm.durationWriter = nil
			nm = nm.handleUndo()
			track, _ = nm.library.Index.FindByHash("h1")
			if track.Duration != 183.7 {
				t.Fatalf("undo duration = %v, want 183.7", track.Duration)
			}
		})
	}
}

func TestBulkMetadataSecondsPersistAndUndoPrecisely(t *testing.T) {
	m := precisionMetadataModel()
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Duration: 200.9})
	durations := &stubDurationWriter{}
	m.durationWriter = durations
	ed := m.bulkEditorForHashes([]string{"h1", "h2"})
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m.overlay.metadataEditor = &ed
	m = typeIntoEditor(m, "90")
	result, cmd := m.handleMetadataSave()
	m = result.(Model)
	if cmd == nil {
		t.Fatal("bulk seconds edit did not produce a save")
	}
	if msg := cmd().(metadataEditedMsg); msg.err != nil {
		t.Fatal(msg.err)
	}
	if len(durations.calls) != 2 {
		t.Fatalf("duration writes = %d, want 2", len(durations.calls))
	}
	for _, call := range durations.calls {
		if call.durationMs != 90000 {
			t.Fatalf("persisted duration = %d, want 90000", call.durationMs)
		}
	}
	m.durationWriter = nil
	m = m.handleUndo()
	for hash, want := range map[string]float64{"h1": 183.7, "h2": 200.9} {
		track, _ := m.library.Index.FindByHash(hash)
		if track.Duration != want {
			t.Fatalf("undo %s duration = %v, want %v", hash, track.Duration, want)
		}
	}
}
