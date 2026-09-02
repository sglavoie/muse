package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestSortNotification(t *testing.T) {
	tests := []struct {
		state sorting.SortState
		want  string
	}{
		{sorting.SortState{Field: sorting.SortDefault}, "Sort: default"},
		{sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}, "Sort: Artist ▲"},
		{sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}, "Sort: Title ▼"},
		{sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortAsc}, "Sort: Duration ▲"},
		{sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}, "Sort: Name ▼"},
		{sorting.SortState{Field: sorting.SortTrackCount, Dir: sorting.SortAsc}, "Sort: Track Count ▲"},
		{sorting.SortState{Field: sorting.SortSource, Dir: sorting.SortDesc}, "Sort: Source ▼"},
	}
	for _, tt := range tests {
		got := sortNotification(tt.state)
		if got != tt.want {
			t.Errorf("sortNotification(%+v) = %q, want %q", tt.state, got, tt.want)
		}
	}
}
