package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
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
	if !strings.Contains(m.statusBar.topNotification().msg, "on") {
		t.Errorf("expected notification containing 'on', got %q", m.statusBar.topNotification().msg)
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
	if !strings.Contains(m.statusBar.topNotification().msg, "off") {
		t.Errorf("expected notification containing 'off', got %q", m.statusBar.topNotification().msg)
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

// TestRadioEnqueuePendingSetByTrackChanged verifies that processing a
// TrackChangedMsg with radio enabled and remaining <= radioLowWaterMark sets
// enqueuePending on the returned model. This is a regression test for a
// value-receiver bug where the mutation was lost.
func TestRadioEnqueuePendingSetByTrackChanged(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newModelWithTracks()
	m.batchEnqueuer = be
	m.radio.enabled = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a1.flac", Artist: "Alice", Title: "Track A1"},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentIndex = 0

	next, _ := m.Update(TrackChangedMsg{Hash: "h1", Artist: "Alice", Title: "Track A1", Index: 0})
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
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a1.flac", Artist: "Alice", Title: "Track A1"},
		{ID: 2, Hash: "h2", Path: "/a2.flac", Artist: "Alice", Title: "Track A2"},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentIndex = 0

	// First TrackChangedMsg triggers a radio batch.
	next, cmd1 := m.Update(TrackChangedMsg{Hash: "h1", Artist: "Alice", Title: "Track A1", Index: 0})
	m = next.(Model)
	if cmd1 == nil {
		t.Fatal("expected non-nil cmd (radio batch) from first TrackChangedMsg")
	}
	if !m.radio.enqueuePending {
		t.Fatal("expected enqueuePending=true after first TrackChangedMsg")
	}

	// Second TrackChangedMsg should NOT trigger another batch.
	next, _ = m.Update(TrackChangedMsg{Hash: "h2", Artist: "Alice", Title: "Track A2", Index: 1})
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
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a1.flac"},
	}
	m.playback.currentIndex = 0

	rm, _, _ := m.handlePlaybackKeyAction(actionRadioToggle)
	nm := rm.(Model)

	if !nm.radio.enabled {
		t.Fatal("expected radio.enabled=true after toggle")
	}
	if !nm.radio.enqueuePending {
		t.Error("expected enqueuePending=true after enabling radio with remaining <= radioLowWaterMark")
	}
}

// TestBuildTimeGenreBonus_TopTwo verifies that the top genre for the given hour
// receives +3 and the runner-up receives +1.
func TestBuildTimeGenreBonus_TopTwo(t *testing.T) {
	affinities := []db.HourGenreAffinity{
		{Hour: 10, Genre: "Rock", Count: 8},
		{Hour: 10, Genre: "Jazz", Count: 5},
		{Hour: 10, Genre: "Pop", Count: 3},
	}
	bonus := buildTimeGenreBonus(affinities, 10)
	if bonus["Rock"] != 3 {
		t.Errorf("Rock bonus = %v, want 3", bonus["Rock"])
	}
	if bonus["Jazz"] != 1 {
		t.Errorf("Jazz bonus = %v, want 1", bonus["Jazz"])
	}
	if _, ok := bonus["Pop"]; ok {
		t.Errorf("Pop should not have a bonus, got %v", bonus["Pop"])
	}
}

// TestBuildTimeGenreBonus_OtherHoursIgnored verifies that genres from other
// hours do not appear in the result.
func TestBuildTimeGenreBonus_OtherHoursIgnored(t *testing.T) {
	affinities := []db.HourGenreAffinity{
		{Hour: 8, Genre: "Classical", Count: 10},
		{Hour: 10, Genre: "Rock", Count: 6},
		{Hour: 10, Genre: "Jazz", Count: 4},
	}
	bonus := buildTimeGenreBonus(affinities, 10)
	if _, ok := bonus["Classical"]; ok {
		t.Errorf("Classical (hour 8) should not appear in bonus for hour 10")
	}
	if bonus["Rock"] != 3 {
		t.Errorf("Rock bonus = %v, want 3", bonus["Rock"])
	}
}

// TestBuildTimeGenreBonus_Empty verifies that an empty affinity slice returns
// an empty (non-nil) map.
func TestBuildTimeGenreBonus_Empty(t *testing.T) {
	bonus := buildTimeGenreBonus(nil, 14)
	if len(bonus) != 0 {
		t.Errorf("expected empty map for nil affinities, got %v", bonus)
	}
	bonus2 := buildTimeGenreBonus([]db.HourGenreAffinity{}, 14)
	if len(bonus2) != 0 {
		t.Errorf("expected empty map for empty affinities, got %v", bonus2)
	}
}

// TestRadioStatusBarIndicator verifies that the always-on 4-glyph RSND mode
// cluster contains the "D" radio glyph in both states; only its per-glyph
// styling changes when radioMode toggles (verbose "DJ"/"Auto-DJ" forms are
// gone). The cluster is always 4 columns wide for stable position.
func TestRadioStatusBarIndicator(t *testing.T) {
	sb := StatusBar{
		artist:        "Art",
		title:         "Song",
		playbackState: PlaybackPlaying,
		radioMode:     true,
	}
	out := sb.render(0, styles.Styles{})
	if !strings.Contains(out, "RSND") {
		t.Errorf("expected RSND cluster in status bar when radioMode=true, got %q", out)
	}

	sb.radioMode = false
	out = sb.render(0, styles.Styles{})
	if !strings.Contains(out, "RSND") {
		t.Errorf("expected RSND cluster (always shown) when radioMode=false, got %q", out)
	}
}
