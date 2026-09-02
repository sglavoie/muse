package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/keys"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestPlayNextTagsTrackLevel(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewTags
	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "th1", Title: "T1", Artist: "A", Path: "/t1.mp3", DurationMs: 180000},
		{ID: 2, Hash: "th2", Title: "T2", Artist: "B", Path: "/t2.mp3", DurationMs: 200000},
	}
	m.tagsView.SelectedTag = db.TagRow{ID: 1, Name: "TestTag"}
	m.tagsView.Cursor = 1
	m.tagsView.Height = 40

	_, cmd, handled := m.handleLibraryQueueAction(keys.ActionPlayNext)
	if !handled {
		t.Fatal("expected actionPlayNext to be handled")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for play-next on tags track")
	}
}

func TestPlayNextTagsNoOpAtTagLevel(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView.Tags = []db.TagRow{{ID: 1, Name: "T1"}}
	m.tagsView.TagCounts = map[int64]int{1: 3}

	_, cmd, handled := m.handleLibraryQueueAction(keys.ActionPlayNext)
	if !handled {
		t.Fatal("expected actionPlayNext to be handled")
	}
	if cmd != nil {
		t.Error("expected nil cmd for play-next at TagLevel (no focused track)")
	}
}
