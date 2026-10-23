package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func restoreTestBrowsing(t *testing.T, m Model, saved browsingSession) Model {
	t.Helper()
	m.browsingRestore = &browsingRestore{Session: saved}
	next, cmd := m.Update(browsingResumeMsg{})
	return browsingTestDrain(t, next.(Model), cmd)
}

func TestBrowsingSessionFocusAndViewportUseTrackIdentities(t *testing.T) {
	var tracks []library.Track
	for i := range 50 {
		tracks = append(tracks, library.Track{Hash: fmt.Sprint(i), Title: fmt.Sprintf("Song %02d", i), Artist: "Alice"})
	}
	m := newRichFixture().handleWindowSizeMsg(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.library.Index.BulkLoad(tracks)
	m.library = m.library.RefreshView()
	m.library.Cursor, m.library.ScrollOffset = 25, 20
	saved := m.captureBrowsingSession()
	tracks = append([]library.Track{{Hash: "new", Title: "AAA", Artist: "Alice"}}, tracks...)
	fresh := newRichFixture().handleWindowSizeMsg(tea.WindowSizeMsg{Width: 80, Height: 24})
	fresh.library.Index.BulkLoad(tracks)
	fresh.library = fresh.library.RefreshView()
	fresh = restoreTestBrowsing(t, fresh, saved)
	keys := fresh.browsingKeys()
	if keys[fresh.library.Cursor] != "25" || keys[fresh.library.ScrollOffset] != "20" {
		t.Fatalf("lost focus/viewport: %v", fresh.browsingPosition())
	}
	fresh = fresh.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: 15})
	if fresh.browsingPosition().Key != "25" || fresh.library.ScrollOffset > fresh.library.Cursor {
		t.Fatal("resize lost focused identity")
	}
}

func TestBrowsingSessionQueueDistinguishesRepeatedSongs(t *testing.T) {
	m := newRichFixture().switchToView(viewQueue)
	m.queue.Entries = []queue.Entry{{ID: 21, Hash: "song", Title: "Song"}, {ID: 22, Hash: "song", Title: "Song"}}
	m.queue.Cursor = 1
	saved := m.captureBrowsingSession()
	fresh := newRichFixture()
	fresh.queue.Entries = []queue.Entry{{ID: 22, Hash: "song", Title: "Song"}, {ID: 21, Hash: "song", Title: "Song"}}
	fresh = restoreTestBrowsing(t, fresh, saved)
	if fresh.browsingPosition().Key != "id:22" {
		t.Fatal("restored a different occurrence of the song")
	}
}

func TestBrowsingSessionDeletedTrackClampsAndRetainsFilter(t *testing.T) {
	m := newRichFixture()
	m.library.Filter = filter.State{Text: "a:Alice"}
	m.library = m.library.RefreshView()
	m.library.Cursor = 2
	saved := m.captureBrowsingSession()
	fresh := newRichFixture()
	fresh.library.Index.BulkLoad(richLibraryTracks()[:1])
	fresh.library = fresh.library.RefreshView()
	fresh = restoreTestBrowsing(t, fresh, saved)
	if fresh.library.Cursor != 0 || fresh.library.Filter.Text != "a:Alice" {
		t.Fatal("deleted focus did not clamp within the restored filter")
	}
}

func TestBrowsingSessionRestoresLibraryModeSortAndTagFilter(t *testing.T) {
	for _, mode := range []libtui.LibraryMode{libtui.LibraryModeAll, libtui.LibraryModeRecent, libtui.LibraryModeHistory} {
		m := newRichFixture().handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
		m.library = m.library.SwitchMode(mode)
		m = m.switchToView(viewLibrary)
		fields, _ := m.activeSortFields()
		m.library.SortState = sorting.SortState{Field: fields[len(fields)-1], Dir: sorting.SortDesc}
		m.library = m.library.RefreshView()
		if mode == libtui.LibraryModeAll {
			id := m.tags[0].ID
			m.activeTagFilter = &id
			m = m.applyTagFilterToViews()
		}
		browsingTestRoundTrip(t, m)
	}
}

func TestBrowsingSessionFavoriteAlbumAndTrackCategories(t *testing.T) {
	for _, category := range []int{1, 2} {
		m := newRichFixture().handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30}).switchToView(viewFavorites)
		m.favorites.Cursor = category
		m = browsingTestEnter(t, m)
		browsingTestRoundTrip(t, m)
		if category == 1 {
			m = browsingTestEnter(t, m)
			browsingTestRoundTrip(t, m)
		}
	}
}

func TestBrowsingSessionLoadsThroughStartupAndKeepsPositionOnFirstResize(t *testing.T) {
	m := newRichFixture()
	m.dataDir = t.TempDir()
	m.library.Cursor = 2
	if err := m.SaveBrowsingSession(); err != nil {
		t.Fatal(err)
	}
	fresh := New(m.appCtx, WithDataDir(m.dataDir), WithLibraryTracks(richLibraryTracks()))
	if fresh.browsingRestore == nil {
		t.Fatal("New did not load browsing context")
	}
	init := fresh.Init()().(tea.BatchMsg)
	resume, ok := init[0]().(browsingResumeMsg)
	if !ok {
		t.Fatal("Init did not schedule restoration")
	}
	next, cmd := fresh.Update(resume)
	fresh = browsingTestDrain(t, next.(Model), cmd)
	fresh = fresh.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: 15})
	if fresh.browsingPosition().Key != m.browsingPosition().Key {
		t.Fatal("first window size lost restored focus")
	}
}
