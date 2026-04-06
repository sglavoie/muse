package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
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
	if ctx.album != "Alpha" {
		t.Errorf("album: want Alpha, got %q", ctx.album)
	}
	if ctx.genre != "Rock" {
		t.Errorf("genre: want Rock, got %q", ctx.genre)
	}
}

func TestFocusedGoToContext_Queue(t *testing.T) {
	tracks := []library.Track{{Artist: "Bob", Album: "Beta", Genre: "Jazz", Title: "T1", Hash: "h1", Path: "/b.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{{Artist: "Bob", Album: "Beta", Hash: "h1", SourcePlaylist: "MyPL"}}
	m.queue.cursor = 0
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
	if opts[0].Kind != goToAlbum {
		t.Errorf("opts[0]: want goToAlbum, got %v", opts[0].Kind)
	}
	if opts[1].Kind != goToArtist {
		t.Errorf("opts[1]: want goToArtist, got %v", opts[1].Kind)
	}
	if opts[2].Kind != goToGenre {
		t.Errorf("opts[2]: want goToGenre, got %v", opts[2].Kind)
	}
}

func TestBuildGoToOptions_QueueWithSource(t *testing.T) {
	tracks := []library.Track{{Artist: "Bob", Album: "Beta", Genre: "Jazz", Title: "T1", Hash: "h1", Path: "/b.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{{Artist: "Bob", Album: "Beta", Hash: "h1", SourcePlaylist: "MyPL"}}
	m.queue.cursor = 0
	ctx := m.focusedGoToContext()
	opts := m.buildGoToOptions(ctx)

	if len(opts) != 4 {
		t.Fatalf("want 4 options, got %d", len(opts))
	}
	if opts[3].Kind != goToPlaylist || opts[3].Value != "MyPL" {
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
	if opts[2].Kind != goToGenre || opts[2].Value != "Jazz" {
		t.Errorf("opts[2]: want goToGenre/Jazz, got %v/%q", opts[2].Kind, opts[2].Value)
	}
	if opts[3].Kind != goToGenre || opts[3].Value != "Rock" {
		t.Errorf("opts[3]: want goToGenre/Rock, got %v/%q", opts[3].Kind, opts[3].Value)
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
