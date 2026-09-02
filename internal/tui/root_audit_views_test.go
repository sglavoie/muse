package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/sorting"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestDurationAuditBatchSyncsRecentlyAdded(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0, CreatedAt: 100})
	m.library = m.library.SwitchMode(libtui.LibraryModeRecent)
	m = m.syncRecentlyAddedFromLibrary()
	if len(m.library.SourceTracks) == 0 || m.library.SourceTracks[0].Duration != 120.0 {
		t.Fatalf("pre-audit library Recent duration: got %+v", m.library.SourceTracks)
	}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if len(nm.library.SourceTracks) == 0 {
		t.Fatal("library.sourceTracks is empty after audit batch")
	}
	if got := nm.library.SourceTracks[0].Duration; got != 240.0 {
		t.Errorf("library Recent duration after audit: got %f, want 240.0", got)
	}
}

func TestDurationAuditBatchSyncsHistoryTracks(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.library = m.library.SwitchMode(libtui.LibraryModeHistory)
	m.library.SourceTracks = []library.Track{{Hash: "h1", Path: "/a.mp3", Duration: 120.0}}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if got := nm.library.SourceTracks[0].Duration; got != 240.0 {
		t.Errorf("library History duration after audit: got %f, want 240.0", got)
	}
}

func TestDurationAuditBatchSyncsPlaylistTracks(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = []db.PlaylistTrackRow{{Hash: "h1", DurationMs: 120000}}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if got := nm.playlists.Tracks[0].DurationMs; got != 240000 {
		t.Errorf("playlist track DurationMs after audit: got %d, want 240000", got)
	}
}

func TestDurationAuditBatchSyncsFavoritesTracks(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	m.favorites.Tracks = []library.Track{{Hash: "h1", Duration: 120.0}}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if got := nm.favorites.Tracks[0].Duration; got != 240.0 {
		t.Errorf("favorites track duration after audit: got %f, want 240.0", got)
	}
}

func TestDurationAuditBatchSyncsTagsTracks(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = []db.PlaylistTrackRow{{Hash: "h1", DurationMs: 120000}}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if got := nm.tagsView.Tracks[0].DurationMs; got != 240000 {
		t.Errorf("tags track DurationMs after audit: got %d, want 240000", got)
	}
}

func TestDurationAuditBatchRecomputesSortedLibrary(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 300.0})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 100.0})
	m.library.SortState = sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortAsc}
	m.library = m.library.RecomputeSortedTracks()
	if len(m.library.SortedTracks) != 2 || m.library.SortedTracks[0].Hash != "h2" {
		t.Fatalf("pre-audit sorted tracks = %+v; want h2 first", m.library.SortedTracks)
	}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h2", DurationMs: 500000})
	if len(nm.library.SortedTracks) != 2 {
		t.Fatalf("sorted tracks length after audit: got %d, want 2", len(nm.library.SortedTracks))
	}
	if got := nm.library.SortedTracks[0].Hash; got != "h1" {
		t.Errorf("post-audit sorted[0] hash: got %s, want h1", got)
	}
}

func TestDurationAuditBatchSyncsArtistsTracks(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Artist: "A", Album: "X", Duration: 120.0})
	m.browse.artists.level = arttui.TrackLevel
	m.browse.artists.selectedArtist = "A"
	m.browse.artists.selectedAlbum = "X"
	m.browse.artists.tracks = []library.Track{{Hash: "h1", Artist: "A", Album: "X", Duration: 120.0}}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if got := nm.browse.artists.tracks[0].Duration; got != 240.0 {
		t.Errorf("artists track duration after audit: got %f, want 240.0", got)
	}
}

func TestDurationAuditBatchSyncsAlbumsTracks(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Artist: "A", Album: "X", Duration: 120.0})
	m.browse.albums.level = albtui.TrackLevel
	m.browse.albums.selectedAlbum = "X"
	m.browse.albums.tracks = []library.Track{{Hash: "h1", Artist: "A", Album: "X", Duration: 120.0}}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if got := nm.browse.albums.tracks[0].Duration; got != 240.0 {
		t.Errorf("albums track duration after audit: got %f, want 240.0", got)
	}
}

func TestDurationAuditBatchSyncsCachedMixTracks(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: {{Hash: "h1", Duration: 120.0}}}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	cached := nm.mixes.cachedMixes[db.MixRediscover]
	if len(cached) == 0 {
		t.Fatal("mixes cache missing or empty after audit")
	}
	if got := cached[0].Duration; got != 240.0 {
		t.Errorf("cached mix track duration after audit: got %f, want 240.0", got)
	}
}

func TestDurationAuditBatchSyncsActiveMixTrackView(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: {{Hash: "h1", Duration: 120.0}}}
	m.mixes.openMix(db.MixRediscover)
	if m.mixes.level != 1 {
		t.Fatalf("expected level=1 after openMix, got %d", m.mixes.level)
	}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if len(nm.mixes.trackView.SourceTracks) == 0 {
		t.Fatal("mixes trackView sourceTracks empty after audit")
	}
	if got := nm.mixes.trackView.SourceTracks[0].Duration; got != 240.0 {
		t.Errorf("mixes trackView duration after audit: got %f, want 240.0", got)
	}
}

func TestDurationAuditBatchSyncsGenresTracks(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Artist: "A", Genre: "Rock", Duration: 120.0})
	m.browse.genres.level = gentui.TrackLevel
	m.browse.genres.selectedGenre = "Rock"
	m.browse.genres.selectedArtist = "A"
	m.browse.genres.tracks = []library.Track{{Hash: "h1", Artist: "A", Genre: "Rock", Duration: 120.0}}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if got := nm.browse.genres.tracks[0].Duration; got != 240.0 {
		t.Errorf("genres track duration after audit: got %f, want 240.0", got)
	}
}

func TestDurationAuditBatchRecomputesQueueSortCache(t *testing.T) {
	m := newDurationAuditModel()
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 300.0})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 100.0})
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "h1", Path: "/a.mp3", DurationMs: 300000}, {ID: 2, Hash: "h2", Path: "/b.mp3", DurationMs: 100000}}
	m.queue.SortState = sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortAsc}
	m.queue = m.queue.RecomputeDisplayOrder()
	if len(m.queue.DisplayOrder) != 2 || m.queue.Entries[m.queue.DisplayOrder[0]].Hash != "h2" {
		t.Fatalf("pre-audit displayOrder = %v; want h2 first", m.queue.DisplayOrder)
	}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h2", DurationMs: 500000})
	if len(nm.queue.DisplayOrder) != 2 {
		t.Fatalf("post-audit displayOrder len: got %d, want 2", len(nm.queue.DisplayOrder))
	}
	if got := nm.queue.Entries[nm.queue.DisplayOrder[0]].Hash; got != "h1" {
		t.Errorf("post-audit displayOrder[0] hash: got %s, want h1", got)
	}
}
