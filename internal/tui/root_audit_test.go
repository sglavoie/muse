package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestDurationAuditBatchUpdatesIndex verifies that a DurationAuditBatchMsg
// updates the library index and matching queue entries.
func TestDurationAuditBatchUpdatesIndex(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 200.0})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.mp3", DurationMs: 120000},
		{ID: 2, Hash: "h2", Path: "/b.mp3", DurationMs: 200000},
	}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{
			{Hash: "h1", DurationMs: 240000},
		},
	})
	nm := next.(Model)

	// Library index should reflect the corrected duration.
	t1, ok := nm.library.index.FindByHash("h1")
	if !ok {
		t.Fatal("h1 not found in index")
	}
	if t1.Duration != 240.0 {
		t.Errorf("library index duration for h1: got %f, want 240.0", t1.Duration)
	}

	// Queue entry for h1 should be updated.
	if nm.queue.entries[0].DurationMs != 240000 {
		t.Errorf("queue entry h1 DurationMs: got %d, want 240000", nm.queue.entries[0].DurationMs)
	}

	// Queue entry for h2 should be unchanged.
	if nm.queue.entries[1].DurationMs != 200000 {
		t.Errorf("queue entry h2 DurationMs: got %d, want 200000 (unmodified)", nm.queue.entries[1].DurationMs)
	}
}

// TestDurationAuditBatchChainsNextRead verifies that handling a
// DurationAuditBatchMsg returns a cmd to read the next batch (non-nil cmd).
func TestDurationAuditBatchChainsNextRead(t *testing.T) {
	ch := make(chan library.DurationFix)
	m := New(context.Background(), WithDurationAuditChannel(ch))
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})

	_, cmd := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	if cmd == nil {
		t.Error("expected non-nil cmd to chain next audit batch read, got nil")
	}
}

// TestDurationAuditBatchNilChannelReturnsNilCmd verifies that if auditCh
// is nil when a batch arrives, no follow-up cmd is scheduled.
func TestDurationAuditBatchNilChannelReturnsNilCmd(t *testing.T) {
	m := New(context.Background()) // no audit channel
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})

	_, cmd := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	if cmd != nil {
		t.Error("expected nil cmd when auditCh is nil, got non-nil")
	}
}

// TestDurationAuditDoneShowsNotification verifies that when corrections were
// applied in preceding batches, a status bar notification is shown on done.
func TestDurationAuditDoneShowsNotification(t *testing.T) {
	ch := make(chan library.DurationFix)
	m := New(context.Background(), WithDurationAuditChannel(ch))
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 200.0})
	m.library.index.Insert(library.Track{Hash: "h3", Path: "/c.mp3", Duration: 300.0})

	// Simulate batch with 3 corrections arriving before the done message.
	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{
			{Hash: "h1", DurationMs: 240000},
			{Hash: "h2", DurationMs: 250000},
			{Hash: "h3", DurationMs: 350000},
		},
	})
	m = next.(Model)

	next, _ = m.Update(library.DurationAuditDoneMsg{})
	nm := next.(Model)

	if nm.statusBar.topNotification().msg == "" {
		t.Error("expected a non-empty notification after audit with corrections, got empty")
	}
	if nm.auditCh != nil {
		t.Error("expected auditCh to be nil after DurationAuditDoneMsg")
	}
	if nm.auditCorrected != 0 {
		t.Errorf("expected auditCorrected to be reset to 0, got %d", nm.auditCorrected)
	}
}

// TestDurationAuditCorrectedAccumulatesAcrossBatches verifies that the
// auditCorrected counter sums fixes from multiple batch messages.
func TestDurationAuditCorrectedAccumulatesAcrossBatches(t *testing.T) {
	ch := make(chan library.DurationFix)
	m := New(context.Background(), WithDurationAuditChannel(ch))
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 100.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 200.0})
	m.library.index.Insert(library.Track{Hash: "h3", Path: "/c.mp3", Duration: 300.0})

	// First batch: 1 correction.
	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 150000}},
	})
	m = next.(Model)
	if m.auditCorrected != 1 {
		t.Fatalf("after batch 1: auditCorrected = %d, want 1", m.auditCorrected)
	}

	// Second batch: 2 corrections.
	next, _ = m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{
			{Hash: "h2", DurationMs: 250000},
			{Hash: "h3", DurationMs: 350000},
		},
	})
	m = next.(Model)
	if m.auditCorrected != 3 {
		t.Fatalf("after batch 2: auditCorrected = %d, want 3", m.auditCorrected)
	}

	// Done: notification should mention 3.
	next, _ = m.Update(library.DurationAuditDoneMsg{})
	nm := next.(Model)
	if nm.statusBar.topNotification().msg == "" {
		t.Error("expected notification after 3 corrections")
	}
	if nm.auditCorrected != 0 {
		t.Errorf("auditCorrected should be reset after done, got %d", nm.auditCorrected)
	}
}

// TestDurationAuditDoneSilentWhenNone verifies that when corrected == 0 no
// notification is shown.
func TestDurationAuditDoneSilentWhenNone(t *testing.T) {
	ch := make(chan library.DurationFix)
	m := New(context.Background(), WithDurationAuditChannel(ch))

	next, _ := m.Update(library.DurationAuditDoneMsg{})
	nm := next.(Model)

	if nm.statusBar.topNotification().msg != "" {
		t.Errorf("expected no notification when corrected=0, got %q", nm.statusBar.topNotification().msg)
	}
	if nm.auditCh != nil {
		t.Error("expected auditCh to be nil after DurationAuditDoneMsg")
	}
}

// TestDurationAuditNilChannelSkipsInit verifies that a model without an audit
// channel leaves auditCh nil (Init will not schedule an audit cmd).
func TestDurationAuditNilChannelSkipsInit(t *testing.T) {
	m := New(context.Background()) // no WithDurationAuditChannel option
	if m.auditCh != nil {
		t.Error("expected auditCh to be nil when no audit channel option is provided")
	}
}

// TestDurationAuditBatchSyncsRecentlyAdded verifies that a duration audit
// batch updates the library's Recent mode source tracks so that the
// duration column does not show stale pre-audit values.
func TestDurationAuditBatchSyncsRecentlyAdded(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0, CreatedAt: 100})
	m.library = m.library.switchMode(LibraryModeRecent)
	m = m.syncRecentlyAddedFromLibrary()

	// Sanity: library sourceTracks should have the old duration.
	if len(m.library.sourceTracks) == 0 {
		t.Fatal("library.sourceTracks is empty after sync")
	}
	if m.library.sourceTracks[0].Duration != 120.0 {
		t.Fatalf("pre-audit library Recent duration: got %f, want 120.0", m.library.sourceTracks[0].Duration)
	}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if len(nm.library.sourceTracks) == 0 {
		t.Fatal("library.sourceTracks is empty after audit batch")
	}
	got := nm.library.sourceTracks[0].Duration
	if got != 240.0 {
		t.Errorf("library Recent duration after audit: got %f, want 240.0", got)
	}
}

// TestDurationAuditBatchSyncsHistoryTracks verifies that a duration audit
// batch propagates corrected durations to library History mode source tracks.
func TestDurationAuditBatchSyncsHistoryTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.library = m.library.switchMode(LibraryModeHistory)
	m.library.sourceTracks = []library.Track{{Hash: "h1", Path: "/a.mp3", Duration: 120.0}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.library.sourceTracks[0].Duration != 240.0 {
		t.Errorf("library History duration after audit: got %f, want 240.0", nm.library.sourceTracks[0].Duration)
	}
}

// TestDurationAuditBatchSyncsPlaylistTracks verifies that a duration audit
// batch propagates corrected durations to playlist tracks at TrackLevel.
func TestDurationAuditBatchSyncsPlaylistTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{{Hash: "h1", DurationMs: 120000}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.playlists.tracks[0].DurationMs != 240000 {
		t.Errorf("playlist track DurationMs after audit: got %d, want 240000", nm.playlists.tracks[0].DurationMs)
	}
}

// TestDurationAuditBatchSyncsFavoritesTracks verifies that a duration audit
// batch propagates corrected durations to favorites tracks.
func TestDurationAuditBatchSyncsFavoritesTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{{Hash: "h1", Duration: 120.0}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.favorites.tracks[0].Duration != 240.0 {
		t.Errorf("favorites track duration after audit: got %f, want 240.0", nm.favorites.tracks[0].Duration)
	}
}

// TestDurationAuditBatchSyncsTagsTracks verifies that a duration audit
// batch propagates corrected durations to tags tracks at TrackLevel.
func TestDurationAuditBatchSyncsTagsTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{{Hash: "h1", DurationMs: 120000}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.tagsView.tracks[0].DurationMs != 240000 {
		t.Errorf("tags track DurationMs after audit: got %d, want 240000", nm.tagsView.tracks[0].DurationMs)
	}
}

// TestDurationAuditBatchRecomputesSortedLibrary verifies that when the library
// has an active sort, the sorted tracks cache is rebuilt after an audit batch
// so that duration-based sort orders reflect the corrected values.
func TestDurationAuditBatchRecomputesSortedLibrary(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 300.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 100.0})
	// Sort by duration ascending: h2 (100) should come before h1 (300).
	m.library.sortState = SortState{Field: SortDuration, Dir: SortAsc}
	m.library = m.library.recomputeSortedTracks()
	if len(m.library.sortedTracks) != 2 {
		t.Fatalf("sorted tracks length: got %d, want 2", len(m.library.sortedTracks))
	}
	if m.library.sortedTracks[0].Hash != "h2" {
		t.Fatalf("pre-audit sorted[0] hash: got %s, want h2", m.library.sortedTracks[0].Hash)
	}

	// Audit corrects h2's duration to 500, which should flip the order.
	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h2", DurationMs: 500000}},
	})
	nm := next.(Model)

	if len(nm.library.sortedTracks) != 2 {
		t.Fatalf("sorted tracks length after audit: got %d, want 2", len(nm.library.sortedTracks))
	}
	// After correction h1=300, h2=500 → ascending: h1 first.
	if nm.library.sortedTracks[0].Hash != "h1" {
		t.Errorf("post-audit sorted[0] hash: got %s, want h1 (sort order should reflect corrected duration)", nm.library.sortedTracks[0].Hash)
	}
}

// TestDurationAuditBatchSyncsArtistsTracks verifies that a duration audit
// batch propagates corrected durations to artists tracks at TrackLevel.
func TestDurationAuditBatchSyncsArtistsTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Artist: "A", Album: "X", Duration: 120.0})
	m.browse.artists.level = arttui.TrackLevel
	m.browse.artists.selectedArtist = "A"
	m.browse.artists.selectedAlbum = "X"
	m.browse.artists.tracks = []library.Track{{Hash: "h1", Artist: "A", Album: "X", Duration: 120.0}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.browse.artists.tracks[0].Duration != 240.0 {
		t.Errorf("artists track duration after audit: got %f, want 240.0", nm.browse.artists.tracks[0].Duration)
	}
}

// TestDurationAuditBatchSyncsAlbumsTracks verifies that a duration audit
// batch propagates corrected durations to albums tracks at TrackLevel.
func TestDurationAuditBatchSyncsAlbumsTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Artist: "A", Album: "X", Duration: 120.0})
	m.browse.albums.level = albtui.TrackLevel
	m.browse.albums.selectedAlbum = "X"
	m.browse.albums.tracks = []library.Track{{Hash: "h1", Artist: "A", Album: "X", Duration: 120.0}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.browse.albums.tracks[0].Duration != 240.0 {
		t.Errorf("albums track duration after audit: got %f, want 240.0", nm.browse.albums.tracks[0].Duration)
	}
}

// TestDurationAuditBatchSyncsCachedMixTracks verifies that a duration audit
// batch propagates corrected durations to the mixes cache so that opening a
// mix later does not show stale pre-audit durations. Regression test for the
// pattern where new derived views miss the audit propagation block.
func TestDurationAuditBatchSyncsCachedMixTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.mixes.cachedMixes = map[db.MixID][]library.Track{
		db.MixRediscover: {{Hash: "h1", Duration: 120.0}},
	}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	cached, ok := nm.mixes.cachedMixes[db.MixRediscover]
	if !ok || len(cached) == 0 {
		t.Fatal("mixes cache missing or empty after audit")
	}
	if cached[0].Duration != 240.0 {
		t.Errorf("cached mix track duration after audit: got %f, want 240.0", cached[0].Duration)
	}
}

// TestDurationAuditBatchSyncsActiveMixTrackView verifies that when the user is
// viewing tracks inside a mix (level 1) at the time of an audit, the trackView's
// view of the corrected duration updates immediately rather than only at next open.
func TestDurationAuditBatchSyncsActiveMixTrackView(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.mixes.cachedMixes = map[db.MixID][]library.Track{
		db.MixRediscover: {{Hash: "h1", Duration: 120.0}},
	}
	m.mixes.openMix(db.MixRediscover)
	if m.mixes.level != 1 {
		t.Fatalf("expected level=1 after openMix, got %d", m.mixes.level)
	}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if len(nm.mixes.trackView.sourceTracks) == 0 {
		t.Fatal("mixes trackView sourceTracks empty after audit")
	}
	if nm.mixes.trackView.sourceTracks[0].Duration != 240.0 {
		t.Errorf("mixes trackView duration after audit: got %f, want 240.0",
			nm.mixes.trackView.sourceTracks[0].Duration)
	}
}

// TestDurationAuditBatchSyncsGenresTracks verifies that a duration audit
// batch propagates corrected durations to genres tracks at TrackLevel.
func TestDurationAuditBatchSyncsGenresTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Artist: "A", Genre: "Rock", Duration: 120.0})
	m.browse.genres.level = gentui.TrackLevel
	m.browse.genres.selectedGenre = "Rock"
	m.browse.genres.selectedArtist = "A"
	m.browse.genres.tracks = []library.Track{{Hash: "h1", Artist: "A", Genre: "Rock", Duration: 120.0}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.browse.genres.tracks[0].Duration != 240.0 {
		t.Errorf("genres track duration after audit: got %f, want 240.0", nm.browse.genres.tracks[0].Duration)
	}
}

// TestDurationAuditBatchRecomputesQueueSortCache verifies that when the queue
// has an active duration sort, the displayOrder cache is recomputed after a
// duration audit batch so that the corrected values flip the visible order.
// Without preserveQueueSort, the queue would keep showing pre-audit ordering
// until the user manually re-triggered a sort cycle.
func TestDurationAuditBatchRecomputesQueueSortCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 300.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 100.0})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.mp3", DurationMs: 300000},
		{ID: 2, Hash: "h2", Path: "/b.mp3", DurationMs: 100000},
	}
	// Sort by duration ascending: h2 (100) before h1 (300).
	m.queue.sortState = SortState{Field: SortDuration, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()
	if len(m.queue.displayOrder) != 2 || m.queue.entries[m.queue.displayOrder[0]].Hash != "h2" {
		t.Fatalf("pre-audit displayOrder[0] = %v; want h2 first", m.queue.displayOrder)
	}

	// Audit corrects h2's duration to 500 — order should flip to h1 first.
	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h2", DurationMs: 500000}},
	})
	nm := next.(Model)

	if len(nm.queue.displayOrder) != 2 {
		t.Fatalf("post-audit displayOrder len: got %d, want 2", len(nm.queue.displayOrder))
	}
	if nm.queue.entries[nm.queue.displayOrder[0]].Hash != "h1" {
		t.Errorf("post-audit displayOrder[0] hash: got %s, want h1 (queue sort cache should reflect corrected durations)",
			nm.queue.entries[nm.queue.displayOrder[0]].Hash)
	}
}
