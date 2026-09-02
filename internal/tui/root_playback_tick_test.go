package tui

import (
	"charm.land/bubbletea/v2"
	"context"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"math"
	"testing"
	"time"
)

// TestMainPlaybackTickCorrectsDuration verifies that when mpv reports a
// duration that significantly differs from the stored value, the library
// index, queue entries, and DB are corrected.
func TestMainPlaybackTickCorrectsDuration(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 10.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 360.0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 360000},
	}
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "h1"

	nm, cmd := m.handleTickMsg()

	// Library index should be corrected to mpv value.
	if t2, ok := nm.library.Index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
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
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 180.5,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 180500},
	}
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "h1"

	nm, _ := m.handleTickMsg()

	// Library index should NOT be changed.
	if t2, ok := nm.library.Index.FindByHash("h1"); !ok || t2.Duration != 180.5 {
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
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 0},
	}
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "h1"

	nm, _ := m.handleTickMsg()

	if t2, ok := nm.library.Index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
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
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 360.0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 360000},
	}
	m.playback.state = playback.StatePlaying
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
	if t2, ok := nm2.library.Index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
		t.Fatalf("expected duration to remain 180.0, got %v", t2.Duration)
	}
}

// TestTrackChangedResetsCorrectFlag verifies that the durationCorrected flag
// is reset when a new track starts.
func TestTrackChangedResetsCorrectFlag(t *testing.T) {
	m := New(context.Background())
	m.playback.durationCorrected = true
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h2", Path: "/b.flac"}}

	nm, _ := m.handleTrackChangedMsg(playback.TrackChangedMsg{
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
	m.playback.state = playback.StatePlaying
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
	m.playback.state = playback.StatePlaying
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

// TestTickMsgRejectsNonFiniteElapsedTotal verifies that NaN/Inf readings from
// mpv do not propagate into playback progress fields. Without the guard, a
// transient bad mpv reading turns peakElapsedSecs into NaN, which then
// corrupts the running session listen-seconds counter on track change.
func TestTickMsgRejectsNonFiniteElapsedTotal(t *testing.T) {
	cases := []struct {
		name           string
		elapsed, total float64
	}{
		{"nan elapsed", math.NaN(), 200.0},
		{"inf elapsed", math.Inf(1), 200.0},
		{"nan total", 10.0, math.NaN()},
		{"inf total", 10.0, math.Inf(1)},
		{"negative elapsed", -5.0, 200.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sp := &stubElapsedReader{elapsed: tc.elapsed, total: tc.total}
			m := New(context.Background(), WithPlayer(sp))
			m.playback.state = playback.StatePlaying
			m.playback.currentHash = "h1"
			m.playback.elapsedSecs = 30.0
			m.playback.totalSecs = 200.0
			m.playback.peakElapsedSecs = 30.0
			m.playback.peakTotalSecs = 200.0

			nm, _ := m.handleTickMsg()

			if math.IsNaN(nm.playback.elapsedSecs) || math.IsInf(nm.playback.elapsedSecs, 0) {
				t.Errorf("elapsedSecs corrupted: %v", nm.playback.elapsedSecs)
			}
			if math.IsNaN(nm.playback.totalSecs) || math.IsInf(nm.playback.totalSecs, 0) {
				t.Errorf("totalSecs corrupted: %v", nm.playback.totalSecs)
			}
			if math.IsNaN(nm.playback.peakElapsedSecs) || math.IsInf(nm.playback.peakElapsedSecs, 0) {
				t.Errorf("peakElapsedSecs corrupted: %v", nm.playback.peakElapsedSecs)
			}
			if nm.playback.peakElapsedSecs != 30.0 {
				t.Errorf("peakElapsedSecs should retain prior value, got %v", nm.playback.peakElapsedSecs)
			}
		})
	}
}

// TestHandlePlayRecordedMsgDedupesExistingHash verifies that re-playing a track
// already present in the History source list does not stack a duplicate at the
// top — the older copy must be dropped so the user sees one row per track.
func TestHandlePlayRecordedMsgDedupesExistingHash(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Title: "A"})
	m.library.Index.Insert(library.Track{Hash: "h2", Title: "B"})
	m.library.Mode = libtui.LibraryModeHistory
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Title: "A", LastPlayedAt: 100},
		{Hash: "h2", Title: "B", LastPlayedAt: 90},
	}

	nm := m.handlePlayRecordedMsg(playback.PlayRecordedMsg{Hash: "h1", PlayedAt: 200, Counted: true})

	if len(nm.library.SourceTracks) != 2 {
		t.Fatalf("expected 2 tracks in history after dedup, got %d", len(nm.library.SourceTracks))
	}
	if nm.library.SourceTracks[0].Hash != "h1" || nm.library.SourceTracks[0].LastPlayedAt != 200 {
		t.Errorf("expected h1 at top with PlayedAt=200, got %+v", nm.library.SourceTracks[0])
	}
	if nm.library.SourceTracks[1].Hash != "h2" {
		t.Errorf("expected h2 second, got %q", nm.library.SourceTracks[1].Hash)
	}
}
