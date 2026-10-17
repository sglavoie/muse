package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestPlaylistUndoFailurePreservesStateAndRetry(t *testing.T) {
	for _, action := range []undoAction{
		undoDeletePlaylistAction{entry: db.PlaylistRow{ID: 99, Name: "Restored"}},
		undoRemovePlaylistTrackAction{entry: db.PlaylistTrackRow{ID: 99, Hash: "restored"}},
		undoBulkRemovePlaylistTracksAction{entries: []db.PlaylistTrackRow{{ID: 99, Hash: "restored"}}, indices: []int{0}},
	} {
		t.Run(action.description(), func(t *testing.T) {
			pd := &stubPlaylistDeleter{undoErr: errors.New("database unavailable")}
			m := newRichFixture()
			m.playlistDeleter = pd
			before := m.playlists
			m.setUndoSlot(action, func() { t.Fatal("cleanup ran while undo is retained") })
			expires := m.undo.slot.expiresAt
			m = m.handleUndo()
			if !reflect.DeepEqual(m.playlists, before) {
				t.Fatal("failed undo changed playlists")
			}
			assertUndoFailureRetryable(t, m, expires)
			if m.undo.slot.cleanup == nil {
				t.Fatal("playlist cleanup not retained")
			}
			pd.undoErr = nil
			m = m.handleUndo()
			if m.undo.slot.action != nil || !strings.Contains(m.statusBar.TopNotification().Msg, "Undone:") {
				t.Fatal("retry failed")
			}
		})
	}
}

func TestTagUndoFailurePreservesStateAndRetry(t *testing.T) {
	for _, action := range []undoAction{
		undoDeleteTagAction{tag: db.TagRow{ID: 99, Name: "Restored"}},
		undoTagToggleAction{tagID: 1, hashes: []string{"rh1"}, wasAdd: true},
		undoTagToggleAction{tagID: 1, hashes: []string{"rh1"}, wasAdd: false},
	} {
		t.Run(action.description(), func(t *testing.T) {
			tw := &stubTagWriter{undoErr: errors.New("database unavailable")}
			m := newRichFixture()
			m.tagWriter = tw
			before := m.tagsView
			var cancel func()
			if action.kind() == undoKindDeleteTag {
				cancel = func() { t.Fatal("cleanup ran while undo is retained") }
			}
			m.setUndoSlot(action, cancel)
			expires := m.undo.slot.expiresAt
			m = m.handleUndo()
			if !reflect.DeepEqual(m.tagsView, before) {
				t.Fatal("failed undo changed tags")
			}
			assertUndoFailureRetryable(t, m, expires)
			if cancel != nil && m.undo.slot.cleanup == nil {
				t.Fatal("tag cleanup not retained")
			}
			tw.undoErr = nil
			m = m.handleUndo()
			if m.undo.slot.action != nil || !strings.Contains(m.statusBar.TopNotification().Msg, "Undone:") {
				t.Fatal("retry failed")
			}
		})
	}
}

func assertUndoFailureRetryable(t *testing.T, m Model, expires time.Time) {
	t.Helper()
	if m.undo.slot.action == nil || m.undo.slot.expiresAt != expires {
		t.Fatal("undo lost its original retry window")
	}
	msg := m.statusBar.TopNotification().Msg
	if !strings.Contains(msg, "Undo failed:") || !strings.Contains(msg, "database unavailable") {
		t.Fatalf("notification: %q", msg)
	}
	history := m.statusBar.RecentMessages()
	if history[len(history)-1].Text != msg {
		t.Fatal("error missing from Recent Messages")
	}
}
