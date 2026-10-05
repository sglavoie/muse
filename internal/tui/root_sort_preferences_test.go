package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestSortPreferencesRestartResetAndModeIsolation(t *testing.T) {
	dir := t.TempDir()
	newSession := func() Model {
		return New(context.Background(), WithDataDir(dir), WithLibraryTracks(sampleLibraryTracks()))
	}
	m := newSession()
	m, _ = updateModel(m, selectSortFieldMsg{sorting.SortDuration})
	m, _ = updateModel(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	want := sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortDesc}
	if m.library.SortState != want {
		t.Fatal(m.library.SortState)
	}
	m, _ = updateModel(m, navigateLibraryModeMsg{libtui.LibraryModeRecent})
	m, _ = updateModel(m, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	m = newSession()
	if m.library.SortState != want || m.library.Panel().VisibleTracks()[0].Hash != "h6" {
		t.Fatal("restart lost library sort or failed to rebuild displayed rows")
	}
	m, _ = updateModel(m, navigateLibraryModeMsg{libtui.LibraryModeRecent})
	if m.library.SortState != (sorting.SortState{Field: sorting.SortDateAdded, Dir: sorting.SortAsc}) {
		t.Fatal("Recent preference contaminated by All", m.library.SortState)
	}
	m, _ = updateModel(m, navigateLibraryModeMsg{libtui.LibraryModeAll})
	if m.library.SortState != want {
		t.Fatal("returning to All lost saved sort")
	}
	// A view operation can reset its live sort before the user explicitly
	// chooses default order. That choice must still replace the saved value.
	m = m.resetLibrarySort()
	m, _ = updateModel(m, selectSortFieldMsg{sorting.SortDefault})
	if got := newSession().library.SortState; got != (sorting.SortState{}) {
		t.Fatal("default reset did not survive restart", got)
	}
}

func TestSortPreferencesSeparatePlaylistListAndTracks(t *testing.T) {
	dir := t.TempDir()
	m := newRichFixture()
	m.dataDir = dir
	m.ActiveView = viewPlaylists
	m, _ = updateModel(m, selectSortFieldMsg{sorting.SortName})
	m, _ = updateModel(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	m = drillIntoPlaylistTrackLevel(m, richPlaylistTracks()[1])
	m, _ = updateModel(m, selectSortFieldMsg{sorting.SortDuration})
	m, _ = updateModel(m, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})

	restored := newRichFixture()
	restored.dataDir = dir
	restored = restored.loadSortPreferences()
	restored, _ = updateModel(restored, commandPaletteActionMsg{action: keys.ActionSwitchPlaylists})
	if restored.playlists.Level != pltui.ListLevel || restored.playlists.SortState != (sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}) {
		t.Fatal("playlist list sort was not restored", restored.playlists.SortState)
	}
	// Arriving at track level is a separate scope, as when the database reply
	// opens a playlist. Its preference must not inherit the list's Name field.
	restored = drillIntoPlaylistTrackLevel(restored, richPlaylistTracks()[1])
	restored = restored.restoreSortPreference()
	if restored.playlists.SortState != (sorting.SortState{Field: sorting.SortDuration}) {
		t.Fatal("playlist tracks did not restore their own sort", restored.playlists.SortState)
	}
}

func TestSortPreferencesQueuePreservesPlaybackOrderAndFocus(t *testing.T) {
	m := newRichFixture()
	m.ActiveView = viewQueue
	before := append([]string(nil), m.filterRowKeys()...)
	m.queue.Cursor = 1
	focused := before[1]
	m.sortPreferences = map[string]sorting.SortState{"queue": {Field: sorting.SortDuration, Dir: sorting.SortDesc}}
	m = m.restoreSortPreference()
	if m.filterRowKeys()[m.queue.Cursor] != focused {
		t.Fatal("restoring sort moved focus to a different queue occurrence")
	}
	// Reset only the display sort to inspect the original playback rows.
	m = m.resetQueueSort()
	if got := strings.Join(m.filterRowKeys(), ","); got != strings.Join(before, ",") {
		t.Fatal("display preference reordered underlying queue", got)
	}
}

func TestSortPreferencesApplyAfterAsyncMixLoad(t *testing.T) {
	m := newRichFixture()
	m.ActiveView = viewMixes
	m.sortPreferences = map[string]sorting.SortState{"mixes.1": {Field: sorting.SortDuration, Dir: sorting.SortDesc}}
	m.mixes.CachedMixes = nil
	m.mixes.OpenMix(db.MixRediscover)
	m = m.restoreSortPreference()
	m, _ = updateModel(m, mixLoadedMsg{mixID: db.MixRediscover, tracks: sampleLibraryTracks()})
	if got := m.mixes.TrackView.Panel().VisibleTracks()[0].Hash; got != "h6" {
		t.Fatal("initial async population lost restored sort", got)
	}
}

func TestSortPreferencesPreserveJumpTargetInAlreadyOpenBrowse(t *testing.T) {
	m := newModelWithTracks().jumpToArtist("Alice")
	m, _ = updateModel(m, selectSortFieldMsg{sorting.SortName})
	m, _ = updateModel(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	m = m.jumpToArtist("Alice")
	name, ok := m.browse.Artists.FocusedArtist()
	if !ok || name != "Alice" || m.browse.Artists.Artists[0] != "Charlie" {
		t.Fatal("jump lost its target or saved order", name, m.browse.Artists.Artists)
	}
}

func TestSortPreferencesInvalidFilesAndSaveFailure(t *testing.T) {
	for _, data := range []string{
		`{`,
		`{"version":99,"views":{"library.0":{"Field":4,"Dir":1}}}`,
		`{"version":1,"views":{"library.0":{"Field":999,"Dir":1}}}`,
		`{"version":1,"views":{"library.0":{"Field":4,"Dir":999}}}`,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, sortPreferencesFile), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		m := New(context.Background(), WithDataDir(dir), WithLibraryTracks(sampleLibraryTracks()))
		if m.library.SortState != (sorting.SortState{}) {
			t.Fatalf("invalid preferences applied: %s", data)
		}
	}
	m := newModelWithTracks()
	m.dataDir = filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(m.dataDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ = updateModel(m, selectSortFieldMsg{sorting.SortDuration})
	if m.library.SortState.Field != sorting.SortDuration || !strings.Contains(m.statusBar.TopNotification().Msg, "not saved") {
		t.Fatal("save failure lost live sort or was not reported")
	}
}
