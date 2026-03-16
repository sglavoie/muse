package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// TestUpdateScanBatchMsgAppendsTracksToLibrary verifies that receiving a
// ScanBatchMsg inserts all tracks into the library index.
func TestUpdateScanBatchMsgAppendsTracksToLibrary(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
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
	m := New("", false, ch, nil, "", nil, nil)

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
	m := New("", false, ch, nil, "", nil, nil)

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
	m := New("", false, nil, nil, "", nil, nil)
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
	m := New("", false, nil, nil, "", nil, nil)
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
	m := New("", false, nil, nil, root, stubExtractor{}, nil)
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
	m := New("", false, ch, nil, root, stubExtractor{}, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd != nil {
		t.Fatal("R key during active scan must return nil cmd")
	}
}

// TestRescanHotkeyIsNoOpWithoutRoot verifies that pressing R when no library
// root is configured returns nil.
func TestRescanHotkeyIsNoOpWithoutRoot(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
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
	m := New("", false, nil, nil, root, stubExtractor{}, nil)

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
	m := New("", false, nil, nil, "", nil, nil)
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
	m := New("", false, nil, nil, "", nil, nil)

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
	m := New("", false, nil, nil, root, stubExtractor{}, nil)

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
	nm.library = nm.library.ActivateFilter()
	nm.library = nm.library.AppendFilterChar('T')
	nm.library = nm.library.CommitFilter()
	if nm.library.filterText != "T" {
		t.Fatalf("setup: expected filterText='T', got %q", nm.library.filterText)
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
	if nm.library.filterActive {
		t.Fatal("rescan: expected filterActive=false")
	}
	if nm.library.filterText != "" {
		t.Fatalf("rescan: expected filterText='', got %q", nm.library.filterText)
	}
	if nm.library.filteredMatches != nil {
		t.Fatal("rescan: expected filteredMatches=nil")
	}
}

// TestShiftRFiresRescan verifies that pressing 'R' (Shift+r) with a configured
// library root triggers a rescan, while lowercase 'r' does not.
func TestShiftRFiresRescan(t *testing.T) {
	root := t.TempDir()
	m := New("", false, nil, nil, root, stubExtractor{}, nil)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd == nil {
		t.Fatal("R key: expected non-nil rescan cmd")
	}
}
