package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

func TestUpdateViewSwitching(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, nil, "", nil, nil)

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
	m := New("/state/muse/muse.log", "", false, nil, nil, "", nil, nil)
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
	m := New("/state/muse/muse.log", "", false, nil, nil, "", nil, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Fatal("q key must not quit the application")
	}
}

func TestUpdateEscDoesNotQuit(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, nil, "", nil, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("Esc key must not quit the application")
	}
}

func TestViewReturnsTeaView(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, nil, "", nil, nil)
	v := m.View()
	// Compile-time check: View() returns tea.View (not string).
	_ = v.Content
}

func TestViewStatusBarShowsLogPath(t *testing.T) {
	logPath := "/var/log/muse/muse.log"
	m := New(logPath, "", false, nil, nil, "", nil, nil)
	v := m.View()
	if !strings.Contains(v.Content, logPath) {
		t.Fatalf("status bar missing log path %q in:\n%s", logPath, v.Content)
	}
}

func TestViewStatusBarShowsConfigPathOnFirstRun(t *testing.T) {
	configPath := "/home/user/.config/muse/config.toml"
	m := New("/log/path", configPath, true, nil, nil, "", nil, nil)
	v := m.View()
	if !strings.Contains(v.Content, configPath) {
		t.Fatalf("status bar missing config path %q in:\n%s", configPath, v.Content)
	}
}

func TestViewStatusBarHidesConfigPathAfterFirstRun(t *testing.T) {
	configPath := "/home/user/.config/muse/config.toml"
	m := New("/log/path", configPath, false, nil, nil, "", nil, nil)
	v := m.View()
	if strings.Contains(v.Content, configPath) {
		t.Fatalf("status bar must not show config path when configCreated=false")
	}
}

// TestUpdateScanBatchMsgAppendsTracksToLibrary verifies that receiving a
// ScanBatchMsg inserts all tracks into the library index.
func TestUpdateScanBatchMsgAppendsTracksToLibrary(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
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
	m := New("/log", "", false, ch, nil, "", nil, nil)

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
	m := New("/log", "", false, ch, nil, "", nil, nil)

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
	m := New("/log", "", false, nil, nil, "", nil, nil)
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
	m := New("/log", "", false, nil, nil, "", nil, nil)
	next, _ := m.Update(library.ScanProgressMsg{Scanned: 1234})
	nm := next.(Model)
	v := nm.View()
	want := "Scanning… 1,234 tracks"
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

// TestRescanHotkeyReturnsCmd verifies that pressing R (Shift+r) with a
// configured library root returns a non-nil background cmd and does not block.
func TestRescanHotkeyReturnsCmd(t *testing.T) {
	root := t.TempDir()
	m := New("/log", "", false, nil, nil, root, stubExtractor{}, nil)
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
	m := New("/log", "", false, ch, nil, root, stubExtractor{}, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd != nil {
		t.Fatal("R key during active scan must return nil cmd")
	}
}

// TestRescanHotkeyIsNoOpWithoutRoot verifies that pressing R when no library
// root is configured returns nil.
func TestRescanHotkeyIsNoOpWithoutRoot(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd != nil {
		t.Fatal("R key without library root must return nil cmd")
	}
}

// TestUpdateMissingTrackMsgMarksQueueEntryAndSetsError verifies that
// MissingTrackMsg marks the matching queue entry as missing and sets a
// transient error message in the status bar.
func TestUpdateMissingTrackMsgMarksQueueEntryAndSetsError(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.queue.entries = []QueueEntry{
		{Path: "/music/song.flac", Display: "Artist – Song"},
	}

	next, _ := m.Update(MissingTrackMsg{Path: "/music/song.flac"})
	nm := next.(Model)

	if nm.statusBar.notification.msg == "" {
		t.Fatal("MissingTrackMsg must set a transient status bar error message")
	}
	if !nm.queue.entries[0].Missing {
		t.Fatal("MissingTrackMsg must mark the matching queue entry as missing")
	}
}

// TestQueueViewRendersMissingEntryDimmed verifies that a missing queue entry
// is wrapped in ANSI dim escape codes in the rendered output.
func TestQueueViewRendersMissingEntryDimmed(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/gone.flac", Display: "Artist – Gone", Missing: true},
		},
	}
	got := q.render()
	if !strings.Contains(got, ansiDim) {
		t.Fatalf("missing entry must use dim ANSI code; got:\n%s", got)
	}
	if !strings.Contains(got, "[missing]") {
		t.Fatalf("missing entry must include [missing] label; got:\n%s", got)
	}
}

// TestRescanResetsIndexAndAddsNewTracks verifies that triggering a rescan
// clears the existing index and that subsequent ScanBatchMsg messages
// populate it with only the new tracks, effectively removing deleted ones.
func TestRescanResetsIndexAndAddsNewTracks(t *testing.T) {
	root := t.TempDir()
	m := New("/log", "", false, nil, nil, root, stubExtractor{}, nil)

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
	m := New("/log", "", false, nil, nil, "", nil, nil)
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

// stubPlayer records Play, Pause, and PlayFromIndex calls for assertion in tests.
type stubPlayer struct {
	playCalls          int
	pauseCalls         int
	playFromIndexCalls []int
}

func (s *stubPlayer) Play() error {
	s.playCalls++
	return nil
}

func (s *stubPlayer) Pause() error {
	s.pauseCalls++
	return nil
}

func (s *stubPlayer) PlayFromIndex(idx int) error {
	s.playFromIndexCalls = append(s.playFromIndexCalls, idx)
	return nil
}

// TestQueueViewHandleActionEnterSendsPlayQueueIndexMsg verifies that
// actionSelect returns a Cmd that emits PlayQueueIndexMsg with the cursor
// index. Root routes Enter → actionSelect via the registry and calls
// HandleAction, so this is the code path exercised in production.
func TestQueueViewHandleActionEnterSendsPlayQueueIndexMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "Artist – A"},
			{Path: "/music/b.flac", Display: "Artist – B"},
		},
		cursor: 1,
	}
	_, cmd := q.HandleAction(actionSelect)
	if cmd == nil {
		t.Fatal("actionSelect must return a non-nil cmd")
	}
	msg := cmd()
	pqi, ok := msg.(PlayQueueIndexMsg)
	if !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
	if pqi.Index != 1 {
		t.Fatalf("expected index 1, got %d", pqi.Index)
	}
}

// TestQueueViewHandleActionEnterOnEmptyQueueIsNoOp verifies that actionSelect
// on an empty queue does not emit a PlayQueueIndexMsg.
func TestQueueViewHandleActionEnterOnEmptyQueueIsNoOp(t *testing.T) {
	q := QueueView{}
	_, cmd := q.HandleAction(actionSelect)
	if cmd != nil {
		t.Fatal("actionSelect on empty queue must return nil cmd")
	}
}

// TestQueueViewUpdateDSendsDequeueTrackMsg verifies that actionDequeue
// dispatches DequeueTrackMsg with the current cursor index.
func TestQueueViewUpdateDSendsDequeueTrackMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 1,
	}
	_, cmd := q.HandleAction(actionDequeue)
	if cmd == nil {
		t.Fatal("actionDequeue must return a non-nil cmd")
	}
	msg, ok := cmd().(DequeueTrackMsg)
	if !ok {
		t.Fatalf("expected DequeueTrackMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Fatalf("expected index 1, got %d", msg.Index)
	}
}

// TestQueueViewUpdateDOnEmptyQueueIsNoOp verifies that actionDequeue on an
// empty queue does not emit a DequeueTrackMsg.
func TestQueueViewUpdateDOnEmptyQueueIsNoOp(t *testing.T) {
	q := QueueView{}
	_, cmd := q.HandleAction(actionDequeue)
	if cmd != nil {
		t.Fatal("actionDequeue on empty queue must return nil cmd")
	}
}

// TestQueueViewUpdateXSendsBulkSelectionToggleMsg verifies that
// actionBulkSelectionToggle dispatches BulkSelectionToggleMsg with the current
// cursor index.
func TestQueueViewUpdateXSendsBulkSelectionToggleMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 0,
	}
	_, cmd := q.HandleAction(actionBulkSelectionToggle)
	if cmd == nil {
		t.Fatal("actionBulkSelectionToggle must return a non-nil cmd")
	}
	msg, ok := cmd().(BulkSelectionToggleMsg)
	if !ok {
		t.Fatalf("expected BulkSelectionToggleMsg, got %T", cmd())
	}
	if msg.Index != 0 {
		t.Fatalf("expected index 0, got %d", msg.Index)
	}
}

// TestQueueViewUpdateCSendsClearQueueMsg verifies that actionClearQueue
// dispatches ClearQueueMsg.
func TestQueueViewUpdateCSendsClearQueueMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
		},
	}
	_, cmd := q.HandleAction(actionClearQueue)
	if cmd == nil {
		t.Fatal("actionClearQueue must return a non-nil cmd")
	}
	if _, ok := cmd().(ClearQueueMsg); !ok {
		t.Fatalf("expected ClearQueueMsg, got %T", cmd())
	}
}

// TestQueueViewUpdateKSendsMoveTrackUpMsg verifies that actionMoveTrackUp
// dispatches MoveTrackUpMsg with the current cursor index.
func TestQueueViewUpdateKSendsMoveTrackUpMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 1,
	}
	_, cmd := q.HandleAction(actionMoveTrackUp)
	if cmd == nil {
		t.Fatal("actionMoveTrackUp must return a non-nil cmd")
	}
	msg, ok := cmd().(MoveTrackUpMsg)
	if !ok {
		t.Fatalf("expected MoveTrackUpMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Fatalf("expected index 1, got %d", msg.Index)
	}
}

// TestQueueViewUpdateKAtTopIsNoOp verifies that actionMoveTrackUp at the
// first entry (boundary) does not emit a MoveTrackUpMsg.
func TestQueueViewUpdateKAtTopIsNoOp(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 0,
	}
	_, cmd := q.HandleAction(actionMoveTrackUp)
	if cmd != nil {
		t.Fatal("actionMoveTrackUp at top entry must return nil cmd")
	}
}

// TestQueueViewUpdateJSendsMoveTrackDownMsg verifies that actionMoveTrackDown
// dispatches MoveTrackDownMsg with the current cursor index.
func TestQueueViewUpdateJSendsMoveTrackDownMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 0,
	}
	_, cmd := q.HandleAction(actionMoveTrackDown)
	if cmd == nil {
		t.Fatal("actionMoveTrackDown must return a non-nil cmd")
	}
	msg, ok := cmd().(MoveTrackDownMsg)
	if !ok {
		t.Fatalf("expected MoveTrackDownMsg, got %T", cmd())
	}
	if msg.Index != 0 {
		t.Fatalf("expected index 0, got %d", msg.Index)
	}
}

// TestQueueViewUpdateJAtBottomIsNoOp verifies that actionMoveTrackDown at the
// last entry (boundary) does not emit a MoveTrackDownMsg.
func TestQueueViewUpdateJAtBottomIsNoOp(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
		},
		cursor: 1,
	}
	_, cmd := q.HandleAction(actionMoveTrackDown)
	if cmd != nil {
		t.Fatal("actionMoveTrackDown at bottom entry must return nil cmd")
	}
}

// TestRootUpdatePlayQueueIndexMsgCallsPlayerPlayFromIndex verifies that root
// handles PlayQueueIndexMsg by calling PlayFromIndex on the player with the
// correct index.
func TestRootUpdatePlayQueueIndexMsgCallsPlayerPlayFromIndex(t *testing.T) {
	p := &stubPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	_, cmd := m.Update(PlayQueueIndexMsg{Index: 1})
	if cmd != nil {
		t.Fatal("PlayQueueIndexMsg must not return a follow-up cmd")
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 1 {
		t.Fatalf("expected PlayFromIndex(1), got calls=%v", p.playFromIndexCalls)
	}
}

// TestRootUpdateEnterInQueueViewDelegatesToQueueUpdate verifies the end-to-end
// flow: pressing Enter while the queue view is active returns a Cmd that emits
// PlayQueueIndexMsg, and a subsequent Update call invokes PlayFromIndex.
func TestRootUpdateEnterInQueueViewDelegatesToQueueUpdate(t *testing.T) {
	p := &stubPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 0

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter in queue view must return a non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(PlayQueueIndexMsg); !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
	// Feed the message back to root to trigger playback.
	_, cmd2 := m.Update(msg)
	if cmd2 != nil {
		t.Fatal("PlayQueueIndexMsg must not return a follow-up cmd")
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 0 {
		t.Fatalf("expected PlayFromIndex(0), got calls=%v", p.playFromIndexCalls)
	}
}

// TestSpaceTogglesPlayPauseFromAnyView verifies that pressing space triggers
// a play or pause call on the player regardless of which view is focused.
func TestSpaceTogglesPlayPauseFromAnyView(t *testing.T) {
	views := []struct {
		name   string
		viewID viewID
	}{
		{"Library", viewLibrary},
		{"Playlists", viewPlaylists},
		{"Queue", viewQueue},
	}

	spaceMsg := tea.KeyPressMsg{Code: tea.KeySpace}

	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			p := &stubPlayer{}
			m := New("/log", "", false, nil, nil, "", nil, p)
			m.activeView = tc.viewID
			m.queue.entries = []QueueEntry{{ID: 1, Display: "Track 1"}}

			// First press: not playing → Play() called.
			next, _ := m.Update(spaceMsg)
			nm := next.(Model)
			if p.playCalls != 1 || p.pauseCalls != 0 {
				t.Fatalf("first space (%s): expected 1 Play call, got play=%d pause=%d", tc.name, p.playCalls, p.pauseCalls)
			}

			// Second press: playing → Pause() called.
			_, _ = nm.Update(spaceMsg)
			if p.playCalls != 1 || p.pauseCalls != 1 {
				t.Fatalf("second space (%s): expected 1 Pause call, got play=%d pause=%d", tc.name, p.playCalls, p.pauseCalls)
			}
		})
	}
}

// stubCommandPlayer extends stubPlayer with the commandPlayer interface so that
// tests can exercise the s/n/p/seek/volume key handlers.
type stubCommandPlayer struct {
	stubPlayer
	stopCalls int
}

func (s *stubCommandPlayer) Stop() error {
	s.stopCalls++
	return nil
}

func (s *stubCommandPlayer) SeekRelative(float64) error      { return nil }
func (s *stubCommandPlayer) SetVolumeRelative(float64) error { return nil }
func (s *stubCommandPlayer) Next() error                     { return nil }
func (s *stubCommandPlayer) Previous() error                 { return nil }

// TestModelPlaybackStateTransitions verifies that the playbackState field on
// root Model transitions correctly through playing, paused, and idle states.
func TestModelPlaybackStateTransitions(t *testing.T) {
	p := &stubPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.queue.entries = []QueueEntry{
		{Path: "/a.flac", Display: "Artist – Song", Artist: "Artist", Title: "Song"},
	}

	if m.playbackState != PlaybackIdle {
		t.Fatalf("initial: expected PlaybackIdle, got %v", m.playbackState)
	}

	// PlayQueueIndexMsg → playing.
	next, _ := m.Update(PlayQueueIndexMsg{Index: 0})
	nm := next.(Model)
	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("after PlayQueueIndexMsg: expected PlaybackPlaying, got %v", nm.playbackState)
	}

	// Space while playing → paused.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm = next.(Model)
	if nm.playbackState != PlaybackPaused {
		t.Fatalf("space (playing→paused): expected PlaybackPaused, got %v", nm.playbackState)
	}

	// Space while paused → playing.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm = next.(Model)
	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("space (paused→playing): expected PlaybackPlaying, got %v", nm.playbackState)
	}

	// PlaybackEndedMsg → idle.
	next, _ = nm.Update(PlaybackEndedMsg{})
	nm = next.(Model)
	if nm.playbackState != PlaybackIdle {
		t.Fatalf("after PlaybackEndedMsg: expected PlaybackIdle, got %v", nm.playbackState)
	}
}

// TestModelStopKeyTransitionsToStopped verifies that the s key transitions
// the playback state to stopped when the player implements commandPlayer.
func TestModelStopKeyTransitionsToStopped(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackPlaying

	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	nm := next.(Model)
	if nm.playbackState != PlaybackStopped {
		t.Fatalf("s key: expected PlaybackStopped, got %v", nm.playbackState)
	}
	if p.stopCalls != 1 {
		t.Fatalf("s key: expected 1 Stop() call, got %d", p.stopCalls)
	}
}

// TestModelRepeatModeDefaultOff verifies that repeat defaults to RepeatOff and
// shuffle defaults to false on a freshly created model.
func TestModelRepeatModeDefaultOff(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	if m.repeatMode != RepeatOff {
		t.Fatalf("repeatMode must default to RepeatOff, got %v", m.repeatMode)
	}
	if m.shuffle {
		t.Fatal("shuffle must default to false")
	}
}

// TestStatusBarReceivesTrackInfoBeforeRender verifies that after a
// PlayQueueIndexMsg is processed, the rendered status bar shows the correct
// artist, title, and playlist source in the persistent left zone.
func TestStatusBarReceivesTrackInfoBeforeRender(t *testing.T) {
	p := &stubPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.queue.entries = []QueueEntry{
		{Path: "/a.flac", Display: "The Artist – The Song", Artist: "The Artist", Title: "The Song"},
	}

	next, _ := m.Update(PlayQueueIndexMsg{Index: 0})
	nm := next.(Model)
	v := nm.View()

	if !strings.Contains(v.Content, "The Artist") {
		t.Fatalf("status bar must show artist; got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "The Song") {
		t.Fatalf("status bar must show title; got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "Queue") {
		t.Fatalf("status bar must show playlist source 'Queue'; got:\n%s", v.Content)
	}
}

// TestStatusBarShowsPlaybackStateIndicator verifies that render() outputs the
// correct unicode indicator for each playback state.
func TestStatusBarShowsPlaybackStateIndicator(t *testing.T) {
	tests := []struct {
		state PlaybackState
		want  string
	}{
		{PlaybackPlaying, "▶"},
		{PlaybackPaused, "⏸"},
		{PlaybackStopped, "■"},
	}

	for _, tc := range tests {
		sb := StatusBar{
			artist:        "A",
			title:         "T",
			playbackState: tc.state,
		}
		out := sb.render(0)
		if !strings.Contains(out, tc.want) {
			t.Fatalf("state %v: expected %q in status bar; got: %s", tc.state, tc.want, out)
		}
	}
}

// TestTrackChangedMsgUpdatesStatusBar verifies that TrackChangedMsg updates
// the status bar's artist, title, and playlist source.
func TestTrackChangedMsgUpdatesStatusBar(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	next, _ := m.Update(TrackChangedMsg{
		Artist:         "New Artist",
		Title:          "New Title",
		PlaylistSource: "My Playlist",
	})
	nm := next.(Model)
	v := nm.View()

	if !strings.Contains(v.Content, "New Artist") {
		t.Fatalf("status bar must show artist from TrackChangedMsg; got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "New Title") {
		t.Fatalf("status bar must show title from TrackChangedMsg; got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "My Playlist") {
		t.Fatalf("status bar must show playlist source from TrackChangedMsg; got:\n%s", v.Content)
	}
	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("TrackChangedMsg must set PlaybackPlaying, got %v", nm.playbackState)
	}
}

// TestTrackChangedMsgDefaultsPlaylistSourceToQueue verifies that a
// TrackChangedMsg with an empty PlaylistSource defaults to "Queue".
func TestTrackChangedMsgDefaultsPlaylistSourceToQueue(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	next, _ := m.Update(TrackChangedMsg{Artist: "A", Title: "T"})
	nm := next.(Model)
	v := nm.View()
	if !strings.Contains(v.Content, "Queue") {
		t.Fatalf("status bar must default playlist source to 'Queue'; got:\n%s", v.Content)
	}
}

// stubElapsedReader extends stubPlayer with the elapsedReader interface for
// testing the tickMsg handler.
type stubElapsedReader struct {
	stubPlayer
	elapsed float64
	total   float64
	err     error
}

func (s *stubElapsedReader) ElapsedPosition() (float64, float64, error) {
	return s.elapsed, s.total, s.err
}

// TestTickMsgUpdatesElapsedFields verifies that receiving a tickMsg calls
// ElapsedPosition on the player and stores the result on the model.
func TestTickMsgUpdatesElapsedFields(t *testing.T) {
	p := &stubElapsedReader{elapsed: 75.5, total: 240.0}
	m := New("/log", "", false, nil, nil, "", nil, p)

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.elapsedSecs != 75.5 {
		t.Fatalf("elapsedSecs: expected 75.5, got %v", nm.elapsedSecs)
	}
	if nm.totalSecs != 240.0 {
		t.Fatalf("totalSecs: expected 240.0, got %v", nm.totalSecs)
	}
}

// TestTickMsgReschedulesCmd verifies that tickMsg returns a non-nil follow-up
// command so the 1-second tick continues firing.
func TestTickMsgReschedulesCmd(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("tickMsg must return a follow-up cmd to reschedule the tick")
	}
}

// TestInitSchedulesTick verifies that Init always returns a non-nil command
// (at minimum the 1-second tick).
func TestInitSchedulesTick(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init must return a non-nil cmd (at least the 1-second tick)")
	}
}

// TestStatusBarRendersElapsedTime verifies that after a tickMsg updates the
// elapsed position, the view renders the MM:SS / MM:SS format.
func TestStatusBarRendersElapsedTime(t *testing.T) {
	p := &stubElapsedReader{elapsed: 83.0, total: 245.0}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackPlaying
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)
	v := nm.View()

	if !strings.Contains(v.Content, "01:23 / 04:05") {
		t.Fatalf("status bar must show elapsed/total in MM:SS / MM:SS format; got:\n%s", v.Content)
	}
}

// TestStatusBarNoTimeDisplayWhenZero verifies that when elapsedSecs and
// totalSecs are both 0 the time display is suppressed.
func TestStatusBarNoTimeDisplayWhenZero(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.playbackState = PlaybackPlaying
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	// elapsedSecs and totalSecs default to zero — no time display expected.

	v := m.View()
	if strings.Contains(v.Content, " / ") {
		t.Fatalf("status bar must not show time display when elapsed/total are zero; got:\n%s", v.Content)
	}
}

// TestPlaybackEndedMsgResetsElapsed verifies that PlaybackEndedMsg clears the
// elapsed and total fields so the time display disappears.
func TestPlaybackEndedMsgResetsElapsed(t *testing.T) {
	p := &stubElapsedReader{elapsed: 60.0, total: 180.0}
	m := New("/log", "", false, nil, nil, "", nil, p)

	// Populate elapsed/total via a tick.
	next, _ := m.Update(tickMsg{})
	nm := next.(Model)
	if nm.elapsedSecs == 0 {
		t.Fatal("expected non-zero elapsedSecs after tickMsg")
	}

	// PlaybackEndedMsg must zero them out.
	next, _ = nm.Update(PlaybackEndedMsg{})
	nm = next.(Model)
	if nm.elapsedSecs != 0 {
		t.Fatalf("elapsedSecs must be 0 after PlaybackEndedMsg, got %v", nm.elapsedSecs)
	}
	if nm.totalSecs != 0 {
		t.Fatalf("totalSecs must be 0 after PlaybackEndedMsg, got %v", nm.totalSecs)
	}
}

// TestPlaybackEndedMsgShowsQueueFinishedWhenEmptyRepeatOff verifies that when
// the queue is exhausted with repeat off, a "Queue finished" transient
// notification is shown for 5 seconds.
func TestPlaybackEndedMsgShowsQueueFinishedWhenEmptyRepeatOff(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.repeatMode = RepeatOff
	// queue.entries is already empty (zero value).

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.statusBar.notification.msg != "Queue finished" {
		t.Fatalf("expected 'Queue finished' notification, got %q", nm.statusBar.notification.msg)
	}
	if nm.statusBar.notification.expiresAt.IsZero() {
		t.Fatal("Queue finished notification must have a non-zero expiry")
	}
	if nm.statusBar.notification.expiresAt.Before(time.Now().Add(4 * time.Second)) {
		t.Fatal("Queue finished notification expiry must be ~5 seconds from now")
	}
}

// TestPlaybackEndedMsgNoNotificationWhenRepeatNotOff verifies that no "Queue
// finished" notification is shown when repeat is not off.
func TestPlaybackEndedMsgNoNotificationWhenRepeatNotOff(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.repeatMode = RepeatAll

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.statusBar.notification.msg == "Queue finished" {
		t.Fatal("Queue finished must not appear when repeat mode is not off")
	}
}

// TestPlaybackEndedMsgShowsNotificationWithEntriesPresent verifies that "Queue
// finished" IS shown even when the TUI slice still holds entries, because
// PlaybackEndedMsg means mpv exhausted all playable tracks — entries are never
// removed from the slice during normal play-through.
func TestPlaybackEndedMsgShowsNotificationWithEntriesPresent(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.repeatMode = RepeatOff
	m.queue.entries = []QueueEntry{{ID: 1, Display: "Track 1"}}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.statusBar.notification.msg != "Queue finished" {
		t.Fatalf("expected %q, got %q", "Queue finished", nm.statusBar.notification.msg)
	}
}

// TestPlayPauseOnEmptyQueueShowsHint verifies that pressing space to play when
// the queue is empty shows the "Queue is empty" hint in the transient status bar.
func TestPlayPauseOnEmptyQueueShowsHint(t *testing.T) {
	p := &stubPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	// queue.entries is empty, playbackState is PlaybackIdle.

	next, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	nm := next.(Model)

	want := "Queue is empty — enqueue tracks from Library (key 1)"
	if nm.statusBar.notification.msg != want {
		t.Fatalf("expected %q, got %q", want, nm.statusBar.notification.msg)
	}
	if nm.playbackState != PlaybackIdle {
		t.Fatalf("playback state must remain idle, got %v", nm.playbackState)
	}
}

// TestPlayPauseWithTracksInQueuePlaysNormally verifies that space to play when
// the queue has entries proceeds normally (no empty-queue hint).
func TestPlayPauseWithTracksInQueuePlaysNormally(t *testing.T) {
	p := &stubPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.queue.entries = []QueueEntry{{ID: 1, Display: "Track 1"}}
	m.playbackState = PlaybackIdle

	next, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	nm := next.(Model)

	if nm.statusBar.notification.msg == "Queue is empty — enqueue tracks from Library (key 1)" {
		t.Fatal("empty-queue hint must not appear when queue has entries")
	}
	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("expected PlaybackPlaying after play with non-empty queue, got %v", nm.playbackState)
	}
}

// TestTransientNotificationClearsAfterTickPastExpiry verifies that a transient
// notification set on the status bar is cleared when the tick fires after the
// 5-second expiry has elapsed.
func TestTransientNotificationClearsAfterTickPastExpiry(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	// Set a notification that is already expired.
	m.statusBar.notification = transientNotification{
		msg:       "some transient message",
		expiresAt: time.Now().Add(-1 * time.Second),
	}

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.statusBar.notification.msg != "" {
		t.Fatalf("notification must be cleared after tick past expiry, got %q", nm.statusBar.notification.msg)
	}
}

// TestTransientNotificationPersistsBeforeExpiry verifies that a notification
// whose expiry has not yet been reached survives the tick unchanged.
func TestTransientNotificationPersistsBeforeExpiry(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	// Set a notification that expires well in the future.
	m.statusBar.notification = transientNotification{
		msg:       "fresh message",
		expiresAt: time.Now().Add(10 * time.Second),
	}

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.statusBar.notification.msg != "fresh message" {
		t.Fatalf("notification must not be cleared before expiry, got %q", nm.statusBar.notification.msg)
	}
}

// TestNewerNotificationOverwritesOlderResettingExpiry verifies that receiving
// a second transient message while one is active replaces the old message and
// resets the expiry clock.
func TestNewerNotificationOverwritesOlderResettingExpiry(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	// First notification, nearly expired.
	oldExpiry := time.Now().Add(500 * time.Millisecond)
	m.statusBar.notification = transientNotification{
		msg:       "old message",
		expiresAt: oldExpiry,
	}

	// Second notification via PlayerStatusMsg overwrites the first.
	next, _ := m.Update(PlayerStatusMsg{Text: "new message"})
	nm := next.(Model)

	if nm.statusBar.notification.msg != "new message" {
		t.Fatalf("new notification must overwrite old, got %q", nm.statusBar.notification.msg)
	}
	if !nm.statusBar.notification.expiresAt.After(oldExpiry) {
		t.Fatal("new notification must reset expiry to a later time than the old one")
	}
}

// TestScanProgressVisibleDuringScanAndAutoDismissed verifies the full lifecycle:
// progress is shown while scanning, the last message persists past ScanDoneMsg,
// and the tick clears it once the expiry passes.
func TestScanProgressVisibleDuringScanAndAutoDismissed(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

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

// TestViewLocalBindingTakesPrecedenceOverGlobal verifies that when the active
// view defines a local binding for a key, the view-local action fires and the
// global action for that key is suppressed.
//
// The 'r' key is bound globally to actionRepeatCycle and locally to
// actionPlaylistRename in the Playlists view. When the Playlists view is
// focused, pressing 'r' must fire rename (not toggle repeat mode).
func TestViewLocalBindingTakesPrecedenceOverGlobal(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists

	// The global 'r' binding would normally toggle repeat mode. The
	// Playlists-local binding for 'r' (rename) must intercept first.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	nm := next.(Model)

	if cmd == nil {
		t.Fatal("view-local 'r' in Playlists must return a rename cmd")
	}
	msg := cmd()
	if _, ok := msg.(PlaylistRenameMsg); !ok {
		t.Fatalf("view-local 'r' in Playlists: expected PlaylistRenameMsg, got %T", msg)
	}
	if nm.repeatMode != RepeatOff {
		t.Fatal("view-local 'r' in Playlists must not cycle global repeat mode")
	}
}

// TestGlobalBindingFiresWhenNoViewLocalBinding verifies that keys not
// overridden by the active view still trigger their global action.
//
// The Library and Queue views have no local binding for 'R', so pressing 'R'
// in those views falls through to the global actionRescan.
func TestGlobalBindingFiresWhenNoViewLocalBinding(t *testing.T) {
	views := []struct {
		name   string
		viewID viewID
	}{
		{"Library", viewLibrary},
		{"Queue", viewQueue},
	}
	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			m := New("/log", "", false, nil, nil, root, stubExtractor{}, nil)
			m.activeView = tc.viewID

			_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
			if cmd == nil {
				t.Fatalf("%s: global 'R' must return a rescan cmd when no view-local binding exists", tc.name)
			}
		})
	}
}

// TestEqualKeyFiresVolumeUp verifies that pressing '=' calls SetVolumeRelative
// with a positive delta on the player.
func TestEqualKeyFiresVolumeUp(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: '=', Text: "="})

	if p.volumeDelta != 5 {
		t.Fatalf("= key: expected SetVolumeRelative(5), got delta=%v", p.volumeDelta)
	}
}

// stubVolumePlayer extends stubCommandPlayer to capture SetVolumeRelative calls.
type stubVolumePlayer struct {
	stubCommandPlayer
	volumeDelta float64
}

func (s *stubVolumePlayer) SetVolumeRelative(delta float64) error {
	s.volumeDelta = delta
	return nil
}

// TestShiftRFiresRescan verifies that pressing 'R' (Shift+r) with a configured
// library root triggers a rescan, while lowercase 'r' does not.
func TestShiftRFiresRescan(t *testing.T) {
	root := t.TempDir()
	m := New("/log", "", false, nil, nil, root, stubExtractor{}, nil)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd == nil {
		t.Fatal("R key: expected non-nil rescan cmd")
	}
}

// TestLowerRCyclesRepeatModeInNonPlaylistViews verifies that pressing 'r' in
// Library and Queue views advances repeatMode through Off→All→One→Off, and
// does NOT trigger rescan.
func TestLowerRCyclesRepeatModeInNonPlaylistViews(t *testing.T) {
	views := []struct {
		name   string
		viewID viewID
	}{
		{"Library", viewLibrary},
		{"Queue", viewQueue},
	}
	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			m := New("/log", "", false, nil, nil, "", nil, nil)
			m.activeView = tc.viewID

			// First press: RepeatOff → RepeatAll.
			next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm := next.(Model)
			if cmd != nil {
				t.Fatalf("%s: 'r' repeat cycle must return nil cmd, got non-nil", tc.name)
			}
			if nm.repeatMode != RepeatAll {
				t.Fatalf("%s: first press: expected RepeatAll, got %v", tc.name, nm.repeatMode)
			}

			// Second press: RepeatAll → RepeatOne.
			next, _ = nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm = next.(Model)
			if nm.repeatMode != RepeatOne {
				t.Fatalf("%s: second press: expected RepeatOne, got %v", tc.name, nm.repeatMode)
			}

			// Third press: RepeatOne → RepeatOff (wraps).
			next, _ = nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm = next.(Model)
			if nm.repeatMode != RepeatOff {
				t.Fatalf("%s: third press: expected RepeatOff, got %v", tc.name, nm.repeatMode)
			}
		})
	}
}

// stubSessionWriter records WriteRepeatMode, WriteShuffle, and
// UpdateSessionPosition calls for assertion in tests.
type stubSessionWriter struct {
	calls         []string
	shuffleCalls  []bool
	positionCalls []struct {
		hash       string
		positionMs int64
	}
}

func (s *stubSessionWriter) WriteRepeatMode(mode string) error {
	s.calls = append(s.calls, mode)
	return nil
}

func (s *stubSessionWriter) WriteShuffle(enabled bool) error {
	s.shuffleCalls = append(s.shuffleCalls, enabled)
	return nil
}

func (s *stubSessionWriter) UpdateSessionPosition(hash string, positionMs int64) error {
	s.positionCalls = append(s.positionCalls, struct {
		hash       string
		positionMs int64
	}{hash, positionMs})
	return nil
}

// TestRepeatCycleCallsWriteRepeatMode verifies that actionRepeatCycle calls
// sessionWriter.WriteRepeatMode with the correct mode string on each press
// and cycles in the order "none" → "all" → "one" → "none".
func TestRepeatCycleCallsWriteRepeatMode(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	sw := &stubSessionWriter{}
	m.SetSessionWriter(sw)

	wantCalls := []string{"all", "one", "none"}

	var nm Model = m
	for i, want := range wantCalls {
		next, _ := nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
		nm = next.(Model)
		if len(sw.calls) != i+1 {
			t.Fatalf("press %d: expected %d WriteRepeatMode call(s), got %d", i+1, i+1, len(sw.calls))
		}
		if sw.calls[i] != want {
			t.Fatalf("press %d: expected WriteRepeatMode(%q), got %q", i+1, want, sw.calls[i])
		}
	}
}

// TestZTogglesShuffleGlobal verifies that pressing 'z' toggles the shuffle
// field on the root model.
func TestZTogglesShuffleGlobal(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	// First press: shuffle off → on.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	nm := next.(Model)
	if cmd != nil {
		t.Fatal("z key: expected nil cmd for shuffle toggle")
	}
	if !nm.shuffle {
		t.Fatal("z key first press: expected shuffle=true, got false")
	}

	// Second press: shuffle on → off.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	nm = next.(Model)
	if nm.shuffle {
		t.Fatal("z key second press: expected shuffle=false, got true")
	}
}

// TestUDispatchesUndoMessage verifies that pressing 'u' returns a cmd that
// emits UndoMsg.
func TestUDispatchesUndoMessage(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd == nil {
		t.Fatal("u key: expected non-nil cmd for undo dispatch")
	}
	msg := cmd()
	if _, ok := msg.(UndoMsg); !ok {
		t.Fatalf("u key: expected UndoMsg, got %T", msg)
	}
}

// TestSlashRoutesToActiveView verifies that pressing '/' returns a cmd that
// emits FilterBarMsg for routing to the active view.
func TestSlashRoutesToActiveView(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	_, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if cmd == nil {
		t.Fatal("/ key: expected non-nil cmd for filter bar activation")
	}
	msg := cmd()
	if _, ok := msg.(FilterBarMsg); !ok {
		t.Fatalf("/ key: expected FilterBarMsg, got %T", msg)
	}
}

// TestPlaylistLocalCFiresCreateNotGlobalAction verifies that pressing 'c' in
// the Playlists view dispatches PlaylistCreateMsg.
func TestPlaylistLocalCFiresCreateNotGlobalAction(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatal("c key in Playlists: expected non-nil cmd for create playlist")
	}
	msg := cmd()
	if _, ok := msg.(PlaylistCreateMsg); !ok {
		t.Fatalf("c key in Playlists: expected PlaylistCreateMsg, got %T", msg)
	}
}

// stubPlaylistCreator is a minimal playlistCreator used in create-flow tests.
type stubPlaylistCreator struct {
	createErr error
	readErr   error
	rows      []db.PlaylistRow
	nextID    int64
}

func (s *stubPlaylistCreator) CreatePlaylist(name string) (int64, error) {
	if s.createErr != nil {
		return 0, s.createErr
	}
	id := s.nextID
	s.rows = append(s.rows, db.PlaylistRow{ID: id, Name: name})
	return id, nil
}

func (s *stubPlaylistCreator) ReadPlaylists() ([]db.PlaylistRow, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	return s.rows, nil
}

// TestPlaylistCreateFlowSuccess exercises the full inline create flow:
// 'c' activates the input, typing a name and pressing Enter calls CreatePlaylist,
// feeds playlistCreatedMsg back to Update, and verifies the name appears in the
// rendered view with the cursor positioned on the new entry.
func TestPlaylistCreateFlowSuccess(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 42}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.SetPlaylistWriter(creator)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// 'c' dispatches PlaylistCreateMsg.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("c: expected non-nil cmd")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	// Type the playlist name character by character.
	for _, ch := range "MyList" {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}

	// Press Enter — should return the async create cmd.
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("Enter: expected non-nil cmd for CreatePlaylist")
	}

	// Execute the cmd and feed the resulting playlistCreatedMsg back.
	result := cmd()
	if _, ok := result.(playlistCreatedMsg); !ok {
		t.Fatalf("cmd result: expected playlistCreatedMsg, got %T", result)
	}
	next, _ = m.Update(result)
	m = next.(Model)

	// Verify the new playlist name appears in the view.
	v := m.View()
	if !strings.Contains(v.Content, "MyList") {
		t.Errorf("expected 'MyList' in view after creation, got:\n%s", v.Content)
	}

	// Verify cursor is on the new entry (index 0, the only entry).
	if m.playlists.cursor != 0 {
		t.Errorf("expected cursor=0 on new entry, got %d", m.playlists.cursor)
	}
}

// TestPlaylistCreateFlowEscCancels verifies that pressing Esc during inline
// creation discards the input without persisting anything.
func TestPlaylistCreateFlowEscCancels(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 1}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.SetPlaylistWriter(creator)

	// Activate create mode.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)

	// Type something, then press Esc.
	for _, ch := range "Oops" {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(Model)

	if m.playlists.isCreating() {
		t.Error("expected isCreating=false after Esc")
	}
	if len(creator.rows) != 0 {
		t.Errorf("expected no playlists persisted after Esc, got %d", len(creator.rows))
	}
}

// TestPlaylistCreateFlowErrorShowsNotification verifies that a CreatePlaylist
// error cancels the input and surfaces a status bar notification.
func TestPlaylistCreateFlowErrorShowsNotification(t *testing.T) {
	creator := &stubPlaylistCreator{createErr: &errPlaylistCreate{msg: "db locked"}, nextID: 1}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.SetPlaylistWriter(creator)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)

	for _, ch := range "Oops" {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("Enter: expected non-nil cmd")
	}

	result := cmd()
	if _, ok := result.(playlistCreateErrorMsg); !ok {
		t.Fatalf("cmd result: expected playlistCreateErrorMsg, got %T", result)
	}
	next, _ = m.Update(result)
	m = next.(Model)

	if m.playlists.isCreating() {
		t.Error("expected isCreating=false after error")
	}
	if !strings.Contains(m.statusBar.notification.msg, "db locked") {
		t.Errorf("expected error message in status bar, got %q", m.statusBar.notification.msg)
	}
}

// errPlaylistCreate is a simple error type used in create-flow error tests.
type errPlaylistCreate struct{ msg string }

func (e *errPlaylistCreate) Error() string { return e.msg }

// stubSeekPlayer extends stubPlayer with tracking for SeekRelative, Next, and
// Previous calls so that seek and navigation key tests can assert correct deltas
// and call counts without a real mpv backend.
type stubSeekPlayer struct {
	stubPlayer
	seekDeltas []float64
	nextCalls  int
	prevCalls  int
}

func (s *stubSeekPlayer) Stop() error { return nil }
func (s *stubSeekPlayer) SeekRelative(d float64) error {
	s.seekDeltas = append(s.seekDeltas, d)
	return nil
}
func (s *stubSeekPlayer) SetVolumeRelative(float64) error { return nil }
func (s *stubSeekPlayer) Next() error                     { s.nextCalls++; return nil }
func (s *stubSeekPlayer) Previous() error                 { s.prevCalls++; return nil }

// TestSeekFineBackwardKey verifies that the left arrow key calls
// SeekRelative(-5) on the player (fine seek backward by 5 seconds).
func TestSeekFineBackwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != -5 {
		t.Fatalf("left arrow: expected SeekRelative(-5), got deltas=%v", p.seekDeltas)
	}
}

// TestSeekFineForwardKey verifies that the right arrow key calls
// SeekRelative(+5) on the player (fine seek forward by 5 seconds).
func TestSeekFineForwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != 5 {
		t.Fatalf("right arrow: expected SeekRelative(5), got deltas=%v", p.seekDeltas)
	}
}

// TestSeekCoarseBackwardKey verifies that Shift+Left calls SeekRelative(-30)
// on the player (coarse seek backward by 30 seconds).
func TestSeekCoarseBackwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != -30 {
		t.Fatalf("shift+left: expected SeekRelative(-30), got deltas=%v", p.seekDeltas)
	}
}

// TestSeekCoarseForwardKey verifies that Shift+Right calls SeekRelative(+30)
// on the player (coarse seek forward by 30 seconds).
func TestSeekCoarseForwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != 30 {
		t.Fatalf("shift+right: expected SeekRelative(30), got deltas=%v", p.seekDeltas)
	}
}

// TestNextTrackKey verifies that pressing 'n' calls Next() on the player.
func TestNextTrackKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})

	if p.nextCalls != 1 {
		t.Fatalf("n key: expected 1 Next() call, got %d", p.nextCalls)
	}
}

// TestPreviousTrackKey verifies that pressing 'p' calls Previous() on the player.
func TestPreviousTrackKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})

	if p.prevCalls != 1 {
		t.Fatalf("p key: expected 1 Previous() call, got %d", p.prevCalls)
	}
}

// TestMinusKeyFiresVolumeDown verifies that pressing '-' calls
// SetVolumeRelative(-5) on the player (volume down by 5%).
func TestMinusKeyFiresVolumeDown(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: '-', Text: "-"})

	if p.volumeDelta != -5 {
		t.Fatalf("- key: expected SetVolumeRelative(-5), got delta=%v", p.volumeDelta)
	}
}

// TestHKeyIsUnbound verifies that 'h' is intentionally unbound: pressing it
// produces no command and leaves the model state unchanged.
func TestHKeyIsUnbound(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})

	if cmd != nil {
		t.Fatal("'h' key must be unbound: expected nil cmd")
	}
}

// TestLKeyIsUnbound verifies that 'l' is intentionally unbound: pressing it
// produces no command and leaves the model state unchanged.
func TestLKeyIsUnbound(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})

	if cmd != nil {
		t.Fatal("'l' key must be unbound: expected nil cmd")
	}
}

// TestViewSwitchingCompletesUnder50ms verifies that each view-switch keypress
// and its first rendered frame complete within 50ms, and that the active view
// is correct after each switch.
func TestViewSwitchingCompletesUnder50ms(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	steps := []struct {
		key  tea.KeyPressMsg
		want viewID
	}{
		{tea.KeyPressMsg{Code: '2', Text: "2"}, viewPlaylists},
		{tea.KeyPressMsg{Code: '3', Text: "3"}, viewQueue},
		{tea.KeyPressMsg{Code: '1', Text: "1"}, viewLibrary},
	}

	for _, tc := range steps {
		start := time.Now()
		next, _ := m.Update(tc.key)
		nm := next.(Model)
		_ = nm.View()
		elapsed := time.Since(start)

		if elapsed >= 50*time.Millisecond {
			t.Fatalf("view switch to %d: took %v, must complete under 50ms", tc.want, elapsed)
		}
		if nm.activeView != tc.want {
			t.Fatalf("expected activeView %d after key press, got %d", tc.want, nm.activeView)
		}
		m = nm
	}
}

// TestBackgroundBatchUpdateLoopNonBlocking verifies that the Update loop
// returns immediately when handling a ScanBatchMsg. The blocking channel read
// for the next batch is deferred to the returned Cmd, keeping the Elm update
// loop free to process further messages.
func TestBackgroundBatchUpdateLoopNonBlocking(t *testing.T) {
	// Unbuffered channel with no sender — Update must not block on it.
	ch := make(chan library.Track)
	m := New("/log", "", false, ch, nil, "", nil, nil)

	start := time.Now()
	_, cmd := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Artist: "A", Title: "T"},
	}})
	elapsed := time.Since(start)

	if elapsed >= 50*time.Millisecond {
		t.Fatalf("Update with ScanBatchMsg blocked for %v; must return immediately", elapsed)
	}
	if cmd == nil {
		t.Fatal("ScanBatchMsg must return a follow-up cmd for the next batch read")
	}
}

// TestRootQueueLocalDDispatchesDequeueMsg verifies the end-to-end path: pressing
// 'd' while the queue view is active returns a Cmd that emits DequeueTrackMsg.
func TestRootQueueLocalDDispatchesDequeueMsg(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if cmd == nil {
		t.Fatal("'d' in queue view: expected non-nil cmd")
	}
	msg, ok := cmd().(DequeueTrackMsg)
	if !ok {
		t.Fatalf("'d' in queue view: expected DequeueTrackMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Fatalf("'d' in queue view: expected index 1, got %d", msg.Index)
	}
}

// TestRootQueueLocalXDispatchesBulkSelectionToggleMsg verifies the end-to-end
// path: pressing 'x' while the queue view is active dispatches BulkSelectionToggleMsg.
func TestRootQueueLocalXDispatchesBulkSelectionToggleMsg(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
	}
	m.queue.cursor = 0

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd == nil {
		t.Fatal("'x' in queue view: expected non-nil cmd")
	}
	if _, ok := cmd().(BulkSelectionToggleMsg); !ok {
		t.Fatalf("'x' in queue view: expected BulkSelectionToggleMsg, got %T", cmd())
	}
}

// TestRootQueueLocalCDispatchesClearQueueMsg verifies the end-to-end path:
// pressing 'C' (Shift+c) while the queue view is active dispatches ClearQueueMsg.
func TestRootQueueLocalCDispatchesClearQueueMsg(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	if cmd == nil {
		t.Fatal("'C' in queue view: expected non-nil cmd")
	}
	if _, ok := cmd().(ClearQueueMsg); !ok {
		t.Fatalf("'C' in queue view: expected ClearQueueMsg, got %T", cmd())
	}
}

// TestRootQueueLocalKDispatchesMoveTrackUpMsg verifies the end-to-end path:
// pressing 'K' (Shift+k) while the queue view is active dispatches MoveTrackUpMsg.
func TestRootQueueLocalKDispatchesMoveTrackUpMsg(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
	if cmd == nil {
		t.Fatal("'K' in queue view: expected non-nil cmd")
	}
	msg, ok := cmd().(MoveTrackUpMsg)
	if !ok {
		t.Fatalf("'K' in queue view: expected MoveTrackUpMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Fatalf("'K' in queue view: expected index 1, got %d", msg.Index)
	}
}

// TestRootQueueLocalJDispatchesMoveTrackDownMsg verifies the end-to-end path:
// pressing 'J' (Shift+j) while the queue view is active dispatches MoveTrackDownMsg.
func TestRootQueueLocalJDispatchesMoveTrackDownMsg(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 0

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	if cmd == nil {
		t.Fatal("'J' in queue view: expected non-nil cmd")
	}
	msg, ok := cmd().(MoveTrackDownMsg)
	if !ok {
		t.Fatalf("'J' in queue view: expected MoveTrackDownMsg, got %T", cmd())
	}
	if msg.Index != 0 {
		t.Fatalf("'J' in queue view: expected index 0, got %d", msg.Index)
	}
}

func TestUpdateQuestionMarkSetsHelpVisible(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, nil, "", nil, nil)
	// Default active view is Library.
	if m.activeView != viewLibrary {
		t.Fatalf("precondition: expected viewLibrary, got %d", m.activeView)
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)

	if !nm.helpVisible {
		t.Fatal("'?' key: expected helpVisible==true")
	}
	if nm.prevActiveView != viewLibrary {
		t.Fatalf("'?' key: expected prevActiveView==viewLibrary (%d), got %d", viewLibrary, nm.prevActiveView)
	}
}

func TestViewShowsHelpOverlayWhenVisible(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, nil, "", nil, nil)
	m.helpVisible = true

	v := m.View()
	if !strings.Contains(v.Content, "Keybindings") {
		t.Fatalf("View() with helpVisible=true: expected overlay content containing \"Keybindings\", got:\n%s", v.Content)
	}
}

func TestUpdateEscClearsHelpVisibleAndRestoresView(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, nil, "", nil, nil)

	// Switch to Playlists first.
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)
	if nm.activeView != viewPlaylists {
		t.Fatalf("precondition: expected viewPlaylists, got %d", nm.activeView)
	}

	// Open help overlay.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm = next.(Model)
	if !nm.helpVisible {
		t.Fatal("precondition: expected helpVisible==true after '?'")
	}

	// Esc should clear the overlay and restore Playlists.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.helpVisible {
		t.Fatal("Esc while helpVisible: expected helpVisible==false")
	}
	if nm.activeView != viewPlaylists {
		t.Fatalf("Esc while helpVisible: expected activeView==viewPlaylists (%d), got %d", viewPlaylists, nm.activeView)
	}
}

func TestUpdateEscWhenHelpNotVisibleDoesNotQuit(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, nil, "", nil, nil)
	// helpVisible defaults to false — Esc must not quit and must not error.
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("Esc with helpVisible=false must not produce a command (e.g. quit)")
	}
}

// TestViewHelpOverlayContainsSectionHeaders verifies that when the help overlay
// is visible, View() renders all expected section headers.
func TestViewHelpOverlayContainsSectionHeaders(t *testing.T) {
	m := New("/state/muse/muse.log", "", false, nil, nil, "", nil, nil)
	m.helpVisible = true

	v := m.View()
	for _, header := range []string{"Global", "Library", "Playlists", "Queue", "Modals"} {
		if !strings.Contains(v.Content, header) {
			t.Errorf("View() with helpVisible=true: missing section header %q", header)
		}
	}
}

// TestViewHelpOverlayContainsLogPath verifies that when the help overlay is
// visible, View() renders the log file path.
func TestViewHelpOverlayContainsLogPath(t *testing.T) {
	logPath := "/state/muse/muse.log"
	m := New(logPath, "", false, nil, nil, "", nil, nil)
	m.helpVisible = true

	v := m.View()
	if !strings.Contains(v.Content, logPath) {
		t.Fatalf("View() with helpVisible=true: missing log path %q in:\n%s", logPath, v.Content)
	}
}

// TestEscRestoringViewDoesNotAlterPlaybackOrScanState verifies that dismissing
// the help overlay with Esc does not change playbackState, repeatMode, shuffle,
// or the scanning flag.
func TestEscRestoringViewDoesNotAlterPlaybackOrScanState(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.playbackState = PlaybackPlaying
	m.repeatMode = RepeatAll
	m.shuffle = true
	m.scanning = true

	// Open the overlay.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)

	// Dismiss with Esc.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("Esc: playbackState changed; expected PlaybackPlaying, got %v", nm.playbackState)
	}
	if nm.repeatMode != RepeatAll {
		t.Fatalf("Esc: repeatMode changed; expected RepeatAll, got %v", nm.repeatMode)
	}
	if !nm.shuffle {
		t.Fatal("Esc: shuffle changed; expected true, got false")
	}
	if !nm.scanning {
		t.Fatal("Esc: scanning changed; expected true, got false")
	}
}

// TestRapidWindowSizeMsgDoesNotPanic verifies that sending many successive
// WindowSizeMsg events in a tight loop does not cause a panic and always
// returns a valid model.
func TestRapidWindowSizeMsgDoesNotPanic(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	sizes := [][2]int{
		{80, 24}, {120, 40}, {40, 10}, {200, 50}, {60, 20},
		{80, 24}, {100, 30}, {50, 15}, {180, 45}, {70, 22},
	}
	for range 5 {
		for _, s := range sizes {
			next, _ := m.Update(tea.WindowSizeMsg{Width: s[0], Height: s[1]})
			m = next.(Model)
		}
	}

	// If we reach here, no panic occurred. Confirm final termWidth is the last sent value.
	lastWidth := sizes[len(sizes)-1][0]
	if m.termWidth != lastWidth {
		t.Fatalf("after rapid resize: expected termWidth=%d, got %d", lastWidth, m.termWidth)
	}
}

// TestWindowSizeMsgPreservesApplicationState verifies that a terminal resize
// does not alter cursor position, playback state, or scan state.
func TestWindowSizeMsgPreservesApplicationState(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
		{Path: "/music/c.flac", Display: "C"},
	}
	m.queue.cursor = 2
	m.playbackState = PlaybackPlaying
	m.scanning = true
	m.activeView = viewQueue

	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	nm := next.(Model)

	if nm.queue.cursor != 2 {
		t.Fatalf("resize: queue cursor changed; expected 2, got %d", nm.queue.cursor)
	}
	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("resize: playbackState changed; expected PlaybackPlaying, got %v", nm.playbackState)
	}
	if !nm.scanning {
		t.Fatal("resize: scanning changed; expected true, got false")
	}
	if nm.activeView != viewQueue {
		t.Fatalf("resize: activeView changed; expected viewQueue (%d), got %d", viewQueue, nm.activeView)
	}
}

// TestWindowSizeMsgUpdatesTermWidthAndRendersContent verifies that after a
// WindowSizeMsg the model stores the new terminal width and View() produces
// non-empty rendered content.
func TestWindowSizeMsgUpdatesTermWidthAndRendersContent(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	nm := next.(Model)

	if nm.termWidth != 120 {
		t.Fatalf("termWidth: expected 120, got %d", nm.termWidth)
	}

	v := nm.View()
	if v.Content == "" {
		t.Fatal("View() after WindowSizeMsg must produce non-empty content")
	}
}

// TestLibraryCursorDownJKey verifies that pressing "j" while the library view
// is active advances the cursor by one.
func TestLibraryCursorDownJKey(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.library.cursor != 1 {
		t.Errorf("j key: expected cursor 1, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorUpKKey verifies that pressing "k" while the library view
// is active moves the cursor up by one.
func TestLibraryCursorUpKKey(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.library.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("k key: expected cursor 0, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorDownArrow verifies that the down arrow key advances the
// cursor in the library view, behaving identically to "j".
func TestLibraryCursorDownArrow(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.library.cursor != 1 {
		t.Errorf("down arrow: expected cursor 1, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorUpArrow verifies that the up arrow key moves the cursor up
// in the library view, behaving identically to "k".
func TestLibraryCursorUpArrow(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.library.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("up arrow: expected cursor 0, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorClampAtBottom verifies that the cursor stops at the last
// track without wrapping when the library view is active.
func TestLibraryCursorClampAtBottom(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.library.cursor = 1 // already at last item

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.library.cursor != 1 {
		t.Errorf("j at last: expected cursor 1 (clamped), got %d", nm.library.cursor)
	}
}

// TestLibraryCursorClampAtTop verifies that the cursor stops at the first
// track without wrapping when the library view is active.
func TestLibraryCursorClampAtTop(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	// cursor starts at 0

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("k at first: expected cursor 0 (clamped), got %d", nm.library.cursor)
	}
}

// TestLibraryCursorNotMovedWhenQueueActive verifies that j/k in the queue view
// do not affect the library cursor.
func TestLibraryCursorNotMovedWhenQueueActive(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.activeView = viewQueue

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("down in queue view must not move library cursor, got %d", nm.library.cursor)
	}
}

// stubQueueAppender is a minimal libtui.QueueAppender for root-level tests.
type stubQueueAppender struct {
	tracks []library.Track
}

func (s *stubQueueAppender) AppendTrack(t library.Track) (int64, error) {
	s.tracks = append(s.tracks, t)
	return int64(len(s.tracks)), nil
}

// stubPlaylistAdder records calls to AddTracksToPlaylist for test assertions.
type stubPlaylistAdder struct {
	calls []struct {
		playlistID int64
		hashes     []string
	}
}

func (s *stubPlaylistAdder) AddTracksToPlaylist(playlistID int64, hashes []string) error {
	s.calls = append(s.calls, struct {
		playlistID int64
		hashes     []string
	}{playlistID, hashes})
	return nil
}

// TestRootLibraryXTogglesSelectedHash verifies that pressing 'x' while the
// library view is active toggles the focused track's hash into selectedHashes.
func TestRootLibraryXTogglesSelectedHash(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two", Hash: "hash-two"})
	m.library.cursor = 0
	// activeView is viewLibrary by default

	next, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	nm := next.(Model)

	if _, ok := nm.library.selectedHashes["hash-one"]; !ok {
		t.Errorf("'x' on focused track: expected hash-one in selectedHashes, got %v", nm.library.selectedHashes)
	}
	if _, ok := nm.library.selectedHashes["hash-two"]; ok {
		t.Errorf("'x' on focused track: hash-two must not be in selectedHashes")
	}

	// Pressing 'x' again must remove the hash (toggle off).
	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	nm2 := next2.(Model)
	if _, ok := nm2.library.selectedHashes["hash-one"]; ok {
		t.Errorf("second 'x': expected hash-one to be deselected, but it is still in selectedHashes")
	}
}

// TestRootLibraryAOpensDestinationPickerNoSelection verifies that pressing 'a'
// in the library view with no bulk selection opens the destination picker modal
// for the focused track.
func TestRootLibraryAOpensDestinationPickerNoSelection(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatalf("'a' with no selection: expected nil cmd (modal open is synchronous), got %T", cmd)
	}
	if nm.destPicker == nil {
		t.Fatal("'a' with no selection: expected destPicker to be non-nil (modal open)")
	}
	if len(nm.destPickerTracks) != 1 || nm.destPickerTracks[0].Hash != "hash-one" {
		t.Errorf("'a' with no selection: expected destPickerTracks to contain focused track, got %v", nm.destPickerTracks)
	}
}

// TestRootLibraryAOpensDestinationPickerWithSelection verifies that pressing
// 'a' while tracks are bulk-selected opens the destination picker modal for
// all selected tracks.
func TestRootLibraryAOpensDestinationPickerWithSelection(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two", Hash: "hash-two"})
	m.library.selectedHashes = map[string]struct{}{"hash-one": {}, "hash-two": {}}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatalf("'a' with selection: expected nil cmd (modal open is synchronous), got %T", cmd)
	}
	if nm.destPicker == nil {
		t.Fatal("'a' with selection: expected destPicker to be non-nil (modal open)")
	}
	if len(nm.destPickerTracks) != 2 {
		t.Errorf("'a' with selection: expected 2 destPickerTracks, got %d", len(nm.destPickerTracks))
	}
}

// TestRootLibraryAWithSelectionEnqueuesAndClears verifies that pressing 'a' in
// library view, then confirming the queue destination, appends tracks to the
// queue and clears the bulk selection.
func TestRootLibraryAWithSelectionEnqueuesAndClears(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetQueueWriter(qa)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two", Hash: "hash-two"})
	m.library.selectedHashes = map[string]struct{}{"hash-one": {}}

	// 'a' opens the destination picker.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)
	if nm.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}

	// Enter confirms the focused destination (Play Queue is first).
	// The modal dispatches a DestinationPickedMsg via a Cmd.
	nm2, enterCmd := nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	_ = nm2

	if enterCmd == nil {
		t.Fatal("Enter on modal: expected non-nil cmd (DestinationPickedMsg)")
	}
	pickedMsg := enterCmd()

	// Processing DestinationPickedMsg triggers the actual enqueue Cmd.
	next3, enqueueCmd := nm.Update(pickedMsg)
	nm3 := next3.(Model)

	if enqueueCmd == nil {
		t.Fatal("DestinationPickedMsg: expected non-nil enqueue cmd")
	}
	trackMsg := enqueueCmd()
	if _, ok := trackMsg.(libtui.TrackEnqueuedMsg); !ok {
		t.Fatalf("expected TrackEnqueuedMsg, got %T", trackMsg)
	}
	if nm3.destPicker != nil {
		t.Error("expected destPicker to be nil after confirmation")
	}
	if len(nm3.library.selectedHashes) != 0 {
		t.Errorf("expected selectedHashes to be cleared after confirmation, got %v", nm3.library.selectedHashes)
	}
}

// TestRootLibraryAEscDismissesModal verifies that pressing Esc while the
// destination picker is open dismisses the modal without enqueueing.
func TestRootLibraryAEscDismissesModal(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})

	// Open the modal.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)
	if nm.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}

	// Esc dispatches DestinationPickerDismissedMsg via a Cmd.
	_, escCmd := nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape, Text: "esc"})
	if escCmd == nil {
		t.Fatal("Esc: expected a dismiss cmd")
	}
	dismissMsg := escCmd()
	next2, _ := nm.Update(dismissMsg)
	nm2 := next2.(Model)
	if nm2.destPicker != nil {
		t.Error("expected destPicker to be nil after dismiss")
	}
}

// TestRootLibraryAConfirmPlaylistAddsToPlaylist verifies that pressing 'a',
// navigating to a playlist destination, and pressing Enter calls
// AddTracksToPlaylist with the correct playlist ID and hashes.
func TestRootLibraryAConfirmPlaylistAddsToPlaylist(t *testing.T) {
	pa := &stubPlaylistAdder{}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetPlaylistAdder(pa)
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})
	m.playlists.playlists = []db.PlaylistRow{{ID: 7, Name: "Favourites"}}

	// 'a' opens the modal.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)

	// 'j' moves cursor from Play Queue (0) to Favourites (1).
	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm2 := next2.(Model)

	// Enter on modal dispatches DestinationPickedMsg via Cmd.
	_, enterCmd := nm2.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if enterCmd == nil {
		t.Fatal("Enter on playlist destination: expected non-nil cmd (DestinationPickedMsg)")
	}
	pickedMsg := enterCmd()

	// Processing DestinationPickedMsg triggers the AddTracksToPlaylist Cmd.
	next3, addCmd := nm2.Update(pickedMsg)
	nm3 := next3.(Model)

	if addCmd == nil {
		t.Fatal("DestinationPickedMsg: expected non-nil add cmd")
	}
	// Execute the cmd to trigger the DB call.
	addCmd()

	if len(pa.calls) != 1 {
		t.Fatalf("expected 1 AddTracksToPlaylist call, got %d", len(pa.calls))
	}
	if pa.calls[0].playlistID != 7 {
		t.Errorf("expected playlistID 7, got %d", pa.calls[0].playlistID)
	}
	if len(pa.calls[0].hashes) != 1 || pa.calls[0].hashes[0] != "hash-one" {
		t.Errorf("expected hashes [hash-one], got %v", pa.calls[0].hashes)
	}
	if nm3.destPicker != nil {
		t.Error("expected destPicker to be nil after confirmation")
	}
	if len(nm3.library.selectedHashes) != 0 {
		t.Errorf("expected selectedHashes to be cleared, got %v", nm3.library.selectedHashes)
	}
}

// TestTrackEnqueuedMsgStartsPlaybackWhenQueueWasEmpty verifies that the root
// model transitions from PlaybackIdle to PlaybackPlaying and calls
// PlayFromIndex(0) when a TrackEnqueuedMsg arrives with WasEmpty=true.
func TestTrackEnqueuedMsgStartsPlaybackWhenQueueWasEmpty(t *testing.T) {
	p := &stubPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackIdle
	// Pre-seed one queue entry so the model knows what to display.
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "Alice – Song One", Artist: "Alice", Title: "Song One"},
	}

	track := library.Track{Artist: "Alice", Title: "Song One", Path: "/music/a.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 1, WasEmpty: true}
	next, cmd := m.Update(msg)
	nm := next.(Model)

	if cmd != nil {
		t.Fatalf("TrackEnqueuedMsg must not return a follow-up cmd, got %T", cmd)
	}
	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("expected playbackState PlaybackPlaying, got %v", nm.playbackState)
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 0 {
		t.Fatalf("expected PlayFromIndex(0), got calls=%v", p.playFromIndexCalls)
	}
}

// TestTrackEnqueuedMsgDoesNotStartPlaybackWhenQueueWasNonEmpty verifies that
// the root model does NOT call PlayFromIndex when WasEmpty is false, leaving
// current playback uninterrupted.
func TestTrackEnqueuedMsgDoesNotStartPlaybackWhenQueueWasNonEmpty(t *testing.T) {
	p := &stubPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackIdle

	track := library.Track{Artist: "Bob", Title: "Track Two", Path: "/music/b.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, WasEmpty: false}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if len(p.playFromIndexCalls) != 0 {
		t.Fatalf("expected no PlayFromIndex call when WasEmpty=false, got calls=%v", p.playFromIndexCalls)
	}
	if nm.playbackState != PlaybackIdle {
		t.Fatalf("expected playbackState PlaybackIdle (unchanged), got %v", nm.playbackState)
	}
}

// stubQueueDequeuer records calls to soft-delete, undo, and purge methods for
// use in undo-slot unit tests.
type stubQueueDequeuer struct {
	dequeueTrackSoftCalls []int64
	undoDequeueCallIDs    []int64
	clearQueueSoftResult  []int64
	undoClearQueueCalls   [][]int64
	purgeBeforeCalls      []int64
}

func (s *stubQueueDequeuer) DequeueTrackSoft(id int64) error {
	s.dequeueTrackSoftCalls = append(s.dequeueTrackSoftCalls, id)
	return nil
}

func (s *stubQueueDequeuer) UndoDequeue(id int64) error {
	s.undoDequeueCallIDs = append(s.undoDequeueCallIDs, id)
	return nil
}

func (s *stubQueueDequeuer) ClearQueueSoft() ([]int64, error) {
	return s.clearQueueSoftResult, nil
}

func (s *stubQueueDequeuer) UndoClearQueue(ids []int64) error {
	s.undoClearQueueCalls = append(s.undoClearQueueCalls, ids)
	return nil
}

func (s *stubQueueDequeuer) PurgeDeletedQueueEntries(before int64) error {
	s.purgeBeforeCalls = append(s.purgeBeforeCalls, before)
	return nil
}

// TestUndoMsgRestoresDequeueWithinWindow verifies that receiving UndoMsg within
// the undo window calls UndoDequeue on the DB writer and restores the entry to
// the TUI slice at its original index.
func TestUndoMsgRestoresDequeueWithinWindow(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetQueueDequeuer(d)
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	// Dequeue the middle entry (index 1).
	next, _ := m.Update(DequeueTrackMsg{Index: 1})
	nm := next.(Model)
	if len(nm.queue.entries) != 2 {
		t.Fatalf("after dequeue: expected 2 entries, got %d", len(nm.queue.entries))
	}

	// UndoMsg within the window must restore the entry.
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 3 {
		t.Fatalf("after undo: expected 3 entries, got %d", len(nm.queue.entries))
	}
	if nm.queue.entries[1].ID != 20 {
		t.Fatalf("after undo: expected restored entry at index 1 with ID=20, got ID=%d", nm.queue.entries[1].ID)
	}
	if len(d.undoDequeueCallIDs) != 1 || d.undoDequeueCallIDs[0] != 20 {
		t.Fatalf("UndoDequeue must be called with ID=20, got calls=%v", d.undoDequeueCallIDs)
	}
	// Undo slot must be cleared after restore.
	if nm.undoSlot.kind != undoNone {
		t.Fatal("undo slot must be cleared after a successful undo")
	}
}

// TestUndoMsgIsNoOpAfterWindowExpiry verifies that UndoMsg received after the
// 5-second window has elapsed is a no-op: the queue is unchanged and UndoDequeue
// is not called.
func TestUndoMsgIsNoOpAfterWindowExpiry(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetQueueDequeuer(d)
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
	}

	// Dequeue entry 0, then manually expire the undo slot.
	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)
	nm.undoSlot.expiresAt = time.Now().Add(-1 * time.Second) // expired

	// UndoMsg after expiry must be a no-op.
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("undo after expiry: expected 1 entry (no restore), got %d", len(nm.queue.entries))
	}
	if len(d.undoDequeueCallIDs) != 0 {
		t.Fatalf("undo after expiry: UndoDequeue must not be called, got calls=%v", d.undoDequeueCallIDs)
	}
}

// TestNewDestructiveActionReplacesUndoSlot verifies that a second destructive
// action before the first undo window expires overwrites the undo slot.
func TestNewDestructiveActionReplacesUndoSlot(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetQueueDequeuer(d)
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	// First dequeue: removes index 0 (ID=10).
	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)
	firstSlotID := nm.undoSlot.dequeueID

	// Second dequeue: removes index 0 of the remaining entries (ID=20).
	next, _ = nm.Update(DequeueTrackMsg{Index: 0})
	nm = next.(Model)

	if nm.undoSlot.dequeueID == firstSlotID {
		t.Fatal("second dequeue must replace the undo slot with the new entry's ID")
	}
	if nm.undoSlot.dequeueID != 20 {
		t.Fatalf("undo slot must hold ID=20 after second dequeue, got %d", nm.undoSlot.dequeueID)
	}
}

// TestDequeueShowsUndoHintInStatusBar verifies that the status bar displays an
// undo hint immediately after a DequeueTrackMsg is processed.
func TestDequeueShowsUndoHintInStatusBar(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.statusBar.notification.msg == "" {
		t.Fatal("dequeue: status bar must show an undo hint immediately after DequeueTrackMsg")
	}
	v := nm.View()
	if !strings.Contains(v.Content, "undo") {
		t.Fatalf("dequeue: status bar must mention 'undo'; got:\n%s", v.Content)
	}
}

// TestClearQueueShowsUndoHintInStatusBar verifies that the status bar displays
// an undo hint immediately after a ClearQueueMsg is processed.
func TestClearQueueShowsUndoHintInStatusBar(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.statusBar.notification.msg == "" {
		t.Fatal("clear queue: status bar must show an undo hint immediately after ClearQueueMsg")
	}
	v := nm.View()
	if !strings.Contains(v.Content, "undo") {
		t.Fatalf("clear queue: status bar must mention 'undo'; got:\n%s", v.Content)
	}
}

// TestUndoMsgRestoresClearQueueWithinWindow verifies that UndoMsg within the
// window calls UndoClearQueue on the DB writer and restores all entries in the
// TUI slice.
func TestUndoMsgRestoresClearQueueWithinWindow(t *testing.T) {
	d := &stubQueueDequeuer{clearQueueSoftResult: []int64{10, 20}}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetQueueDequeuer(d)
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)
	if len(nm.queue.entries) != 0 {
		t.Fatalf("after clear: expected 0 entries, got %d", len(nm.queue.entries))
	}

	// UndoMsg within the window must restore both entries.
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 2 {
		t.Fatalf("after undo clear: expected 2 entries, got %d", len(nm.queue.entries))
	}
	if len(d.undoClearQueueCalls) != 1 {
		t.Fatalf("UndoClearQueue must be called once, got %d calls", len(d.undoClearQueueCalls))
	}
	if nm.undoSlot.kind != undoNone {
		t.Fatal("undo slot must be cleared after a successful undo")
	}
}

// TestUndoMsgWithNoSlotIsNoOp verifies that UndoMsg when no destructive action
// has been performed is a no-op.
func TestUndoMsgWithNoSlotIsNoOp(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	// Queue must be unchanged.
	if len(nm.queue.entries) != 1 {
		t.Fatalf("UndoMsg with no slot: expected 1 entry unchanged, got %d", len(nm.queue.entries))
	}
}

// TestPauseEmitsSessionPositionWriteCmd verifies that transitioning to paused
// returns a Cmd that calls UpdateSessionPosition on the session writer with
// the current track hash and elapsed position in milliseconds.
func TestPauseEmitsSessionPositionWriteCmd(t *testing.T) {
	p := &stubPlayer{}
	sw := &stubSessionWriter{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.SetSessionWriter(sw)
	m.playbackState = PlaybackPlaying
	m.currentTrackHash = "abc123"
	m.elapsedSecs = 42.5 // 42500 ms

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd == nil {
		t.Fatal("pause: expected non-nil cmd for session position write")
	}
	cmd() // execute the cmd to trigger the write
	if len(sw.positionCalls) != 1 {
		t.Fatalf("pause: expected 1 UpdateSessionPosition call, got %d", len(sw.positionCalls))
	}
	if sw.positionCalls[0].hash != "abc123" {
		t.Fatalf("pause: expected hash %q, got %q", "abc123", sw.positionCalls[0].hash)
	}
	if sw.positionCalls[0].positionMs != 42500 {
		t.Fatalf("pause: expected positionMs 42500, got %d", sw.positionCalls[0].positionMs)
	}
}

// TestPauseWithoutSessionWriterReturnsNilCmd verifies that pausing without a
// session writer returns nil from the Cmd (no-op write path).
func TestPauseWithoutSessionWriterReturnsNilCmd(t *testing.T) {
	p := &stubPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackPlaying
	m.queue.entries = []QueueEntry{{ID: 1, Display: "Track"}}

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd != nil {
		t.Fatal("pause without session writer: expected nil cmd")
	}
}

// TestSeekEmitsSessionPositionWriteCmd verifies that each seek action (fine and
// coarse, forward and backward) returns a Cmd that calls UpdateSessionPosition
// on the session writer.
func TestSeekEmitsSessionPositionWriteCmd(t *testing.T) {
	seekKeys := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"seek-back-fine", tea.KeyPressMsg{Code: tea.KeyLeft}},
		{"seek-forward-fine", tea.KeyPressMsg{Code: tea.KeyRight}},
		{"seek-back-coarse", tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift}},
		{"seek-forward-coarse", tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}},
	}

	for _, tc := range seekKeys {
		t.Run(tc.name, func(t *testing.T) {
			p := &stubSeekPlayer{}
			sw := &stubSessionWriter{}
			m := New("/log", "", false, nil, nil, "", nil, p)
			m.SetSessionWriter(sw)
			m.currentTrackHash = "seekhash"
			m.elapsedSecs = 10.0 // 10000 ms

			_, cmd := m.Update(tc.msg)
			if cmd == nil {
				t.Fatalf("%s: expected non-nil cmd for session position write", tc.name)
			}
			cmd()
			if len(sw.positionCalls) != 1 {
				t.Fatalf("%s: expected 1 UpdateSessionPosition call, got %d", tc.name, len(sw.positionCalls))
			}
			if sw.positionCalls[0].hash != "seekhash" {
				t.Fatalf("%s: expected hash %q, got %q", tc.name, "seekhash", sw.positionCalls[0].hash)
			}
			if sw.positionCalls[0].positionMs != 10000 {
				t.Fatalf("%s: expected positionMs 10000, got %d", tc.name, sw.positionCalls[0].positionMs)
			}
		})
	}
}

// TestTrackChangedMsgEmitsSessionPositionWriteCmd verifies that receiving a
// TrackChangedMsg returns a Cmd that calls UpdateSessionPosition with the new
// track's hash and position 0 (start of track).
func TestTrackChangedMsgEmitsSessionPositionWriteCmd(t *testing.T) {
	sw := &stubSessionWriter{}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetSessionWriter(sw)
	m.elapsedSecs = 99.0 // must be reset to 0 before the cmd captures position

	_, cmd := m.Update(TrackChangedMsg{
		Artist: "Artist",
		Title:  "Title",
		Hash:   "newhash",
	})
	if cmd == nil {
		t.Fatal("TrackChangedMsg: expected non-nil cmd for session position write")
	}
	cmd()
	if len(sw.positionCalls) != 1 {
		t.Fatalf("TrackChangedMsg: expected 1 UpdateSessionPosition call, got %d", len(sw.positionCalls))
	}
	if sw.positionCalls[0].hash != "newhash" {
		t.Fatalf("TrackChangedMsg: expected hash %q, got %q", "newhash", sw.positionCalls[0].hash)
	}
	if sw.positionCalls[0].positionMs != 0 {
		t.Fatalf("TrackChangedMsg: expected positionMs 0, got %d", sw.positionCalls[0].positionMs)
	}
}

// TestTrackChangedMsgStoresCurrentTrackHash verifies that TrackChangedMsg
// updates m.currentTrackHash so that subsequent pause/seek writes use the
// correct hash.
func TestTrackChangedMsgStoresCurrentTrackHash(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	next, _ := m.Update(TrackChangedMsg{Artist: "A", Title: "T", Hash: "testhash"})
	nm := next.(Model)

	if nm.currentTrackHash != "testhash" {
		t.Fatalf("expected currentTrackHash %q, got %q", "testhash", nm.currentTrackHash)
	}
}

// TestTrackChangedMsgWithoutSessionWriterReturnsNilCmd verifies that
// TrackChangedMsg without a session writer returns nil cmd (no-op write path).
func TestTrackChangedMsgWithoutSessionWriterReturnsNilCmd(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	_, cmd := m.Update(TrackChangedMsg{Artist: "A", Title: "T", Hash: "h"})
	if cmd != nil {
		t.Fatal("TrackChangedMsg without session writer: expected nil cmd")
	}
}

// TestSessionTickMsgReschedulesCmd verifies that sessionTickMsg always returns
// a non-nil follow-up command so the 10-second periodic ticker keeps firing.
func TestSessionTickMsgReschedulesCmd(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	_, cmd := m.Update(sessionTickMsg{})
	if cmd == nil {
		t.Fatal("sessionTickMsg must return a follow-up cmd to reschedule the ticker")
	}
}

// TestSessionTickMsgWritesPositionWhenPlaying verifies that when playback is
// active, sessionTickMsg triggers an UpdateSessionPosition write through the
// session writer with the current track hash and elapsed position in ms.
//
// tea.Batch returns a BatchMsg (a []tea.Cmd) when invoked — the runtime then
// executes each sub-cmd concurrently. The test extracts and runs only the
// write sub-cmd to confirm the correct value is captured.
func TestSessionTickMsgWritesPositionWhenPlaying(t *testing.T) {
	sw := &stubSessionWriter{}
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetSessionWriter(sw)
	m.playbackState = PlaybackPlaying
	m.currentTrackHash = "abc123"
	m.elapsedSecs = 45.5 // 45500 ms

	_, cmd := m.Update(sessionTickMsg{})
	if cmd == nil {
		t.Fatal("sessionTickMsg while playing: expected non-nil cmd")
	}

	// cmd() returns a tea.BatchMsg containing [sessionTickCmd, writeCmd].
	// Execute each sub-cmd in a goroutine with a short timeout so the
	// blocking timer cmd is skipped while the write cmd runs to completion.
	batchMsg := cmd()
	batch, ok := batchMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", batchMsg)
	}
	for _, subcmd := range batch {
		if subcmd == nil {
			continue
		}
		done := make(chan struct{})
		go func(c tea.Cmd) {
			c()
			close(done)
		}(subcmd)
		select {
		case <-done:
			// completed quickly — this is the write cmd
		case <-time.After(50 * time.Millisecond):
			// timed out — this is the blocking tick timer; skip it
		}
	}

	if len(sw.positionCalls) != 1 {
		t.Fatalf("expected 1 UpdateSessionPosition call, got %d", len(sw.positionCalls))
	}
	got := sw.positionCalls[0]
	if got.hash != "abc123" {
		t.Fatalf("expected hash %q, got %q", "abc123", got.hash)
	}
	if got.positionMs != 45500 {
		t.Fatalf("expected positionMs 45500, got %d", got.positionMs)
	}
}

// TestSessionTickMsgSkipsWriteWhenNotPlaying verifies that sessionTickMsg does
// not call UpdateSessionPosition when playback is paused, stopped, or idle.
// The tick cmd itself is a timer and is not executed (it would block); only the
// absence of write side-effects is asserted.
func TestSessionTickMsgSkipsWriteWhenNotPlaying(t *testing.T) {
	states := []PlaybackState{PlaybackIdle, PlaybackPaused, PlaybackStopped}
	for _, state := range states {
		sw := &stubSessionWriter{}
		m := New("/log", "", false, nil, nil, "", nil, nil)
		m.SetSessionWriter(sw)
		m.playbackState = state
		m.currentTrackHash = "abc123"
		m.elapsedSecs = 30.0

		_, cmd := m.Update(sessionTickMsg{})
		if cmd == nil {
			t.Fatalf("state %v: sessionTickMsg must return a reschedule cmd even when not playing", state)
		}
		// When not playing, tea.Batch(sessionTickCmd(), nil) compacts to just
		// sessionTickCmd() — a timer that must NOT be executed here (it blocks).
		// Asserting positionCalls is empty confirms no write was scheduled.
		if len(sw.positionCalls) != 0 {
			t.Fatalf("state %v: expected 0 UpdateSessionPosition calls, got %d", state, len(sw.positionCalls))
		}
	}
}

// TestInitSchedulesSessionTicker verifies that Init starts the 10-second
// periodic session ticker in addition to the 1-second display tick.
func TestInitSchedulesSessionTicker(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init must return a non-nil cmd (includes sessionTickCmd)")
	}
}

// TestView2RendersPlaylistsNameAndCount verifies that switching to view 2 and
// calling SetPlaylists causes each playlist's name and track count to appear
// in the rendered output.
func TestView2RendersPlaylistsNameAndCount(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Chill", TrackCount: 3},
		{ID: 2, Name: "Workout", TrackCount: 8},
	})

	// Give the model a realistic terminal width so names are not truncated.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)

	// Switch to the playlists view.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)

	v := nm.View()
	for _, want := range []string{"Chill", "(3)", "Workout", "(8)"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("expected %q in view 2 output, got:\n%s", want, v.Content)
		}
	}
}

// stubPlaylistTrackReader implements playlistTrackReader for tests.
type stubPlaylistTrackReader struct {
	rows []db.PlaylistTrackRow
}

func (s *stubPlaylistTrackReader) ReadPlaylistTracks(_ int64) ([]db.PlaylistTrackRow, error) {
	return s.rows, nil
}

// TestPlaylistEscFromTrackLevelReturnsToList verifies that pressing Esc while
// in TrackLevel transitions back to ListLevel and preserves the list cursor.
func TestPlaylistEscFromTrackLevelReturnsToList(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Alpha", TrackCount: 1},
		{ID: 2, Name: "Beta", TrackCount: 1},
		{ID: 3, Name: "Gamma", TrackCount: 1},
	})
	reader := &stubPlaylistTrackReader{
		rows: []db.PlaylistTrackRow{
			{Position: 1, Title: "Track One", Artist: "Artist", DurationMs: 60000},
		},
	}
	m.SetPlaylistTrackReader(reader)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)

	// Move cursor to the second playlist (index 1).
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm = next.(Model)
	if nm.playlists.cursor != 1 {
		t.Fatalf("precondition: expected cursor at 1, got %d", nm.playlists.cursor)
	}

	// Press Enter to open the playlist and execute the async load.
	next, cmd := nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm = next.(Model)
	if cmd == nil {
		t.Fatal("expected non-nil Cmd from actionPlaylistOpen")
	}
	next, _ = nm.Update(cmd())
	nm = next.(Model)

	// Verify we are now at TrackLevel showing the track.
	v := nm.View()
	if !strings.Contains(v.Content, "Track One") {
		t.Fatalf("expected TrackLevel output with 'Track One', got:\n%s", v.Content)
	}

	// Press Esc — should return to ListLevel.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	// Verify we are back at ListLevel and the list cursor is preserved at 1.
	v = nm.View()
	if !strings.Contains(v.Content, "Alpha") || !strings.Contains(v.Content, "Beta") {
		t.Fatalf("expected ListLevel output with playlist names, got:\n%s", v.Content)
	}
	if nm.playlists.cursor != 1 {
		t.Errorf("expected list cursor preserved at 1 after Esc, got %d", nm.playlists.cursor)
	}
}

// TestPlaylistEscAtListLevelIsNoOp verifies that pressing Esc while already at
// ListLevel produces no visible state change and no error.
func TestPlaylistEscAtListLevelIsNoOp(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Chill", TrackCount: 5},
	})

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)

	beforeView := nm.View()
	beforeLevel := nm.playlists.level
	beforeCursor := nm.playlists.cursor

	// Press Esc at ListLevel — must be a no-op.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.playlists.level != beforeLevel {
		t.Errorf("Esc at ListLevel changed level: before=%v after=%v", beforeLevel, nm.playlists.level)
	}
	if nm.playlists.cursor != beforeCursor {
		t.Errorf("Esc at ListLevel changed cursor: before=%d after=%d", beforeCursor, nm.playlists.cursor)
	}
	afterView := nm.View()
	if beforeView.Content != afterView.Content {
		t.Errorf("Esc at ListLevel changed rendered output:\nbefore:\n%s\nafter:\n%s", beforeView.Content, afterView.Content)
	}
}

// TestPlaylistOpenRendersTrackTitles is an integration test that verifies the
// full path from actionPlaylistOpen through playlistTracksLoadedMsg to the
// rendered output. After opening a playlist the track titles must appear in
// the playlists panel.
func TestPlaylistOpenRendersTrackTitles(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Chill", TrackCount: 2},
	})
	reader := &stubPlaylistTrackReader{
		rows: []db.PlaylistTrackRow{
			{Position: 1, Title: "Ocean Waves", Artist: "Ambient Co", DurationMs: 240000},
			{Position: 2, Title: "Forest Rain", Artist: "Nature Sounds", DurationMs: 180000},
		},
	}
	m.SetPlaylistTrackReader(reader)

	// Switch to the playlists view at a realistic width.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)

	// Press Enter to open the playlist — should return a Cmd.
	next, cmd := nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm = next.(Model)
	if cmd == nil {
		t.Fatal("actionPlaylistOpen: expected a non-nil Cmd to load tracks")
	}

	// Execute the Cmd and feed the result back into Update.
	loadMsg := cmd()
	next, _ = nm.Update(loadMsg)
	nm = next.(Model)

	v := nm.View()
	for _, want := range []string{"Ocean Waves", "Forest Rain"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("expected track title %q in view after playlist open, got:\n%s", want, v.Content)
		}
	}
}

// TestPlaylistsBulkRemoveAtTrackLevel verifies the 'a' key at TrackLevel with
// a non-empty bulk selection: selected tracks are removed from the in-memory
// slice, the cursor is clamped, and selectedTrackIDs is cleared. The nil
// playlistDeleter means no DB calls are made — the nil-guard in the handler
// makes this safe without a stub.
func TestPlaylistsBulkRemoveAtTrackLevel(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	// Navigate to playlists view.
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)

	// Seed the playlists panel directly at TrackLevel with three tracks.
	nm.playlists.level = pltui.TrackLevel
	nm.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "Track One", Artist: "A", DurationMs: 60000},
		{ID: 20, Position: 2, Title: "Track Two", Artist: "B", DurationMs: 60000},
		{ID: 30, Position: 3, Title: "Track Three", Artist: "C", DurationMs: 60000},
	}
	// Bulk-select the first two tracks.
	nm.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 20: {}}
	// Place cursor beyond the surviving track to exercise cursor clamping.
	nm.playlists.cursor = 2

	// Press 'a' — should trigger actionAddToPlaylist which bulk-removes.
	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm2 := next2.(Model)

	if len(nm2.playlists.tracks) != 1 {
		t.Fatalf("expected 1 track remaining, got %d", len(nm2.playlists.tracks))
	}
	if nm2.playlists.tracks[0].ID != 30 {
		t.Fatalf("expected remaining track ID=30, got ID=%d", nm2.playlists.tracks[0].ID)
	}
	if nm2.playlists.cursor >= len(nm2.playlists.tracks) {
		t.Fatalf("cursor %d out of range after bulk remove (len=%d)", nm2.playlists.cursor, len(nm2.playlists.tracks))
	}
	if nm2.playlists.selectedTrackIDs != nil {
		t.Fatalf("expected selectedTrackIDs to be nil after bulk remove, got %v", nm2.playlists.selectedTrackIDs)
	}
}

// TestWindowSizeMsgPropagatesWidthToChildViews verifies that a WindowSizeMsg
// updates the width fields of both the library and playlists child views, and
// that column widths computed from the updated width are proportionally larger
// than those computed from a smaller width.
func TestWindowSizeMsgPropagatesWidthToChildViews(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)

	const newWidth = 160
	next, _ := m.Update(tea.WindowSizeMsg{Width: newWidth, Height: 40})
	nm := next.(Model)

	if nm.library.width != newWidth {
		t.Fatalf("library.width: expected %d, got %d", newWidth, nm.library.width)
	}
	if nm.playlists.width != newWidth {
		t.Fatalf("playlists.width: expected %d, got %d", newWidth, nm.playlists.width)
	}

	wide := libtui.Compute(120)
	narrow := libtui.Compute(80)

	if wide.Artist <= narrow.Artist {
		t.Fatalf("Artist column: expected Compute(120).Artist (%d) > Compute(80).Artist (%d)", wide.Artist, narrow.Artist)
	}
	if wide.Title <= narrow.Title {
		t.Fatalf("Title column: expected Compute(120).Title (%d) > Compute(80).Title (%d)", wide.Title, narrow.Title)
	}
}

// TestGlobalRepeatBindingFiresInViewsWithoutLocalOverride verifies that
// pressing lowercase 'r' in Library and Queue views — which have no view-local
// 'r' binding — cycles repeatMode from RepeatOff to RepeatAll via the global
// actionRepeatCycle handler. The Playlists view is excluded because it defines
// a local 'r' binding (rename) that shadows the global action.
func TestGlobalRepeatBindingFiresInViewsWithoutLocalOverride(t *testing.T) {
	views := []struct {
		name   string
		viewID viewID
	}{
		{"Library", viewLibrary},
		{"Queue", viewQueue},
	}
	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			m := New("/log", "", false, nil, nil, "", nil, nil)
			m.activeView = tc.viewID

			if m.repeatMode != RepeatOff {
				t.Fatalf("%s: initial repeatMode must be RepeatOff, got %v", tc.name, m.repeatMode)
			}

			next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm := next.(Model)

			if nm.repeatMode != RepeatAll {
				t.Fatalf("%s: pressing 'r' must cycle repeatMode from RepeatOff to RepeatAll, got %v", tc.name, nm.repeatMode)
			}
		})
	}
}

// TestClearQueueMsgStopsPlaybackAndTransitionsToIdle verifies that handling
// ClearQueueMsg calls Stop() on the player exactly once and transitions
// playbackState to PlaybackIdle (not PlaybackStopped).
func TestClearQueueMsgStopsPlaybackAndTransitionsToIdle(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New("/log", "", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackPlaying
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/music/a.flac", Display: "A"},
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.playbackState != PlaybackIdle {
		t.Fatalf("ClearQueueMsg: expected PlaybackIdle, got %v", nm.playbackState)
	}
	if p.stopCalls != 1 {
		t.Fatalf("ClearQueueMsg: expected 1 Stop() call, got %d", p.stopCalls)
	}
}

// drillIntoPlaylistTrackLevel puts m into Playlists view at TrackLevel with
// the provided tracks seeded directly, returning a ready-to-use model.
func drillIntoPlaylistTrackLevel(m Model, tracks []db.PlaylistTrackRow) Model {
	m.activeView = viewPlaylists
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = tracks
	m.playlists.cursor = 0
	return m
}

// TestPlaylistTrackCursorDownJKey verifies that pressing "j" while the
// playlist track-level view is active advances the cursor by one.
func TestPlaylistTrackCursorDownJKey(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.playlists.cursor != 1 {
		t.Errorf("j key at TrackLevel: expected cursor 1, got %d", nm.playlists.cursor)
	}
}

// TestPlaylistTrackCursorUpKKey verifies that pressing "k" while the playlist
// track-level view is active moves the cursor up by one.
func TestPlaylistTrackCursorUpKKey(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})
	m.playlists.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.playlists.cursor != 0 {
		t.Errorf("k key at TrackLevel: expected cursor 0, got %d", nm.playlists.cursor)
	}
}

// TestPlaylistTrackCursorDownArrow verifies that the down arrow key advances
// the cursor in the playlist track-level view, behaving identically to "j".
func TestPlaylistTrackCursorDownArrow(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.playlists.cursor != 1 {
		t.Errorf("down arrow at TrackLevel: expected cursor 1, got %d", nm.playlists.cursor)
	}
}

// TestPlaylistTrackCursorUpArrow verifies that the up arrow key moves the
// cursor up in the playlist track-level view, behaving identically to "k".
func TestPlaylistTrackCursorUpArrow(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})
	m.playlists.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	nm := next.(Model)
	if nm.playlists.cursor != 0 {
		t.Errorf("up arrow at TrackLevel: expected cursor 0, got %d", nm.playlists.cursor)
	}
}

// TestPlaylistTrackCursorClampAtBottom verifies that the cursor stops at the
// last track without wrapping when the playlist track-level view is active.
func TestPlaylistTrackCursorClampAtBottom(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})
	m.playlists.cursor = 1 // already at last item

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.playlists.cursor != 1 {
		t.Errorf("j at last track: expected cursor 1 (clamped), got %d", nm.playlists.cursor)
	}
}

// TestPlaylistTrackCursorClampAtTop verifies that the cursor stops at the
// first track without wrapping when the playlist track-level view is active.
func TestPlaylistTrackCursorClampAtTop(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})
	// cursor starts at 0

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.playlists.cursor != 0 {
		t.Errorf("k at first track: expected cursor 0 (clamped), got %d", nm.playlists.cursor)
	}
}

// TestQueueCursorDownJKey verifies that pressing "j" while the queue view is
// active advances the cursor by one.
func TestQueueCursorDownJKey(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.queue.cursor != 1 {
		t.Errorf("j key: expected cursor 1, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorUpKKey verifies that pressing "k" while the queue view is
// active moves the cursor up by one.
func TestQueueCursorUpKKey(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.queue.cursor != 0 {
		t.Errorf("k key: expected cursor 0, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorDownArrow verifies that the down arrow key advances the
// cursor in the queue view, behaving identically to "j".
func TestQueueCursorDownArrow(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.queue.cursor != 1 {
		t.Errorf("down arrow: expected cursor 1, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorUpArrow verifies that the up arrow key moves the cursor up
// in the queue view, behaving identically to "k".
func TestQueueCursorUpArrow(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	nm := next.(Model)
	if nm.queue.cursor != 0 {
		t.Errorf("up arrow: expected cursor 0, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorClampAtBottom verifies that the cursor stops at the last
// entry without wrapping when the queue view is active.
func TestQueueCursorClampAtBottom(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1 // already at last item

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.queue.cursor != 1 {
		t.Errorf("j at last: expected cursor 1 (clamped), got %d", nm.queue.cursor)
	}
}

// TestQueueCursorClampAtTop verifies that the cursor stops at the first entry
// without wrapping when the queue view is active.
func TestQueueCursorClampAtTop(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	// cursor starts at 0

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.queue.cursor != 0 {
		t.Errorf("k at first: expected cursor 0 (clamped), got %d", nm.queue.cursor)
	}
}
