package playlists

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestPlaylistSummaryCountsEntriesAndHiddenSelections(t *testing.T) {
	v := PlaylistsView{Level: TrackLevel, Width: 100, Height: 20,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Hash: "same", Title: "First", DurationMs: 60000},
			{ID: 2, Hash: "same", Title: "Again", DurationMs: 60000},
			{ID: 3, Hash: "unknown", Title: "Unknown"},
		}, SelectedTrackIDs: map[int64]struct{}{1: {}, 2: {}, 3: {}},
		Filter: filter.State{Text: "First"},
	}
	out := ansi.Strip(v.Render(styles.NewStyles(true), nil, nil, nil, "", ""))
	for _, want := range []string{"1/3 tracks", "3 selected", "2 hidden", "2:00+"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	v.Filter.Text = "no match"
	out = ansi.Strip(v.Render(styles.NewStyles(true), nil, nil, nil, "", ""))
	for _, want := range []string{"0/3 tracks", "3 hidden", "2:00+"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in empty filtered view: %s", want, out)
		}
	}
	for _, width := range []int{30, 60, 100} {
		v.Width = width
		for _, line := range strings.Split(v.Render(styles.NewStyles(true), nil, nil, nil, "", ""), "\n") {
			if ansi.StringWidth(line) > width {
				t.Errorf("width %d overflow: %s", width, line)
			}
		}
	}
}

func TestPlaylistSummaryEmptyAndSingle(t *testing.T) {
	v := View{Width: 60}
	if got := ansi.Strip(v.renderTrackSummary()); !strings.Contains(got, "0 tracks") {
		t.Fatal(got)
	}
	v.Tracks = []db.PlaylistTrackRow{{DurationMs: 123000}}
	if got := ansi.Strip(v.renderTrackSummary()); !strings.Contains(got, "1 track") || !strings.Contains(got, "2:03") {
		t.Fatal(got)
	}
}
