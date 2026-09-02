package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestLibraryEnterEnqueuesSelectionAsBatch(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "h1", Path: "/one.flac"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two", Hash: "h2", Path: "/two.flac"})
	m.library.index.Insert(library.Track{Artist: "C", Title: "Focused", Hash: "h3", Path: "/focused.flac"})
	m.library.cursor = 2
	m.library.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}
	m.library.filter = filter.State{Text: "selected"}
	m.visual = visualState{active: true, view: viewLibrary}

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm := next.(Model)
	if cmd == nil {
		t.Fatal("Enter with a Library selection must dispatch a batch enqueue")
	}
	if len(qa.tracks) != 0 {
		t.Fatalf("single-track appender called %d times; want 0", len(qa.tracks))
	}
	if len(nm.library.selectedHashes) != 0 {
		t.Fatalf("selection must clear when batch is dispatched, got %v", nm.library.selectedHashes)
	}
	if nm.library.filter.Text != "" {
		t.Fatalf("committed filter must clear after enqueue, got %q", nm.library.filter.Text)
	}
	if nm.visual.active {
		t.Fatal("batch enqueue must exit visual mode")
	}

	rawMsg := cmd()
	msg, ok := rawMsg.(BatchEnqueuedMsg)
	if !ok {
		t.Fatalf("enqueue command returned %T, want BatchEnqueuedMsg", rawMsg)
	}
	if len(be.appendTracksCalls) != 1 {
		t.Fatalf("AppendTracks calls = %d, want 1", len(be.appendTracksCalls))
	}
	assertTrackHashes(t, be.appendTracksCalls[0], "h1", "h2")
	if len(be.appendTracksSources) != 1 || be.appendTracksSources[0] != "" {
		t.Fatalf("persisted queue source = %v, want one empty source", be.appendTracksSources)
	}
	if !msg.WasEmpty || msg.PlaylistName != "Queue" {
		t.Fatalf("batch metadata = WasEmpty:%v PlaylistName:%q", msg.WasEmpty, msg.PlaylistName)
	}

	queued, followup := nm.Update(msg)
	if followup != nil {
		t.Fatalf("BatchEnqueuedMsg returned unexpected follow-up command %T", followup)
	}
	qm := queued.(Model)
	if len(qm.queue.entries) != 2 {
		t.Fatalf("queue entries = %d, want 2", len(qm.queue.entries))
	}
	if qm.queue.entries[0].Hash != "h1" || qm.queue.entries[1].Hash != "h2" {
		t.Fatalf("queue hashes = [%q %q], want [h1 h2]", qm.queue.entries[0].Hash, qm.queue.entries[1].Hash)
	}
}

func TestLibraryEnterSelectionRespectsSortOrder(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithBatchEnqueuer(be))
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Hash: "hb"})
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Hash: "ha"})
	m.library.sortedTracks = sorting.SortTracks(m.library.index.Tracks(), sorting.SortTitle, sorting.SortAsc)
	m.library.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.library.selectedHashes = map[string]struct{}{"ha": {}, "hb": {}}

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter with sorted selection must dispatch a batch enqueue")
	}
	rawMsg := cmd()
	if _, ok := rawMsg.(BatchEnqueuedMsg); !ok {
		t.Fatalf("enqueue command returned %T, want BatchEnqueuedMsg", rawMsg)
	}
	if len(be.appendTracksCalls) != 1 {
		t.Fatalf("AppendTracks calls = %d, want 1", len(be.appendTracksCalls))
	}
	assertTrackHashes(t, be.appendTracksCalls[0], "ha", "hb")
}

func TestLibraryEnterSelectionUnavailablePreservesState(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "h1"})
	m.library.selectedHashes = map[string]struct{}{"h1": {}}
	m.library.filter = filter.State{Text: "one"}

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm := next.(Model)
	if cmd != nil {
		t.Fatalf("missing batch enqueuer returned command %T", cmd)
	}
	if len(nm.library.selectedHashes) != 1 || nm.library.filter.Text != "one" {
		t.Fatalf("selection/filter changed when enqueue unavailable: selection=%v filter=%q", nm.library.selectedHashes, nm.library.filter.Text)
	}
	if got := nm.statusBar.TopNotification().Msg; !strings.Contains(got, "Enqueue not available") {
		t.Fatalf("notification = %q, want enqueue unavailable", got)
	}
}

func TestLibraryEnterSelectionBatchFailureLeavesQueueUnchanged(t *testing.T) {
	be := &stubBatchEnqueuer{appendTracksErr: errors.New("db locked")}
	m := New(context.Background(), WithBatchEnqueuer(be))
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "h1"})
	m.library.selectedHashes = map[string]struct{}{"h1": {}}
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "existing"}}

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm := next.(Model)
	if cmd == nil {
		t.Fatal("Enter with a selection must dispatch a batch enqueue")
	}
	msg, ok := cmd().(BatchEnqueuedMsg)
	if !ok || msg.Err == nil {
		t.Fatalf("enqueue command returned %#v, want failed BatchEnqueuedMsg", msg)
	}

	failed, _ := nm.Update(msg)
	fm := failed.(Model)
	if len(fm.queue.entries) != 1 || fm.queue.entries[0].Hash != "existing" {
		t.Fatalf("queue changed after failed batch: %+v", fm.queue.entries)
	}
	got := fm.statusBar.TopNotification().Msg
	if !strings.Contains(got, "Enqueue failed") || strings.Contains(got, "playlist") {
		t.Fatalf("failure notification = %q", got)
	}
}

func assertTrackHashes(t *testing.T, tracks []library.Track, want ...string) {
	t.Helper()
	if len(tracks) != len(want) {
		t.Fatalf("track count = %d, want %d", len(tracks), len(want))
	}
	for i, hash := range want {
		if tracks[i].Hash != hash {
			t.Fatalf("track %d hash = %q, want %q", i, tracks[i].Hash, hash)
		}
	}
}
