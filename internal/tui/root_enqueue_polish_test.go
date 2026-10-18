package tui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/albums"
	"github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/tags"
)

func TestTrackListEnqueueUsesHiddenSelection(t *testing.T) {
	tracks := []library.Track{{Hash: "one", Title: "One"}, {Hash: "two", Title: "Two"}, {Hash: "focus", Title: "Focus"}}
	selection := func() map[string]struct{} { return map[string]struct{}{"one": {}, "two": {}} }
	query := filter.State{Text: "Focus"}
	for _, tc := range []struct {
		name  string
		setup func(*Model)
	}{
		{"artists", func(m *Model) {
			m.ActiveView = viewBrowse
			m.browse.Mode = browse.BrowseModeArtists
			m.browse.Artists.Level = artists.TrackLevel
			m.browse.Artists.Tracks = tracks
			m.browse.Artists.SelectedHashes = selection()
			m.browse.Artists.Filter = query
		}},
		{"albums", func(m *Model) {
			m.ActiveView = viewBrowse
			m.browse.Mode = browse.BrowseModeAlbums
			m.browse.Albums.Level = albums.TrackLevel
			m.browse.Albums.Tracks = tracks
			m.browse.Albums.SelectedHashes = selection()
			m.browse.Albums.Filter = query
		}},
		{"genres", func(m *Model) {
			m.ActiveView = viewBrowse
			m.browse.Mode = browse.BrowseModeGenres
			m.browse.Genres.Level = genres.TrackLevel
			m.browse.Genres.Tracks = tracks
			m.browse.Genres.SelectedHashes = selection()
			m.browse.Genres.Filter = query
		}},
		{"favorite tracks", func(m *Model) {
			m.ActiveView = viewFavorites
			m.favorites.Level = favorites.ListLevel
			m.favorites.Category = favorites.CategoryTracks
			m.favorites.Tracks = tracks
			m.favorites.SelectedItems = selection()
			m.favorites.Filter = query
		}},
		{"favorite drill", func(m *Model) {
			m.ActiveView = viewFavorites
			m.favorites.Level = favorites.TrackLevel
			m.favorites.DrilldownTracks = tracks
			m.favorites.SelectedItems = selection()
			m.favorites.Filter = query
		}},
		{"tags", func(m *Model) {
			m.ActiveView = viewTags
			m.tagsView.Level = tags.TrackLevel
			m.tagsView.Tracks = []db.PlaylistTrackRow{{Hash: "one", Title: "One"}, {Hash: "two", Title: "Two"}, {Hash: "focus", Title: "Focus"}}
			m.tagsView.SelectedHashes = selection()
			m.tagsView.Filter = query
		}},
		{"mixes", func(m *Model) {
			m.ActiveView = viewMixes
			m.mixes.Level = 1
			m.mixes.TrackView.SourceTracks = tracks
			m.mixes.TrackView.Fuzzy = library.SahilmFuzzySearcher{}
			m.mixes.TrackView.SelectedHashes = selection()
			m.mixes.TrackView.Filter = query
			m.mixes.TrackView = m.mixes.TrackView.RebuildFilterCache()
		}},
		{"insights", func(m *Model) {
			m.ActiveView = viewInsights
			m.insights.OpenDrill("Test", tracks)
			m.insights.TrackView.SelectedHashes = selection()
			m.insights.TrackView.Filter = query
			m.insights.TrackView = m.insights.TrackView.RebuildFilterCache()
		}},
	} {
		for _, id := range []string{"queue.enqueue.focused", "queue.play_next", "queue.enqueue.replace"} {
			for _, palette := range []bool{false, true} {
				t.Run(tc.name+"/"+id+map[bool]string{false: "/Enter", true: "/palette"}[palette], func(t *testing.T) {
					be := &stubBatchEnqueuer{}
					store := &replacementStore{}
					m := New(t.Context(), WithBatchEnqueuer(be), WithQueueWriter(store))
					m.termWidth, m.termHeight = 60, 15
					tc.setup(&m)
					command := queuePaletteCommand(t, m, id)
					if !command.IsEnabled() || !strings.Contains(command.Detail, "2 selected tracks") {
						t.Fatalf("wrong palette scope: %+v", command)
					}
					if hint := ansi.Strip(m.renderFooterHint()); !strings.Contains(hint, "enqueue 2 selected") || !strings.Contains(hint, "X clear") {
						t.Fatalf("wrong footer: %s", hint)
					}
					var next tea.Model
					var cmd tea.Cmd
					if palette {
						next, cmd = m.Update(command.Exec()())
					} else {
						key := tea.KeyPressMsg{Code: tea.KeyEnter}
						if id == "queue.play_next" {
							key = tea.KeyPressMsg{Code: 'N', Text: "N"}
						}
						if id == "queue.enqueue.replace" {
							key.Mod = tea.ModShift
						}
						next, cmd = m.Update(key)
					}
					if cmd == nil {
						t.Fatal("missing enqueue command")
					}
					updated := next.(Model)
					switch msg := cmd().(type) {
					case BatchEnqueuedMsg:
						if msg.Err != nil {
							t.Fatal(msg.Err)
						}
						assertTrackHashes(t, be.appendTracksCalls[0], "one", "two")
						if msg.PlayNext != (id == "queue.play_next") {
							t.Fatal("wrong queue placement")
						}
					case queueReplaceRequestedMsg:
						updated = updated.handleQueueReplace(msg)
						assertTrackHashes(t, store.tracks, "one", "two")
					default:
						t.Fatalf("unexpected result %T", msg)
					}
					if updated.activeViewSelectionCount() != 0 {
						t.Fatal("selection not consumed")
					}
					if !updated.hasCommittedFilter() {
						t.Fatal("filter was cleared")
					}
				})
			}
		}
	}

}

func TestPlaylistSelectionEnqueueKeepsOccurrencesAndSort(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := New(t.Context(), WithBatchEnqueuer(be))
	m.ActiveView = viewPlaylists
	m.playlists.Level = playlists.TrackLevel
	m.playlists.Playlists = []db.PlaylistRow{{ID: 1, Name: "Focus"}}
	m.playlists.Tracks = []db.PlaylistTrackRow{{ID: 1, Hash: "same"}, {ID: 2, Hash: "other"}, {ID: 3, Hash: "same"}, {ID: 4, Hash: "unselected"}}
	m.playlists.SelectedTrackIDs = map[int64]struct{}{1: {}, 2: {}, 3: {}}
	m.playlists.SortedTrackOrder = []int{2, 0, 1, 3}
	m.playlists.Filter = filter.State{Text: "no results"}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no command for hidden playlist selection")
	}
	msg := cmd().(BatchEnqueuedMsg)
	assertTrackHashes(t, be.appendTracksCalls[0], "same", "same", "other")
	if msg.PlaylistName != "Focus" || be.appendTracksSources[0] != "Focus" {
		t.Fatal("playlist source lost")
	}
	if next.(Model).playlists.Filter.Text != "no results" {
		t.Fatal("filter lost")
	}
}

func TestLibraryQueueActionsKeepFocusAndFilter(t *testing.T) {
	for _, action := range []keys.Action{keys.ActionLibraryEnqueue, keys.ActionPlayNext, keys.ActionLibraryEnqueueReplace} {
		m := newRichFixture()
		m.library.Filter = filter.State{Text: "a:Alice"}
		m.library = m.library.RebuildFilterCache()
		m.library.Cursor, m.library.ScrollOffset = 1, 1
		before, _ := m.library.Panel().FocusedTrack()
		next, cmd := m.handleKeyAction(action)
		if cmd == nil {
			t.Fatalf("no command for %v", action)
		}
		updated := next.(Model)
		after, _ := updated.library.Panel().FocusedTrack()
		if updated.library.Filter.Text != "a:Alice" || updated.library.ScrollOffset != 1 || after.Hash != before.Hash {
			t.Fatalf("context changed for %v", action)
		}
	}
}
