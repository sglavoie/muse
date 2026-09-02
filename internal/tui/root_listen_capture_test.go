package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// listensByHash counts RecordListen calls per hash so the exactly-once
// assertions read as "this track was recorded N times" rather than as an
// offset into a shared slice.
func listensByHash(calls []db.PlayRecord) map[string]int {
	counts := make(map[string]int, len(calls))
	for _, c := range calls {
		counts[c.Hash]++
	}
	return counts
}

// armedModel returns a model mid-listen on hash: playing, armed, and with
// enough accumulated time that the listen clears both the capture floor and the
// play-count threshold.
func armedModel(t *testing.T, hr *stubHistoryRecorder, opts ...Option) Model {
	t.Helper()
	m := New(context.Background(), append([]Option{WithHistoryRecorder(hr)}, opts...)...)
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "h1"
	m.playback.currentIndex = 0
	m.playback.listenedSecs = 200
	m.playback.peakTotalSecs = 300
	return m.armPendingListen("h1", "Alpha")
}

// TestRepeatAllWrapRecordsListenOnce covers the regression the ledger exists
// for: the repeat-all wrap ends the listen and then immediately replays the
// same hash, so a hash-comparing capture would record it twice.
func TestRepeatAllWrapRecordsListenOnce(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := armedModel(t, hr, WithPlayer(&stubPlayer{}))
	m.playback.repeatMode = playback.RepeatAll
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a.flac"}}

	next, cmd := m.Update(playback.PlaybackEndedMsg{})
	m = next.(Model)
	drainCmd(cmd)

	next, cmd = m.Update(playback.TrackChangedMsg{Hash: "h1", Index: 0, Artist: "Alpha", Title: "One"})
	m = next.(Model)
	drainCmd(cmd)

	if got := listensByHash(hr.calls)["h1"]; got != 1 {
		t.Fatalf("expected 1 RecordListen for h1 across the repeat-all wrap, got %d (%+v)", got, hr.calls)
	}
}

// TestReconnectRecordsListenOnce verifies that an mpv disconnect ends the
// listen and the start-file event that follows the reconnect does not record it
// a second time.
func TestReconnectRecordsListenOnce(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := armedModel(t, hr)
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a.flac"}}

	next, cmd := m.Update(playback.PlayerDisconnectedMsg{})
	m = next.(Model)
	drainCmd(cmd)

	next, cmd = m.Update(playback.TrackChangedMsg{Hash: "h1", Index: 0, Artist: "Alpha", Title: "One"})
	m = next.(Model)
	drainCmd(cmd)

	if got := listensByHash(hr.calls)["h1"]; got != 1 {
		t.Fatalf("expected 1 RecordListen for h1 across the reconnect, got %d (%+v)", got, hr.calls)
	}
}

// TestTrackChangeMidQueueRecordsListenOnce covers the ordinary end-of-file
// transition, where only a TrackChangedMsg arrives.
func TestTrackChangeMidQueueRecordsListenOnce(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := armedModel(t, hr)
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac"},
		{ID: 2, Hash: "h2", Path: "/b.flac"},
	}

	next, cmd := m.Update(playback.TrackChangedMsg{Hash: "h2", Index: 1, Artist: "Beta", Title: "Two"})
	m = next.(Model)
	drainCmd(cmd)

	counts := listensByHash(hr.calls)
	if counts["h1"] != 1 {
		t.Fatalf("expected 1 RecordListen for h1, got %d (%+v)", counts["h1"], hr.calls)
	}
	if counts["h2"] != 0 {
		t.Fatalf("expected no RecordListen for the incoming track, got %d", counts["h2"])
	}
}

// TestStopRecordsListenOnce verifies that stopping ends the listen and a second
// stop has nothing left to record.
func TestStopRecordsListenOnce(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := armedModel(t, hr, WithPlayer(&stubCommandPlayer{}))
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a.flac"}}

	stop := tea.KeyPressMsg{Code: 's', Text: "s"}

	next, cmd := m.Update(stop)
	m = next.(Model)
	drainCmd(cmd)
	if got := listensByHash(hr.calls)["h1"]; got != 1 {
		t.Fatalf("expected 1 RecordListen after stop, got %d (%+v)", got, hr.calls)
	}

	next, cmd = m.Update(stop)
	_ = next.(Model)
	drainCmd(cmd)
	if got := listensByHash(hr.calls)["h1"]; got != 1 {
		t.Fatalf("expected the second stop to record nothing, got %d total (%+v)", got, hr.calls)
	}
}

// TestClearQueueWhilePlayingRecordsListenOnce verifies that clearing the queue
// mid-track captures the listen, and that the stale TrackChangedMsg mpv's
// gapless prefetch may deliver afterwards does not add a second record.
func TestClearQueueWhilePlayingRecordsListenOnce(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := armedModel(t, hr, WithPlayer(&stubCommandPlayer{}))
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a.flac"}}

	next, cmd := m.Update(ClearQueueMsg{})
	m = next.(Model)
	drainCmd(cmd)
	if got := listensByHash(hr.calls)["h1"]; got != 1 {
		t.Fatalf("expected 1 RecordListen after clearing the queue, got %d (%+v)", got, hr.calls)
	}

	next, cmd = m.Update(playback.TrackChangedMsg{Hash: "h1", Index: 0, Artist: "Alpha", Title: "One"})
	_ = next.(Model)
	drainCmd(cmd)
	if got := listensByHash(hr.calls)["h1"]; got != 1 {
		t.Fatalf("expected the trailing TrackChangedMsg to record nothing, got %d total (%+v)", got, hr.calls)
	}
}

// TestDequeueCurrentRecordsListenOnce verifies that removing the playing track
// captures its listen even though playback continues into the next entry.
func TestDequeueCurrentRecordsListenOnce(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := armedModel(t, hr, WithPlayer(&stubPlayerQueueRemover{}))
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac"},
		{ID: 2, Hash: "h2", Path: "/b.flac"},
	}

	next, cmd := m.Update(DequeueTrackMsg{Index: 0})
	m = next.(Model)
	drainCmd(cmd)

	if got := listensByHash(hr.calls)["h1"]; got != 1 {
		t.Fatalf("expected 1 RecordListen after dequeuing the current track, got %d (%+v)", got, hr.calls)
	}
}

// TestPlayQueueIndexRecordsPreviousListenBeforeReset covers the manual
// queue-selection transition: handlePlayQueueIndexMsg optimistically resets the
// progress fields, so the listen has to be ended above that reset or its
// measurement is zeroed before the TrackChangedMsg that follows can consume it.
func TestPlayQueueIndexRecordsPreviousListenBeforeReset(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := armedModel(t, hr, WithPlayer(&stubPlayer{}))
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Alpha", Title: "One"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Beta", Title: "Two"},
	}

	next, cmd := m.Update(playback.PlayQueueIndexMsg{Index: 1})
	m = next.(Model)
	drainCmd(cmd)

	if got := listensByHash(hr.calls)["h1"]; got != 1 {
		t.Fatalf("expected 1 RecordListen for h1 after selecting another queue row, got %d (%+v)", got, hr.calls)
	}
	rec := hr.calls[0]
	if rec.ListenedSecs != 200 {
		t.Errorf("ListenedSecs = %d, want the pre-reset 200", rec.ListenedSecs)
	}
	if rec.DurationSecs != 300 {
		t.Errorf("DurationSecs = %v, want the pre-reset 300", rec.DurationSecs)
	}
	if !rec.Counted {
		t.Error("a 200s listen of a 300s track must be counted")
	}

	// The player confirms the jump; the previous listen must not be recorded
	// again, and the incoming track must not be recorded at all.
	next, cmd = m.Update(playback.TrackChangedMsg{Hash: "h2", Index: 1, Artist: "Beta", Title: "Two"})
	m = next.(Model)
	drainCmd(cmd)

	counts := listensByHash(hr.calls)
	if counts["h1"] != 1 {
		t.Fatalf("expected the confirming TrackChangedMsg to record nothing more for h1, got %d (%+v)", counts["h1"], hr.calls)
	}
	if counts["h2"] != 0 {
		t.Fatalf("expected no RecordListen for the incoming track, got %d (%+v)", counts["h2"], hr.calls)
	}
	if !m.playback.pending.armed || m.playback.pending.hash != "h2" {
		t.Fatalf("expected the incoming track to be armed, got %+v", m.playback.pending)
	}
}

// TestPlayQueueIndexFailureKeepsPreviousListenOnce verifies that a
// PlayFromIndex that fails after the optimistic update neither erases the
// already-flushed previous listen nor produces a second copy of it.
func TestPlayQueueIndexFailureKeepsPreviousListenOnce(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := armedModel(t, hr, WithPlayer(&stubPlayer{playFromIndexErr: errStubPlay}))
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Alpha", Title: "One"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Beta", Title: "Two"},
	}

	next, cmd := m.Update(playback.PlayQueueIndexMsg{Index: 1})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("PlayQueueIndexMsg must return an async Cmd")
	}
	// The batch carries both the flush and the PlayFromIndex call; drive it and
	// route the failure result back through Update the way the runtime would.
	drainCmd(cmd)
	next, cmd = m.Update(playback.PlayFromIndexResultMsg{Index: 1, Path: "/b.flac", Hash: "h2", Err: errStubPlay})
	m = next.(Model)
	drainCmd(cmd)

	if m.playback.state != playback.StateIdle {
		t.Fatalf("precondition failed: the failure must reset playback, got %v", m.playback.state)
	}
	counts := listensByHash(hr.calls)
	if counts["h1"] != 1 {
		t.Fatalf("expected h1 recorded exactly once across the failed jump, got %d (%+v)", counts["h1"], hr.calls)
	}
	if counts["h2"] != 0 {
		t.Fatalf("expected no RecordListen for the track that never started, got %d (%+v)", counts["h2"], hr.calls)
	}
}

// TestPreviewDoesNotRecordListen verifies that auditioning a track leaves the
// main listen untouched: previewing is not listening.
func TestPreviewDoesNotRecordListen(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := armedModel(t, hr)
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a.flac"}}

	next, cmd := m.Update(previewStartedMsg{overlay: overlay.PreviewOverlay{Hash: "h2", Artist: "Beta", Title: "Two"}})
	m = next.(Model)
	drainCmd(cmd)

	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	drainCmd(cmd)
	if m.overlay.preview != nil {
		t.Fatal("precondition failed: escape must dismiss the preview overlay")
	}

	if len(hr.calls) != 0 {
		t.Fatalf("expected no RecordListen across preview start/dismiss, got %+v", hr.calls)
	}
	if !m.playback.pending.armed || m.playback.pending.hash != "h1" {
		t.Fatalf("expected the main listen to stay armed, got %+v", m.playback.pending)
	}
}

// TestListenFloorAndCountedFlag pins the two independent thresholds: the floor
// below which a listen is not recorded at all, and the higher bar above which
// it counts as a play.
func TestListenFloorAndCountedFlag(t *testing.T) {
	tests := []struct {
		name        string
		listened    float64
		total       float64
		wantRecords int
		wantCounted bool
	}{
		{"below floor", 2, 300, 0, false},
		{"above floor below threshold", 5, 300, 1, false},
		{"above threshold", 200, 300, 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hr := &stubHistoryRecorder{}
			m := New(context.Background(), WithHistoryRecorder(hr))
			m.playback.state = playback.StatePlaying
			m.playback.currentHash = "h1"
			m = m.armPendingListen("h1", "Alpha")
			m.playback.listenedSecs = tc.listened
			m.playback.peakTotalSecs = tc.total

			next, cmd := m.Update(playback.PlaybackEndedMsg{})
			_ = next.(Model)
			drainCmd(cmd)

			if len(hr.calls) != tc.wantRecords {
				t.Fatalf("expected %d RecordListen calls, got %d (%+v)", tc.wantRecords, len(hr.calls), hr.calls)
			}
			if tc.wantRecords == 0 {
				return
			}
			if hr.calls[0].Counted != tc.wantCounted {
				t.Errorf("Counted = %v, want %v", hr.calls[0].Counted, tc.wantCounted)
			}
			if hr.calls[0].ListenedSecs != int(tc.listened) {
				t.Errorf("ListenedSecs = %d, want %d", hr.calls[0].ListenedSecs, int(tc.listened))
			}
		})
	}
}

// TestTakePendingListenReturnsRecordOnce verifies the quit path can drain the
// in-flight listen exactly once, so a second caller cannot double-write it.
func TestTakePendingListenReturnsRecordOnce(t *testing.T) {
	m := New(context.Background())
	m.playback.state = playback.StatePlaying
	m = m.armPendingListen("h1", "Alpha")
	m.playback.listenedSecs = 200
	m.playback.peakTotalSecs = 300

	rec, ok := m.TakePendingListen()
	if !ok {
		t.Fatal("expected TakePendingListen to yield the in-flight listen")
	}
	if rec.Hash != "h1" || !rec.Counted || rec.ListenedSecs != 200 {
		t.Fatalf("unexpected record: %+v", rec)
	}

	if _, ok := m.TakePendingListen(); ok {
		t.Fatal("expected the second TakePendingListen to yield nothing")
	}
}

// TestTakePendingListenSkipsBelowFloor verifies the quit path honours the same
// floor as every other end site.
func TestTakePendingListenSkipsBelowFloor(t *testing.T) {
	m := New(context.Background())
	m.playback.state = playback.StatePlaying
	m = m.armPendingListen("h1", "Alpha")
	m.playback.listenedSecs = 2
	m.playback.peakTotalSecs = 300

	if _, ok := m.TakePendingListen(); ok {
		t.Fatal("expected a 2s listen to be dropped at quit")
	}
}

// TestListenAccumulationIgnoresForwardSeek verifies that skipping ahead resyncs
// the baseline instead of crediting the skipped audio, which is what stops a
// seek-to-end from counting as a play.
func TestListenAccumulationIgnoresForwardSeek(t *testing.T) {
	er := &stubElapsedReader{total: 300}
	m := New(context.Background(), WithPlayer(er))
	m.playback.state = playback.StatePlaying

	for _, elapsed := range []float64{1, 2, 3} {
		er.elapsed = elapsed
		next, _ := m.Update(tickMsg{})
		m = next.(Model)
	}
	if got := m.playback.listenedSecs; got != 3 {
		t.Fatalf("expected 3s accumulated from three 1s ticks, got %v", got)
	}

	// Seek to the end, then keep playing for two more ticks.
	er.elapsed = 295
	next, _ := m.Update(tickMsg{})
	m = next.(Model)
	if got := m.playback.listenedSecs; got != 3 {
		t.Fatalf("a forward seek must not accumulate, got %v", got)
	}

	for _, elapsed := range []float64{296, 297} {
		er.elapsed = elapsed
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	if got := m.playback.listenedSecs; got != 5 {
		t.Fatalf("expected accumulation to resume after the seek (5s), got %v", got)
	}
	if shouldCountPlay(m.playback.listenedSecs, m.playback.totalSecs) {
		t.Error("seeking to the end must not produce a counted play")
	}
}

// TestListenAccumulationIgnoresPause verifies that a paused player, which
// reports the same position on every tick, adds no listen time.
func TestListenAccumulationIgnoresPause(t *testing.T) {
	er := &stubElapsedReader{elapsed: 10, total: 300}
	m := New(context.Background(), WithPlayer(er))
	m.playback.state = playback.StatePlaying

	next, _ := m.Update(tickMsg{})
	m = next.(Model)
	baseline := m.playback.listenedSecs

	m.playback.state = playback.StatePaused
	for range 5 {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}

	if got := m.playback.listenedSecs; got != baseline {
		t.Fatalf("expected no accumulation while paused, got %v (baseline %v)", got, baseline)
	}
}

// TestListenAccumulationIgnoresBackwardSeek verifies that rewinding resyncs the
// baseline rather than subtracting, and that re-listened audio accrues again.
func TestListenAccumulationIgnoresBackwardSeek(t *testing.T) {
	er := &stubElapsedReader{total: 300}
	m := New(context.Background(), WithPlayer(er))
	m.playback.state = playback.StatePlaying

	for _, elapsed := range []float64{1, 2, 3} {
		er.elapsed = elapsed
		next, _ := m.Update(tickMsg{})
		m = next.(Model)
	}

	er.elapsed = 1
	next, _ := m.Update(tickMsg{})
	m = next.(Model)
	if got := m.playback.listenedSecs; got != 3 {
		t.Fatalf("a backward seek must not subtract, got %v", got)
	}

	er.elapsed = 2
	next, _ = m.Update(tickMsg{})
	m = next.(Model)
	if got := m.playback.listenedSecs; got != 4 {
		t.Fatalf("expected re-listened audio to accrue again (4s), got %v", got)
	}
}
