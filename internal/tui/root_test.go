package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

func TestUpdateViewSwitching(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil)

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
	m := New("/state/muse/muse.log", "", false, nil)
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
	m := New("/state/muse/muse.log", "", false, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Fatal("q key must not quit the application")
	}
}

func TestUpdateEscDoesNotQuit(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("Esc key must not quit the application")
	}
}

func TestViewReturnsTeaView(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil)
	v := m.View()
	// Compile-time check: View() returns tea.View (not string).
	_ = v.Content
}

func TestViewStatusBarShowsLogPath(t *testing.T) {
	logPath := "/var/log/muse/muse.log"
	m := New(logPath, "", false, nil)
	v := m.View()
	if !strings.Contains(v.Content, logPath) {
		t.Fatalf("status bar missing log path %q in:\n%s", logPath, v.Content)
	}
}

func TestViewStatusBarShowsConfigPathOnFirstRun(t *testing.T) {
	configPath := "/home/user/.config/muse/config.toml"
	m := New("/log/path", configPath, true, nil)
	v := m.View()
	if !strings.Contains(v.Content, configPath) {
		t.Fatalf("status bar missing config path %q in:\n%s", configPath, v.Content)
	}
}

func TestViewStatusBarHidesConfigPathAfterFirstRun(t *testing.T) {
	configPath := "/home/user/.config/muse/config.toml"
	m := New("/log/path", configPath, false, nil)
	v := m.View()
	if strings.Contains(v.Content, configPath) {
		t.Fatalf("status bar must not show config path when configCreated=false")
	}
}

// TestUpdateScanBatchMsgAppendsTracksToLibrary verifies that receiving a
// ScanBatchMsg inserts all tracks into the library index.
func TestUpdateScanBatchMsgAppendsTracksToLibrary(t *testing.T) {
	m := New("/log", "", false, nil)
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
	m := New("/log", "", false, ch)

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
	m := New("/log", "", false, ch)

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
	m := New("/log", "", false, nil)
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

// TestViewStatusBarShowsScanProgress verifies that the status bar includes a
// scanning indicator while a scan is in flight.
func TestViewStatusBarShowsScanProgress(t *testing.T) {
	ch := make(chan library.Track)
	close(ch)
	m := New("/log", "", false, ch)

	// Inject some scanned count by handling a batch message.
	tracks := []library.Track{
		{Artist: "A", Title: "T1"},
		{Artist: "B", Title: "T2"},
	}
	next, _ := m.Update(library.ScanBatchMsg{Tracks: tracks})
	nm := next.(Model)
	v := nm.View()
	if !strings.Contains(v.Content, "Scanning") {
		t.Fatalf("status bar must show scan progress while scanning; got:\n%s", v.Content)
	}
}

// TestViewStatusBarHidesScanProgressWhenDone verifies that the scanning
// indicator is removed from the status bar after ScanDoneMsg.
func TestViewStatusBarHidesScanProgressWhenDone(t *testing.T) {
	m := New("/log", "", false, nil)
	next, _ := m.Update(library.ScanDoneMsg{})
	nm := next.(Model)
	v := nm.View()
	if strings.Contains(v.Content, "Scanning") {
		t.Fatalf("status bar must not show scan progress after scan done; got:\n%s", v.Content)
	}
}
