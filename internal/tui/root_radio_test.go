package tui

import (
	"context"
	"github.com/sglavoie/muse/internal/tui/styles"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// TestRadioToggleOnOff verifies that actionRadioToggle flips radio.enabled and
// calls WriteRadioMode on the session writer each time.
func TestRadioToggleOnOff(t *testing.T) {
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithSessionWriter(sw))

	// Toggle on.
	rm, _, handled := m.handlePlaybackKeyAction(keys.ActionRadioToggle)
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
	if !strings.Contains(m.statusBar.TopNotification().Msg, "on") {
		t.Errorf("expected notification containing 'on', got %q", m.statusBar.TopNotification().Msg)
	}

	// Toggle off.
	rm, _, _ = m.handlePlaybackKeyAction(keys.ActionRadioToggle)
	m = rm.(Model)
	if m.radio.enabled {
		t.Error("expected radio.enabled=false after second toggle")
	}
	if len(sw.radioModeCalls) != 2 || sw.radioModeCalls[1] {
		t.Errorf("expected WriteRadioMode(false), got radioModeCalls=%v", sw.radioModeCalls)
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "off") {
		t.Errorf("expected notification containing 'off', got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestRadioSuppressedWhenRepeatAll verifies that radioMaybeEnqueueCmd returns
// nil when the repeat mode is RepeatAll.
func TestRadioSuppressedWhenRepeatAll(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.playback.repeatMode = playback.RepeatAll
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "h1"}}
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
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "h1"}}
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
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "h1"}}
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
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "h1", Path: "/a1.flac"}}
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
	m.queue.Entries = make([]queue.Entry, 4)
	for i := range m.queue.Entries {
		m.queue.Entries[i] = queue.Entry{ID: int64(i + 1)}
	}
	m.playback.currentIndex = 0

	if cmd := m.radioMaybeEnqueueCmd(); cmd != nil {
		t.Error("expected nil cmd when remaining > radioLowWaterMark")
	}
}

// TestRadioNoTriggerWithOutOfRangeCurrentIndex verifies stale out-of-range
// playback indices do not make radio think the queue is nearly exhausted.
func TestRadioNoTriggerWithOutOfRangeCurrentIndex(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.batchEnqueuer = &stubBatchEnqueuer{}
	m.queue.Entries = make([]queue.Entry, 4)
	for i := range m.queue.Entries {
		m.queue.Entries[i] = queue.Entry{ID: int64(i + 1)}
	}
	m.playback.currentIndex = 99

	if cmd := m.radioMaybeEnqueueCmd(); cmd != nil {
		t.Error("expected nil cmd when stale currentIndex is out of range and queue has ample entries")
	}
}

// TestRadioSeedEvolutionOnTrackChanged verifies that handleTrackChangedMsg
// updates radio.seedHash to the newly playing track's hash.
func TestRadioSeedEvolutionOnTrackChanged(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.radio.seedHash = "h1"
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/a1.flac", Artist: "Alice", Title: "Track A1"},
		{ID: 2, Hash: "h2", Path: "/a2.flac", Artist: "Alice", Title: "Track A2"},
		{ID: 3, Hash: "h3", Path: "/b1.flac", Artist: "Alice", Title: "Track B1"},
		{ID: 4, Hash: "h4", Path: "/g1.flac", Artist: "Bob", Title: "Track G1"},
	}
	m.playback.state = playback.StatePlaying

	next, _ := m.Update(playback.TrackChangedMsg{Hash: "h2", Artist: "Alice", Title: "Track A2", Index: 1})
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

// TestRadioEnqueuePendingSetByTrackChanged verifies that processing a
// TrackChangedMsg with radio enabled and remaining <= radioLowWaterMark sets
// enqueuePending on the returned model. This is a regression test for a
// value-receiver bug where the mutation was lost.
func TestRadioEnqueuePendingSetByTrackChanged(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newModelWithTracks()
	m.batchEnqueuer = be
	m.radio.enabled = true
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/a1.flac", Artist: "Alice", Title: "Track A1"},
	}
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 0

	next, _ := m.Update(playback.TrackChangedMsg{Hash: "h1", Artist: "Alice", Title: "Track A1", Index: 0})
	nm := next.(Model)

	if !nm.radio.enqueuePending {
		t.Error("expected enqueuePending=true after TrackChangedMsg with remaining <= radioLowWaterMark")
	}
}

// TestRadioNoDuplicateBatch verifies that a second TrackChangedMsg does not
// trigger another radio batch while one is already in flight.
func TestRadioNoDuplicateBatch(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newModelWithTracks()
	m.batchEnqueuer = be
	m.radio.enabled = true
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/a1.flac", Artist: "Alice", Title: "Track A1"},
		{ID: 2, Hash: "h2", Path: "/a2.flac", Artist: "Alice", Title: "Track A2"},
	}
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 0

	// First TrackChangedMsg triggers a radio batch.
	next, cmd1 := m.Update(playback.TrackChangedMsg{Hash: "h1", Artist: "Alice", Title: "Track A1", Index: 0})
	m = next.(Model)
	if cmd1 == nil {
		t.Fatal("expected non-nil cmd (radio batch) from first TrackChangedMsg")
	}
	if !m.radio.enqueuePending {
		t.Fatal("expected enqueuePending=true after first TrackChangedMsg")
	}

	// Second TrackChangedMsg should NOT trigger another batch.
	next, _ = m.Update(playback.TrackChangedMsg{Hash: "h2", Artist: "Alice", Title: "Track A2", Index: 1})
	m = next.(Model)

	cmd := m.radioMaybeEnqueueCmd()
	if cmd != nil {
		t.Error("expected nil radio cmd while enqueuePending=true (duplicate batch suppressed)")
	}
}

// TestRadioEnqueuePendingSetByToggle verifies that enabling radio mode with
// remaining <= radioLowWaterMark sets enqueuePending on the returned model.
func TestRadioEnqueuePendingSetByToggle(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newModelWithTracks()
	m.batchEnqueuer = be
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/a1.flac"},
	}
	m.playback.currentIndex = 0

	rm, _, _ := m.handlePlaybackKeyAction(keys.ActionRadioToggle)
	nm := rm.(Model)

	if !nm.radio.enabled {
		t.Fatal("expected radio.enabled=true after toggle")
	}
	if !nm.radio.enqueuePending {
		t.Error("expected enqueuePending=true after enabling radio with remaining <= radioLowWaterMark")
	}
}

// TestRadioStatusBarIndicator verifies that the always-on 4-glyph RSND mode
// cluster contains the "D" radio glyph in both states; only its per-glyph
// styling changes when radioMode toggles (verbose "DJ"/"Auto-DJ" forms are
// gone). The cluster is always 4 columns wide for stable position.
func TestRadioStatusBarIndicator(t *testing.T) {
	sb := statusbar.StatusBar{
		Artist:        "Art",
		Title:         "Song",
		PlaybackState: playback.StatePlaying,
		RadioMode:     true,
	}
	out := sb.Render(0, styles.Styles{})
	if !strings.Contains(out, "RSND") {
		t.Errorf("expected RSND cluster in status bar when radioMode=true, got %q", out)
	}

	sb.RadioMode = false
	out = sb.Render(0, styles.Styles{})
	if !strings.Contains(out, "RSND") {
		t.Errorf("expected RSND cluster (always shown) when radioMode=false, got %q", out)
	}
}
