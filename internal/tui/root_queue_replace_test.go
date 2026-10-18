package tui

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
)

type replacementStore struct {
	uitestQueueStore
	err      error
	undoErr  error
	tracks   []library.Track
	source   string
	restored []int64
}

func (s *replacementStore) ReplaceQueue(_ []int64, tracks []library.Track, source string) ([]int64, error) {
	s.tracks, s.source = slices.Clone(tracks), source
	if s.err != nil {
		return nil, s.err
	}
	return s.AppendTracks(tracks, source)
}
func (s *replacementStore) UndoReplaceQueue(_, restore []int64) error {
	if s.undoErr != nil {
		return s.undoErr
	}
	s.restored = slices.Clone(restore)
	return nil
}

func TestQueueReplacementPreservesStateUntilCommitAndSupportsUndoRetry(t *testing.T) {
	store := &replacementStore{uitestQueueStore: uitestQueueStore{nextID: 100}, err: errors.New("disk full")}
	p := &stubCommandPlayer{}
	m := newRichFixture()
	m.queueWriter, m.undoer, m.player = store, store, p
	m.queue.Filter = filter.State{Text: "old filter"}
	old := slices.Clone(m.queue.Entries)
	m.playback.currentIndex, m.playback.currentHash, m.playback.state = 1, old[1].Hash, playback.StatePaused
	m.playback.elapsedSecs = 42
	cleaned := false
	m.setUndoSlot(&undoClearAction{}, func() { cleaned = true })
	prior := m.undo.slot.action
	next, cmd := m.handleKeyAction(keys.ActionLibraryEnqueueReplace)
	m = next.(Model)
	if cmd == nil || p.stopCalls != 0 || !reflect.DeepEqual(old, m.queue.Entries) {
		t.Fatal("replacement changed state before saving")
	}
	if _, again := m.handleKeyAction(keys.ActionLibraryEnqueueReplace); again != nil {
		t.Fatal("duplicate replacement dispatched")
	}
	m = m.handleQueueReplace(cmd().(queueReplaceRequestedMsg))
	if p.stopCalls != 0 || cleaned || m.undo.slot.action != prior || !reflect.DeepEqual(old, m.queue.Entries) || m.playback.state != playback.StatePaused {
		t.Fatal("failed save changed playback, queue, or undo")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "Replace queue failed") {
		t.Fatal("failure not surfaced")
	}
	store.err = nil
	next, cmd = m.handleKeyAction(keys.ActionLibraryEnqueueReplace)
	m = next.(Model).handleQueueReplace(cmd().(queueReplaceRequestedMsg))
	if cleaned || len(m.queue.Entries) != 1 || m.queue.Filter.Text != "" {
		t.Fatal("replacement failed or purged restorable rows")
	}
	replacement := slices.Clone(m.queue.Entries)
	store.undoErr = errors.New("busy")
	m = m.handleUndo()
	if !reflect.DeepEqual(replacement, m.queue.Entries) || !m.canUndo() || cleaned {
		t.Fatal("failed undo changed queue or lost retry")
	}
	store.undoErr = nil
	m = m.handleUndo()
	if !reflect.DeepEqual(old, m.queue.Entries) || !reflect.DeepEqual(store.restored, queueEntryIDs(old)) || !cleaned || m.canUndo() {
		t.Fatal("undo did not restore original entries and release old cleanup")
	}
	if m.queue.Filter.Text != "old filter" || m.playback.state != playback.StatePaused || m.playback.currentIndex != 1 || m.playback.elapsedSecs != 42 {
		t.Fatal("undo lost queue context or paused position")
	}
}

func TestQueueReplacementSelectionFailureRetainsDraft(t *testing.T) {
	store := &replacementStore{err: errors.New("write failed")}
	m := newRichFixture()
	m.queueWriter = store
	m.library.SelectedHashes = map[string]struct{}{"rh1": {}, "rh2": {}}
	m.library.Filter = filter.State{Text: "no matches"}
	m.library = m.library.RebuildFilterCache()
	next, cmd := m.handleKeyAction(keys.ActionLibraryEnqueueReplace)
	if cmd == nil {
		t.Fatal("hidden selection unavailable")
	}
	m = next.(Model).handleQueueReplace(cmd().(queueReplaceRequestedMsg))
	if m.activeViewSelectionCount() != 2 || m.library.Filter.Text != "no matches" {
		t.Fatal("failed replacement consumed selection/filter")
	}
	store.err = nil
	next, cmd = m.handleKeyAction(keys.ActionLibraryEnqueueReplace)
	m = next.(Model).handleQueueReplace(cmd().(queueReplaceRequestedMsg))
	assertTrackHashes(t, store.tracks, "rh1", "rh2")
	if m.activeViewSelectionCount() != 0 || m.library.Filter.Text != "no matches" {
		t.Fatal("successful replacement must consume selection and keep filter")
	}
}

func TestSelectedPlayNextInsertsWholeBatchInOrder(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := New(t.Context(), WithBatchEnqueuer(be))
	m.library.Index.Insert(library.Track{Hash: "one", Title: "One"})
	m.library.Index.Insert(library.Track{Hash: "two", Title: "Two"})
	m.library.SelectedHashes = map[string]struct{}{"one": {}, "two": {}}
	m.library.Filter = filter.State{Text: "no match"}
	m.queue.Entries = []queue.Entry{{ID: 50, Hash: "old"}, {ID: 51, Hash: "tail"}}
	m.playback.currentIndex = 0
	next, cmd := m.handleKeyAction(keys.ActionPlayNext)
	if cmd == nil {
		t.Fatal("missing selected play-next")
	}
	result := cmd().(BatchEnqueuedMsg)
	m = next.(Model).handleBatchEnqueuedMsg(result)
	hashes := []string{}
	for _, e := range m.queue.Entries {
		hashes = append(hashes, e.Hash)
	}
	if !reflect.DeepEqual(hashes, []string{"old", "one", "two", "tail"}) {
		t.Fatalf("order: %v", hashes)
	}
	if !strings.Contains(m.undo.slot.action.description(), "Queued next: 2 tracks") {
		t.Fatal("wrong feedback")
	}
	m = m.handleUndo()
	if len(m.queue.Entries) != 2 || m.queue.Entries[0].Hash != "old" || m.queue.Entries[1].Hash != "tail" {
		t.Fatal("undo did not remove batch")
	}
}

func TestPlaylistNextAndReplacePreserveDuplicateSelectionOrder(t *testing.T) {
	for _, action := range []keys.Action{keys.ActionPlayNext, keys.ActionLibraryEnqueueReplace} {
		store := &replacementStore{}
		be := &stubBatchEnqueuer{}
		m := New(t.Context(), WithQueueWriter(store), WithBatchEnqueuer(be))
		m.ActiveView = viewPlaylists
		m.playlists.Level = playlists.TrackLevel
		m.playlists.Playlists = []db.PlaylistRow{{ID: 1, Name: "Focus"}}
		m.playlists.Tracks = []db.PlaylistTrackRow{{ID: 1, Hash: "same"}, {ID: 2, Hash: "other"}, {ID: 3, Hash: "same"}}
		m.playlists.SelectedTrackIDs = map[int64]struct{}{1: {}, 2: {}, 3: {}}
		m.playlists.SortedTrackOrder = []int{2, 0, 1}
		m.playlists.Filter = filter.State{Text: "no results"}
		next, cmd := m.handleKeyAction(action)
		if cmd == nil {
			t.Fatal("no command")
		}
		if action == keys.ActionPlayNext {
			msg := cmd().(BatchEnqueuedMsg)
			assertTrackHashes(t, be.appendTracksCalls[0], "same", "same", "other")
			if !msg.PlayNext || be.appendTracksSources[0] != "Focus" {
				t.Fatal("wrong placement or source")
			}
		} else {
			next.(Model).handleQueueReplace(cmd().(queueReplaceRequestedMsg))
			assertTrackHashes(t, store.tracks, "same", "same", "other")
			if store.source != "Focus" {
				t.Fatal("source lost")
			}
		}
	}
}

func TestReplacementRefusesPendingPlaybackWithoutCancelingIt(t *testing.T) {
	m := newRichFixture()
	store := &replacementStore{}
	m.queueWriter = store
	req := m.playback.requests.dispatch(m.playback.requestSeq)
	m.playback.requestSeq = req
	old := slices.Clone(m.queue.Entries)
	m = m.handleQueueReplace(queueReplaceRequestedMsg{tracks: []library.Track{{Hash: "rh1"}}})
	if len(store.tracks) != 0 || !reflect.DeepEqual(old, m.queue.Entries) {
		t.Fatal("replacement ran during pending playback")
	}
	ran := false
	_, accepted := m.playback.requests.run(req, func() error { ran = true; return nil })
	if !ran || !accepted {
		t.Fatal("failed replacement canceled previous playback gesture")
	}
	// The player call finished, but the model still awaits its result. Its
	// optimistic current row must not become the saved playback position.
	m.playback.pendingSelection = true
	m = m.handleQueueReplace(queueReplaceRequestedMsg{tracks: []library.Track{{Hash: "rh1"}}})
	if len(store.tracks) != 0 || !reflect.DeepEqual(old, m.queue.Entries) || !m.playback.pendingSelection {
		t.Fatal("replacement ran before playback acknowledgement")
	}
}
