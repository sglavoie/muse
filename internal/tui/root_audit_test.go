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

	if nm.statusBar.notification.msg == "" {
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
	if nm.statusBar.notification.msg == "" {
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

	if nm.statusBar.notification.msg != "" {
		t.Errorf("expected no notification when corrected=0, got %q", nm.statusBar.notification.msg)
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
// batch updates the recently-added view's cached source tracks so that the
// duration column does not show stale pre-audit values.
func TestDurationAuditBatchSyncsRecentlyAdded(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0, CreatedAt: 100})
	m = m.syncRecentlyAddedFromLibrary()

	// Sanity: recently-added should have the old duration.
	if len(m.recentlyAdded.sourceTracks) == 0 {
		t.Fatal("recentlyAdded.sourceTracks is empty after sync")
	}
	if m.recentlyAdded.sourceTracks[0].Duration != 120.0 {
		t.Fatalf("pre-audit recentlyAdded duration: got %f, want 120.0", m.recentlyAdded.sourceTracks[0].Duration)
	}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if len(nm.recentlyAdded.sourceTracks) == 0 {
		t.Fatal("recentlyAdded.sourceTracks is empty after audit batch")
	}
	got := nm.recentlyAdded.sourceTracks[0].Duration
	if got != 240.0 {
		t.Errorf("recentlyAdded duration after audit: got %f, want 240.0", got)
	}
}

// TestDurationAuditBatchSyncsHistoryTracks verifies that a duration audit
// batch propagates corrected durations to history source tracks.
func TestDurationAuditBatchSyncsHistoryTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.history.sourceTracks = []library.Track{{Hash: "h1", Path: "/a.mp3", Duration: 120.0}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.history.sourceTracks[0].Duration != 240.0 {
		t.Errorf("history duration after audit: got %f, want 240.0", nm.history.sourceTracks[0].Duration)
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
	m.artists.level = arttui.TrackLevel
	m.artists.selectedArtist = "A"
	m.artists.selectedAlbum = "X"
	m.artists.tracks = []library.Track{{Hash: "h1", Artist: "A", Album: "X", Duration: 120.0}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.artists.tracks[0].Duration != 240.0 {
		t.Errorf("artists track duration after audit: got %f, want 240.0", nm.artists.tracks[0].Duration)
	}
}

// TestDurationAuditBatchSyncsAlbumsTracks verifies that a duration audit
// batch propagates corrected durations to albums tracks at TrackLevel.
func TestDurationAuditBatchSyncsAlbumsTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Artist: "A", Album: "X", Duration: 120.0})
	m.albums.level = albtui.TrackLevel
	m.albums.selectedAlbum = "X"
	m.albums.tracks = []library.Track{{Hash: "h1", Artist: "A", Album: "X", Duration: 120.0}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.albums.tracks[0].Duration != 240.0 {
		t.Errorf("albums track duration after audit: got %f, want 240.0", nm.albums.tracks[0].Duration)
	}
}

// TestDurationAuditBatchSyncsGenresTracks verifies that a duration audit
// batch propagates corrected durations to genres tracks at TrackLevel.
func TestDurationAuditBatchSyncsGenresTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Artist: "A", Genre: "Rock", Duration: 120.0})
	m.genres.level = gentui.TrackLevel
	m.genres.selectedGenre = "Rock"
	m.genres.selectedArtist = "A"
	m.genres.tracks = []library.Track{{Hash: "h1", Artist: "A", Genre: "Rock", Duration: 120.0}}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	nm := next.(Model)

	if nm.genres.tracks[0].Duration != 240.0 {
		t.Errorf("genres track duration after audit: got %f, want 240.0", nm.genres.tracks[0].Duration)
	}
}
