package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
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
	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("notification must still be set immediately after ScanDoneMsg")
	}
	// Manually expire the notification to simulate 5 seconds passing.
	if len(nm.statusBar.Notifications) > 0 {
		nm.statusBar.Notifications[0].ExpiresAt = time.Now().Add(-10 * time.Second)
	}
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
	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("scan progress notification must persist immediately after ScanDoneMsg")
	}

	// Expire the notification and fire a tick.
	if len(nm.statusBar.Notifications) > 0 {
		nm.statusBar.Notifications[0].ExpiresAt = time.Now().Add(-10 * time.Second)
	}
	next, _ = nm.Update(tickMsg{})
	nm = next.(Model)
	v = nm.View()
	if strings.Contains(v.Content, "Scanning") {
		t.Fatalf("scan progress must be auto-dismissed after expiry tick; got:\n%s", v.Content)
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
	m.library.filter, _ = m.library.filter.Update(tea.KeyPressMsg{Text: "a"})
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

// TestScanBatchRecomputesSortedTracks verifies that when a sort is
// active and new tracks arrive via a scan batch, the sorted cache is
// rebuilt to include the new tracks.
func TestScanBatchRecomputesSortedTracks(t *testing.T) {
	m := New(context.Background())

	// Seed two tracks and sort.
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.sortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}
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
