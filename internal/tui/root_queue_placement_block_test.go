package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// A shuffle timeline is a non-positional display: the panel is showing the order
// the player will visit rows in, so the row drawn at position 3 is not the queue
// row stored at position 3. Position-keyed edits — K/J and cursor-position paste
// — must refuse while it is up, and must say so in terms of the shuffle order
// rather than of a sort the user never applied. Everything that addresses a row
// by identity rather than by position stays available.

// placementModel returns a Queue-focused model over a four-row queue whose
// player traverses it in the given order.
func placementModel(t *testing.T, order []int) (Model, *fakeTraversalPlayer) {
	t.Helper()
	p := newFakeTraversalPlayer(order, 0)
	qa := &stubQueueAppender{}
	m := newModelWithTracks()
	m.queueWriter = qa
	m.player = p
	m.queue.Traversal = queueTraversalSource(p)
	m.queue.Entries = sampleQueueEntries()
	m.ActiveView = viewQueue
	return m, p
}

// cmdMsg runs cmd and asserts it produced a message of type T, so a caller can
// go straight to the fields it wants to check.
func cmdMsg[T tea.Msg](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	var want T
	if cmd == nil {
		t.Fatalf("expected a command producing %T, got none", want)
	}
	msg, ok := cmd().(T)
	if !ok {
		t.Fatalf("expected %T, got %T", want, cmd())
	}
	return msg
}

// notifyFromCmd runs cmd and feeds whatever it produced back into the model, so
// a transient notification command becomes a visible status-bar message.
func notifyFromCmd(t *testing.T, m Model, cmd tea.Cmd) string {
	t.Helper()
	if cmd == nil {
		return ""
	}
	next, _ := m.Update(cmd())
	return next.(Model).statusBar.TopNotification().Msg
}

// TestPlacementEditsRefuseUnderShuffleTimeline covers both position-keyed edits
// under the timeline, and checks the message names the shuffle order.
func TestPlacementEditsRefuseUnderShuffleTimeline(t *testing.T) {
	moves := []struct {
		name   string
		action keys.Action
	}{
		{"move up", keys.ActionMoveTrackUp},
		{"move down", keys.ActionMoveTrackDown},
	}
	for _, tc := range moves {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := placementModel(t, []int{2, 0, 3, 1})
			m.queue.Cursor = 1

			rm, cmd, handled := m.handleQueueKeyAction(tc.action)
			if !handled {
				t.Fatal("the move must be handled")
			}
			msg := notifyFromCmd(t, rm.(Model), cmd)
			if !strings.Contains(msg, "Cannot reorder while showing shuffle order") {
				t.Errorf("notification = %q, want it to refuse and name the shuffle order", msg)
			}
		})
	}

	t.Run("paste", func(t *testing.T) {
		m, _ := placementModel(t, []int{2, 0, 3, 1})
		m.yankBuffer = []queue.Entry{m.queue.Entries[0]}
		m.queue.Cursor = 1

		rm, cmd, handled := m.handleQueueKeyAction(keys.ActionPaste)
		if !handled {
			t.Fatal("the paste must be handled")
		}
		if cmd != nil {
			t.Fatal("a refused paste must not run the append command")
		}
		msg := rm.(Model).statusBar.TopNotification().Msg
		if !strings.Contains(msg, "Cannot paste while showing shuffle order") {
			t.Errorf("notification = %q, want it to refuse and name the shuffle order", msg)
		}
	})
}

// TestPlacementRefusalNamesSortRatherThanShuffle guards the copy the previous
// behaviour already had: a sort or a filter still reports itself as such, so the
// new shuffle wording cannot swallow the old cases.
func TestPlacementRefusalNamesSortRatherThanShuffle(t *testing.T) {
	tests := []struct {
		name  string
		apply func(m Model, p *fakeTraversalPlayer) Model
	}{
		{
			// The timeline is live underneath, and the sort must still win the
			// message: an explicit sort suppresses the timeline entirely.
			name: "explicit sort",
			apply: func(m Model, _ *fakeTraversalPlayer) Model {
				m.queue.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
				m.queue = m.queue.RecomputeDisplayOrder()
				return m
			},
		},
		{
			// A filter narrows rows without reordering them, so it cannot
			// suppress a live timeline; shuffle off is the only state where the
			// filter is the sole reason placement is blocked.
			name: "text filter",
			apply: func(m Model, p *fakeTraversalPlayer) Model {
				p.shuffled = false
				m.queue.Filter = filter.State{Active: true, Text: "Artist"}
				return m
			},
		},
		{
			name: "tag filter",
			apply: func(m Model, p *fakeTraversalPlayer) Model {
				p.shuffled = false
				m.queue.TagFilterHashes = map[string]struct{}{"h2": {}, "h3": {}}
				return m
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, p := placementModel(t, []int{2, 0, 3, 1})
			m = tc.apply(m, p)
			m.yankBuffer = []queue.Entry{m.queue.Entries[0]}
			m.queue.Cursor = 1

			rm, _, _ := m.handleQueueKeyAction(keys.ActionPaste)
			msg := rm.(Model).statusBar.TopNotification().Msg
			if !strings.Contains(msg, "Cannot paste while sorted/filtered") {
				t.Errorf("notification = %q, want the existing sorted/filtered wording", msg)
			}
		})
	}
}

// TestPlacementRefusalPrefersShuffleOverFilter pins the tie-break: a filter
// leaves the timeline ordering the rows, so the refusal names the order the
// user is looking at rather than the narrowing they applied.
func TestPlacementRefusalPrefersShuffleOverFilter(t *testing.T) {
	m, _ := placementModel(t, []int{2, 0, 3, 1})
	m.queue.Filter = filter.State{Active: true, Text: "Artist"}
	m.queue.Cursor = 1

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionMoveTrackUp)
	msg := notifyFromCmd(t, m, cmd)
	if !strings.Contains(msg, "Cannot reorder while showing shuffle order") {
		t.Errorf("notification = %q, want the shuffle order to win over the filter", msg)
	}
}

// TestPlacementEditsAllowedOnceShuffleIsOff verifies the guard is scoped to the
// timeline being up: with shuffle disabled the panel is persisted order again
// and both edits work on the cursor position exactly as before.
func TestPlacementEditsAllowedOnceShuffleIsOff(t *testing.T) {
	m, p := placementModel(t, []int{0, 1, 2, 3})
	p.shuffled = false
	m.queue.Cursor = 2

	if reason := m.queue.PlacementBlockReason(); reason != "" {
		t.Fatalf("placement block reason = %q, want none with shuffle off", reason)
	}

	rm, cmd, handled := m.handleQueueKeyAction(keys.ActionMoveTrackUp)
	if !handled {
		t.Fatal("the move must be handled")
	}
	if move := cmdMsg[queue.MoveTrackUpMsg](t, cmd); move.Index != 2 {
		t.Errorf("move index = %d, want the cursor position 2", move.Index)
	}

	m = rm.(Model)
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}
	_, pasteCmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	if pasted := cmdMsg[QueuePastedMsg](t, pasteCmd); pasted.InsertPos != 3 {
		t.Errorf("insert position = %d, want just after the cursor (3)", pasted.InsertPos)
	}
}

// TestIdentityKeyedQueueActionsSurviveTheTimeline proves the block is as narrow
// as it claims: play, dequeue, bulk selection, filtering, and sorting all
// address rows by identity or rebuild the display, so none of them are refused
// while the timeline is up.
func TestIdentityKeyedQueueActionsSurviveTheTimeline(t *testing.T) {
	stubFileExistsTrue(t)
	order := []int{2, 0, 3, 1}

	t.Run("play targets the underlying row", func(t *testing.T) {
		m, _ := placementModel(t, order)
		m.queue.Cursor = 0 // display row 0 is queue row 2
		_, cmd, _ := m.handleQueueKeyAction(keys.ActionSelect)
		if play := cmdMsg[playback.PlayQueueIndexMsg](t, cmd); play.Index != 2 {
			t.Errorf("play index = %d, want underlying row 2", play.Index)
		}
	})

	t.Run("dequeue targets the underlying row", func(t *testing.T) {
		m, _ := placementModel(t, order)
		m.queue.Cursor = 3 // display row 3 is queue row 1
		_, cmd, _ := m.handleQueueKeyAction(keys.ActionDequeue)
		if dq := cmdMsg[queue.DequeueTrackMsg](t, cmd); dq.Index != 1 {
			t.Errorf("dequeue index = %d, want underlying row 1", dq.Index)
		}
	})

	t.Run("bulk selection targets the underlying row", func(t *testing.T) {
		m, _ := placementModel(t, order)
		m.queue.Cursor = 2 // display row 2 is queue row 3
		_, cmd := m.queue.HandleAction(keys.ActionBulkSelectionToggle)
		if sel := cmdMsg[queue.BulkSelectionToggleMsg](t, cmd); sel.Index != 3 {
			t.Errorf("selection index = %d, want underlying row 3", sel.Index)
		}
	})

	t.Run("filtering still narrows the timeline", func(t *testing.T) {
		m, _ := placementModel(t, order)
		// Field-scoped: a bare "B" would also match every row's album.
		m.queue.Filter = filter.State{Active: true, Text: "t:B"}
		filtered := m.queue.FilteredEntries()
		if len(filtered) != 1 || filtered[0].Title != "B" {
			t.Fatalf("filtered rows = %d, want the single matching row", len(filtered))
		}
	})

	t.Run("an explicit sort replaces the timeline order", func(t *testing.T) {
		m, _ := placementModel(t, order)
		m.queue.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
		m.queue = m.queue.RecomputeDisplayOrder()
		got := make([]string, 0, len(m.queue.Entries))
		for _, e := range m.queue.FilteredEntries() {
			got = append(got, e.Title)
		}
		if strings.Join(got, "") != "DCBA" {
			t.Errorf("sorted titles = %v, want the explicit sort to win over the timeline", got)
		}
	})
}
