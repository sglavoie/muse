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
	if nm.library.Index.Len() != 2 {
		t.Fatalf("expected 2 tracks in index, got %d", nm.library.Index.Len())
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

// Completion replaces progress immediately, survives in history, and expires
// without resurrecting old progress or dropping unrelated notifications.
func TestScanCompletionNotification(t *testing.T) {
	for _, tc := range []struct {
		count int
		want  string
	}{
		{0, "Scan complete — 0 tracks"},
		{1, "Scan complete — 1 track"},
		{1234, "Scan complete — 1,234 tracks"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			m := New(context.Background(), WithScanChannel(make(chan library.Track)))
			m.scanned = tc.count
			m.statusBar.NotifyInfo("Volume: 50%")
			m.statusBar.HandleScanProgress(library.ScanProgressMsg{Scanned: tc.count})
			next, _ := m.Update(library.ScanDoneMsg{})
			m = next.(Model)
			if got := m.statusBar.TopNotification().Msg; got != tc.want {
				t.Fatalf("notification = %q, want %q", got, tc.want)
			}
			history := m.statusBar.RecentMessages()
			if len(history) != 1 || history[0].Text != tc.want {
				t.Fatalf("history = %+v", history)
			}
			for i, n := range m.statusBar.Notifications {
				if strings.Contains(n.Msg, "Scanning") {
					t.Fatalf("stale progress retained: %q", n.Msg)
				}
				if n.Msg == tc.want {
					m.statusBar.Notifications[i].ExpiresAt = time.Now().Add(-time.Second)
				}
			}
			m.statusBar.PruneNotifications()
			if got := m.statusBar.TopNotification().Msg; got != "Volume: 50%" {
				t.Fatalf("unrelated notification lost: %q", got)
			}
			if len(m.statusBar.RecentMessages()) != 1 {
				t.Fatal("completion disappeared from history after expiry")
			}
		})
	}
}

func TestScanCompletionDoesNotReportCanceledOrIncompleteAsSuccess(t *testing.T) {
	for _, outcome := range []string{"canceled", "incomplete", "inactive"} {
		t.Run(outcome, func(t *testing.T) {
			m := New(context.Background(), WithScanChannel(make(chan library.Track)))
			m.statusBar.HandleScanProgress(library.ScanProgressMsg{Scanned: 5})
			switch outcome {
			case "canceled":
				m, _, _ = m.handleScanCancelKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
				// A final batch may still drain after cancellation.
				m, _ = m.handleScanBatchMsg(library.ScanBatchMsg{})
			case "incomplete":
				m.scanOutcome = &library.StageOutcome{}
				m.scanOutcome.Fail(library.ErrHashWorkerPanicked)
			case "inactive":
				m.scanning = false
			}
			next, _ := m.Update(library.ScanDoneMsg{})
			m = next.(Model)
			for _, n := range m.statusBar.Notifications {
				if strings.Contains(n.Msg, "Scan complete") || strings.Contains(n.Msg, "Scanning") {
					t.Fatalf("unexpected notification: %q", n.Msg)
				}
			}
			if outcome == "canceled" && m.statusBar.TopNotification().Msg != "Scan canceled" {
				t.Fatal("cancellation notice was lost")
			}
			if outcome == "incomplete" && !strings.Contains(m.statusBar.TopNotification().Msg, "Rescan incomplete") {
				t.Fatal("incomplete scan notice was lost")
			}
			for _, entry := range m.statusBar.RecentMessages() {
				if strings.Contains(entry.Text, "Scan complete") {
					t.Fatal("unsuccessful scan recorded as complete")
				}
			}
		})
	}
}

// TestScanBatchInvalidatesFilterCache verifies that a ScanBatchMsg that adds
// tracks while a filter is active rebuilds the cached fuzzy-match results so
// that TrackIndex values stay consistent with the modified index.
func TestScanBatchInvalidatesFilterCache(t *testing.T) {
	m := New(context.Background())
	m.library.Fuzzy = library.FakeFuzzySearcher{}

	// Seed two tracks and set up a filter.
	m.library.Index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.Index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})
	m.library.Filter, _ = m.library.Filter.Activate()
	m.library.Filter, _ = m.library.Filter.Update(tea.KeyPressMsg{Text: "a"})
	m.library.Cursor = 0
	m.library.ScrollOffset = 0
	m.library = m.library.RebuildFilterCache()
	m.library.Filter = m.library.Filter.Commit()

	oldMatches := len(m.library.FilteredMatches)
	if oldMatches != 2 {
		t.Fatalf("setup: expected 2 matches, got %d", oldMatches)
	}

	// Insert a new track that also matches the filter.
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Artist: "AA", Title: "Gamma", Path: "/g", Hash: "h3"},
	}})
	nm := next.(Model)

	// The filter cache must have been rebuilt to include the new track.
	if len(nm.library.FilteredMatches) <= oldMatches {
		t.Fatalf("expected filteredMatches to grow after ScanBatchMsg; got %d (was %d)",
			len(nm.library.FilteredMatches), oldMatches)
	}

	// All TrackIndex values must be in range.
	allTracks := nm.library.Index.Tracks()
	for i, match := range nm.library.FilteredMatches {
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
	m.library.Index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})
	m.library.Index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.SortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}
	m.library = m.library.RecomputeSortedTracks()
	if len(m.library.SortedTracks) != 2 {
		t.Fatalf("setup: expected 2 sorted tracks, got %d", len(m.library.SortedTracks))
	}

	// Deliver a scan batch with one new track.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "C", Title: "Charlie", Path: "/c", Hash: "h3"},
		},
	})
	nm := next.(Model)

	// The sorted cache must include the new track.
	if len(nm.library.SortedTracks) != 3 {
		t.Fatalf("expected 3 sorted tracks after scan, got %d", len(nm.library.SortedTracks))
	}
}
