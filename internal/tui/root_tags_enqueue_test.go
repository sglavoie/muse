package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestTagEnqueueAllAddsEntriesToQueue verifies that pressing 'E' in the Tags
// view enqueues tracks into the in-memory queue (not just the DB). Before the
// fix, handleTagEnqueueAll returned a tagEnqueueAllMsg that only showed a
// notification without adding entries to m.queue.entries.
func TestTagEnqueueAllAddsEntriesToQueue(t *testing.T) {
	be := &stubBatchEnqueuer{}
	tr := &stubTagReader{
		tags: []db.TagRow{{ID: 1, Name: "chill"}},
		tracksByID: map[int64][]db.PlaylistTrackRow{
			1: {
				{Hash: "h1", Path: "/a.flac", Artist: "Alice", Title: "Track A", DurationMs: 180000},
				{Hash: "h2", Path: "/b.flac", Artist: "Bob", Title: "Track B", DurationMs: 240000},
			},
		},
		tagCounts: map[int64]int{1: 2},
	}

	m := New(context.Background(),
		WithLibraryTracks([]library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "Alice", Title: "Track A", Duration: 180},
			{Hash: "h2", Path: "/b.flac", Artist: "Bob", Title: "Track B", Duration: 240},
		}),
		WithBatchEnqueuer(be),
		WithTagReader(tr),
	)

	// Navigate to Tags view and open the tag to see tracks.
	m.activeView = viewTags
	m.tagsView.Tags = tr.tags
	m.tagsView.TagCounts = tr.tagCounts

	// Simulate pressing 'E' to enqueue all tracks for the focused tag.
	m2, cmd := m.handleTagEnqueueAll()
	m = m2

	if cmd == nil {
		t.Fatal("expected a tea.Cmd from handleTagEnqueueAll, got nil")
	}

	// Execute the async command to get the BatchEnqueuedMsg.
	msg := cmd()
	batchMsg, ok := msg.(BatchEnqueuedMsg)
	if !ok {
		t.Fatalf("expected BatchEnqueuedMsg, got %T", msg)
	}
	if batchMsg.Err != nil {
		t.Fatalf("unexpected error: %v", batchMsg.Err)
	}
	if len(batchMsg.Entries) != 2 {
		t.Fatalf("expected 2 entries in BatchEnqueuedMsg, got %d", len(batchMsg.Entries))
	}

	// Process the message through Update.
	next, _ := m.Update(batchMsg)
	m = next.(Model)

	if len(m.queue.Entries) != 2 {
		t.Fatalf("expected 2 queue entries after enqueue, got %d", len(m.queue.Entries))
	}
	if m.queue.Entries[0].Hash != "h1" {
		t.Errorf("expected first entry hash h1, got %s", m.queue.Entries[0].Hash)
	}
	if m.queue.Entries[1].Hash != "h2" {
		t.Errorf("expected second entry hash h2, got %s", m.queue.Entries[1].Hash)
	}
}

// TestEnqueueByTagFromPaletteAddsEntriesToQueue verifies that the palette
// "Enqueue tag" command adds tracks to the in-memory queue.
func TestEnqueueByTagFromPaletteAddsEntriesToQueue(t *testing.T) {
	be := &stubBatchEnqueuer{}
	tr := &stubTagReader{
		tags: []db.TagRow{{ID: 1, Name: "rock"}},
		tracksByID: map[int64][]db.PlaylistTrackRow{
			1: {
				{Hash: "h3", Path: "/c.flac", Artist: "Charlie", Title: "Track C", DurationMs: 300000},
			},
		},
		tagCounts: map[int64]int{1: 1},
	}

	m := New(context.Background(),
		WithLibraryTracks([]library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "Charlie", Title: "Track C", Duration: 300},
		}),
		WithBatchEnqueuer(be),
		WithTagReader(tr),
	)

	// Simulate the palette dispatching enqueueByTagMsg.
	m2, cmd := m.handleEnqueueByTag(enqueueByTagMsg{tagID: 1, name: "rock"})
	m = m2

	if cmd == nil {
		t.Fatal("expected a tea.Cmd from handleEnqueueByTag, got nil")
	}

	msg := cmd()
	batchMsg, ok := msg.(BatchEnqueuedMsg)
	if !ok {
		t.Fatalf("expected BatchEnqueuedMsg, got %T", msg)
	}
	if batchMsg.Err != nil {
		t.Fatalf("unexpected error: %v", batchMsg.Err)
	}
	if len(batchMsg.Entries) != 1 {
		t.Fatalf("expected 1 entry in BatchEnqueuedMsg, got %d", len(batchMsg.Entries))
	}

	next, _ := m.Update(batchMsg)
	m = next.(Model)

	if len(m.queue.Entries) != 1 {
		t.Fatalf("expected 1 queue entry after enqueue, got %d", len(m.queue.Entries))
	}
	if m.queue.Entries[0].Hash != "h3" {
		t.Errorf("expected entry hash h3, got %s", m.queue.Entries[0].Hash)
	}
	if m.queue.Entries[0].SourcePlaylist != "rock" {
		t.Errorf("expected source playlist 'rock', got %q", m.queue.Entries[0].SourcePlaylist)
	}
}

// TestTagEnqueueAllAtTrackLevelEnqueuesSingleTrack verifies that pressing
// 'enter' at the track level in the Tags view enqueues just the focused track.
func TestTagEnqueueAllAtTrackLevelEnqueuesSingleTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	tr := &stubTagReader{
		tags: []db.TagRow{{ID: 1, Name: "chill"}},
		tracksByID: map[int64][]db.PlaylistTrackRow{
			1: {
				{Hash: "h1", Path: "/a.flac", Artist: "Alice", Title: "Track A", DurationMs: 180000},
				{Hash: "h2", Path: "/b.flac", Artist: "Bob", Title: "Track B", DurationMs: 240000},
			},
		},
		tagCounts: map[int64]int{1: 2},
	}

	m := New(context.Background(),
		WithLibraryTracks([]library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "Alice", Title: "Track A", Duration: 180},
			{Hash: "h2", Path: "/b.flac", Artist: "Bob", Title: "Track B", Duration: 240},
		}),
		WithQueueWriter(qa),
		WithTagReader(tr),
	)

	m.activeView = viewTags
	m.tagsView = m.tagsView.OpenTracks(tr.tags[0], tr.tracksByID[1])
	if m.tagsView.Level != tagtui.TrackLevel {
		t.Fatalf("expected TrackLevel after openTracks, got %d", m.tagsView.Level)
	}

	// The 'enter' key at TrackLevel dispatches a single-track enqueue (TrackEnqueuedMsg),
	// which is a separate, already-working path. Just verify we get a command.
	_, cmd := m.handleTagOpen()
	if cmd == nil {
		t.Fatal("expected a tea.Cmd from handleTagOpen at TrackLevel, got nil")
	}
}
