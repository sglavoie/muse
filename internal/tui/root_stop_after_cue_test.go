package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// stubCuePlayer is a stop-after-capable player that can cue the next song.
type stubCuePlayer struct {
	stubStopAfterPlayer
	cues   int
	cueErr error
}

func (s *stubCuePlayer) CueNext() error {
	s.cues++
	return s.cueErr
}

func cueModel(t *testing.T, p playPauser) Model {
	t.Helper()
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.queue.Entries = []queue.Entry{
		{ID: 1, Path: "/a.flac", Hash: "a", Artist: "A", Title: "One"},
		{ID: 2, Path: "/b.flac", Hash: "b", Artist: "B", Title: "Two"},
		{ID: 3, Path: "/c.flac", Hash: "c", Artist: "C", Title: "Three"},
	}
	m.playback.currentIndex, m.playback.currentHash = 0, "a"
	m = m.setStopAfterCurrent(true)
	return m
}

// endWithStopAfter delivers the end of the song and runs the cue it dispatches.
func endWithStopAfter(t *testing.T, m Model) (Model, stopAfterCueResultMsg) {
	t.Helper()
	next, cmd := m.Update(playback.PlaybackEndedMsg{})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("the stop dispatched no cue")
	}
	msg, ok := cmd().(stopAfterCueResultMsg)
	if !ok {
		t.Fatal("the stop's command was not the cue")
	}
	return m, msg
}

func TestStopAfterCurrentCuesNextSongPaused(t *testing.T) {
	p := &stubCuePlayer{}
	m := cueModel(t, p)

	m, result := endWithStopAfter(t, m)
	if m.playback.stopAfterCurrent {
		t.Error("the flag outlived the stop")
	}
	if m.playback.state != playback.StatePaused {
		t.Errorf("state while the cue is on its way = %v, want paused", m.playback.state)
	}
	next, _ := m.Update(result)
	m = next.(Model)
	if p.cues != 1 || !result.ran || result.err != nil {
		t.Fatalf("cue ran %d times (ran=%v err=%v)", p.cues, result.ran, result.err)
	}

	// The cued song's report arrives as a start, but it must leave the view
	// paused on the new row: mpv loaded it paused.
	next, _ = m.Update(playback.TrackChangedMsg{Hash: "b", Artist: "B", Title: "Two", Index: 1, QueueID: 2})
	m = next.(Model)
	if m.playback.state != playback.StatePaused || m.playback.currentIndex != 1 {
		t.Fatalf("after the cue: state=%v index=%d, want paused on row 1", m.playback.state, m.playback.currentIndex)
	}
	if m.playback.queueExhausted {
		t.Error("a stop-after-current is not the end of the queue")
	}
}

func TestStopAfterCurrentCueReportBeforeResult(t *testing.T) {
	p := &stubCuePlayer{}
	m := cueModel(t, p)
	m, result := endWithStopAfter(t, m)

	// The event reader can deliver the start before the command returns.
	next, _ := m.Update(playback.TrackChangedMsg{Hash: "b", Index: 1, QueueID: 2})
	m = next.(Model)
	next, _ = m.Update(result)
	m = next.(Model)
	if m.playback.state != playback.StatePaused || m.playback.currentHash != "b" {
		t.Fatalf("state=%v hash=%q, want paused on b", m.playback.state, m.playback.currentHash)
	}
	// mpv's own pause report then changes nothing.
	next, _ = m.Update(playback.PauseChangedMsg{Paused: true})
	if next.(Model).playback.state != playback.StatePaused {
		t.Error("the pause report moved the state")
	}
}

func TestStopAfterCurrentNothingToCueStops(t *testing.T) {
	p := &stubCuePlayer{cueErr: player.ErrPlaybackFailed}
	m := cueModel(t, p)
	m, result := endWithStopAfter(t, m)
	next, _ := m.Update(result)
	m = next.(Model)
	if m.playback.state != playback.StateIdle {
		t.Errorf("state = %v, want idle when nothing follows", m.playback.state)
	}
	if got := m.statusBar.TopNotification().Msg; got != "Stopped after track" {
		t.Errorf("notice = %q", got)
	}
	if !errors.Is(result.err, player.ErrPlaybackFailed) {
		t.Errorf("result err = %v", result.err)
	}
}

func TestStopAfterCurrentSupersededCueChangesNothing(t *testing.T) {
	p := &stubCuePlayer{}
	m := cueModel(t, p)
	m, result := endWithStopAfter(t, m)
	// A newer request (the user picking a row, say) owns playback now.
	m.playback.requestSeq++
	next, _ := m.Update(result)
	m = next.(Model)
	if m.stopCue.awaitingTrack {
		t.Error("a superseded cue still claims the next report")
	}
	next, _ = m.Update(playback.TrackChangedMsg{Hash: "c", Index: 2, QueueID: 3})
	if next.(Model).playback.state != playback.StatePlaying {
		t.Error("the newer selection's start was forced paused")
	}
}

func TestStopAfterCurrentClearedByManualSkip(t *testing.T) {
	p := &stubCuePlayer{}
	m := cueModel(t, p)

	// A reconnect re-announcing the same row leaves it armed.
	next, _ := m.Update(playback.TrackChangedMsg{Hash: "a", Index: 0, QueueID: 1})
	m = next.(Model)
	if !m.playback.stopAfterCurrent || !p.stopAfter {
		t.Fatal("a re-announce of the same song spent the flag")
	}

	next, _ = m.Update(playback.TrackChangedMsg{Hash: "b", Index: 1, QueueID: 2})
	m = next.(Model)
	if m.playback.stopAfterCurrent || p.stopAfter {
		t.Error("skipping to another song left stop-after armed")
	}
	if m.playback.state != playback.StatePlaying {
		t.Errorf("a skip is not a stop: state = %v", m.playback.state)
	}
}

func TestStopAfterCurrentWithoutCuerKeepsOldStop(t *testing.T) {
	p := &stubStopAfterPlayer{}
	m := cueModel(t, p)
	next, cmd := m.Update(playback.PlaybackEndedMsg{})
	m = next.(Model)
	if m.playback.state != playback.StateIdle || cmd != nil {
		t.Fatalf("a player that cannot cue: state=%v cmd=%v, want an idle stop", m.playback.state, cmd != nil)
	}
}
