package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

func TestUpdateViewSwitching(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, "", nil)

	if m.activeView != viewLibrary {
		t.Fatalf("expected viewLibrary (%d), got %d", viewLibrary, m.activeView)
	}

	// 2 → Playlists
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)
	if nm.activeView != viewPlaylists {
		t.Fatalf("expected viewPlaylists (%d), got %d", viewPlaylists, nm.activeView)
	}

	// 3 → Queue
	next, _ = nm.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm = next.(Model)
	if nm.activeView != viewQueue {
		t.Fatalf("expected viewQueue (%d), got %d", viewQueue, nm.activeView)
	}

	// 1 → Library
	next, _ = nm.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	nm = next.(Model)
	if nm.activeView != viewLibrary {
		t.Fatalf("expected viewLibrary (%d), got %d", viewLibrary, nm.activeView)
	}
}

func TestUpdateCtrlCQuits(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, "", nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c: expected a quit command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c: expected tea.QuitMsg, got %T", msg)
	}
}

func TestUpdateQDoesNotQuit(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, "", nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Fatal("q key must not quit the application")
	}
}

func TestUpdateEscDoesNotQuit(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, "", nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("Esc key must not quit the application")
	}
}

func TestViewReturnsTeaView(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, "", nil)
	v := m.View()
	// Compile-time check: View() returns tea.View (not string).
	_ = v.Content
}

func TestViewStatusBarShowsLogPath(t *testing.T) {
	logPath := "/var/log/muse/muse.log"
	m := New(logPath, "", false, nil, "", nil)
	v := m.View()
	if !strings.Contains(v.Content, logPath) {
		t.Fatalf("status bar missing log path %q in:\n%s", logPath, v.Content)
	}
}

func TestViewStatusBarShowsConfigPathOnFirstRun(t *testing.T) {
	configPath := "/home/user/.config/muse/config.toml"
	m := New("/log/path", configPath, true, nil, "", nil)
	v := m.View()
	if !strings.Contains(v.Content, configPath) {
		t.Fatalf("status bar missing config path %q in:\n%s", configPath, v.Content)
	}
}

func TestViewStatusBarHidesConfigPathAfterFirstRun(t *testing.T) {
	configPath := "/home/user/.config/muse/config.toml"
	m := New("/log/path", configPath, false, nil, "", nil)
	v := m.View()
	if strings.Contains(v.Content, configPath) {
		t.Fatalf("status bar must not show config path when configCreated=false")
	}
}

// TestUpdateScanBatchMsgAppendsTracksToLibrary verifies that receiving a
// ScanBatchMsg inserts all tracks into the library index.
func TestUpdateScanBatchMsgAppendsTracksToLibrary(t *testing.T) {
	m := New("/log", "", false, nil, "", nil)
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
	m := New("/log", "", false, ch, "", nil)

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
	m := New("/log", "", false, ch, "", nil)

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
	m := New("/log", "", false, nil, "", nil)
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
// formatted "X / Y tracks" progress string after a ScanProgressMsg.
func TestViewStatusBarShowsScanProgress(t *testing.T) {
	m := New("/log", "", false, nil, "", nil)
	next, _ := m.Update(library.ScanProgressMsg{Scanned: 1234, Total: 5000})
	nm := next.(Model)
	v := nm.View()
	want := "Scanning… 1,234 / 5,000 tracks"
	if !strings.Contains(v.Content, want) {
		t.Fatalf("status bar must show %q after ScanProgressMsg; got:\n%s", want, v.Content)
	}
}

// stubExtractor is a no-op MetadataExtractor used in rescan tests to avoid
// a real exiftool dependency.
type stubExtractor struct{}

func (stubExtractor) Extract(files []library.HashedFile) ([]library.Track, error) {
	return nil, nil
}

// TestRescanHotkeyReturnsCmd verifies that pressing r with a configured
// library root returns a non-nil background cmd and does not block.
func TestRescanHotkeyReturnsCmd(t *testing.T) {
	root := t.TempDir()
	m := New("/log", "", false, nil, root, stubExtractor{})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r key: expected non-nil cmd for rescan, got nil")
	}
}

// TestRescanHotkeyIsNoOpWhenAlreadyScanning verifies that pressing r during
// an in-progress scan does nothing.
func TestRescanHotkeyIsNoOpWhenScanning(t *testing.T) {
	ch := make(chan library.Track)
	defer close(ch)
	root := t.TempDir()
	m := New("/log", "", false, ch, root, stubExtractor{})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd != nil {
		t.Fatal("r key during active scan must return nil cmd")
	}
}

// TestRescanHotkeyIsNoOpWithoutRoot verifies that pressing r when no library
// root is configured returns nil.
func TestRescanHotkeyIsNoOpWithoutRoot(t *testing.T) {
	m := New("/log", "", false, nil, "", nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd != nil {
		t.Fatal("r key without library root must return nil cmd")
	}
}

// TestRescanResetsIndexAndAddsNewTracks verifies that triggering a rescan
// clears the existing index and that subsequent ScanBatchMsg messages
// populate it with only the new tracks, effectively removing deleted ones.
func TestRescanResetsIndexAndAddsNewTracks(t *testing.T) {
	root := t.TempDir()
	m := New("/log", "", false, nil, root, stubExtractor{})

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
	next, cmd := nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
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
// indicator is removed from the status bar after ScanDoneMsg.
func TestViewStatusBarHidesScanProgressWhenDone(t *testing.T) {
	m := New("/log", "", false, nil, "", nil)
	// First set progress, then clear it.
	next, _ := m.Update(library.ScanProgressMsg{Scanned: 100, Total: 500})
	nm := next.(Model)
	next, _ = nm.Update(library.ScanDoneMsg{})
	nm = next.(Model)
	v := nm.View()
	if strings.Contains(v.Content, "Scanning") {
		t.Fatalf("status bar must not show scan progress after ScanDoneMsg; got:\n%s", v.Content)
	}
}
