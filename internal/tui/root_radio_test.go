package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestRadioToggleOnOff verifies that actionRadioToggle flips radio.enabled and
// calls WriteRadioMode on the session writer each time.
func TestRadioToggleOnOff(t *testing.T) {
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithSessionWriter(sw))

	// Toggle on.
	rm, _, handled := m.handlePlaybackKeyAction(actionRadioToggle)
	if !handled {
		t.Fatal("actionRadioToggle should be handled")
	}
	m = rm.(Model)
	if !m.radio.enabled {
		t.Error("expected radio.enabled=true after first toggle")
	}
	if len(sw.radioModeCalls) != 1 || !sw.radioModeCalls[0] {
		t.Errorf("expected WriteRadioMode(true), got radioModeCalls=%v", sw.radioModeCalls)
	}
	if !strings.Contains(m.statusBar.notification.msg, "on") {
		t.Errorf("expected notification containing 'on', got %q", m.statusBar.notification.msg)
	}

	// Toggle off.
	rm, _, _ = m.handlePlaybackKeyAction(actionRadioToggle)
	m = rm.(Model)
	if m.radio.enabled {
		t.Error("expected radio.enabled=false after second toggle")
	}
	if len(sw.radioModeCalls) != 2 || sw.radioModeCalls[1] {
		t.Errorf("expected WriteRadioMode(false), got radioModeCalls=%v", sw.radioModeCalls)
	}
	if !strings.Contains(m.statusBar.notification.msg, "off") {
		t.Errorf("expected notification containing 'off', got %q", m.statusBar.notification.msg)
	}
}

// TestRadioSuppressedWhenRepeatAll verifies that radioMaybeEnqueueCmd returns
// nil when the repeat mode is RepeatAll.
func TestRadioSuppressedWhenRepeatAll(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.playback.repeatMode = RepeatAll
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1"}}
	m.playback.currentIndex = 0

	if cmd := m.radioMaybeEnqueueCmd(); cmd != nil {
		t.Error("expected nil cmd when RepeatAll suppresses radio")
	}
}

// TestRadioSuppressedWhenStopAfterCurrent verifies that radioMaybeEnqueueCmd
// returns nil when stopAfterCurrent is armed.
func TestRadioSuppressedWhenStopAfterCurrent(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.playback.stopAfterCurrent = true
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1"}}
	m.playback.currentIndex = 0

	if cmd := m.radioMaybeEnqueueCmd(); cmd != nil {
		t.Error("expected nil cmd when stopAfterCurrent suppresses radio")
	}
}

// TestRadioSuppressedWhenEnqueuePending verifies that a second call to
// radioMaybeEnqueueCmd is suppressed while a batch is in flight.
func TestRadioSuppressedWhenEnqueuePending(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.radio.enqueuePending = true
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1"}}
	m.playback.currentIndex = 0

	if cmd := m.radioMaybeEnqueueCmd(); cmd != nil {
		t.Error("expected nil cmd while enqueuePending=true")
	}
}

// TestRadioLowWaterMarkTrigger verifies that radioMaybeEnqueueCmd returns a
// non-nil Cmd when remaining queue entries <= radioLowWaterMark.
func TestRadioLowWaterMarkTrigger(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newModelWithTracks()
	m.batchEnqueuer = be
	m.radio.enabled = true
	// One entry total, currentIndex=0 → remaining=0.
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a1.flac"}}
	m.playback.currentIndex = 0

	cmd := m.radioMaybeEnqueueCmd()
	if cmd == nil {
		t.Error("expected non-nil cmd when remaining <= radioLowWaterMark")
	}
}

// TestRadioNoTriggerAboveLowWaterMark verifies that radioMaybeEnqueueCmd
// returns nil when enough tracks remain in the queue.
func TestRadioNoTriggerAboveLowWaterMark(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	// currentIndex=0, 4 entries → remaining=3 > radioLowWaterMark(2).
	m.queue.entries = make([]QueueEntry, 4)
	for i := range m.queue.entries {
		m.queue.entries[i] = QueueEntry{ID: int64(i + 1)}
	}
	m.playback.currentIndex = 0

	if cmd := m.radioMaybeEnqueueCmd(); cmd != nil {
		t.Error("expected nil cmd when remaining > radioLowWaterMark")
	}
}

// TestRadioSeedEvolutionOnTrackChanged verifies that handleTrackChangedMsg
// updates radio.seedHash to the newly playing track's hash.
func TestRadioSeedEvolutionOnTrackChanged(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.radio.seedHash = "h1"
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a1.flac", Artist: "Alice", Title: "Track A1"},
		{ID: 2, Hash: "h2", Path: "/a2.flac", Artist: "Alice", Title: "Track A2"},
		{ID: 3, Hash: "h3", Path: "/b1.flac", Artist: "Alice", Title: "Track B1"},
		{ID: 4, Hash: "h4", Path: "/g1.flac", Artist: "Bob", Title: "Track G1"},
	}
	m.playback.state = PlaybackPlaying

	next, _ := m.Update(TrackChangedMsg{Hash: "h2", Artist: "Alice", Title: "Track A2", Index: 1})
	nm := next.(Model)

	if nm.radio.seedHash != "h2" {
		t.Errorf("expected seedHash=%q after TrackChangedMsg, got %q", "h2", nm.radio.seedHash)
	}
}

// TestWithRadioMode verifies that the WithRadioMode option pre-sets radio.enabled.
func TestWithRadioMode(t *testing.T) {
	m := New(context.Background(), WithRadioMode(true))
	if !m.radio.enabled {
		t.Error("expected radio.enabled=true from WithRadioMode(true)")
	}

	m2 := New(context.Background(), WithRadioMode(false))
	if m2.radio.enabled {
		t.Error("expected radio.enabled=false from WithRadioMode(false)")
	}
}

// TestRadioStatusBarIndicator verifies that the status bar shows the DJ
// indicator when radio mode is active.
func TestRadioStatusBarIndicator(t *testing.T) {
	sb := StatusBar{
		artist:        "Art",
		title:         "Song",
		playbackState: PlaybackPlaying,
		radioMode:     true,
	}
	out := sb.render(0, styles.Styles{})
	if !strings.Contains(out, "DJ") {
		t.Errorf("expected 'DJ' indicator in status bar when radioMode=true, got %q", out)
	}

	sb.radioMode = false
	out = sb.render(0, styles.Styles{})
	if strings.Contains(out, "  DJ") {
		t.Errorf("did not expect '  DJ' in status bar when radioMode=false, got %q", out)
	}
}
