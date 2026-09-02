package tui

import "testing"

func TestSnapshotPrimaryViewFollowsActiveView(t *testing.T) {
	m, err := BuildFixture(FixtureRich)
	if err != nil {
		t.Fatalf("BuildFixture: %v", err)
	}
	m = applyResize(t, m, 100, 30)

	cases := []struct {
		view viewID
		want string
	}{
		{viewLibrary, "Library"},
		{viewQueue, "Queue"},
		{viewBrowse, "Browse"},
		{viewFavorites, "Favorites"},
		{viewTags, "Tags"},
		{viewPlaylists, "Playlists"},
		{viewMixes, "Mixes"},
	}
	for _, tc := range cases {
		got := m.switchToView(tc.view).Snapshot().PrimaryView
		if got != tc.want {
			t.Errorf("switchToView(%d).Snapshot().PrimaryView = %q, want %q", tc.view, got, tc.want)
		}
	}
}

func TestSnapshotQueueSelectionCount(t *testing.T) {
	m, err := BuildFixture(FixtureRich)
	if err != nil {
		t.Fatalf("BuildFixture: %v", err)
	}
	m = applyResize(t, m, 100, 30)
	m = m.switchToView(viewQueue)
	m.queue.Entries[0].Selected = true
	m.queue.Entries[2].Selected = true

	snap := m.Snapshot()
	if snap.SelectionCount != 2 {
		t.Fatalf("queue SelectionCount = %d, want 2", snap.SelectionCount)
	}
	if snap.VisibleCount != len(richQueueEntries()) {
		t.Fatalf("queue VisibleCount = %d, want %d", snap.VisibleCount, len(richQueueEntries()))
	}
	if snap.FocusedHash == "" {
		t.Fatal("queue FocusedHash empty, want the focused entry hash")
	}
}

func TestSnapshotOverlayName(t *testing.T) {
	m, err := BuildFixture(FixtureEmpty)
	if err != nil {
		t.Fatalf("BuildFixture: %v", err)
	}
	m = applyResize(t, m, 100, 30)

	if got := m.Snapshot().Overlay; got != "" {
		t.Fatalf("default Overlay = %q, want empty", got)
	}

	m.helpVisible = true
	if got := m.Snapshot().Overlay; got != "help" {
		t.Fatalf("help Overlay = %q, want %q", got, "help")
	}

	m.helpVisible = false
	m.debugVisible = true
	if got := m.Snapshot().Overlay; got != "debug" {
		t.Fatalf("debug Overlay = %q, want %q", got, "debug")
	}
}

func TestSnapshotLibraryFocusedTrack(t *testing.T) {
	m, err := BuildFixture(FixtureRich)
	if err != nil {
		t.Fatalf("BuildFixture: %v", err)
	}
	m = applyResize(t, m, 100, 30)

	snap := m.Snapshot()
	want := richLibraryTracks()[0]
	if snap.FocusedHash != want.Hash {
		t.Fatalf("library FocusedHash = %q, want %q", snap.FocusedHash, want.Hash)
	}
	if snap.FocusedLabel != trackLabel(want) {
		t.Fatalf("library FocusedLabel = %q, want %q", snap.FocusedLabel, trackLabel(want))
	}
}
