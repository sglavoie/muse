package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// stubPreviewer is a minimal Previewer implementation for testing.
type stubPreviewer struct {
	startCalled    bool
	loadPath       string
	closeCalled    bool
	elapsed, total float64
	elapsedErr     error
}

func (s *stubPreviewer) Start() error { s.startCalled = true; return nil }
func (s *stubPreviewer) LoadAndPlay(path string) error {
	s.loadPath = path
	return nil
}
func (s *stubPreviewer) ElapsedPosition() (float64, float64, error) {
	return s.elapsed, s.total, s.elapsedErr
}
func (s *stubPreviewer) Close() error { s.closeCalled = true; return nil }

// setupLibraryWithTrack creates a model with a single track in the library.
func setupLibraryWithTrack(t *testing.T) Model {
	t.Helper()
	m := New(context.Background())
	m.activeView = viewLibrary
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T",
	})
	return m
}

// TestPreviewKeyActivatesPreview verifies that pressing P when a track is
// focused in the library returns a non-nil command to start the preview.
func TestPreviewKeyActivatesPreview(t *testing.T) {
	m := setupLibraryWithTrack(t)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	if cmd == nil {
		t.Fatal("expected non-nil command from P key with focused track")
	}
}

// TestPreviewKeyNoTrackShowsNotification verifies that pressing P with no
// focused track shows a notification.
func TestPreviewKeyNoTrackShowsNotification(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary
	// library.index is empty by default

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	nm := next.(Model)
	if cmd != nil {
		t.Fatal("expected nil command when no track to preview")
	}
	if nm.statusBar.notification.msg != "No track to preview" {
		t.Fatalf("expected 'No track to preview' notification, got %q", nm.statusBar.notification.msg)
	}
}

// TestPreviewStartedMsgSetsOverlay verifies that receiving a
// previewStartedMsg sets the preview overlay and player.
func TestPreviewStartedMsgSetsOverlay(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{}

	next, _ := m.Update(previewStartedMsg{
		player: sp,
		overlay: PreviewOverlay{
			artist: "Test",
			title:  "Song",
		},
	})
	nm := next.(Model)

	if nm.overlay.preview == nil {
		t.Fatal("expected preview overlay to be set")
	}
	if nm.overlay.preview.artist != "Test" || nm.overlay.preview.title != "Song" {
		t.Fatalf("unexpected overlay content: %+v", *nm.overlay.preview)
	}
	if nm.overlay.previewPlayer != sp {
		t.Fatal("expected previewPlayer to be set")
	}
}

// TestPreviewDismissWithEsc verifies that pressing Esc while preview is
// active dismisses it.
func TestPreviewDismissWithEsc(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{}
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = sp

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview to be nil after Esc")
	}
	if !sp.closeCalled {
		t.Fatal("expected Close to be called on preview player")
	}
}

// TestPreviewDismissWithP verifies that pressing P while preview is
// active dismisses it.
func TestPreviewDismissWithP(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{}
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = sp

	next, _ := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview to be nil after P")
	}
	if !sp.closeCalled {
		t.Fatal("expected Close to be called on preview player")
	}
}

// TestPreviewSwallowsKeys verifies that while preview is active, other
// keys are swallowed (not processed).
func TestPreviewSwallowsKeys(t *testing.T) {
	m := New(context.Background())
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = &stubPreviewer{}
	m.activeView = viewLibrary

	// Press '3' (switch to queue) — should be swallowed.
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)

	if nm.activeView != viewLibrary {
		t.Fatalf("expected view to stay at library while preview active, got %d", nm.activeView)
	}
	if nm.overlay.preview == nil {
		t.Fatal("preview should still be active after swallowed key")
	}
}

// TestPreviewCtrlCDismissesAndQuits verifies that ctrl+c during preview
// dismisses the preview and returns a quit command.
func TestPreviewCtrlCDismissesAndQuits(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{}
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = sp

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview to be dismissed on ctrl+c")
	}
	if !sp.closeCalled {
		t.Fatal("expected Close to be called on preview player")
	}
	if cmd == nil {
		t.Fatal("expected quit command from ctrl+c during preview")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected tea.QuitMsg from ctrl+c command")
	}
}

// TestPreviewDismissResumesPlayback verifies that dismissing preview
// resumes main playback when it was playing before preview.
func TestPreviewDismissResumesPlayback(t *testing.T) {
	sp := &stubPlayer{}
	m := New(context.Background())
	m.player = sp
	m.playback.state = PlaybackPaused
	m.overlay.previewWasPlaying = true
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = &stubPreviewer{}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview to be dismissed")
	}
	if sp.playCalls != 1 {
		t.Fatalf("expected 1 Play call to resume playback, got %d", sp.playCalls)
	}
	if nm.overlay.previewWasPlaying {
		t.Fatal("expected previewWasPlaying to be reset to false")
	}
}

// TestPreviewDismissDoesNotResumeIfNotPlaying verifies that dismissing
// preview does not resume main playback when it was not playing.
func TestPreviewDismissDoesNotResumeIfNotPlaying(t *testing.T) {
	sp := &stubPlayer{}
	m := New(context.Background())
	m.player = sp
	m.playback.state = PlaybackPaused
	m.overlay.previewWasPlaying = false
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = &stubPreviewer{}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if sp.playCalls != 0 {
		t.Fatalf("expected 0 Play calls when not previously playing, got %d", sp.playCalls)
	}
	_ = nm
}

// TestPreviewStartErrorMsgShowsNotification verifies that a
// previewStartErrorMsg shows an error notification.
func TestPreviewStartErrorMsgShowsNotification(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(previewStartErrorMsg{err: errTest("boom")})
	nm := next.(Model)

	if nm.statusBar.notification.msg != "Preview error: boom" {
		t.Fatalf("expected error notification, got %q", nm.statusBar.notification.msg)
	}
}

// TestPreviewStartErrorResumesPlayback verifies that a previewStartErrorMsg
// resumes main playback if it was paused for the preview.
func TestPreviewStartErrorResumesPlayback(t *testing.T) {
	sp := &stubPlayer{}
	m := New(context.Background())
	m.player = sp
	m.overlay.previewWasPlaying = true

	next, _ := m.Update(previewStartErrorMsg{err: errTest("fail")})
	nm := next.(Model)

	if sp.playCalls != 1 {
		t.Fatalf("expected 1 Play call to resume, got %d", sp.playCalls)
	}
	if nm.overlay.previewWasPlaying {
		t.Fatal("expected previewWasPlaying reset")
	}
}

// TestPreviewOverlayRenderedInView verifies that when preview is active,
// View() renders the preview overlay content.
func TestPreviewOverlayRenderedInView(t *testing.T) {
	m := New(context.Background())
	m.overlay.preview = &PreviewOverlay{
		artist:  "Preview Artist",
		title:   "Preview Song",
		elapsed: 30,
		total:   120,
	}
	m.termWidth = 80
	m.termHeight = 24

	v := m.View()
	if v.Content == "" {
		t.Fatal("expected non-empty View content with preview active")
	}
}

// TestPreviewTickUpdatesProgress verifies that a tickMsg updates the
// preview overlay's elapsed and total values.
func TestPreviewTickUpdatesProgress(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{elapsed: 45.5, total: 200.0}
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = sp

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.overlay.preview == nil {
		t.Fatal("expected preview to still be active after tick")
	}
	if nm.overlay.preview.elapsed != 45.5 {
		t.Errorf("expected elapsed=45.5, got %v", nm.overlay.preview.elapsed)
	}
	if nm.overlay.preview.total != 200.0 {
		t.Errorf("expected total=200.0, got %v", nm.overlay.preview.total)
	}
}

// TestPreviewTickAutoDismissOnError verifies that if ElapsedPosition
// returns an error during tick, the preview is auto-dismissed.
func TestPreviewTickAutoDismissOnError(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{elapsedErr: errTest("eof")}
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = sp

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview to be auto-dismissed on ElapsedPosition error")
	}
}

// TestFocusedTrackForPreview_Queue verifies that focusedTrackForPreview
// returns the queue entry under the cursor.
func TestFocusedTrackForPreview_Queue(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T1", DurationMs: 5000},
		{Hash: "h2", Path: "/b.flac", Artist: "B", Title: "T2", DurationMs: 6000},
	}
	m.queue.cursor = 1

	track, ok := m.focusedTrackForPreview()
	if !ok {
		t.Fatal("expected track from queue")
	}
	if track.Hash != "h2" || track.Artist != "B" {
		t.Fatalf("unexpected track: %+v", track)
	}
	if track.Duration != 6.0 {
		t.Fatalf("expected duration 6.0, got %v", track.Duration)
	}
}

// TestFocusedTrackForPreview_QueueMissing verifies that a missing queue
// entry returns false.
func TestFocusedTrackForPreview_QueueMissing(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Missing: true},
	}
	m.queue.cursor = 0

	_, ok := m.focusedTrackForPreview()
	if ok {
		t.Fatal("expected false for missing queue entry")
	}
}

// TestFocusedTrackForPreview_EmptyQueue verifies that an empty queue
// returns false.
func TestFocusedTrackForPreview_EmptyQueue(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = nil
	m.queue.cursor = 0

	_, ok := m.focusedTrackForPreview()
	if ok {
		t.Fatal("expected false for empty queue")
	}
}

// TestPreviewPausesMainPlayback verifies that pressing P while main
// player is playing pauses it.
func TestPreviewPausesMainPlayback(t *testing.T) {
	sp := &stubPlayer{}
	m := setupLibraryWithTrack(t)
	m.player = sp
	m.playback.state = PlaybackPlaying

	next, _ := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	nm := next.(Model)

	if sp.pauseCalls != 1 {
		t.Fatalf("expected 1 Pause call, got %d", sp.pauseCalls)
	}
	if !nm.overlay.previewWasPlaying {
		t.Fatal("expected previewWasPlaying to be set")
	}
	if nm.playback.state != PlaybackPaused {
		t.Fatal("expected playback state to be paused")
	}
}

// TestFocusedTrackForPreview_QueueWithFilter verifies that
// focusedTrackForPreview uses the filtered entry list when a queue filter
// is active, rather than indexing into the unfiltered entries slice.
func TestFocusedTrackForPreview_QueueWithFilter(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "Alice", Title: "Alpha", DurationMs: 3000},
		{Hash: "h2", Path: "/b.flac", Artist: "Bob", Title: "Beta", DurationMs: 4000},
		{Hash: "h3", Path: "/c.flac", Artist: "Carol", Title: "Charlie", DurationMs: 5000},
	}
	// Activate a filter that matches only "Bob".
	m.queue.filter.Text = "bob"
	m.queue.cursor = 0 // first (and only) filtered entry

	track, ok := m.focusedTrackForPreview()
	if !ok {
		t.Fatal("expected track from filtered queue")
	}
	if track.Hash != "h2" {
		t.Fatalf("expected hash h2 (Bob), got %q", track.Hash)
	}
}

// TestFocusedTrackForPreview_QueueWithSort verifies that
// focusedTrackForPreview respects the queue display order when sorted.
func TestFocusedTrackForPreview_QueueWithSort(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "Zeta", Title: "Zulu", DurationMs: 3000},
		{Hash: "h2", Path: "/b.flac", Artist: "Alpha", Title: "Able", DurationMs: 4000},
	}
	// Sort by artist ascending: Alpha first, then Zeta.
	m.queue.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()
	m.queue.cursor = 0 // first display entry after sort = Alpha

	track, ok := m.focusedTrackForPreview()
	if !ok {
		t.Fatal("expected track from sorted queue")
	}
	// The first display entry after sorting by artist asc should be Alpha.
	if track.Hash != "h2" {
		t.Fatalf("expected hash h2 (Alpha), got %q", track.Hash)
	}
}

// errTest is a simple error type for test assertions.
type errTest string

func (e errTest) Error() string { return string(e) }
