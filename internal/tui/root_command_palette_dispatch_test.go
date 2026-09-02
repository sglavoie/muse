package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/keys"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// dispatchPaletteAction sends msg through Update and returns the resulting
// model, mirroring how the palette's Exec command reaches the root.
func dispatchPaletteAction(t *testing.T, m Model, msg commandPaletteActionMsg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	out, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return out
}

func TestCommandPaletteActionSwitchesToViewThenRuns(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary

	out := dispatchPaletteAction(t, m, commandPaletteActionMsg{action: keys.ActionTagCreate, view: viewTags})

	if out.activeView != viewTags {
		t.Fatalf("activeView = %v, want viewTags", out.activeView)
	}
	if !out.tagsView.isCreating() {
		t.Error("tag create input is not active: the action did not run after the switch")
	}
}

func TestCommandPaletteActionWithoutViewRunsInPlace(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary

	out := dispatchPaletteAction(t, m, commandPaletteActionMsg{action: keys.ActionTagCreate})

	if out.activeView != viewLibrary {
		t.Fatalf("activeView = %v, want viewLibrary", out.activeView)
	}
	if out.tagsView.isCreating() {
		t.Error("tag create input is active: the action ran outside the Tags view")
	}
}

// TestCommandPaletteActionSameViewKeepsState pins the m.activeView != msg.view
// guard: switchToView resets every sort and cancels in-flight create/rename
// inputs, so re-switching to the active view would discard user state.
func TestCommandPaletteActionSameViewKeepsState(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists
	m.playlists.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	m.playlists = m.playlists.startRename("Mix tape")

	out := dispatchPaletteAction(t, m, commandPaletteActionMsg{action: keys.ActionQueueSummary, view: viewPlaylists})

	if out.playlists.sortState != (sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}) {
		t.Errorf("playlists sortState = %+v, want it preserved", out.playlists.sortState)
	}
	if !out.playlists.isRenaming() {
		t.Error("in-flight rename was cancelled by a same-view dispatch")
	}
	if out.overlay.queueSummary == nil {
		t.Error("queue summary overlay is nil: the action did not run")
	}
}

// TestCommandPaletteActionSwitchToMixesKeepsLoadCmd covers why the switch is
// routed through handleKeyAction: actionSwitchMixes returns mixesMaybeLoadCmd(),
// which calling switchToView directly would drop.
func TestCommandPaletteActionSwitchToMixesKeepsLoadCmd(t *testing.T) {
	m := New(context.Background(), WithRecommendationReader(&stubRecReader{sufficient: true}))
	m.activeView = viewLibrary

	next, cmd := m.Update(commandPaletteActionMsg{action: keys.ActionQueueSummary, view: viewMixes})
	out, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}

	if out.activeView != viewMixes {
		t.Fatalf("activeView = %v, want viewMixes", out.activeView)
	}
	if cmd == nil {
		t.Error("no command returned: the mixes load command was dropped")
	}
	if out.overlay.queueSummary == nil {
		t.Error("queue summary overlay is nil: the action did not run")
	}
}

// TestCommandPaletteActionSwitchToQueueFocusesNowPlaying covers the second
// reason for routing through handleKeyAction: actionSwitchQueue's
// FocusNowPlaying is a state change, not a tea.Cmd, so tea.Batch alone could
// not preserve it.
func TestCommandPaletteActionSwitchToQueueFocusesNowPlaying(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.mp3"},
		{Hash: "h2", Path: "/b.mp3"},
		{Hash: "h3", Path: "/c.mp3"},
	}
	m.playback.currentIndex = 2

	out := dispatchPaletteAction(t, m, commandPaletteActionMsg{action: keys.ActionQueueSummary, view: viewQueue})

	if out.activeView != viewQueue {
		t.Fatalf("activeView = %v, want viewQueue", out.activeView)
	}
	if out.queue.cursor != 2 {
		t.Errorf("queue cursor = %d, want 2 (FocusNowPlaying side effect lost)", out.queue.cursor)
	}
}

// paletteCommandEnabledForTest reports whether the palette would let the user
// select the named command against m.
func paletteCommandEnabledForTest(t *testing.T, m Model, name string) bool {
	t.Helper()
	for _, command := range m.buildPaletteCommands() {
		if command.Name == name {
			return command.Enabled == nil || command.Enabled()
		}
	}
	t.Fatalf("palette command %q not found", name)
	return false
}

// TestPaletteDeletePlaylistOnlyAtListLevel pins the palette-side guard on
// actionPlaylistDelete: the shared handler removes the focused or selected
// tracks at playlist track level, which the "Delete playlist" row does not
// advertise, so only the list-level deletion may run from the palette.
func TestPaletteDeletePlaylistOnlyAtListLevel(t *testing.T) {
	t.Run("track level removes nothing", func(t *testing.T) {
		pd := &stubPlaylistDeleter{}
		m := New(context.Background(), WithPlaylistDeleter(pd),
			WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Alpha"}, {ID: 2, Name: "Beta"}}))
		m.activeView = viewPlaylists
		m.playlists.level = pltui.TrackLevel
		m.playlists.tracks = []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "A"}, {ID: 20, Position: 2, Title: "B"}}
		m.playlists.cursor = 1

		if paletteCommandEnabledForTest(t, m, "Delete playlist") {
			t.Error("Delete playlist is selectable at playlist track level")
		}

		out := runPaletteCommandForTest(t, m, "Delete playlist")

		if n := len(out.playlists.tracks); n != 2 {
			t.Errorf("playlist tracks = %d, want 2: the palette command removed tracks", n)
		}
		if n := len(pd.removePlaylistTrackSoftCalls); n != 0 {
			t.Errorf("RemovePlaylistTrackSoft calls = %d, want 0", n)
		}
		if n := len(out.playlists.playlists); n != 2 {
			t.Errorf("playlists = %d, want 2", n)
		}
		if len(pd.deletePlaylistSoftCalls) != 0 {
			t.Errorf("DeletePlaylistSoft calls = %v, want none", pd.deletePlaylistSoftCalls)
		}
		if out.statusBar.TopNotification().Msg == "" {
			t.Error("the refusal is silent: no notification was shown")
		}
	})

	t.Run("list level deletes the focused playlist", func(t *testing.T) {
		pd := &stubPlaylistDeleter{}
		m := New(context.Background(), WithPlaylistDeleter(pd),
			WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Alpha"}, {ID: 2, Name: "Beta"}}))
		m.activeView = viewPlaylists
		m.playlists.cursor = 1

		if !paletteCommandEnabledForTest(t, m, "Delete playlist") {
			t.Error("Delete playlist is dimmed at playlist list level")
		}

		out := runPaletteCommandForTest(t, m, "Delete playlist")

		if n := len(out.playlists.playlists); n != 1 {
			t.Fatalf("playlists = %d, want 1", n)
		}
		if out.playlists.playlists[0].ID != 1 {
			t.Errorf("remaining playlist ID = %d, want 1", out.playlists.playlists[0].ID)
		}
		if len(pd.deletePlaylistSoftCalls) != 1 || pd.deletePlaylistSoftCalls[0] != 2 {
			t.Errorf("DeletePlaylistSoft calls = %v, want [2]", pd.deletePlaylistSoftCalls)
		}
	})
}

// applyCmdMsgs feeds every message cmd's tree produces back through Update.
// Commands returned by those updates are dropped, which is enough to observe a
// create input started by a single follow-up message such as PlaylistCreateMsg.
func applyCmdMsgs(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = applyCmdMsgs(t, m, c)
		}
		return m
	}
	if msg == nil {
		return m
	}
	next, _ := m.Update(msg)
	out, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return out
}

// runPaletteCommandWithFollowUps runs the named palette command like
// runPaletteCommandForTest, then applies the messages its command tree emits.
func runPaletteCommandWithFollowUps(t *testing.T, m Model, name string) Model {
	t.Helper()
	for _, command := range m.buildPaletteCommands() {
		if command.Name != name {
			continue
		}
		if command.Exec == nil {
			t.Fatalf("palette command %q has no Exec", name)
		}
		next, cmd := m.Update(command.Exec()())
		out, ok := next.(Model)
		if !ok {
			t.Fatalf("palette command %q returned model %T", name, next)
		}
		return applyCmdMsgs(t, out, cmd)
	}
	t.Fatalf("palette command %q not found", name)
	return m
}

// TestPaletteCreateCommandsHandleNestedDestination pins the drill-out that
// makes the create commands work from another view: switchToView preserves the
// destination's level, and every create handler is guarded on the collection
// level, so without it an enabled pick moved the user and did nothing.
func TestPaletteCreateCommandsHandleNestedDestination(t *testing.T) {
	nestedPlaylists := func() Model {
		m := New(context.Background(), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Alpha"}, {ID: 2, Name: "Beta"}}))
		m.activeView = viewLibrary
		m.playlists.level = pltui.TrackLevel
		m.playlists.tracks = []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "A"}}
		m.playlists.cursor = 0
		return m
	}

	t.Run("create playlist", func(t *testing.T) {
		m := nestedPlaylists()
		if !paletteCommandEnabledForTest(t, m, "Create playlist") {
			t.Fatal("Create playlist is dimmed, so the no-op is not reachable through it")
		}

		out := runPaletteCommandWithFollowUps(t, m, "Create playlist")

		if out.activeView != viewPlaylists {
			t.Fatalf("activeView = %v, want viewPlaylists", out.activeView)
		}
		if !out.playlists.atListLevel() {
			t.Error("playlists view is still at track level after the switch")
		}
		if !out.playlists.isCreating() {
			t.Error("playlist create input is not active: the command was a silent no-op")
		}
	})

	t.Run("create smart playlist", func(t *testing.T) {
		m := nestedPlaylists()
		if !paletteCommandEnabledForTest(t, m, "Create smart playlist") {
			t.Fatal("Create smart playlist is dimmed, so the no-op is not reachable through it")
		}

		out := runPaletteCommandWithFollowUps(t, m, "Create smart playlist")

		if out.activeView != viewPlaylists {
			t.Fatalf("activeView = %v, want viewPlaylists", out.activeView)
		}
		if !out.playlists.atListLevel() {
			t.Error("playlists view is still at track level after the switch")
		}
		if out.overlay.smartRuleEditor == nil {
			t.Error("smart rule editor is nil: the command was a silent no-op")
		}
	})

	t.Run("create tag", func(t *testing.T) {
		m := New(context.Background())
		m.activeView = viewLibrary
		m.tagsView.level = tagtui.TrackLevel
		m.tagsView.tags = []db.TagRow{{ID: 1, Name: "chill"}}
		m.tagsView.selectedTag = db.TagRow{ID: 1, Name: "chill"}

		if !paletteCommandEnabledForTest(t, m, "Create tag") {
			t.Fatal("Create tag is dimmed, so the no-op is not reachable through it")
		}

		out := runPaletteCommandWithFollowUps(t, m, "Create tag")

		if out.activeView != viewTags {
			t.Fatalf("activeView = %v, want viewTags", out.activeView)
		}
		if out.tagsView.level != tagtui.TagLevel {
			t.Error("tags view is still at track level after the switch")
		}
		if !out.tagsView.isCreating() {
			t.Error("tag create input is not active: the command was a silent no-op")
		}
	})
}

func TestViewSwitchActionsCoverPrimaryViews(t *testing.T) {
	for _, id := range viewOrder {
		if _, ok := viewSwitchActions[id]; !ok {
			t.Errorf("view %q (%d) has no switch action", viewName(id), id)
		}
	}
	if len(viewSwitchActions) != len(viewOrder) {
		t.Errorf("viewSwitchActions has %d entries, want %d (one per primary view)", len(viewSwitchActions), len(viewOrder))
	}
	for _, legacy := range []viewID{viewArtists, viewAlbums, viewGenres} {
		if _, ok := viewSwitchActions[legacy]; ok {
			t.Errorf("legacy sub-view %q is in viewSwitchActions", viewName(legacy))
		}
	}
}
