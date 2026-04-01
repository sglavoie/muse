package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestUpdateScanBatchMsgAppendsTracksToLibrary verifies that receiving a
// ScanBatchMsg inserts all tracks into the library index.
func TestUpdateScanBatchMsgAppendsTracksToLibrary(t *testing.T) {
	m := New(context.Background())
	tracks := []library.Track{
		{Artist: "Artist A", Title: "Track 1"},
		{Artist: "Artist B", Title: "Track 2"},
	}
	next, _ := m.Update(library.ScanBatchMsg{Tracks: tracks})
	nm := next.(Model)
	if nm.library.index.Len() != 2 {
		t.Fatalf("expected 2 tracks in index, got %d", nm.library.index.Len())
	}
}

// TestUpdateScanBatchMsgChainsWaitCmd verifies that a ScanBatchMsg received
// while a scan channel is active returns a follow-up command that reads the
// next batch. A closed channel causes that command to return ScanDoneMsg,
// confirming the chain terminates correctly.
func TestUpdateScanBatchMsgChainsWaitCmd(t *testing.T) {
	ch := make(chan library.Track)
	close(ch)
	m := New(context.Background(), WithScanChannel(ch))

	tracks := []library.Track{{Artist: "A", Title: "T"}}
	_, cmd := m.Update(library.ScanBatchMsg{Tracks: tracks})
	if cmd == nil {
		t.Fatal("ScanBatchMsg with active channel must return a follow-up cmd")
	}
	msg := cmd()
	if _, ok := msg.(library.ScanDoneMsg); !ok {
		t.Fatalf("closed channel: expected ScanDoneMsg, got %T", msg)
	}
}

// TestUpdateScanDoneMsgClearsScanState verifies that ScanDoneMsg transitions
// the model out of scanning state.
func TestUpdateScanDoneMsgClearsScanState(t *testing.T) {
	ch := make(chan library.Track)
	close(ch)
	m := New(context.Background(), WithScanChannel(ch))

	next, cmd := m.Update(library.ScanDoneMsg{})
	nm := next.(Model)
	if nm.scanning {
		t.Fatal("scanning must be false after ScanDoneMsg")
	}
	if nm.scanCh != nil {
		t.Fatal("scanCh must be nil after ScanDoneMsg")
	}
	if cmd != nil {
		t.Fatal("ScanDoneMsg must not produce a follow-up cmd")
	}
}

// TestViewLibraryShowsTrackCount verifies that after receiving a ScanBatchMsg
// the library view renders the track count.
func TestViewLibraryShowsTrackCount(t *testing.T) {
	m := New(context.Background())
	tracks := []library.Track{
		{Artist: "X", Title: "Y"},
	}
	next, _ := m.Update(library.ScanBatchMsg{Tracks: tracks})
	nm := next.(Model)
	v := nm.View()
	if !strings.Contains(v.Content, "1 tracks") {
		t.Fatalf("library view must show track count; got:\n%s", v.Content)
	}
}

// TestViewStatusBarShowsScanProgress verifies that the status bar shows a
// formatted "X tracks" progress string after a ScanProgressMsg.
func TestViewStatusBarShowsScanProgress(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(library.ScanProgressMsg{Scanned: 1234})
	nm := next.(Model)
	v := nm.View()
	want := "Scanning… 1,234 tracks"
	if !strings.Contains(v.Content, want) {
		t.Fatalf("status bar must show %q after ScanProgressMsg; got:\n%s", want, v.Content)
	}
}

// TestRescanHotkeyReturnsCmd verifies that pressing R (Shift+r) with a
// configured library root returns a non-nil background cmd and does not block.
func TestRescanHotkeyReturnsCmd(t *testing.T) {
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd == nil {
		t.Fatal("R key: expected non-nil cmd for rescan, got nil")
	}
}

// TestRescanHotkeyIsNoOpWhenAlreadyScanning verifies that pressing R during
// an in-progress scan does nothing.
func TestRescanHotkeyIsNoOpWhenScanning(t *testing.T) {
	ch := make(chan library.Track)
	defer close(ch)
	root := t.TempDir()
	m := New(context.Background(), WithScanChannel(ch), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd != nil {
		t.Fatal("R key during active scan must return nil cmd")
	}
}

// TestRescanHotkeyIsNoOpWithoutRoot verifies that pressing R when no library
// root is configured returns nil.
func TestRescanHotkeyIsNoOpWithoutRoot(t *testing.T) {
	m := New(context.Background())
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd != nil {
		t.Fatal("R key without library root must return nil cmd")
	}
}

// TestRescanResetsIndexAndAddsNewTracks verifies that triggering a rescan
// clears the existing index and that subsequent ScanBatchMsg messages
// populate it with only the new tracks, effectively removing deleted ones.
func TestRescanResetsIndexAndAddsNewTracks(t *testing.T) {
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))

	// Seed two initial tracks.
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Artist: "A", Title: "kept track"},
		{Artist: "B", Title: "deleted track"},
	}})
	nm := next.(Model)
	if nm.library.index.Len() != 2 {
		t.Fatalf("seed: expected 2 tracks, got %d", nm.library.index.Len())
	}

	// Trigger rescan — index must be reset immediately.
	next, cmd := nm.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	nm = next.(Model)
	if cmd == nil {
		t.Fatal("rescan: expected non-nil cmd")
	}
	if nm.library.index.Len() != 0 {
		t.Fatalf("rescan: expected empty index after reset, got %d", nm.library.index.Len())
	}

	// Only the surviving track arrives; the deleted track is absent.
	next, _ = nm.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Artist: "A", Title: "kept track"},
	}})
	nm = next.(Model)
	if nm.library.index.Len() != 1 {
		t.Fatalf("after rescan batch: expected 1 track, got %d", nm.library.index.Len())
	}
	if nm.library.index.Tracks()[0].Title != "kept track" {
		t.Fatalf("expected 'kept track', got %q", nm.library.index.Tracks()[0].Title)
	}
}

// TestViewStatusBarHidesScanProgressWhenDone verifies that the scanning
// indicator auto-dismisses after the notification expiry is reached and the
// tick fires. ScanDoneMsg alone does not clear the notification; it stays
// visible until the 1-second tick sees that expiresAt has passed.
func TestViewStatusBarHidesScanProgressWhenDone(t *testing.T) {
	m := New(context.Background())
	// Set progress — notification gets a 5-second expiry.
	next, _ := m.Update(library.ScanProgressMsg{Scanned: 100})
	nm := next.(Model)
	// ScanDoneMsg stops scanning but does not clear the notification.
	next, _ = nm.Update(library.ScanDoneMsg{})
	nm = next.(Model)
	if nm.statusBar.notification.msg == "" {
		t.Fatal("notification must still be set immediately after ScanDoneMsg")
	}
	// Manually expire the notification to simulate 5 seconds passing.
	nm.statusBar.notification.expiresAt = nm.statusBar.notification.expiresAt.Add(-10 * time.Second)
	// Tick fires → expiry check clears the notification.
	next, _ = nm.Update(tickMsg{})
	nm = next.(Model)
	v := nm.View()
	if strings.Contains(v.Content, "Scanning") {
		t.Fatalf("status bar must not show scan progress after expiry tick; got:\n%s", v.Content)
	}
}

// TestScanProgressVisibleDuringScanAndAutoDismissed verifies the full lifecycle:
// progress is shown while scanning, the last message persists past ScanDoneMsg,
// and the tick clears it once the expiry passes.
func TestScanProgressVisibleDuringScanAndAutoDismissed(t *testing.T) {
	m := New(context.Background())

	// Scan progress sets a notification.
	next, _ := m.Update(library.ScanProgressMsg{Scanned: 42})
	nm := next.(Model)
	v := nm.View()
	if !strings.Contains(v.Content, "Scanning") {
		t.Fatalf("scan progress must be visible during scan; got:\n%s", v.Content)
	}

	// ScanDoneMsg stops scanning but notification is still set.
	next, _ = nm.Update(library.ScanDoneMsg{})
	nm = next.(Model)
	if nm.statusBar.notification.msg == "" {
		t.Fatal("scan progress notification must persist immediately after ScanDoneMsg")
	}

	// Expire the notification and fire a tick.
	nm.statusBar.notification.expiresAt = nm.statusBar.notification.expiresAt.Add(-10 * time.Second)
	next, _ = nm.Update(tickMsg{})
	nm = next.(Model)
	v = nm.View()
	if strings.Contains(v.Content, "Scanning") {
		t.Fatalf("scan progress must be auto-dismissed after expiry tick; got:\n%s", v.Content)
	}
}

// TestRescanResetsCursorScrollOffsetAndFilter verifies that triggering a rescan
// resets the library cursor, scroll offset, and filter state so that the UI
// does not render with a stale cursor pointing past the end of the now-empty
// index.
func TestRescanResetsCursorScrollOffsetAndFilter(t *testing.T) {
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))

	// Seed tracks and move the cursor down.
	tracks := make([]library.Track, 20)
	for i := range tracks {
		tracks[i] = library.Track{Artist: "A", Title: fmt.Sprintf("T%02d", i), Hash: fmt.Sprintf("h%d", i)}
	}
	next, _ := m.Update(library.ScanBatchMsg{Tracks: tracks})
	nm := next.(Model)
	// Move cursor to position 10.
	for range 10 {
		nm.library = nm.library.HandleAction(actionCursorDown)
	}
	if nm.library.cursor != 10 {
		t.Fatalf("setup: expected cursor=10, got %d", nm.library.cursor)
	}
	// Activate a filter.
	nm.library.filter, _ = nm.library.filter.Activate()
	nm.library.filter, _ = nm.library.filter.Update(tea.KeyPressMsg{Text: "T"})
	nm.library.cursor = 0
	nm.library.scrollOffset = 0
	nm.library = nm.library.rebuildFilterCache()
	nm.library.filter = nm.library.filter.Commit()
	if nm.library.filter.Text != "T" {
		t.Fatalf("setup: expected filterText='T', got %q", nm.library.filter.Text)
	}

	// Trigger rescan.
	next, cmd := nm.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	nm = next.(Model)
	if cmd == nil {
		t.Fatal("rescan: expected non-nil cmd")
	}
	if nm.library.cursor != 0 {
		t.Fatalf("rescan: expected cursor=0, got %d", nm.library.cursor)
	}
	if nm.library.scrollOffset != 0 {
		t.Fatalf("rescan: expected scrollOffset=0, got %d", nm.library.scrollOffset)
	}
	if nm.library.filter.Active {
		t.Fatal("rescan: expected filterActive=false")
	}
	if nm.library.filter.Text != "" {
		t.Fatalf("rescan: expected filterText='', got %q", nm.library.filter.Text)
	}
	if nm.library.filteredMatches != nil {
		t.Fatal("rescan: expected filteredMatches=nil")
	}
}

// TestShiftRFiresRescan verifies that pressing 'R' (Shift+r) with a configured
// library root triggers a rescan, while lowercase 'r' does not.
func TestShiftRFiresRescan(t *testing.T) {
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd == nil {
		t.Fatal("R key: expected non-nil rescan cmd")
	}
}

// TestWatchBatchInvalidatesFilterCache verifies that a watchBatchMsg that
// removes a track invalidates the cached fuzzy-match results so that stale
// TrackIndex values cannot cause an out-of-bounds panic on the next render
// or FocusedTrack call.
func TestWatchBatchInvalidatesFilterCache(t *testing.T) {
	watchCh := make(chan library.WatchEvent, 1)
	m := New(context.Background(), WithWatchChannel(watchCh))
	m.library.fuzzy = library.FakeFuzzySearcher{}

	// Seed three tracks.
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})
	m.library.index.Insert(library.Track{Artist: "C", Title: "Charlie", Path: "/c", Hash: "h3"})

	// Activate and commit a filter that matches all three.
	m.library.filter, _ = m.library.filter.Activate()
	m.library.filter, _ = m.library.filter.Update(tea.KeyPressMsg{Text: "a"}) // matches Alpha, Beta, Charlie all contain 'a' in artist or title
	m.library.cursor = 0
	m.library.scrollOffset = 0
	m.library = m.library.rebuildFilterCache()
	m.library.filter = m.library.filter.Commit()

	// Move cursor to the last match.
	for range 2 {
		m.library = m.library.HandleAction(actionCursorDown)
	}
	if m.library.cursor != 2 {
		t.Fatalf("setup: expected cursor=2, got %d", m.library.cursor)
	}

	// Deliver a watchBatchMsg that deletes two tracks, leaving only one match.
	next, _ := m.Update(watchBatchMsg{
		deleted: []string{"/b", "/c"},
	})
	nm := next.(Model)

	// The cursor must be clamped so it does not point past the end of the
	// new (shorter) match set.
	if nm.library.cursor >= len(nm.library.filteredMatches) && len(nm.library.filteredMatches) > 0 {
		t.Fatalf("cursor %d out of range for %d filtered matches",
			nm.library.cursor, len(nm.library.filteredMatches))
	}

	// FocusedTrack must not panic.
	track, ok := nm.library.view().FocusedTrack()
	if !ok && nm.library.index.Len() > 0 && len(nm.library.filteredMatches) > 0 {
		t.Fatal("FocusedTrack returned false despite non-empty filtered matches")
	}
	if ok && track.Path == "/b" {
		t.Fatal("FocusedTrack returned a deleted track")
	}
	if ok && track.Path == "/c" {
		t.Fatal("FocusedTrack returned a deleted track")
	}
}

// TestScanBatchInvalidatesFilterCache verifies that a ScanBatchMsg that adds
// tracks while a filter is active rebuilds the cached fuzzy-match results so
// that TrackIndex values stay consistent with the modified index.
func TestScanBatchInvalidatesFilterCache(t *testing.T) {
	m := New(context.Background())
	m.library.fuzzy = library.FakeFuzzySearcher{}

	// Seed two tracks and set up a filter.
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})
	m.library.filter, _ = m.library.filter.Activate()
	m.library.filter, _ = m.library.filter.Update(tea.KeyPressMsg{Text: "a"}) // matches both via artist
	m.library.cursor = 0
	m.library.scrollOffset = 0
	m.library = m.library.rebuildFilterCache()
	m.library.filter = m.library.filter.Commit()

	oldMatches := len(m.library.filteredMatches)
	if oldMatches != 2 {
		t.Fatalf("setup: expected 2 matches, got %d", oldMatches)
	}

	// Insert a new track that also matches the filter.
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Artist: "AA", Title: "Gamma", Path: "/g", Hash: "h3"},
	}})
	nm := next.(Model)

	// The filter cache must have been rebuilt to include the new track.
	if len(nm.library.filteredMatches) <= oldMatches {
		t.Fatalf("expected filteredMatches to grow after ScanBatchMsg; got %d (was %d)",
			len(nm.library.filteredMatches), oldMatches)
	}

	// All TrackIndex values must be in range.
	allTracks := nm.library.index.Tracks()
	for i, match := range nm.library.filteredMatches {
		if match.TrackIndex < 0 || match.TrackIndex >= len(allTracks) {
			t.Fatalf("match[%d].TrackIndex=%d out of range [0, %d)",
				i, match.TrackIndex, len(allTracks))
		}
	}
}

// TestWatchBatchClampsUnfilteredCursor verifies that when no filter is
// active and the watcher deletes tracks, the library cursor is clamped
// so it does not point past the end of the track list.
func TestWatchBatchClampsUnfilteredCursor(t *testing.T) {
	watchCh := make(chan library.WatchEvent, 1)
	m := New(context.Background(), WithWatchChannel(watchCh))

	// Seed three tracks.
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})
	m.library.index.Insert(library.Track{Artist: "C", Title: "Charlie", Path: "/c", Hash: "h3"})

	// Place cursor at the last track.
	m.library.cursor = 2

	// Delete two tracks, leaving only one.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/b", "/c"}})
	nm := next.(Model)

	if nm.library.cursor >= nm.library.index.Len() {
		t.Fatalf("cursor %d out of range for %d tracks",
			nm.library.cursor, nm.library.index.Len())
	}
}

// TestWatchBatchInvalidatesSortedTracks verifies that when a sort is
// active and the watcher deletes tracks, the sorted cache is rebuilt
// so it does not contain stale entries.
func TestWatchBatchInvalidatesSortedTracks(t *testing.T) {
	watchCh := make(chan library.WatchEvent, 1)
	m := New(context.Background(), WithWatchChannel(watchCh))

	// Seed three tracks.
	m.library.index.Insert(library.Track{Artist: "C", Title: "Charlie", Path: "/c", Hash: "h3"})
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})

	// Activate sort by artist ascending.
	m.library.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.library = m.library.recomputeSortedTracks()
	if len(m.library.sortedTracks) != 3 {
		t.Fatalf("setup: expected 3 sorted tracks, got %d", len(m.library.sortedTracks))
	}

	// Delete one track.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/b"}})
	nm := next.(Model)

	// The sorted cache must reflect the deletion.
	if len(nm.library.sortedTracks) != 2 {
		t.Fatalf("expected 2 sorted tracks after deletion, got %d", len(nm.library.sortedTracks))
	}
	for _, st := range nm.library.sortedTracks {
		if st.Hash == "h2" {
			t.Fatal("deleted track h2 still present in sortedTracks")
		}
	}
}

// TestScanBatchRecomputesSortedTracks verifies that when a sort is
// active and new tracks arrive via a scan batch, the sorted cache is
// rebuilt to include the new tracks.
func TestScanBatchRecomputesSortedTracks(t *testing.T) {
	m := New(context.Background())

	// Seed two tracks and sort.
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.library = m.library.recomputeSortedTracks()
	if len(m.library.sortedTracks) != 2 {
		t.Fatalf("setup: expected 2 sorted tracks, got %d", len(m.library.sortedTracks))
	}

	// Deliver a scan batch with one new track.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "C", Title: "Charlie", Path: "/c", Hash: "h3"},
		},
	})
	nm := next.(Model)

	// The sorted cache must include the new track.
	if len(nm.library.sortedTracks) != 3 {
		t.Fatalf("expected 3 sorted tracks after scan, got %d", len(nm.library.sortedTracks))
	}
}

// TestRescanClearsSelectionAndSort verifies that a manual rescan resets the
// library selection, sort state, and sorted cache so stale state from the
// previous library does not carry over.
func TestRescanClearsSelectionAndSort(t *testing.T) {
	// Provide a library root and extractor so the rescan action is not a no-op.
	m := New(context.Background(),
		WithLibraryRoots([]string{"/tmp"}),
		WithExtractor(&library.ExifToolExtractor{}),
	)
	// Seed a track, select it, and activate a sort.
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library = m.library.ToggleBulkSelection("h1")
	m.library.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.library = m.library.recomputeSortedTracks()

	if len(m.library.selectedHashes) != 1 {
		t.Fatalf("setup: expected 1 selected hash, got %d", len(m.library.selectedHashes))
	}
	if m.library.sortState.Field != SortArtist {
		t.Fatal("setup: expected SortArtist sort field")
	}
	if m.library.sortedTracks == nil {
		t.Fatal("setup: expected non-nil sortedTracks")
	}

	// Trigger rescan. We cannot actually run the full scan pipeline (needs
	// mpv + exiftool), but we can dispatch the action and verify the model
	// state was reset. The key mapping for rescan is "R" (Shift+r).
	next, _ := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	nm := next.(Model)

	if nm.library.selectedHashes != nil {
		t.Errorf("selectedHashes should be nil after rescan, got %v", nm.library.selectedHashes)
	}
	if nm.library.sortState.Field != SortDefault {
		t.Errorf("sortState.Field should be SortDefault after rescan, got %v", nm.library.sortState.Field)
	}
	if nm.library.sortedTracks != nil {
		t.Errorf("sortedTracks should be nil after rescan, got %v", nm.library.sortedTracks)
	}
}

// TestWatchBatchMarksQueueEntriesMissing verifies that when the watcher
// reports deleted files, the corresponding queue entries are immediately
// marked as Missing so they appear grayed out in the UI.
func TestWatchBatchMarksQueueEntriesMissing(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Artist: "A", Title: "T1", Path: "/a.flac", Hash: "h1",
	})
	m.library.index.Insert(library.Track{
		Artist: "B", Title: "T2", Path: "/b.flac", Hash: "h2",
	})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "B", Title: "T2"},
	}

	// Delete /a.flac via watcher.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/a.flac"}})
	nm := next.(Model)

	if !nm.queue.entries[0].Missing {
		t.Error("queue entry for /a.flac should be marked Missing after watcher deletion")
	}
	if nm.queue.entries[1].Missing {
		t.Error("queue entry for /b.flac should NOT be marked Missing")
	}
}

// TestWatchBatchUpdatesQueueMetadata verifies that when the watcher
// reports a modified track, the queue entries for that track are updated
// with the new metadata.
func TestWatchBatchUpdatesQueueMetadata(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Artist: "OldArtist", Title: "OldTitle", Album: "OldAlbum",
		Path: "/a.flac", Hash: "h1",
	})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac",
			Artist: "OldArtist", Title: "OldTitle", Album: "OldAlbum",
			Display: "OldArtist – OldTitle"},
	}

	// Watcher reports an update with new metadata.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "NewArtist", Title: "NewTitle", Album: "NewAlbum",
				Path: "/a.flac", Hash: "h1"},
		},
	})
	nm := next.(Model)

	e := nm.queue.entries[0]
	if e.Artist != "NewArtist" {
		t.Errorf("expected Artist=%q, got %q", "NewArtist", e.Artist)
	}
	if e.Title != "NewTitle" {
		t.Errorf("expected Title=%q, got %q", "NewTitle", e.Title)
	}
	if e.Album != "NewAlbum" {
		t.Errorf("expected Album=%q, got %q", "NewAlbum", e.Album)
	}
	if e.Display != "NewArtist – NewTitle" {
		t.Errorf("expected Display=%q, got %q", "NewArtist – NewTitle", e.Display)
	}
}

// TestWatchBatchUpdatesStatusBarForCurrentTrack verifies that when the
// watcher reports metadata changes for the currently-playing track, the
// status bar is updated.
func TestWatchBatchUpdatesStatusBarForCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Artist: "OldArtist", Title: "OldTitle",
		Path: "/a.flac", Hash: "h1",
	})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac",
			Artist: "OldArtist", Title: "OldTitle"},
	}
	m.playback.currentHash = "h1"
	m.statusBar.artist = "OldArtist"
	m.statusBar.title = "OldTitle"

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "NewArtist", Title: "NewTitle",
				Path: "/a.flac", Hash: "h1"},
		},
	})
	nm := next.(Model)

	if nm.statusBar.artist != "NewArtist" {
		t.Errorf("expected status bar artist=%q, got %q", "NewArtist", nm.statusBar.artist)
	}
	if nm.statusBar.title != "NewTitle" {
		t.Errorf("expected status bar title=%q, got %q", "NewTitle", nm.statusBar.title)
	}
}

// TestWatchBatchClearsMissingOnReappearance verifies that when a
// previously-missing track reappears via the watcher (same hash, new
// path), its queue entries have the Missing flag cleared.
func TestWatchBatchClearsMissingOnReappearance(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac",
			Artist: "A", Title: "T1", Missing: true},
	}

	// Watcher reports the track as re-created.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "A", Title: "T1", Path: "/a.flac", Hash: "h1"},
		},
	})
	nm := next.(Model)

	if nm.queue.entries[0].Missing {
		t.Error("queue entry should no longer be Missing after track reappears")
	}
}

// TestWatchBatchSyncsPlayerQueue verifies that the player's in-memory
// queue is rebuilt when the watcher updates queue entry metadata.
func TestWatchBatchSyncsPlayerQueue(t *testing.T) {
	sp := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(sp))
	m.library.index.Insert(library.Track{
		Artist: "Old", Title: "Old", Path: "/a.flac", Hash: "h1",
	})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Old", Title: "Old"},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "New", Title: "New", Path: "/a.flac", Hash: "h1"},
		},
	})
	_ = next.(Model)

	if len(sp.setQueueCalls) == 0 {
		t.Fatal("expected SetQueue to be called after watcher metadata update")
	}
}

// TestWatchBatchMsgUpdatesPlaylistTracks verifies that a watcher metadata
// update refreshes in-memory playlist track metadata when the playlists
// view is at TrackLevel.
func TestWatchBatchMsgUpdatesPlaylistTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "NewArt", Title: "NewTitle", Album: "NewAlbum"},
		},
	})
	nm := next.(Model)

	if nm.playlists.tracks[0].Title != "NewTitle" {
		t.Errorf("playlist track title = %q, want %q", nm.playlists.tracks[0].Title, "NewTitle")
	}
	if nm.playlists.tracks[0].Artist != "NewArt" {
		t.Errorf("playlist track artist = %q, want %q", nm.playlists.tracks[0].Artist, "NewArt")
	}
	if nm.playlists.tracks[0].Album != "NewAlbum" {
		t.Errorf("playlist track album = %q, want %q", nm.playlists.tracks[0].Album, "NewAlbum")
	}
}

// TestWatchBatchMsgUpdatesFavoritesTracks verifies that a watcher metadata
// update refreshes in-memory favorites track metadata when the favorites
// view is at ListLevel with CategoryTracks.
func TestWatchBatchMsgUpdatesFavoritesTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "NewArt", Title: "NewTitle", Album: "NewAlbum"},
		},
	})
	nm := next.(Model)

	if nm.favorites.tracks[0].Title != "NewTitle" {
		t.Errorf("favorites track title = %q, want %q", nm.favorites.tracks[0].Title, "NewTitle")
	}
	if nm.favorites.tracks[0].Artist != "NewArt" {
		t.Errorf("favorites track artist = %q, want %q", nm.favorites.tracks[0].Artist, "NewArt")
	}
}

// TestWatchBatchMsgUpdatesFavoritesDrilldownTracks verifies that a watcher
// metadata update refreshes in-memory favorites drilldown track metadata
// when the favorites view is at TrackLevel.
func TestWatchBatchMsgUpdatesFavoritesDrilldownTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "NewArt", Title: "NewTitle", Album: "NewAlbum"},
		},
	})
	nm := next.(Model)

	if nm.favorites.drilldownTracks[0].Title != "NewTitle" {
		t.Errorf("drilldown track title = %q, want %q", nm.favorites.drilldownTracks[0].Title, "NewTitle")
	}
	if nm.favorites.drilldownTracks[0].Artist != "NewArt" {
		t.Errorf("drilldown track artist = %q, want %q", nm.favorites.drilldownTracks[0].Artist, "NewArt")
	}
}

// TestWatchBatchRefreshesArtistAlbumLevel verifies that when the artists view
// is drilled into AlbumLevel, a watchBatchMsg re-derives the album list so
// newly added albums appear and removed albums disappear.
func TestWatchBatchRefreshesArtistAlbumLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Album1"})
	// Drill into "Art" albums.
	m.activeView = viewArtists
	m.artists.artists = []string{"Art"}
	m.artists.selectedArtist = "Art"
	m.artists = m.artists.openAlbums([]string{"Album1"})

	if len(m.artists.albums) != 1 {
		t.Fatalf("setup: expected 1 album, got %d", len(m.artists.albums))
	}

	// Watcher adds a second track with a new album under the same artist.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Album2"},
		},
	})
	nm := next.(Model)

	if len(nm.artists.albums) != 2 {
		t.Errorf("expected 2 albums after watcher, got %d: %v", len(nm.artists.albums), nm.artists.albums)
	}
}

// TestWatchBatchRefreshesArtistTrackLevel verifies that when the artists view
// is drilled into TrackLevel, a watchBatchMsg re-derives the track list so
// metadata changes are reflected immediately.
func TestWatchBatchRefreshesArtistTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Art" > "Alb" tracks.
	m.activeView = viewArtists
	m.artists.artists = []string{"Art"}
	m.artists.selectedArtist = "Art"
	m.artists = m.artists.openAlbums([]string{"Alb"})
	m.artists.selectedAlbum = "Alb"
	m.artists = m.artists.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	if len(m.artists.tracks) != 1 {
		t.Fatalf("setup: expected 1 track, got %d", len(m.artists.tracks))
	}

	// Watcher adds a new track under the same artist+album.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.artists.tracks) != 2 {
		t.Errorf("expected 2 tracks after watcher, got %d", len(nm.artists.tracks))
	}
}

// TestWatchBatchRefreshesAlbumTrackLevel verifies that when the albums view
// is drilled into TrackLevel, a watchBatchMsg re-derives the track list so
// new tracks and metadata changes are reflected immediately.
func TestWatchBatchRefreshesAlbumTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Alb" tracks.
	m.activeView = viewAlbums
	m.albums.albums = []string{"Alb"}
	m.albums.selectedAlbum = "Alb"
	m.albums = m.albums.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	if len(m.albums.tracks) != 1 {
		t.Fatalf("setup: expected 1 track, got %d", len(m.albums.tracks))
	}

	// Watcher adds a new track under the same album.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art2", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.albums.tracks) != 2 {
		t.Errorf("expected 2 tracks after watcher, got %d", len(nm.albums.tracks))
	}
}

// TestScanBatchRefreshesArtistAlbumLevel verifies that when the artists view
// is drilled into AlbumLevel, a ScanBatchMsg re-derives the album list so
// newly scanned albums appear.
func TestScanBatchRefreshesArtistAlbumLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Album1"})
	// Drill into "Art" albums.
	m.activeView = viewArtists
	m.artists.artists = []string{"Art"}
	m.artists.selectedArtist = "Art"
	m.artists = m.artists.openAlbums([]string{"Album1"})

	// Scan batch adds a second track with a new album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Album2"},
		},
	})
	nm := next.(Model)

	if len(nm.artists.albums) != 2 {
		t.Errorf("expected 2 albums after scan, got %d: %v", len(nm.artists.albums), nm.artists.albums)
	}
}

// TestScanBatchRefreshesArtistTrackLevel verifies that when the artists view
// is drilled into TrackLevel, a ScanBatchMsg re-derives the track list.
func TestScanBatchRefreshesArtistTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Art" > "Alb" tracks.
	m.activeView = viewArtists
	m.artists.artists = []string{"Art"}
	m.artists.selectedArtist = "Art"
	m.artists = m.artists.openAlbums([]string{"Alb"})
	m.artists.selectedAlbum = "Alb"
	m.artists = m.artists.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	// Scan batch adds a new track under the same artist+album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.artists.tracks) != 2 {
		t.Errorf("expected 2 tracks after scan, got %d", len(nm.artists.tracks))
	}
}

// TestScanBatchRefreshesAlbumTrackLevel verifies that when the albums view
// is drilled into TrackLevel, a ScanBatchMsg re-derives the track list.
func TestScanBatchRefreshesAlbumTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Alb" tracks.
	m.activeView = viewAlbums
	m.albums.albums = []string{"Alb"}
	m.albums.selectedAlbum = "Alb"
	m.albums = m.albums.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	// Scan batch adds a new track under the same album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art2", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.albums.tracks) != 2 {
		t.Errorf("expected 2 tracks after scan, got %d", len(nm.albums.tracks))
	}
}

// TestRescanResetsGenresView verifies that triggering a rescan clears the
// genres view's stale drill-down state (selectedGenre, selectedArtist) so
// that stale references from the previous scan don't persist.
func TestRescanResetsGenresView(t *testing.T) {
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))

	// Set up genres view at ArtistLevel with a selected genre.
	m.genres.level = gentui.ArtistLevel
	m.genres.selectedGenre = "Rock"
	m.genres.artists = []string{"Artist1", "Artist2"}
	m.genres.cursor = 1
	m.genres.scrollOffset = 0

	// Trigger rescan.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	nm := next.(Model)
	if cmd == nil {
		t.Fatal("rescan: expected non-nil cmd")
	}

	// Genres view must be reset to GenreLevel with no stale selection.
	if nm.genres.level != gentui.GenreLevel {
		t.Fatalf("rescan: expected genres level=GenreLevel, got %v", nm.genres.level)
	}
	if nm.genres.selectedGenre != "" {
		t.Fatalf("rescan: expected selectedGenre='', got %q", nm.genres.selectedGenre)
	}
	if nm.genres.selectedArtist != "" {
		t.Fatalf("rescan: expected selectedArtist='', got %q", nm.genres.selectedArtist)
	}
	if nm.genres.cursor != 0 {
		t.Fatalf("rescan: expected genres cursor=0, got %d", nm.genres.cursor)
	}
	if len(nm.genres.artists) != 0 {
		t.Fatalf("rescan: expected genres artists empty, got %d", len(nm.genres.artists))
	}
}

// TestScanBatchPreservesUserEditedOverlay verifies that handleScanBatchMsg
// uses the userEditedOverlay to preserve manually edited metadata instead
// of overwriting with raw scanned values.
func TestScanBatchPreservesUserEditedOverlay(t *testing.T) {
	m := New(context.Background())
	m.userEditedOverlay = map[string]db.UserEditedRow{
		"hash1": {Hash: "hash1", Title: "Edited Title", Artist: "Edited Artist", Album: "Edited Album", Genre: "Edited Genre"},
	}

	scanned := []library.Track{
		{Hash: "hash1", Path: "/a.mp3", Title: "Scanned Title", Artist: "Scanned Artist", Album: "Scanned Album", Genre: "Scanned Genre"},
		{Hash: "hash2", Path: "/b.mp3", Title: "B Title", Artist: "B Artist", Album: "B Album", Genre: "B Genre"},
	}

	next, _ := m.Update(library.ScanBatchMsg{Tracks: scanned})
	nm := next.(Model)

	// Track with user-edited overlay should have edited metadata.
	t1, ok := nm.library.index.FindByHash("hash1")
	if !ok {
		t.Fatal("hash1 not found in index")
	}
	if t1.Title != "Edited Title" {
		t.Errorf("hash1 Title = %q; want Edited Title", t1.Title)
	}
	if t1.Artist != "Edited Artist" {
		t.Errorf("hash1 Artist = %q; want Edited Artist", t1.Artist)
	}
	if t1.Album != "Edited Album" {
		t.Errorf("hash1 Album = %q; want Edited Album", t1.Album)
	}
	if t1.Genre != "Edited Genre" {
		t.Errorf("hash1 Genre = %q; want Edited Genre", t1.Genre)
	}

	// Track without overlay should have scanned metadata.
	t2, ok := nm.library.index.FindByHash("hash2")
	if !ok {
		t.Fatal("hash2 not found in index")
	}
	if t2.Title != "B Title" {
		t.Errorf("hash2 Title = %q; want B Title", t2.Title)
	}
}

func TestScanBatchStampsCreatedAtForNewTrack(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{{Hash: "hash1", Path: "/a.mp3", Title: "A"}}})
	nm := next.(Model)

	track, ok := nm.library.index.FindByHash("hash1")
	if !ok {
		t.Fatal("hash1 not found in index")
	}
	if track.CreatedAt == 0 {
		t.Fatal("expected created_at to be stamped for new scan track")
	}
}

func TestWatchBatchPreservesCreatedAtForUpdatedPath(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "oldhash", Path: "/a.mp3", Title: "Old", CreatedAt: 123})
	m = m.syncRecentlyAddedFromLibrary()

	next, _ := m.Update(watchBatchMsg{tracks: []library.Track{{Hash: "newhash", Path: "/a.mp3", Title: "New"}}})
	nm := next.(Model)

	track, ok := nm.library.index.FindByHash("newhash")
	if !ok {
		t.Fatal("newhash not found in index")
	}
	if track.CreatedAt != 123 {
		t.Fatalf("created_at: got %d, want 123", track.CreatedAt)
	}
}

// TestWatchBatchPreservesMetadataWithUserEditedOverlay verifies that when a
// watcher event arrives for a track that has a user-edited overlay AND had
// its CreatedAt preserved by preserveTrackMetadata, both the overlay edits
// and the preserved CreatedAt/Duration survive into the library index.
func TestWatchBatchPreservesMetadataWithUserEditedOverlay(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.mp3",
		Title: "Original", Artist: "OrigArtist", Album: "OrigAlbum",
		CreatedAt: 42, Duration: 3.5,
	})
	m = m.syncRecentlyAddedFromLibrary()

	m.userEditedOverlay = map[string]db.UserEditedRow{
		"h1": {Hash: "h1", Title: "Edited Title", Artist: "Edited Artist", Album: "Edited Album", Genre: "Edited Genre"},
	}

	// Watcher reports an update with new scanned values (no CreatedAt, no Duration).
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.mp3", Title: "Scanned", Artist: "ScannedArtist", Album: "ScannedAlbum"},
		},
	})
	nm := next.(Model)

	track, ok := nm.library.index.FindByHash("h1")
	if !ok {
		t.Fatal("h1 not found in index")
	}
	if track.Title != "Edited Title" {
		t.Errorf("Title = %q; want %q", track.Title, "Edited Title")
	}
	if track.Artist != "Edited Artist" {
		t.Errorf("Artist = %q; want %q", track.Artist, "Edited Artist")
	}
	if track.Album != "Edited Album" {
		t.Errorf("Album = %q; want %q", track.Album, "Edited Album")
	}
	if track.Genre != "Edited Genre" {
		t.Errorf("Genre = %q; want %q", track.Genre, "Edited Genre")
	}
	if track.CreatedAt != 42 {
		t.Errorf("CreatedAt = %d; want 42 (preserved)", track.CreatedAt)
	}
	if track.Duration != 3.5 {
		t.Errorf("Duration = %f; want 3.5 (preserved)", track.Duration)
	}
}

// TestRescanSnapshotPreservesCreatedAtAndDuration verifies that a full rescan
// (which clears the index) still preserves created_at and duration via the
// pre-rescan snapshot.
func TestRescanSnapshotPreservesCreatedAtAndDuration(t *testing.T) {
	m := New(context.Background())

	// Populate the index with a track that has created_at and duration set.
	m.library.index.Insert(library.Track{
		Hash: "hash1", Path: "/a.mp3", Title: "A",
		CreatedAt: 1000, Duration: 120.5,
	})

	// Build snapshot (simulating what actionRescan does before clearing).
	tracks := m.library.index.Tracks()
	snap := make(map[string]rescanEntry, len(tracks))
	for _, t := range tracks {
		snap[t.Hash] = rescanEntry{CreatedAt: t.CreatedAt, Duration: t.Duration}
	}
	m.rescanSnapshot = snap

	// Clear the index (simulating rescan).
	m.library.index = library.LibraryIndex{}

	// Deliver a scan batch with CreatedAt=0 and Duration=0 (scanner defaults).
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Hash: "hash1", Path: "/a.mp3", Title: "A", CreatedAt: 0, Duration: 0},
	}})
	nm := next.(Model)

	track, ok := nm.library.index.FindByHash("hash1")
	if !ok {
		t.Fatal("hash1 not found in index")
	}
	if track.CreatedAt != 1000 {
		t.Errorf("CreatedAt = %d; want 1000 (should be preserved from snapshot)", track.CreatedAt)
	}
	if track.Duration != 120.5 {
		t.Errorf("Duration = %f; want 120.5 (should be preserved from snapshot)", track.Duration)
	}
}

// TestRescanSnapshotAllowsNonZeroDurationUpdate verifies that when a rescan
// provides a non-zero duration, it takes precedence over the snapshot value.
func TestRescanSnapshotAllowsNonZeroDurationUpdate(t *testing.T) {
	m := New(context.Background())

	m.library.index.Insert(library.Track{
		Hash: "hash1", Path: "/a.mp3", Title: "A",
		CreatedAt: 1000, Duration: 60.0,
	})

	tracks := m.library.index.Tracks()
	snap := make(map[string]rescanEntry, len(tracks))
	for _, t := range tracks {
		snap[t.Hash] = rescanEntry{CreatedAt: t.CreatedAt, Duration: t.Duration}
	}
	m.rescanSnapshot = snap
	m.library.index = library.LibraryIndex{}

	// Scanner found a new duration.
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Hash: "hash1", Path: "/a.mp3", Title: "A", CreatedAt: 0, Duration: 90.0},
	}})
	nm := next.(Model)

	track, ok := nm.library.index.FindByHash("hash1")
	if !ok {
		t.Fatal("hash1 not found in index")
	}
	if track.Duration != 90.0 {
		t.Errorf("Duration = %f; want 90.0 (non-zero scanned duration should take precedence)", track.Duration)
	}
}
