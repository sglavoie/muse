package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// TestMainPlaybackTickCorrectsDuration verifies that when mpv reports a
// duration that significantly differs from the stored value, the library
// index, queue entries, and DB are corrected.
func TestMainPlaybackTickCorrectsDuration(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 10.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 360.0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 360000},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, cmd := m.handleTickMsg()

	// Library index should be corrected to mpv value.
	if t2, ok := nm.library.index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
		t.Fatalf("expected library duration 180.0, got %v", t2.Duration)
	}
	// Queue entry should be corrected.
	if nm.queue.entries[0].DurationMs != 180000 {
		t.Fatalf("expected queue DurationMs 180000, got %d", nm.queue.entries[0].DurationMs)
	}
	// Flag should be set.
	if !nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected to be set")
	}
	// DB write should be scheduled.
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("expected BatchMsg")
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		msg := make(chan tea.Msg, 1)
		go func(fn tea.Cmd) { msg <- fn() }(c)
		select {
		case <-msg:
		case <-time.After(50 * time.Millisecond):
		}
	}
	if len(dw.calls) != 1 {
		t.Fatalf("expected 1 DB write call, got %d", len(dw.calls))
	}
	if dw.calls[0].hash != "h1" || dw.calls[0].durationMs != 180000 {
		t.Fatalf("unexpected DB write: %+v", dw.calls[0])
	}
}

// TestMainPlaybackTickSkipsSmallDifference verifies that when the stored
// duration is within 2 seconds of mpv's total, no correction occurs.
func TestMainPlaybackTickSkipsSmallDifference(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 10.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 180.5,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 180500},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, _ := m.handleTickMsg()

	// Library index should NOT be changed.
	if t2, ok := nm.library.index.FindByHash("h1"); !ok || t2.Duration != 180.5 {
		t.Fatalf("expected library duration to stay 180.5, got %v", t2.Duration)
	}
	if nm.queue.entries[0].DurationMs != 180500 {
		t.Fatalf("expected queue DurationMs to stay 180500, got %d", nm.queue.entries[0].DurationMs)
	}
	if !nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected to be set (to prevent re-checking)")
	}
	if len(dw.calls) != 0 {
		t.Fatalf("expected 0 DB write calls, got %d", len(dw.calls))
	}
}

// TestMainPlaybackTickFillsZeroDuration verifies that when a track has
// Duration=0, the main playback tick fills it from mpv.
func TestMainPlaybackTickFillsZeroDuration(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 5.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 0},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, _ := m.handleTickMsg()

	if t2, ok := nm.library.index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
		t.Fatalf("expected library duration 180.0, got %v", t2.Duration)
	}
	if nm.queue.entries[0].DurationMs != 180000 {
		t.Fatalf("expected queue DurationMs 180000, got %d", nm.queue.entries[0].DurationMs)
	}
}

// TestMainPlaybackTickCorrectOnceOnly verifies that after the flag is set,
// subsequent ticks do not trigger additional corrections.
func TestMainPlaybackTickCorrectOnceOnly(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 10.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 360.0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 360000},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	// First tick corrects.
	nm, _ := m.handleTickMsg()
	if !nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected after first tick")
	}

	// Change the stub to report a different total.
	sp.total = 999.0
	nm2, _ := nm.handleTickMsg()

	// Duration should remain at the first correction (180), not 999.
	if t2, ok := nm2.library.index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
		t.Fatalf("expected duration to remain 180.0, got %v", t2.Duration)
	}
}

// TestTrackChangedResetsCorrectFlag verifies that the durationCorrected flag
// is reset when a new track starts.
func TestTrackChangedResetsCorrectFlag(t *testing.T) {
	m := New(context.Background())
	m.playback.durationCorrected = true

	nm, _ := m.handleTrackChangedMsg(TrackChangedMsg{
		Hash: "h2", Index: 0, Artist: "B", Title: "T2",
	})

	if nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected to be reset after track change")
	}
}

// TestTickMsgUpdatesPeakElapsed verifies that the tick handler updates the
// peak elapsed fields when the elapsed value increases.
func TestTickMsgUpdatesPeakElapsed(t *testing.T) {
	sp := &stubElapsedReader{elapsed: 45.0, total: 200.0}
	m := New(context.Background(), WithPlayer(sp))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, _ := m.handleTickMsg()

	if nm.playback.peakElapsedSecs != 45.0 {
		t.Fatalf("expected peakElapsedSecs=45.0, got %v", nm.playback.peakElapsedSecs)
	}
	if nm.playback.peakTotalSecs != 200.0 {
		t.Fatalf("expected peakTotalSecs=200.0, got %v", nm.playback.peakTotalSecs)
	}
}

// TestTickMsgDoesNotDecreasePeak verifies that the peak does not decrease when
// the tick reports a smaller elapsed value (e.g. after a gapless transition
// where mpv is already playing the next track).
func TestTickMsgDoesNotDecreasePeak(t *testing.T) {
	sp := &stubElapsedReader{elapsed: 0.5, total: 300.0}
	m := New(context.Background(), WithPlayer(sp))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"
	m.playback.peakElapsedSecs = 180.0
	m.playback.peakTotalSecs = 200.0

	nm, _ := m.handleTickMsg()

	if nm.playback.peakElapsedSecs != 180.0 {
		t.Fatalf("expected peakElapsedSecs to stay 180.0, got %v", nm.playback.peakElapsedSecs)
	}
	if nm.playback.peakTotalSecs != 200.0 {
		t.Fatalf("expected peakTotalSecs to stay 200.0, got %v", nm.playback.peakTotalSecs)
	}
}
