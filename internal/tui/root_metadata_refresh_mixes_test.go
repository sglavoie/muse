package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// TestRefreshDerivedViewsUpdatesAllCachedMixes verifies that
// refreshDerivedViews propagates metadata edits to every cached mix, not just
// the currently-active one. Without this, switching back to a previously
// drilled-down mix would surface stale title/artist/album/duration until the
// app is restarted.
func TestRefreshDerivedViewsUpdatesAllCachedMixes(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac",
		Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum",
		Genre: "OldGenre", Duration: 120,
	})

	m.mixes.cachedMixes = map[db.MixID][]library.Track{
		db.MixRediscover: {
			{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre", Duration: 120},
		},
		db.MixHiddenGems: {
			{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre", Duration: 120},
		},
	}
	// Active mix is Rediscover; HiddenGems is cached but not active.
	m.mixes.activeMix = db.MixRediscover
	m.mixes.level = 0 // Stay at list level so trackView is not in play.

	m.library.index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "NewGenre")
	m.library.index.UpdateDuration("h1", 200)
	m = m.refreshDerivedViews()

	for id, cached := range m.mixes.cachedMixes {
		if len(cached) != 1 {
			t.Fatalf("mix %v: expected 1 cached track, got %d", id, len(cached))
		}
		got := cached[0]
		if got.Title != "NewTitle" {
			t.Errorf("mix %v: title = %q, want %q", id, got.Title, "NewTitle")
		}
		if got.Artist != "NewArt" {
			t.Errorf("mix %v: artist = %q, want %q", id, got.Artist, "NewArt")
		}
		if got.Album != "NewAlbum" {
			t.Errorf("mix %v: album = %q, want %q", id, got.Album, "NewAlbum")
		}
		if got.Genre != "NewGenre" {
			t.Errorf("mix %v: genre = %q, want %q", id, got.Genre, "NewGenre")
		}
		if got.Duration != 200 {
			t.Errorf("mix %v: duration = %v, want 200", id, got.Duration)
		}
	}
}
