package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/queue"
)

func TestEnqueueFeedbackDescribesActualPlacement(t *testing.T) {
	for _, tc := range []struct {
		name        string
		insertAfter int
		want        string
	}{
		{"append", -1, "Added to queue:"},
		{"play next", 0, "Queued next:"},
		{"play next at tail", 1, "Queued next:"},
		{"invalid position appends", 9, "Added to queue:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background())
			m.queue.Entries = []queue.Entry{{ID: 1}}
			m = m.handleTrackEnqueuedMsg(libtui.TrackEnqueuedMsg{
				Track:   library.Track{Hash: "new", Title: "New song", Artist: "Artist"},
				QueueID: 2, InsertAfter: tc.insertAfter,
			})
			msg := m.statusBar.TopNotification().Msg
			if !strings.HasPrefix(msg, tc.want) || !strings.Contains(msg, "New song") || !strings.Contains(msg, "press u to undo") {
				t.Fatalf("enqueue feedback = %q", msg)
			}
			m = m.handleUndo()
			if len(m.queue.Entries) != 1 || m.queue.Entries[0].ID != 1 {
				t.Fatal("enqueue feedback change broke undo")
			}
		})
	}
}
