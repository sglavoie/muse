package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// TestQueueFilterSelectMapsToCorrectEntry verifies that selecting an entry
// while filter is active dispatches PlayQueueIndexMsg with the correct
// underlying entry index.
func TestQueueFilterSelectMapsToCorrectEntry(t *testing.T) {
	stubFileExistsTrue(t)
	q := queue.View{
		Entries: []queue.Entry{
			{ID: 1, Path: "/a.flac", Hash: "ha", Title: "Alpha", Artist: "A"},
			{ID: 2, Path: "/b.flac", Hash: "hb", Title: "Beta", Artist: "B"},
			{ID: 3, Path: "/c.flac", Hash: "hc", Title: "Gamma", Artist: "C"},
		},
	}
	q.Filter, _ = q.Filter.Activate()
	q.Filter.Text = "beta"
	q.Cursor = 0
	q.ScrollOffset = 0

	// Cursor 0 in filtered view should be "Beta" (real index 1).
	q.Cursor = 0
	_, cmd := q.HandleAction(keys.ActionSelect)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	pqi, ok := msg.(playback.PlayQueueIndexMsg)
	if !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
	if pqi.Index != 1 {
		t.Fatalf("expected real index 1, got %d", pqi.Index)
	}
}

// TestQueueFilterMoveDisabled verifies that move-up/move-down are disabled
// when a filter is active.
func TestQueueFilterMoveDisabled(t *testing.T) {
	q := queue.View{
		Entries: []queue.Entry{
			{Path: "/a.flac", Title: "Alpha", Artist: "A"},
			{Path: "/b.flac", Title: "Beta", Artist: "B"},
		},
		Cursor: 0,
	}
	q.Filter, _ = q.Filter.Activate()
	q.Filter.Text = "a" // still matches both
	q.Cursor = 0
	q.ScrollOffset = 0

	_, cmd := q.HandleAction(keys.ActionMoveTrackDown)
	if cmd == nil {
		t.Error("move-down when filtered should return a TransientNotifyMsg cmd")
	} else if _, ok := cmd().(statusbar.TransientNotifyMsg); !ok {
		t.Error("move-down when filtered: expected TransientNotifyMsg")
	}

	q.Cursor = 1
	_, cmd = q.HandleAction(keys.ActionMoveTrackUp)
	if cmd == nil {
		t.Error("move-up when filtered should return a TransientNotifyMsg cmd")
	} else if _, ok := cmd().(statusbar.TransientNotifyMsg); !ok {
		t.Error("move-up when filtered: expected TransientNotifyMsg")
	}
}

// TestQueueTagFilterMoveDisabled verifies that move-up/move-down are disabled
// when a cross-view tag filter is active. A tag filter narrows the visible
// queue, so cursor rows no longer map safely to underlying queue indices.
func TestQueueTagFilterMoveDisabled(t *testing.T) {
	q := queue.View{
		Entries: []queue.Entry{
			{Path: "/a.flac", Hash: "hidden", Title: "Hidden", Artist: "A"},
			{Path: "/b.flac", Hash: "h1", Title: "Visible One", Artist: "B"},
			{Path: "/c.flac", Hash: "h2", Title: "Visible Two", Artist: "C"},
		},
		TagFilterHashes: map[string]struct{}{"h1": {}, "h2": {}},
		Cursor:          0,
	}

	_, cmd := q.HandleAction(keys.ActionMoveTrackDown)
	if cmd == nil {
		t.Fatal("move-down when tag-filtered should return a TransientNotifyMsg cmd")
	}
	if _, ok := cmd().(statusbar.TransientNotifyMsg); !ok {
		t.Fatal("move-down when tag-filtered: expected TransientNotifyMsg")
	}

	q.Cursor = 1
	_, cmd = q.HandleAction(keys.ActionMoveTrackUp)
	if cmd == nil {
		t.Fatal("move-up when tag-filtered should return a TransientNotifyMsg cmd")
	}
	if _, ok := cmd().(statusbar.TransientNotifyMsg); !ok {
		t.Fatal("move-up when tag-filtered: expected TransientNotifyMsg")
	}
}
