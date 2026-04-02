package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// stubStopAfterPlayer extends stubCommandPlayer with the stopAfterToggler
// interface so that tests can verify player-side flag syncing.
type stubStopAfterPlayer struct {
	stubCommandPlayer
	stopAfter bool
}

func (s *stubStopAfterPlayer) SetStopAfterCurrent(v bool) { s.stopAfter = v }
func (s *stubStopAfterPlayer) StopAfterCurrent() bool     { return s.stopAfter }

// TestStopAfterCurrentToggleOnOff verifies that actionStopAfterCurrent toggles
// the flag and syncs to the player.
func TestStopAfterCurrentToggleOnOff(t *testing.T) {
	p := &stubStopAfterPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying

	// Toggle on.
	rm, _, handled := m.handlePlaybackKeyAction(actionStopAfterCurrent)
	if !handled {
		t.Fatal("actionStopAfterCurrent should be handled")
	}
	m = rm.(Model)
	if !m.playback.stopAfterCurrent {
		t.Error("expected stopAfterCurrent to be true after first toggle")
	}
	if !p.stopAfter {
		t.Error("expected player stopAfter to be true")
	}
	if !strings.Contains(m.statusBar.notification.msg, "on") {
		t.Errorf("expected notification containing 'on', got %q", m.statusBar.notification.msg)
	}

	// Toggle off.
	rm, _, _ = m.handlePlaybackKeyAction(actionStopAfterCurrent)
	m = rm.(Model)
	if m.playback.stopAfterCurrent {
		t.Error("expected stopAfterCurrent to be false after second toggle")
	}
	if p.stopAfter {
		t.Error("expected player stopAfter to be false")
	}
	if !strings.Contains(m.statusBar.notification.msg, "off") {
		t.Errorf("expected notification containing 'off', got %q", m.statusBar.notification.msg)
	}
}

// TestStopAfterCurrentRequiresPlayback verifies that the toggle does nothing
// when no track is playing.
func TestStopAfterCurrentRequiresPlayback(t *testing.T) {
	p := &stubStopAfterPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackIdle

	rm, _, handled := m.handlePlaybackKeyAction(actionStopAfterCurrent)
	if !handled {
		t.Fatal("actionStopAfterCurrent should be handled even when idle")
	}
	m = rm.(Model)
	if m.playback.stopAfterCurrent {
		t.Error("flag should remain false when idle")
	}
	if !strings.Contains(m.statusBar.notification.msg, "No track playing") {
		t.Errorf("expected 'No track playing' notification, got %q", m.statusBar.notification.msg)
	}
}

// TestStopAfterCurrentResetsOnPlaybackEnded verifies that handlePlaybackEndedMsg
// clears the flag and shows a distinct notification.
func TestStopAfterCurrentResetsOnPlaybackEnded(t *testing.T) {
	m := New(context.Background())
	m.playback.state = PlaybackPlaying
	m.playback.stopAfterCurrent = true

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.stopAfterCurrent {
		t.Error("expected stopAfterCurrent to be cleared after PlaybackEndedMsg")
	}
	if nm.statusBar.notification.msg != "Stopped after track" {
		t.Errorf("expected 'Stopped after track' notification, got %q", nm.statusBar.notification.msg)
	}
	if nm.playback.state != PlaybackIdle {
		t.Errorf("expected PlaybackIdle, got %v", nm.playback.state)
	}
}

// TestStopAfterCurrentOverridesRepeatAll verifies that stop-after-current
// takes precedence over repeat-all mode.
func TestStopAfterCurrentOverridesRepeatAll(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatAll
	m.playback.stopAfterCurrent = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/music/a.flac", Display: "A"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("expected PlaybackIdle (stop-after overrides repeat-all), got %v", nm.playback.state)
	}
	if len(p.playFromIndexCalls) > 0 {
		t.Error("PlayFromIndex should not be called when stop-after-current overrides repeat-all")
	}
}

// TestStopAfterCurrentClearsOnManualStop verifies that pressing Stop clears
// the stop-after-current flag.
func TestStopAfterCurrentClearsOnManualStop(t *testing.T) {
	p := &stubStopAfterPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.stopAfterCurrent = true
	p.stopAfter = true

	rm, _, _ := m.handlePlaybackKeyAction(actionStop)
	m = rm.(Model)

	if m.playback.stopAfterCurrent {
		t.Error("expected stopAfterCurrent to be cleared after manual stop")
	}
	if p.stopAfter {
		t.Error("expected player stopAfter to be cleared after manual stop")
	}
}

// TestStopAfterCurrentStatusBarIndicator verifies that the "N" indicator
// appears in the status bar when the flag is set.
func TestStopAfterCurrentStatusBarIndicator(t *testing.T) {
	sb := StatusBar{
		artist:           "Art",
		title:            "Song",
		playbackState:    PlaybackPlaying,
		stopAfterCurrent: true,
	}
	out := sb.render(0, styles.Styles{})
	if !strings.Contains(out, "N") {
		t.Errorf("expected 'N' indicator in status bar, got %q", out)
	}

	// Without the flag, "N" should not appear as an indicator.
	sb.stopAfterCurrent = false
	out = sb.render(0, styles.Styles{})
	// The letters "N" could appear in artist/title, so check more specifically
	// by verifying the bold "  N" pattern is absent. Since render uses styled
	// output, we just verify the flag-on case differs from flag-off.
	if strings.Contains(out, "  N") {
		t.Errorf("did not expect '  N' in status bar when flag is off, got %q", out)
	}
}
