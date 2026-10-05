package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/queue"
)

func albumIdentityFixture() (Model, library.Track, library.Track) {
	a := library.Track{Hash: "a", Path: "/one/a.flac", Album: "Hits", Artist: "Guest", AlbumArtist: "First Artist"}
	b := library.Track{Hash: "b", Path: "/two/b.flac", Album: "Hits", Artist: "Guest", AlbumArtist: "Second Artist"}
	return New(context.Background(), WithLibraryTracks([]library.Track{a, b})), a, b
}

func TestAlbumNavigationAndRenameStayWithinRelease(t *testing.T) {
	m, a, b := albumIdentityFixture()
	m = m.jumpToAlbum(library.AlbumKey(b))
	key, ok := m.browse.Albums.FocusedAlbum()
	if !ok || key != library.AlbumKey(b) {
		t.Fatalf("wrong release: %s", key)
	}
	ed, ok := m.renameEditorForFocusedEntity()
	if !ok || ed.Album() != "Hits" || len(ed.Hashes()) != 1 || ed.Hashes()[0] != b.Hash {
		t.Fatalf("rename spans releases: %+v", ed)
	}
	m.overlay.metadataEditor = &ed
	m = backspaceInEditor(m, 4)
	m = typeIntoEditor(m, "Renamed")
	next, _ := m.handleMetadataSave()
	m = next.(Model)
	untouched, _ := m.library.Index.FindByHash(a.Hash)
	renamed, _ := m.library.Index.FindByHash(b.Hash)
	if untouched.Album != "Hits" || renamed.Album != "Renamed" {
		t.Fatalf("wrong metadata: %+v / %+v", untouched, renamed)
	}
	focused, _ := m.browse.Albums.FocusedAlbum()
	if focused != library.AlbumKey(renamed) {
		t.Fatal("lost renamed release focus")
	}
}

func TestAlbumTargetsFromQueueAndNowPlayingUseRelease(t *testing.T) {
	m, _, b := albumIdentityFixture()
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{{Hash: b.Hash, Path: b.Path, Artist: b.Artist, Album: b.Album, AlbumArtist: b.AlbumArtist}}
	ctx := m.focusedGoToContext()
	if ctx.album != library.AlbumKey(b) {
		t.Fatalf("queue target: %s", ctx.album)
	}
	var target string
	for _, option := range m.nowPlayingBrowseOptions(b.Hash) {
		if option.Kind == overlay.GoToAlbum {
			target = option.Value
		}
	}
	if target != library.AlbumKey(b) {
		t.Fatalf("playing target: %s", target)
	}
	m = m.jumpToAlbum(target)
	m = pressEnter(m)
	if len(m.browse.Albums.Tracks) != 1 || m.browse.Albums.Tracks[0].Hash != b.Hash {
		t.Fatal("drilldown crossed releases")
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	screen := next.(Model).View().Content
	if !strings.Contains(screen, "Second Artist") {
		t.Fatal("release owner missing from breadcrumb")
	}
	if strings.Contains(screen, "muse:album:") {
		t.Fatal("internal album key leaked into display")
	}
}

func TestCompilationGroupingAndAlbumFilter(t *testing.T) {
	tracks := []library.Track{
		{Hash: "a", Path: "/mix/a.flac", Artist: "A", Album: "Mix"},
		{Hash: "b", Path: "/mix/b.flac", Artist: "B", Album: "Mix"},
		{Hash: "c", Path: "/other/c.flac", Artist: "A", Album: "Mix"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = m.jumpToAlbum(library.AlbumKey(tracks[0]))
	filtered := arttui.FilterStrings(m.browse.Albums.Albums, "/mix")
	if len(filtered) != 1 || filtered[0] != library.AlbumKey(tracks[0]) {
		t.Fatal("folder filter did not isolate release")
	}
	if len(m.browse.Albums.Albums) != 2 {
		t.Fatal("folder fallback failed")
	}
	m = pressEnter(m)
	if len(m.browse.Albums.Tracks) != 2 {
		t.Fatal("compilation was split by track artist")
	}
	if rows := albtui.DeriveAlbumTracks(tracks, library.AlbumKey(tracks[2])); len(rows) != 1 {
		t.Fatal("folder releases mixed")
	}
	// An ambiguous title-only jump must not select an arbitrary release.
	m.ActiveView = viewLibrary
	m = m.jumpToAlbum("Mix")
	if m.ActiveView != viewLibrary {
		t.Fatal("ambiguous title navigated to an arbitrary release")
	}
}

func TestWatchedAlbumMoveKeepsFavoritesAndDrilldown(t *testing.T) {
	track := library.Track{Hash: "a", Path: "/old/a.flac", Album: "Mix", Artist: "A"}
	m := New(context.Background(), WithLibraryTracks([]library.Track{track}))
	m.favoriteAlbums = map[string]struct{}{library.AlbumKey(track): {}}
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeAlbums
	m.browse.Albums.Level = albtui.TrackLevel
	m.browse.Albums.SelectedAlbum = library.AlbumKey(track)
	m.browse.Albums.Tracks = []library.Track{track}
	old := library.AlbumKey(track)
	track.Path = "/new/a.flac"
	m, _ = m.handleWatchBatchMsg(watchBatchMsg{deleted: []string{"/old/a.flac"}})
	m, _ = m.handleWatchBatchMsg(watchBatchMsg{tracks: []library.Track{track}})
	if _, ok := m.favoriteAlbums[library.AlbumKey(track)]; !ok {
		t.Fatal("favorite lost on move")
	}
	if _, ok := m.favoriteAlbums[old]; ok {
		t.Fatal("stale favorite retained after whole album moved")
	}
	if m.browse.Albums.SelectedAlbum != library.AlbumKey(track) || len(m.browse.Albums.Tracks) != 1 {
		t.Fatal("drilldown lost moved release")
	}
}

func TestAlbumDrilldownSurvivesDelayedRescanBatch(t *testing.T) {
	m, a, _ := albumIdentityFixture()
	m = m.jumpToAlbum(library.AlbumKey(a))
	m = pressEnter(m)
	m.rescanSnapshot = map[string]rescanEntry{a.Hash: {AlbumKey: library.AlbumKey(a)}}
	m.library.Index = library.LibraryIndex{}
	m = m.refreshBrowseDerivedViews()
	if len(m.browse.Albums.Tracks) != 0 {
		t.Fatal("rescan should temporarily empty this group")
	}
	a.AlbumArtist = "Corrected Artist"
	m.library.Index.Insert(a)
	m = m.refreshBrowseDerivedViews()
	if m.browse.Albums.SelectedAlbum != library.AlbumKey(a) || len(m.browse.Albums.Tracks) != 1 {
		t.Fatal("rescan lost the album after an earlier empty batch")
	}
}
