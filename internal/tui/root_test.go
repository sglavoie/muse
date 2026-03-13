package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
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

// TestRescanHotkeyReturnsCmd verifies that pressing r with a configured
// library root returns a non-nil background cmd and does not block.
func TestRescanHotkeyReturnsCmd(t *testing.T) {
	root := t.TempDir()
	m := New("/log", "", false, nil, nil, root, stubExtractor{}, nil)
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
	m := New("/log", "", false, ch, nil, root, stubExtractor{}, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd != nil {
		t.Fatal("r key during active scan must return nil cmd")
	}
}

// TestRescanHotkeyIsNoOpWithoutRoot verifies that pressing r when no library
// root is configured returns nil.
func TestRescanHotkeyIsNoOpWithoutRoot(t *testing.T) {
	m := New("/log", "", false, nil, nil, "", nil, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd != nil {
		t.Fatal("r key without library root must return nil cmd")
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

	if nm.statusBar.transientRight == "" {
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
	m := New("/log", "", false, nil, nil, "", nil, nil)
	// First set progress, then clear it.
	next, _ := m.Update(library.ScanProgressMsg{Scanned: 100})
	nm := next.(Model)
	next, _ = nm.Update(library.ScanDoneMsg{})
	nm = next.(Model)
	v := nm.View()
	if strings.Contains(v.Content, "Scanning") {
		t.Fatalf("status bar must not show scan progress after ScanDoneMsg; got:\n%s", v.Content)
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

// TestQueueViewUpdateEnterSendsPlayQueueIndexMsg verifies that pressing Enter
// in the queue view returns a Cmd that emits PlayQueueIndexMsg with the cursor
// index.
func TestQueueViewUpdateEnterSendsPlayQueueIndexMsg(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "Artist – A"},
			{Path: "/music/b.flac", Display: "Artist – B"},
		},
		cursor: 1,
	}
	_, cmd := q.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter key must return a non-nil cmd")
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

// TestQueueViewUpdateEnterOnEmptyQueueIsNoOp verifies that pressing Enter on
// an empty queue does not emit a PlayQueueIndexMsg.
func TestQueueViewUpdateEnterOnEmptyQueueIsNoOp(t *testing.T) {
	q := QueueView{}
	_, cmd := q.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Enter on empty queue must return nil cmd")
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
