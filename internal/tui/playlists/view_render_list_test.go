package playlists

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
)

func TestRenderListEmpty(t *testing.T) {
	v := View{}
	out := v.Render()
	if !strings.Contains(out, "No playlists yet") {
		t.Fatalf("expected 'No playlists yet' title in output, got:\n%s", out)
	}
	// Every CTA must name a key the Playlists view binds: n is the global
	// next-track key and there is no in-app m3u import.
	for _, want := range []string{"c", "create a playlist", "help"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty playlists card missing CTA %q; got:\n%s", want, out)
		}
	}
	for _, stale := range []string{"import m3u", "new"} {
		if strings.Contains(out, stale) {
			t.Errorf("empty playlists card advertises unbound CTA %q; got:\n%s", stale, out)
		}
	}
}

func TestRenderListShowsNameAndCount(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Chill", TrackCount: 5},
			{ID: 2, Name: "Workout", TrackCount: 12},
		},
		Width: 40,
	}
	out := v.Render()
	if !strings.Contains(out, "Chill") {
		t.Errorf("expected 'Chill' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(5)") {
		t.Errorf("expected '(5)' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Workout") {
		t.Errorf("expected 'Workout' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(12)") {
		t.Errorf("expected '(12)' in output, got:\n%s", out)
	}
}

func TestRenderListShowsSortIndicatorWhenSorted(t *testing.T) {
	v := View{
		Width:     40,
		SortField: "Name",
		SortDir:   "▼",
		Playlists: []db.PlaylistRow{{ID: 1, Name: "Chill", TrackCount: 5}},
	}
	out := v.Render()
	if !strings.Contains(out, "Sort: Name ▼") {
		t.Fatalf("expected playlist list sort indicator in output, got:\n%s", out)
	}
}

func TestRenderListPendingSmartCountFitsWidth(t *testing.T) {
	const width = 20
	v := View{
		Width:     width,
		Playlists: []db.PlaylistRow{{ID: 1, Name: "Long Smart Playlist", TrackCount: -1, IsSmart: true}},
	}

	line := ansi.Strip(v.Render())
	if got := ansi.StringWidth(line); got != width {
		t.Fatalf("pending smart playlist row width = %d, want %d; line=%q", got, width, line)
	}
	if !strings.Contains(line, "(smart)") {
		t.Fatalf("expected full smart placeholder to fit, got %q", line)
	}
}

func TestRenderListLargeCountFitsWidth(t *testing.T) {
	const width = 20
	v := View{
		Width:     width,
		Playlists: []db.PlaylistRow{{ID: 1, Name: "Huge", TrackCount: 123456789}},
	}

	line := ansi.Strip(v.Render())
	if got := ansi.StringWidth(line); got != width {
		t.Fatalf("large count playlist row width = %d, want %d; line=%q", got, width, line)
	}
}

func TestRenderListScrollOffsetBeyondCount(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Width:        40,
		ScrollOffset: 100,
		Playlists:    []db.PlaylistRow{{ID: 1, Name: "Only"}},
	}
	out := v.Render()

	if !strings.Contains(out, "Only") {
		t.Fatalf("expected stale scroll offset to clamp to existing playlist row, got:\n%s", out)
	}
}

func TestRenderListNegativeScrollOffsetStartsAtFirstRow(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Width:        40,
		ScrollOffset: -5,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "First"},
			{ID: 2, Name: "Second"},
		},
	}

	out := v.Render()

	if !strings.Contains(out, "First") {
		t.Fatalf("expected negative scroll offset to render from first row, got:\n%s", out)
	}
}

func TestRenderListNegativeCursorOverridesStaleScrollOffset(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Width:        40,
		Cursor:       -1,
		ScrollOffset: 2,
		Playlists:    fakePlaylistRows("First", "Second", "Third"),
	}

	out := v.Render()

	if !strings.Contains(out, "First") {
		t.Fatalf("expected negative cursor to clamp visible window to first row, got:\n%s", out)
	}
}

func TestRenderListCreateInputUsesClampedStaleCursor(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Width:     40,
		Cursor:    99,
		Playlists: fakePlaylistRows("First", "Second"),
	}.StartCreate()
	v.CreateInput.SetValue("New List")

	out := v.Render()

	if !strings.Contains(ansi.Strip(out), "New List") {
		t.Fatalf("expected create input to render at clamped cursor, got:\n%s", out)
	}
}
