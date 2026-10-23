package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// applyResize sends an initial window size to a fixture-backed model and returns
// the resized model, mirroring what the harness driver does before rendering.
func applyResize(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func TestFixtureNames(t *testing.T) {
	names := FixtureNames()
	want := []string{FixtureEmpty, FixtureRich, FixtureNarrow, FixturePlayback, FixtureHealth, FixtureDense}
	if len(names) != len(want) {
		t.Fatalf("FixtureNames() = %v, want %v", names, want)
	}
	for i, n := range want {
		if names[i] != n {
			t.Fatalf("FixtureNames()[%d] = %q, want %q", i, names[i], n)
		}
	}
}

func TestBuildFixtureUnknown(t *testing.T) {
	if _, err := BuildFixture("does-not-exist"); err == nil {
		t.Fatal("BuildFixture(\"does-not-exist\") = nil error, want error")
	}
}

// TestBuildFixtureRendersAndSnapshots verifies every fixture constructs, accepts
// an initial resize, renders non-empty output, and produces a snapshot — all
// without real filesystem or player side effects.
func TestBuildFixtureRendersAndSnapshots(t *testing.T) {
	for _, name := range FixtureNames() {
		t.Run(name, func(t *testing.T) {
			m, err := BuildFixture(name)
			if err != nil {
				t.Fatalf("BuildFixture(%q) error: %v", name, err)
			}
			m = applyResize(t, m, 100, 30)

			content := m.View().Content
			if strings.TrimSpace(content) == "" {
				t.Fatalf("fixture %q rendered blank output", name)
			}

			snap := m.Snapshot()
			if snap == nil {
				t.Fatalf("fixture %q produced nil snapshot", name)
			}
			if snap.PrimaryView != "Library" {
				t.Fatalf("fixture %q PrimaryView = %q, want %q", name, snap.PrimaryView, "Library")
			}
			if snap.Overlay != "" {
				t.Fatalf("fixture %q Overlay = %q, want empty", name, snap.Overlay)
			}
		})
	}
}

func TestRichFixtureLibraryState(t *testing.T) {
	m, err := BuildFixture(FixtureRich)
	if err != nil {
		t.Fatalf("BuildFixture: %v", err)
	}
	m = applyResize(t, m, 100, 30)

	snap := m.Snapshot()
	if got := snap.VisibleCount; got != len(richLibraryTracks()) {
		t.Fatalf("rich VisibleCount = %d, want %d", got, len(richLibraryTracks()))
	}
	if snap.QueueLength != len(richQueueEntries()) {
		t.Fatalf("rich QueueLength = %d, want %d", snap.QueueLength, len(richQueueEntries()))
	}
	if snap.FocusedHash == "" {
		t.Fatal("rich snapshot FocusedHash is empty, want the first track hash")
	}
}

func TestPlaybackFixtureState(t *testing.T) {
	m, err := BuildFixture(FixturePlayback)
	if err != nil {
		t.Fatalf("BuildFixture: %v", err)
	}
	m = applyResize(t, m, 100, 30)

	if got := m.PlaybackStateValue(); got != playback.StatePlaying {
		t.Fatalf("playback state = %v, want PlaybackPlaying", got)
	}
	if got := m.Volume(); got != 65 {
		t.Fatalf("volume = %d, want 65", got)
	}
	snap := m.Snapshot()
	if snap.QueueLength != len(playbackQueueEntries()) {
		t.Fatalf("playback QueueLength = %d, want %d", snap.QueueLength, len(playbackQueueEntries()))
	}
	if snap.CurrentIndex != 1 {
		t.Fatalf("playback CurrentIndex = %d, want 1", snap.CurrentIndex)
	}
}

// TestEmptyFixtureHasNoData confirms the empty fixture is genuinely empty.
func TestEmptyFixtureHasNoData(t *testing.T) {
	m, err := BuildFixture(FixtureEmpty)
	if err != nil {
		t.Fatalf("BuildFixture: %v", err)
	}
	m = applyResize(t, m, 100, 30)

	snap := m.Snapshot()
	if snap.VisibleCount != 0 {
		t.Fatalf("empty VisibleCount = %d, want 0", snap.VisibleCount)
	}
	if snap.QueueLength != 0 {
		t.Fatalf("empty QueueLength = %d, want 0", snap.QueueLength)
	}
}
