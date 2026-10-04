package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestQueueRestoreFailurePreservesStateAndAllowsRetry(t *testing.T) {
	removed := queue.Entry{ID: 20, Hash: "removed", Display: "Removed"}
	for _, action := range []undoAction{
		undoDequeueAction{id: 20, entry: removed, idx: 0},
		undoClearAction{ids: []int64{20}, entries: []queue.Entry{removed}, savedIndex: -1},
		undoBulkDequeueAction{ids: []int64{20}, entries: []queue.Entry{removed}, indices: []int{0}},
	} {
		t.Run(action.description(), func(t *testing.T) {
			d := &stubQueueDequeuer{undoErr: errors.New("database unavailable")}
			p := &stubPlayerQueueSyncer{}
			m := New(context.Background(), WithQueueDequeuer(d), WithPlayer(p))
			m.queue.Entries = []queue.Entry{{ID: 10, Hash: "playing", Selected: true}}
			m.playback.currentIndex = 0
			m.playback.currentHash = "playing"
			m.queue.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
			cancel := make(chan struct{})
			m.setUndoSlot(action, cancel)
			expires := m.undo.slot.expiresAt

			next := m.handleUndo()
			if !reflect.DeepEqual(next.queue.Entries, m.queue.Entries) || next.queue.SortState != m.queue.SortState {
				t.Fatal("failed restore changed visible queue or sort")
			}
			if next.playback.currentIndex != 0 || next.playback.currentHash != "playing" || len(p.setQueueCalls) != 0 || len(p.setCurrentIndexCalls) != 0 {
				t.Fatal("failed restore changed playback")
			}
			if next.undo.slot.action == nil || next.undo.slot.expiresAt != expires {
				t.Fatal("failed restore must retain the action and original expiry")
			}
			select {
			case <-cancel:
			default:
				t.Fatal("original cleanup was not cancelled")
			}
			if next.undo.slot.cancelPurge == nil || next.undo.slot.cancelPurge == cancel {
				t.Fatal("failed restore must re-arm cleanup")
			}
			msg := next.statusBar.TopNotification().Msg
			if !strings.Contains(msg, "Undo failed:") || !strings.Contains(msg, "database unavailable") {
				t.Fatalf("failure notification = %q", msg)
			}
			history := next.statusBar.RecentMessages()
			if history[len(history)-1].Text != msg {
				t.Fatal("failure missing from recent messages")
			}
			d.undoErr = nil
			retried := next.handleUndo()
			if retried.undo.slot.action != nil || !strings.Contains(retried.statusBar.TopNotification().Msg, "Undone:") {
				t.Fatal("successful retry did not complete undo")
			}
			if len(p.setQueueCalls) != 1 {
				t.Fatal("successful retry must sync the player once")
			}
			found := 0
			for _, entry := range retried.queue.Entries {
				if entry.ID == 20 {
					found++
				}
			}
			if found != 1 {
				t.Fatalf("restored row appears %d times", found)
			}
		})
	}
}

func TestQueueRestoreFailureCannotRetryAfterExpiry(t *testing.T) {
	d := &stubQueueDequeuer{undoErr: errors.New("database unavailable")}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.setUndoSlot(undoDequeueAction{id: 20}, nil)
	m = m.handleUndo()
	m.undo.slot.expiresAt = time.Now().Add(-time.Second)
	d.undoErr = nil
	m = m.handleUndo()
	if len(d.undoDequeueCallIDs) != 1 || m.statusBar.TopNotification().Msg != "Nothing to undo" {
		t.Fatal("failed undo was retried after expiry")
	}
}
