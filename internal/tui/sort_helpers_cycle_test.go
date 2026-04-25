package tui

import "testing"

func TestCycleSortField(t *testing.T) {
	fields := []SortField{SortDefault, SortArtist, SortTitle, SortAlbum}

	tests := []struct {
		name    string
		current SortField
		want    SortField
	}{
		{"default to artist", SortDefault, SortArtist},
		{"artist to title", SortArtist, SortTitle},
		{"title to album", SortTitle, SortAlbum},
		{"album wraps to default", SortAlbum, SortDefault},
		{"not in cycle returns first", SortDuration, SortDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CycleSortField(tt.current, fields)
			if got != tt.want {
				t.Errorf("CycleSortField(%d, fields) = %d, want %d", tt.current, got, tt.want)
			}
		})
	}
}

func TestCycleSortFieldSingleElement(t *testing.T) {
	fields := []SortField{SortDefault}
	got := CycleSortField(SortDefault, fields)
	if got != SortDefault {
		t.Errorf("single-element cycle: got %d, want %d", got, SortDefault)
	}
}

func TestCycleSortFieldEmpty(t *testing.T) {
	got := CycleSortField(SortDefault, nil)
	if got != SortDefault {
		t.Errorf("empty cycle: got %d, want %d", got, SortDefault)
	}
}

func TestSortFieldLabel(t *testing.T) {
	tests := []struct {
		field SortField
		want  string
	}{
		{SortDefault, "default"},
		{SortArtist, "Artist"},
		{SortTitle, "Title"},
		{SortAlbum, "Album"},
		{SortDuration, "Duration"},
		{SortDateAdded, "Date Added"},
		{SortName, "Name"},
		{SortTrackCount, "Track Count"},
		{SortSource, "Source"},
	}
	for _, tt := range tests {
		got := SortFieldLabel(tt.field)
		if got != tt.want {
			t.Errorf("SortFieldLabel(%d) = %q, want %q", tt.field, got, tt.want)
		}
	}
}

func TestSortDirIndicator(t *testing.T) {
	if got := SortDirIndicator(SortAsc); got != "▲" {
		t.Errorf("SortAsc indicator = %q, want ▲", got)
	}
	if got := SortDirIndicator(SortDesc); got != "▼" {
		t.Errorf("SortDesc indicator = %q, want ▼", got)
	}
}

func TestSortCycleQueueAllFields(t *testing.T) {
	state := SortState{Field: SortDefault}

	expected := []SortField{SortTitle, SortArtist, SortAlbum, SortDuration, SortSource, SortDefault}
	for _, want := range expected {
		state.Field = CycleSortField(state.Field, queueSortFields)
		if state.Field != want {
			t.Errorf("expected %d, got %d", want, state.Field)
		}
	}
}

func TestSortCyclePlaylistListFields(t *testing.T) {
	state := SortState{Field: SortDefault}
	expected := []SortField{SortName, SortTrackCount, SortDefault}
	for _, want := range expected {
		state.Field = CycleSortField(state.Field, playlistListSortFields)
		if state.Field != want {
			t.Errorf("expected %d, got %d", want, state.Field)
		}
	}
}

func TestSortCyclePlaylistTrackFields(t *testing.T) {
	state := SortState{Field: SortDefault}
	expected := []SortField{SortTitle, SortArtist, SortAlbum, SortDuration, SortDefault}
	for _, want := range expected {
		state.Field = CycleSortField(state.Field, playlistTrackSortFields)
		if state.Field != want {
			t.Errorf("expected %d, got %d", want, state.Field)
		}
	}
}

func TestSortNotification(t *testing.T) {
	tests := []struct {
		state SortState
		want  string
	}{
		{SortState{Field: SortDefault}, "Sort: default"},
		{SortState{Field: SortArtist, Dir: SortAsc}, "Sort: Artist ▲"},
		{SortState{Field: SortTitle, Dir: SortDesc}, "Sort: Title ▼"},
		{SortState{Field: SortDuration, Dir: SortAsc}, "Sort: Duration ▲"},
		{SortState{Field: SortName, Dir: SortDesc}, "Sort: Name ▼"},
		{SortState{Field: SortTrackCount, Dir: SortAsc}, "Sort: Track Count ▲"},
		{SortState{Field: SortSource, Dir: SortDesc}, "Sort: Source ▼"},
	}
	for _, tt := range tests {
		got := sortNotification(tt.state)
		if got != tt.want {
			t.Errorf("sortNotification(%+v) = %q, want %q", tt.state, got, tt.want)
		}
	}
}