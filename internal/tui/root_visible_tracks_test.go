package tui

import (
	"slices"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func visibleHashes(t *testing.T, m Model) []string {
	t.Helper()
	tracks, label, ok := m.visibleTracks()
	if !ok {
		t.Fatalf("visibleTracks refused: %s", label)
	}
	hashes := make([]string, 0, len(tracks))
	for _, tr := range tracks {
		hashes = append(hashes, tr.Hash)
	}
	return hashes
}

// The library case is the one that carries both divergences from
// bulkActionTracks: the fuzzy filter is honored, and an empty selection means
// "everything visible" rather than the focused row.
func TestVisibleTracksLibraryHonorsFilterWithoutFocusedFallback(t *testing.T) {
	tracks := []library.Track{
		{Hash: "h1", Path: "/m/1.mp3", Artist: "A", Title: "One"},
		{Hash: "h2", Path: "/m/2.mp3", Artist: "B", Title: "Two"},
		{Hash: "h3", Path: "/m/3.mp3", Artist: "C", Title: "Three"},
	}
	var idx library.LibraryIndex
	idx.BulkLoad(tracks)

	m := Model{activeView: viewLibrary, library: libtui.View{Index: idx, Cursor: 1}}
	if got := visibleHashes(t, m); len(got) != 3 {
		t.Fatalf("no selection must export every visible row, got %v", got)
	}

	m.library.Filter = filter.State{Text: "q"}
	m.library.FilteredMatches = []library.Match{{TrackIndex: 2}, {TrackIndex: 0}}
	if got := visibleHashes(t, m); !slices.Equal(got, []string{"h3", "h1"}) {
		t.Fatalf("filter must be honored in display order, got %v", got)
	}
}

func TestVisibleTracksLibraryPrefersSelection(t *testing.T) {
	tracks := []library.Track{
		{Hash: "h1", Path: "/m/1.mp3", Artist: "A"},
		{Hash: "h2", Path: "/m/2.mp3", Artist: "B"},
		{Hash: "h3", Path: "/m/3.mp3", Artist: "C"},
	}
	var idx library.LibraryIndex
	idx.BulkLoad(tracks)

	m := Model{
		activeView: viewLibrary,
		library: libtui.View{
			Index:          idx,
			SelectedHashes: map[string]struct{}{"h3": {}, "h1": {}},
		},
	}
	if got := visibleHashes(t, m); !slices.Equal(got, []string{"h1", "h3"}) {
		t.Fatalf("selection must be returned in display order, got %v", got)
	}
}

func TestVisibleTracksQueueUsesFilteredEntries(t *testing.T) {
	m := Model{
		activeView: viewQueue,
		queue: QueueView{
			entries: []QueueEntry{
				{Hash: "h1", Title: "Alpha"},
				{Hash: "h2", Title: "Beta"},
			},
			filter: filter.State{Text: "bet"},
		},
	}
	if got := visibleHashes(t, m); !slices.Equal(got, []string{"h2"}) {
		t.Fatalf("queue filter must be honored, got %v", got)
	}
}

func TestVisibleTracksQueuePrefersSelection(t *testing.T) {
	m := Model{
		activeView: viewQueue,
		queue: QueueView{entries: []QueueEntry{
			{Hash: "h1"},
			{Hash: "h2", Selected: true},
		}},
	}
	if got := visibleHashes(t, m); !slices.Equal(got, []string{"h2"}) {
		t.Fatalf("queue selection must win, got %v", got)
	}
}

func TestVisibleTracksPlaylistsFollowsSortOrder(t *testing.T) {
	m := Model{
		activeView: viewPlaylists,
		playlists: pltui.PlaylistsView{
			Level: pltui.TrackLevel,
			Tracks: []db.PlaylistTrackRow{
				{ID: 1, Hash: "h1"},
				{ID: 2, Hash: "h2"},
			},
			SortedTrackOrder: []int{1, 0},
		},
	}
	if got := visibleHashes(t, m); !slices.Equal(got, []string{"h2", "h1"}) {
		t.Fatalf("playlist sort order must be honored, got %v", got)
	}
}

func TestVisibleTracksTagsPrefersSelection(t *testing.T) {
	m := Model{
		activeView: viewTags,
		tagsView: TagsView{
			level: tagtui.TrackLevel,
			tracks: []db.PlaylistTrackRow{
				{ID: 1, Hash: "h1"},
				{ID: 2, Hash: "h2"},
			},
			selectedHashes: map[string]struct{}{"h1": {}},
		},
	}
	if got := visibleHashes(t, m); !slices.Equal(got, []string{"h1"}) {
		t.Fatalf("tag selection must win, got %v", got)
	}
}

func TestVisibleTracksRefusesLevelsWithNoTrackList(t *testing.T) {
	cases := []struct {
		name  string
		model Model
	}{
		{"favorites category level", Model{activeView: viewFavorites, favorites: FavoritesView{level: favtui.CategoryLevel}}},
		{"playlists list level", Model{activeView: viewPlaylists, playlists: pltui.PlaylistsView{Level: pltui.ListLevel}}},
		{"tags tag level", Model{activeView: viewTags, tagsView: TagsView{level: tagtui.TagLevel}}},
		{"insights briefing", Model{activeView: viewInsights, insights: InsightsView{level: insightsLevelBriefing}}},
		{"mixes list level", Model{activeView: viewMixes, mixes: MixesView{level: 0}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracks, label, ok := tc.model.visibleTracks()
			if ok {
				t.Fatalf("expected a refusal, got %d tracks", len(tracks))
			}
			if label == "" {
				t.Fatal("a refusal must carry a reason to notify with")
			}
		})
	}
}
