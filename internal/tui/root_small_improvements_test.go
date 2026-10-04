package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/desktop"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestFilterHistorySurvivesRestartWithoutApplying(t *testing.T) {
	dir := t.TempDir()
	m := New(context.Background(), WithDataDir(dir))
	m = pressEnter(typeText(pressSlash(m), "Alice"))
	m = pressEsc(m)
	m = pressEsc(typeText(pressSlash(m), "cancelled"))
	if len(m.filterHistory) != 1 {
		t.Fatal("cancelled query entered history")
	}
	restored := New(context.Background(), WithDataDir(dir))
	if restored.hasCommittedFilter() || restored.library.Filter.Active {
		t.Fatal("history applied on restart")
	}
	restored = pressSlash(restored)
	next, _ := restored.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	restored = next.(Model)
	if restored.library.Filter.Text != "Alice" {
		t.Fatal(restored.library.Filter.Text)
	}
}

func TestTrackInfoClipboardResultAndFixtureGuard(t *testing.T) {
	m := New(context.Background())
	info := overlay.NewTrackInfoOverlay("/music/song.flac", nil, 0, nil)
	m.overlay.trackInfo = &info
	m.fixtureSafeCommands = true
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(Model)
	if cmd != nil || !strings.Contains(m.statusBar.TopNotification().Msg, "blocked") {
		t.Fatal("fixture could copy")
	}
	for _, result := range []desktop.ClipboardResultMsg{
		{Text: "/music/song.flac"}, {Err: errors.New("clipboard unavailable")},
	} {
		next, _ = m.Update(result)
		m = next.(Model)
		want := "Copied: /music/song.flac"
		if result.Err != nil {
			want = "Copy failed: clipboard unavailable"
		}
		if m.statusBar.TopNotification().Msg != want {
			t.Fatal(m.statusBar.TopNotification().Msg)
		}
	}
}

func TestFilterPaletteAvailability(t *testing.T) {
	m := newRichFixture()
	for _, view := range []viewID{viewLibrary, viewQueue, viewPlaylists, viewMixes, viewInsights} {
		m.ActiveView = view
		cmd := polishCommand(t, m, "view.filter")
		want := view != viewMixes && view != viewInsights
		if cmd.IsEnabled() != want {
			t.Fatalf("view %v enabled=%v", view, cmd.IsEnabled())
		}
		if !want && cmd.DisabledReason == "" {
			t.Fatal("missing explanation")
		}
	}
}

func TestFilteredPlaylistDeleteUndoAndNoMatches(t *testing.T) {
	m := drillIntoPlaylistTrackLevel(newRichFixture(), []db.PlaylistTrackRow{
		{ID: 11, Hash: "b", Title: "Hidden", Artist: "Bob"},
		{ID: 12, Hash: "a", Title: "Zulu", Artist: "Alice"},
		{ID: 13, Hash: "a", Title: "Zulu", Artist: "Alice"},
		{ID: 14, Hash: "c", Title: "Alpha", Artist: "Alice"},
	})
	m.playlists.SortState = sorting.SortState{Field: sorting.SortTitle}
	m.playlists = m.playlists.RecomputePlaylistSort()
	m = pressEnter(typeText(pressSlash(m), "Alice"))
	m.playlists.Cursor = 2
	m, _ = m.handlePlaylistDeleteSingleTrack()
	if len(m.playlists.Tracks) != 3 || m.playlists.Tracks[1].ID != 12 || m.playlists.Tracks[2].ID != 14 {
		t.Fatal(m.playlists.Tracks)
	}
	m = m.handleUndo()
	rows := m.playlists.DisplayTracks()
	if len(rows) != 3 || rows[m.playlists.Cursor].ID != 13 {
		t.Fatal(rows, m.playlists.Cursor)
	}
	m.playlists.Filter = filter.State{Text: "absent"}
	m, _ = m.handlePlaylistDeleteSingleTrack()
	if len(m.playlists.Tracks) != 4 {
		t.Fatal("no-match delete removed a hidden track")
	}
	if _, cmd, _ := m.handlePlaylistOpen(); cmd != nil {
		t.Fatal("no-match enqueue returned command")
	}
	m.playlists.Filter = filter.State{Text: "Alice"}
	m, cmd := m.shiftPlaylistTrack(1)
	if cmd == nil || m.playlists.Tracks[0].ID != 11 {
		t.Fatal("filtered reordering was not blocked")
	}
}

func TestFilteredPlaylistEnqueueAndVisualSelection(t *testing.T) {
	m := drillIntoPlaylistTrackLevel(newRichFixture(), richPlaylistTracks()[1])
	m.playlists.Filter = filter.State{Text: "Alice"}
	m.playlists.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
	m.playlists = m.playlists.RecomputePlaylistSort()
	_, cmd, _ := m.handlePlaylistOpen()
	if cmd == nil {
		t.Fatal("missing enqueue command")
	}
	result := cmd().(libtui.TrackEnqueuedMsg)
	if result.Err != nil || result.Track.Hash != "rh3" {
		t.Fatal(result)
	}
	m = doNav(doVisual(m), keys.ActionGoToBottom)
	if len(m.playlists.SelectedTrackIDs) != 2 {
		t.Fatal(m.playlists.SelectedTrackIDs)
	}
	if _, ok := m.playlists.SelectedTrackIDs[13]; ok {
		t.Fatal("hidden row selected")
	}
	m, _ = m.handlePlaylistDeleteAtTrackLevel()
	if len(m.playlists.Tracks) != 1 || m.playlists.Tracks[0].ID != 13 {
		t.Fatal(m.playlists.Tracks)
	}
	m = m.handleUndo()
	if len(m.playlists.DisplayTracks()) != 2 {
		t.Fatal("bulk undo failed")
	}
	m = pressEsc(m)
	if m.playlists.Filter.Text != "" {
		t.Fatal("Esc did not clear filter")
	}
}
