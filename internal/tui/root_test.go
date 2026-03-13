package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
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

// TestModelRepeatAndShuffleDefaultFalse verifies that root Model holds repeat
// and shuffle boolean fields that default to false.
func TestModelRepeatAndShuffleDefaultFalse(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	if m.repeat {
		t.Fatal("repeat must default to false")
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
	if nm.repeat {
		t.Fatal("view-local 'r' in Playlists must not toggle global repeat mode")
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
// Library and Queue views toggles the repeat field, and does NOT trigger rescan.
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

			// First press: repeat off → on.
			next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm := next.(Model)
			if cmd != nil {
				t.Fatalf("%s: 'r' repeat cycle must return nil cmd, got non-nil", tc.name)
			}
			if !nm.repeat {
				t.Fatalf("%s: 'r' first press: expected repeat=true, got false", tc.name)
			}

			// Second press: repeat on → off.
			next, _ = nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm = next.(Model)
			if nm.repeat {
				t.Fatalf("%s: 'r' second press: expected repeat=false, got true", tc.name)
			}
		})
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
// the help overlay with Esc does not change playbackState, repeat, shuffle, or
// the scanning flag.
func TestEscRestoringViewDoesNotAlterPlaybackOrScanState(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	m.playbackState = PlaybackPlaying
	m.repeat = true
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
	if !nm.repeat {
		t.Fatal("Esc: repeat changed; expected true, got false")
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
