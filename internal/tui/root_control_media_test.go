package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/control"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// stubAbsoluteSeeker records SeekAbsolute targets.
type stubAbsoluteSeeker struct {
	stubSeekPlayer
	targets []float64
}

func (s *stubAbsoluteSeeker) SeekAbsolute(t float64) error {
	s.targets = append(s.targets, t)
	return nil
}

func controlRequest(t *testing.T, m Model, req control.Request) (Model, control.Reply) {
	t.Helper()
	reply := make(chan control.Reply, 1)
	m, cmd := controlUpdate(t, m, controlRequestMsg{req: req, reply: reply})
	djWakeMessages(cmd)
	return m, <-reply
}

func TestControl_PlayAndPauseSetStateOutright(t *testing.T) {
	tests := []struct {
		name      string
		start     playback.State
		op        string
		want      playback.State
		wantPause int
		wantPlay  int
	}{
		{"pause while playing pauses", playback.StatePlaying, control.OpPause, playback.StatePaused, 1, 0},
		{"pause while paused stays paused", playback.StatePaused, control.OpPause, playback.StatePaused, 0, 0},
		{"play while paused resumes", playback.StatePaused, control.OpPlay, playback.StatePlaying, 0, 1},
		{"play while playing keeps playing", playback.StatePlaying, control.OpPlay, playback.StatePlaying, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &stubPlayer{}
			m, _ := newHostedModel(tt.start, WithPlayer(p))
			m, r := controlRequest(t, m, control.Request{ID: 1, Op: tt.op})
			if !r.OK {
				t.Fatalf("%s reply = %+v, want ok", tt.op, r)
			}
			if m.playback.state != tt.want || p.pauseCalls != tt.wantPause || p.playCalls != tt.wantPlay {
				t.Fatalf("state %v, pause %d, play %d; want %v, %d, %d",
					m.playback.state, p.pauseCalls, p.playCalls, tt.want, tt.wantPause, tt.wantPlay)
			}
		})
	}
}

func TestControl_SeekMovesPlayerAndRepublishes(t *testing.T) {
	p := &stubAbsoluteSeeker{}
	m, pub := newHostedModel(playback.StatePlaying, WithPlayer(p))
	m = m.publishControlState()

	m, r := controlRequest(t, m, control.Request{ID: 1, Op: control.OpSeek, Position: new(500.0)})
	if !r.OK {
		t.Fatalf("seek reply = %+v, want ok", r)
	}
	// The track is 183 s long, so the target is clamped to its end.
	if len(p.targets) != 1 || p.targets[0] != 183 {
		t.Fatalf("SeekAbsolute targets = %v, want [183]", p.targets)
	}
	m = m.publishControlState()
	if len(pub.events) != 2 || pub.events[1].State.Elapsed != 183 {
		t.Fatalf("a seek must republish with the new elapsed: %+v", pub.events)
	}
	_ = m
}

func TestControl_SeekRefusals(t *testing.T) {
	p := &stubAbsoluteSeeker{}
	m, _ := newHostedModel(playback.StatePlaying, WithPlayer(p))
	if _, r := controlRequest(t, m, control.Request{ID: 1, Op: control.OpSeek}); r.OK {
		t.Fatalf("seek without a position = %+v, want a refusal", r)
	}
	m.playback.state = playback.StateIdle
	if _, r := controlRequest(t, m, control.Request{ID: 2, Op: control.OpSeek, Position: new(3.0)}); r.OK {
		t.Fatalf("seek while idle = %+v, want a refusal", r)
	}
	if len(p.targets) != 0 {
		t.Fatalf("a refused seek reached the player: %v", p.targets)
	}
}

func TestControl_TickJumpRepublishes(t *testing.T) {
	p := &stubElapsedReader{elapsed: 10, total: 183}
	m, pub := newHostedModel(playback.StatePlaying, WithPlayer(p))
	m.playback.lastSampleSecs = 9
	m, _ = m.handleTickMsg()
	m = m.publishControlState()
	p.elapsed = 11
	m, _ = m.handleTickMsg()
	m = m.publishControlState()
	if len(pub.events) != 1 {
		t.Fatalf("steady playback published %d events, want 1", len(pub.events))
	}
	p.elapsed = 90
	m, _ = m.handleTickMsg()
	_ = m.publishControlState()
	if len(pub.events) != 2 || pub.events[1].State.Elapsed != 90 {
		t.Fatalf("a jump in elapsed must republish: %+v", pub.events)
	}
}

func TestControl_ObserverHearsStateWithoutHosting(t *testing.T) {
	obs := &recordingControlPublisher{}
	m := New(context.Background(), WithStateObserver(obs), WithStateObserver(nil))
	if m.hosted() {
		t.Fatal("an observer must not mark the session hosted")
	}
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "h1"
	_ = m.publishControlState()
	if len(obs.events) != 1 || obs.events[0].State.Playback != control.PlaybackPlaying {
		t.Fatalf("observer heard %+v, want one playing state", obs.events)
	}
}
