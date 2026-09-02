package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestWatchBatchHealsQueuePathOnRename verifies that a same-hash move/rename
// reported by the watcher updates queue.Entry.Path and rebuilds the player queue
// with the new path so playback resolves the moved file immediately, while
// preserving source/selection state and clearing the missing flag.
func TestWatchBatchHealsQueuePathOnRename(t *testing.T) {
	sp := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(sp))
	m.library.Index.Insert(library.Track{
		Artist: "A", Title: "T1", Path: "/old.flac", Hash: "h1",
	})
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/old.flac", Artist: "A", Title: "T1",
			SourcePlaylist: "Mix1", Selected: true, Missing: true},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "A", Title: "T1", Path: "/new.flac", Hash: "h1"},
		},
	})
	nm := next.(Model)

	e := nm.queue.Entries[0]
	if e.Path != "/new.flac" {
		t.Errorf("queue entry Path = %q; want /new.flac", e.Path)
	}
	if e.Missing {
		t.Error("queue entry should no longer be Missing after rename recovery")
	}
	if e.SourcePlaylist != "Mix1" {
		t.Errorf("SourcePlaylist = %q; want Mix1 (preserved)", e.SourcePlaylist)
	}
	if !e.Selected {
		t.Error("Selected state should be preserved across a rename")
	}

	if len(sp.setQueueCalls) == 0 {
		t.Fatal("expected SetQueue to be called after path rename")
	}
	last := sp.setQueueCalls[len(sp.setQueueCalls)-1]
	if len(last) != 1 || last[0].Path != "/new.flac" {
		t.Fatalf("player queue = %+v; want single track at /new.flac", last)
	}
}

// TestScanBatchHealsQueuePathOnRename verifies that the scan batch handler
// (shared by the startup diff scan and manual rescan) heals a same-hash
// move/rename into the queue exactly like the live watcher does.
func TestScanBatchHealsQueuePathOnRename(t *testing.T) {
	sp := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(sp))
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/old.flac", Artist: "A", Title: "T1", Missing: true},
	}

	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "A", Title: "T1", Path: "/new.flac", Hash: "h1"},
		},
	})
	nm := next.(Model)

	e := nm.queue.Entries[0]
	if e.Path != "/new.flac" {
		t.Errorf("queue entry Path = %q; want /new.flac after rescan", e.Path)
	}
	if e.Missing {
		t.Error("queue entry should no longer be Missing after rescan recovery")
	}
	if len(sp.setQueueCalls) == 0 {
		t.Fatal("expected SetQueue to be called after rescan path rename")
	}
	last := sp.setQueueCalls[len(sp.setQueueCalls)-1]
	if len(last) != 1 || last[0].Path != "/new.flac" {
		t.Fatalf("player queue = %+v; want single track at /new.flac", last)
	}
}

// TestWatchBatchHealsCollectionPaths verifies that a same-hash rename updates
// the stored Path in the loaded playlist, favorites, tags, and mix track lists,
// not just the queue.
func TestWatchBatchHealsCollectionPaths(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Artist: "A", Title: "T1", Path: "/old.flac", Hash: "h1"})

	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = []db.PlaylistTrackRow{{Hash: "h1", Path: "/old.flac", Artist: "A", Title: "T1"}}

	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	m.favorites.Tracks = []library.Track{{Hash: "h1", Path: "/old.flac", Artist: "A", Title: "T1"}}

	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = []db.PlaylistTrackRow{{Hash: "h1", Path: "/old.flac", Artist: "A", Title: "T1"}}

	m.mixes.cachedMixes = map[db.MixID][]library.Track{
		db.MixRediscover: {{Hash: "h1", Path: "/old.flac", Artist: "A", Title: "T1"}},
		db.MixHiddenGems: {{Hash: "h1", Path: "/old.flac", Artist: "A", Title: "T1"}},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{{Artist: "A", Title: "T1", Path: "/new.flac", Hash: "h1"}},
	})
	nm := next.(Model)

	if got := nm.playlists.Tracks[0].Path; got != "/new.flac" {
		t.Errorf("playlist track Path = %q; want /new.flac", got)
	}
	if got := nm.favorites.Tracks[0].Path; got != "/new.flac" {
		t.Errorf("favorites track Path = %q; want /new.flac", got)
	}
	if got := nm.tagsView.Tracks[0].Path; got != "/new.flac" {
		t.Errorf("tags track Path = %q; want /new.flac", got)
	}
	for id, cached := range nm.mixes.cachedMixes {
		if cached[0].Path != "/new.flac" {
			t.Errorf("mix %v cached Path = %q; want /new.flac", id, cached[0].Path)
		}
	}
}
