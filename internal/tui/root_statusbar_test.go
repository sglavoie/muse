package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestViewStatusBarShowsConfigPathOnFirstRun(t *testing.T) {
	configPath := "/home/user/.config/muse/config.toml"
	m := New(context.Background(), configPath, true, nil, nil, "", nil, nil)
	v := m.View()
	if !strings.Contains(v.Content, configPath) {
		t.Fatalf("status bar missing config path %q in:\n%s", configPath, v.Content)
	}
}

func TestViewStatusBarHidesConfigPathAfterFirstRun(t *testing.T) {
	configPath := "/home/user/.config/muse/config.toml"
	m := New(context.Background(), configPath, false, nil, nil, "", nil, nil)
	v := m.View()
	if strings.Contains(v.Content, configPath) {
		t.Fatalf("status bar must not show config path when configCreated=false")
	}
}

// TestStatusBarReceivesTrackInfoBeforeRender verifies that after a
// PlayQueueIndexMsg is processed, the rendered status bar shows the correct
// artist, title, and playlist source in the persistent left zone.
func TestStatusBarReceivesTrackInfoBeforeRender(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), "", false, nil, nil, "", nil, p)
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
		out := sb.render(0, styles.Styles{})
		if !strings.Contains(out, tc.want) {
			t.Fatalf("state %v: expected %q in status bar; got: %s", tc.state, tc.want, out)
		}
	}
}

// TestTrackChangedMsgUpdatesStatusBar verifies that TrackChangedMsg updates
// the status bar's artist, title, and playlist source.
func TestTrackChangedMsgUpdatesStatusBar(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)

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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	next, _ := m.Update(TrackChangedMsg{Artist: "A", Title: "T"})
	nm := next.(Model)
	v := nm.View()
	if !strings.Contains(v.Content, "Queue") {
		t.Fatalf("status bar must default playlist source to 'Queue'; got:\n%s", v.Content)
	}
}

// TestTickMsgUpdatesElapsedFields verifies that receiving a tickMsg calls
// ElapsedPosition on the player and stores the result on the model.
func TestTickMsgUpdatesElapsedFields(t *testing.T) {
	p := &stubElapsedReader{elapsed: 75.5, total: 240.0}
	m := New(context.Background(), "", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackPlaying

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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("tickMsg must return a follow-up cmd to reschedule the tick")
	}
}

// TestInitSchedulesTick verifies that Init always returns a non-nil command
// (at minimum the 1-second tick).
func TestInitSchedulesTick(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init must return a non-nil cmd (at least the 1-second tick)")
	}
}

// TestStatusBarRendersElapsedTime verifies that after a tickMsg updates the
// elapsed position, the view renders the MM:SS / MM:SS format.
func TestStatusBarRendersElapsedTime(t *testing.T) {
	p := &stubElapsedReader{elapsed: 83.0, total: 245.0}
	m := New(context.Background(), "", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackPlaying
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)
	v := nm.View()

	if !strings.Contains(v.Content, "1:23 / 4:05") {
		t.Fatalf("status bar must show elapsed/total in M:SS / M:SS format; got:\n%s", v.Content)
	}
}

// TestStatusBarNoTimeDisplayWhenZero verifies that when elapsedSecs and
// totalSecs are both 0 the time display is suppressed.
func TestStatusBarNoTimeDisplayWhenZero(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, p)
	m.playbackState = PlaybackPlaying

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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, p)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, p)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)

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

// TestDequeueShowsUndoHintInStatusBar verifies that the status bar displays an
// undo hint immediately after a DequeueTrackMsg is processed.
func TestDequeueShowsUndoHintInStatusBar(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
