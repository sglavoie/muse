package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/queue"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestFocusedGoToContext_Library(t *testing.T) {
	tracks := []library.Track{{Artist: "Alice", Album: "Alpha", Genre: "Rock", Title: "T1", Hash: "h1", Path: "/a.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	ctx := m.focusedGoToContext()
	if !ctx.found {
		t.Fatal("expected found=true")
	}
	if ctx.artist != "Alice" {
		t.Errorf("artist: want Alice, got %q", ctx.artist)
	}
	if library.AlbumTitle(ctx.album) != "Alpha" {
		t.Errorf("album: want Alpha, got %q", ctx.album)
	}
	if ctx.genre != "Rock" {
		t.Errorf("genre: want Rock, got %q", ctx.genre)
	}
}

func TestFocusedGoToContext_Queue(t *testing.T) {
	tracks := []library.Track{{Artist: "Bob", Album: "Beta", Genre: "Jazz", Title: "T1", Hash: "h1", Path: "/b.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{{Artist: "Bob", Album: "Beta", Hash: "h1", SourcePlaylist: "MyPL"}}
	m.queue.Cursor = 0
	ctx := m.focusedGoToContext()
	if !ctx.found {
		t.Fatal("expected found=true")
	}
	if ctx.genre != "Jazz" {
		t.Errorf("genre: want Jazz (from library lookup), got %q", ctx.genre)
	}
	if ctx.sourcePlaylist != "MyPL" {
		t.Errorf("sourcePlaylist: want MyPL, got %q", ctx.sourcePlaylist)
	}
}

func TestBuildGoToOptions_Library(t *testing.T) {
	tracks := []library.Track{{Artist: "Alice", Album: "Alpha", Genre: "Rock", Title: "T1", Hash: "h1", Path: "/a.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	ctx := m.focusedGoToContext()
	opts := m.buildGoToOptions(ctx)

	if len(opts) != 3 {
		t.Fatalf("want 3 options, got %d", len(opts))
	}
	if opts[0].Kind != overlay.GoToAlbum {
		t.Errorf("opts[0]: want goToAlbum, got %v", opts[0].Kind)
	}
	if opts[1].Kind != overlay.GoToArtist {
		t.Errorf("opts[1]: want goToArtist, got %v", opts[1].Kind)
	}
	if opts[2].Kind != overlay.GoToGenre {
		t.Errorf("opts[2]: want goToGenre, got %v", opts[2].Kind)
	}
}

func TestBuildGoToOptions_QueueWithSource(t *testing.T) {
	tracks := []library.Track{{Artist: "Bob", Album: "Beta", Genre: "Jazz", Title: "T1", Hash: "h1", Path: "/b.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{{Artist: "Bob", Album: "Beta", Hash: "h1", SourcePlaylist: "MyPL"}}
	m.queue.Cursor = 0
	ctx := m.focusedGoToContext()
	opts := m.buildGoToOptions(ctx)

	if len(opts) != 4 {
		t.Fatalf("want 4 options, got %d", len(opts))
	}
	if opts[3].Kind != overlay.GoToPlaylist || opts[3].Value != "MyPL" {
		t.Errorf("opts[3]: want goToPlaylist/MyPL, got %v/%q", opts[3].Kind, opts[3].Value)
	}
}

func TestBuildGoToOptions_MultiGenre(t *testing.T) {
	tracks := []library.Track{{Artist: "Alice", Album: "Alpha", Genre: "Rock, Jazz", Title: "T1", Hash: "h1", Path: "/a.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	ctx := m.focusedGoToContext()
	opts := m.buildGoToOptions(ctx)

	if len(opts) != 4 {
		t.Fatalf("want 4 options, got %d", len(opts))
	}
	if opts[2].Kind != overlay.GoToGenre || opts[2].Value != "Jazz" {
		t.Errorf("opts[2]: want goToGenre/Jazz, got %v/%q", opts[2].Kind, opts[2].Value)
	}
	if opts[3].Kind != overlay.GoToGenre || opts[3].Value != "Rock" {
		t.Errorf("opts[3]: want goToGenre/Rock, got %v/%q", opts[3].Kind, opts[3].Value)
	}
}

func TestFocusedGoToContext_Tags(t *testing.T) {
	tracks := []library.Track{{Artist: "Alice", Album: "Alpha", Genre: "Rock", Title: "T1", Hash: "th1", Path: "/a.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "th1", Artist: "Alice", Album: "Alpha", Title: "T1", Path: "/a.flac"},
	}
	m.tagsView.Cursor = 0

	ctx := m.focusedGoToContext()
	if !ctx.found {
		t.Fatal("expected found=true")
	}
	if ctx.artist != "Alice" {
		t.Errorf("artist: want Alice, got %q", ctx.artist)
	}
	if library.AlbumTitle(ctx.album) != "Alpha" {
		t.Errorf("album: want Alpha, got %q", ctx.album)
	}
	if ctx.genre != "Rock" {
		t.Errorf("genre: want Rock (from library lookup), got %q", ctx.genre)
	}
}

func TestFocusedGoToContext_TagsTagLevel(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewTags
	// level defaults to TagLevel
	m.tagsView.Tags = []db.TagRow{{ID: 1, Name: "T1"}}
	m.tagsView.TagCounts = map[int64]int{1: 3}

	ctx := m.focusedGoToContext()
	if ctx.found {
		t.Fatal("expected found=false at TagLevel")
	}
}

func TestBuildGoToOptions_Tags(t *testing.T) {
	tracks := []library.Track{{Artist: "Alice", Album: "Alpha", Genre: "Rock", Title: "T1", Hash: "th1", Path: "/a.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "th1", Artist: "Alice", Album: "Alpha", Title: "T1", Path: "/a.flac"},
	}
	m.tagsView.Cursor = 0

	ctx := m.focusedGoToContext()
	opts := m.buildGoToOptions(ctx)

	if len(opts) != 3 {
		t.Fatalf("want 3 options, got %d", len(opts))
	}
	if opts[0].Kind != overlay.GoToAlbum {
		t.Errorf("opts[0]: want goToAlbum, got %v", opts[0].Kind)
	}
	if opts[1].Kind != overlay.GoToArtist {
		t.Errorf("opts[1]: want goToArtist, got %v", opts[1].Kind)
	}
	if opts[2].Kind != overlay.GoToGenre {
		t.Errorf("opts[2]: want goToGenre, got %v", opts[2].Kind)
	}
}

func TestBuildGoToOptions_EmptyFieldsSkipped(t *testing.T) {
	tracks := []library.Track{{Artist: "", Album: "", Genre: "", Title: "T1", Hash: "h1", Path: "/a.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	ctx := m.focusedGoToContext()
	opts := m.buildGoToOptions(ctx)

	if len(opts) != 0 {
		t.Errorf("want 0 options for empty metadata, got %d", len(opts))
	}
}
