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

// TestRadioNoTriggerWithOutOfRangeCurrentIndex verifies stale out-of-range
// playback indices do not make radio think the queue is nearly exhausted.
func TestRadioNoTriggerWithOutOfRangeCurrentIndex(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.batchEnqueuer = &stubBatchEnqueuer{}
	m.queue.entries = make([]QueueEntry, 4)
	for i := range m.queue.entries {
		m.queue.entries[i] = QueueEntry{ID: int64(i + 1)}
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

// TestRadioBatchDiscardedWhenDisabled verifies that a RadioBatchEnqueuedMsg
// arriving after the user has toggled radio off does not resurrect playback by
// appending tracks to a queue the user just cleared/disabled.
func TestRadioBatchDiscardedWhenDisabled(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = false
	m.radio.enqueuePending = true
	beforeLen := len(m.queue.entries)

	msg := RadioBatchEnqueuedMsg{
		Entries: []QueueEntry{
			{ID: 99, Hash: "hx", Path: "/x.flac", Artist: "X", Title: "Y"},
		},
		QueueIDs: []int64{99},
		WasEmpty: true,
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset even when batch is discarded")
	}
	if len(nm.queue.entries) != beforeLen {
		t.Errorf("queue must not grow when radio is disabled; before=%d after=%d",
			beforeLen, len(nm.queue.entries))
	}
	if nm.playback.state == PlaybackPlaying {
		t.Error("playback must not start when radio is disabled")
	}
}

// TestRadioSeedTagsSnapshotted verifies that the radio enqueue closure copies
// the seed tag set instead of holding a live reference to m.trackTags[seedHash].
// We exercise the race by mutating the live map concurrently with the closure
// running: the snapshot must isolate the closure from those mutations.
// Run with `go test -race` for the strongest signal.
func TestRadioSeedTagsSnapshotted(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newModelWithTracks()
	m.batchEnqueuer = be
	m.radio.enabled = true
	m.radio.seedHash = "h1"
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a1.flac"}}
	m.playback.currentIndex = 0
	m.trackTags = map[string]map[int64]struct{}{
		"h1": {7: {}, 8: {}},
		// Populate other tracks so ScoreAll enters the shared-tag inner loop
		// that ranges over Seed.TagIDs for each candidate.
		"h2": {7: {}},
		"h3": {8: {}},
		"h4": {7: {}, 8: {}},
		"h5": {7: {}},
		"h6": {8: {}},
	}

	cmd := m.radioEnqueueCmd()
	if cmd == nil {
		t.Fatal("expected non-nil radio enqueue cmd")
	}

	// Concurrently mutate the live tag set while the closure runs. Without
	// the snapshot the closure's range loop over seed.TagIDs would race
	// with these writes and `go test -race` would flag it.
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				for i := 0; i < 50; i++ {
					m.trackTags["h1"][int64(i)] = struct{}{}
					delete(m.trackTags["h1"], int64(i))
				}
			}
		}
	}()

	// Drive the closure many times so its read window overlaps the mutator.
	for i := 0; i < 50; i++ {
		_ = cmd()
	}
	close(stop)
	<-done
}

// TestRadioBatchDiscardedWhenQueueClearedInFlight verifies that a
// RadioBatchEnqueuedMsg arriving with WasEmpty=false (radio triggered at
// low-water-mark with a non-empty queue) is discarded when the queue is empty
// by the time the batch arrives. Without this guard a Shift+C clear that
// races a radio enqueue would silently re-populate the cleared queue with
// surprise tracks the user did not request.
func TestRadioBatchDiscardedWhenQueueClearedInFlight(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.radio.enqueuePending = true
	// Simulate post-clear state: queue empty, queueExhausted=false (cleared
	// by resetPlaybackState), state Idle.
	m.queue.entries = nil
	m.playback.queueExhausted = false
	m.playback.state = PlaybackIdle

	msg := RadioBatchEnqueuedMsg{
		Entries: []QueueEntry{
			{ID: 50, Hash: "hr", Path: "/r.flac", Artist: "R", Title: "S"},
		},
		QueueIDs: []int64{50},
		WasEmpty: false, // captured when queue still had entries
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset even when batch is discarded")
	}
	if len(nm.queue.entries) != 0 {
		t.Errorf("cleared queue must stay empty; got %d entries", len(nm.queue.entries))
	}
	if nm.playback.state == PlaybackPlaying {
		t.Error("playback must not start when batch is discarded after queue clear")
	}
}

// TestRadioBatchAppliedWhenQueueWasEmptyAtEnqueueStart verifies the
// queue-cleared-in-flight guard does not over-fire: a batch whose closure was
// launched with WasEmpty=true (e.g. after PlaybackEndedMsg with no remaining
// queue) must still apply and auto-start playback on arrival.
func TestRadioBatchAppliedWhenQueueWasEmptyAtEnqueueStart(t *testing.T) {
	p := &stubCommandPlayer{}
	m := newModelWithTracks()
	m.player = p
	m.radio.enabled = true
	m.radio.enqueuePending = true
	m.queue.entries = nil
	m.playback.state = PlaybackIdle

	msg := RadioBatchEnqueuedMsg{
		Entries: []QueueEntry{
			{ID: 60, Hash: "hq", Path: "/q.flac", Artist: "Q", Title: "T"},
		},
		QueueIDs: []int64{60},
		WasEmpty: true, // queue truly was empty when enqueue fired
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset after applying batch")
	}
	if len(nm.queue.entries) != 1 {
		t.Errorf("expected 1 entry appended, got %d", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackPlaying {
		t.Errorf("expected PlaybackPlaying after WasEmpty auto-start, got %v", nm.playback.state)
	}
}

// TestRadioBatchAppliedOnNaturalExhaustion verifies that a RadioBatchEnqueuedMsg
// with WasEmpty=false (launched when the queue still held tracks) is NOT
// discarded just because the queue finished playing in flight. The
// distinguishing signal is playback.queueExhausted, which handlePlaybackEndedMsg
// sets when the last track ends naturally (vs. resetPlaybackState clearing it
// after a user Shift+C clear). Without this, the user is left with an empty
// queue and a dead radio session whenever a batch races a natural exhaustion.
func TestRadioBatchAppliedOnNaturalExhaustion(t *testing.T) {
	p := &stubCommandPlayer{}
	m := newModelWithTracks()
	m.player = p
	m.radio.enabled = true
	m.radio.enqueuePending = true
	// Natural exhaustion path: queue is empty but queueExhausted=true (set by
	// handlePlaybackEndedMsg) so auto-resume should fire when the batch lands.
	m.queue.entries = nil
	m.playback.queueExhausted = true
	m.playback.state = PlaybackIdle

	msg := RadioBatchEnqueuedMsg{
		Entries: []QueueEntry{
			{ID: 70, Hash: "hx", Path: "/x.flac", Artist: "X", Title: "T"},
		},
		QueueIDs: []int64{70},
		WasEmpty: false, // captured at low-water-mark before the last track ended
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset after applying batch")
	}
	if len(nm.queue.entries) != 1 {
		t.Fatalf("expected 1 entry appended on natural exhaustion, got %d", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackPlaying {
		t.Errorf("expected PlaybackPlaying after natural-exhaust auto-resume, got %v", nm.playback.state)
	}
	if nm.playback.queueExhausted {
		t.Error("queueExhausted should be cleared after auto-resume")
	}
}
