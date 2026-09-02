package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/queue"
)

func makeNTracks(n int) []library.Track {
	tracks := make([]library.Track, n)
	for i := 0; i < n; i++ {
		tracks[i] = library.Track{
			Hash:   fmt.Sprintf("h%d", i),
			Title:  fmt.Sprintf("Track %d", i),
			Artist: "Artist",
		}
	}
	return tracks
}

func newLibraryModel(n int) Model {
	m := New(context.Background())
	m.ActiveView = viewLibrary
	m.library.SourceTracks = makeNTracks(n)
	return m
}

func doVisual(m Model) Model {
	m, _, _ = m.handleNavigationAction(keys.ActionVisualMode)
	return m
}

func doNav(m Model, a keys.Action) Model {
	m, _, _ = m.handleNavigationAction(a)
	return m
}

func assertSelectedHashesExact(t *testing.T, got map[string]struct{}, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("selected hash count = %d, want %d", len(got), len(want))
	}
	for _, hash := range want {
		if _, ok := got[hash]; !ok {
			t.Fatalf("expected %q to be selected; got %#v", hash, got)
		}
	}
}

func assertSelectedQueueEntriesExact(t *testing.T, entries []queue.Entry, want ...int) {
	t.Helper()
	selected := make(map[int]struct{}, len(want))
	for _, idx := range want {
		selected[idx] = struct{}{}
	}
	for i, entry := range entries {
		_, shouldBeSelected := selected[i]
		if entry.Selected != shouldBeSelected {
			t.Fatalf("queue entry %d selected = %v, want %v", i, entry.Selected, shouldBeSelected)
		}
	}
}
