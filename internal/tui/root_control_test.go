package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/control"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

type recordingControlPublisher struct{ events []control.Event }

func (r *recordingControlPublisher) Publish(e control.Event) { r.events = append(r.events, e) }

// newHostedModel returns a Model publishing to a fresh recorder, with one
// queued track loaded in state.
func newHostedModel(state playback.State, opts ...Option) (Model, *recordingControlPublisher) {
	pub := &recordingControlPublisher{}
	m := New(context.Background(), append([]Option{WithControlPublisher(pub)}, opts...)...)
	m.queue.Entries = []queue.Entry{
		{ID: 11, Hash: "h1", Title: "Song", Artist: "Band", Album: "Record", DurationMs: 183000},
		{ID: 12, Hash: "h2", Title: "Other", Artist: "Band", Album: "Record", DurationMs: 60000},
	}
	loadControlTrack(&m, 0, state)
	return m, pub
}

func loadControlTrack(m *Model, idx int, state playback.State) {
	e := m.queue.Entries[idx]
	m.playback.state = state
	m.playback.currentIndex = idx
	m.playback.currentHash = e.Hash
	m.playback.totalSecs = float64(e.DurationMs) / 1000
	m.statusBar.Title, m.statusBar.Artist, m.statusBar.Album = e.Title, e.Artist, e.Album
}

func controlUpdate(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func TestControl_PublishesOncePerTrackChange(t *testing.T) {
	m, pub := newHostedModel(playback.StatePlaying)
	m, _ = controlUpdate(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = controlUpdate(t, m, tea.WindowSizeMsg{Width: 121, Height: 40})
	if len(pub.events) != 1 {
		t.Fatalf("published %d events for one track, want 1", len(pub.events))
	}
	got := pub.events[0]
	if got.Event != control.EventState || got.State == nil || got.State.Track == nil {
		t.Fatalf("event = %+v, want a state event with a track", got)
	}
	want := control.Track{ID: "h1", QueueID: 11, Title: "Song", Artist: "Band", Album: "Record", Duration: 183}
	if *got.State.Track != want || got.State.Playback != control.PlaybackPlaying {
		t.Fatalf("state = %+v / %+v, want playing %+v", got.State, *got.State.Track, want)
	}

	loadControlTrack(&m, 1, playback.StatePlaying)
	_, _ = controlUpdate(t, m, tea.WindowSizeMsg{Width: 121, Height: 40})
	if len(pub.events) != 2 || pub.events[1].State.Track.QueueID != 12 {
		t.Fatalf("track change published %d events, want a second for queue row 12", len(pub.events))
	}
}

func TestControl_TicksDoNotRepublish(t *testing.T) {
	m, pub := newHostedModel(playback.StatePlaying)
	m = m.publishControlState()
	for i := 1; i <= 5; i++ {
		m.playback.elapsedSecs = float64(i)
		m = m.publishControlState()
	}
	if len(pub.events) != 1 {
		t.Fatalf("elapsed-only changes published %d events, want 1", len(pub.events))
	}
}

func TestControl_PauseAndIdleStates(t *testing.T) {
	m, pub := newHostedModel(playback.StatePlaying)
	m = m.publishControlState()

	m.playback.state = playback.StatePaused
	m = m.publishControlState()
	if n := len(pub.events); n != 2 || pub.events[1].State.Playback != control.PlaybackPaused || pub.events[1].State.Track == nil {
		t.Fatalf("pause did not publish a paused state with its track: %+v", pub.events)
	}

	m.playback.state = playback.StateIdle
	m.playback.currentHash = ""
	_ = m.publishControlState()
	last := pub.events[len(pub.events)-1].State
	if len(pub.events) != 3 || last.Playback != control.PlaybackStopped || last.Track != nil {
		t.Fatalf("idle did not publish a stopped state without a track: %+v", last)
	}
}

func TestControl_TogglePlayReachesPlayer(t *testing.T) {
	p := &stubPlayer{}
	m, _ := newHostedModel(playback.StatePlaying, WithPlayer(p))
	reply := make(chan control.Reply, 1)
	m, cmd := controlUpdate(t, m, controlRequestMsg{req: control.Request{ID: 1, Op: control.OpTogglePlay}, reply: reply})
	if r := <-reply; !r.OK {
		t.Fatalf("toggle-play reply = %+v, want ok", r)
	}
	if m.playback.state != playback.StatePaused {
		t.Fatalf("state = %v, want paused", m.playback.state)
	}
	djWakeMessages(cmd)
	if p.pauseCalls != 1 {
		t.Fatalf("player Pause calls = %d, want 1", p.pauseCalls)
	}
}

func TestControl_StateOpAnswersSnapshot(t *testing.T) {
	m, _ := newHostedModel(playback.StatePaused)
	reply := make(chan control.Reply, 1)
	_, _ = controlUpdate(t, m, controlRequestMsg{req: control.Request{ID: 2, Op: control.OpState}, reply: reply})
	r := <-reply
	if !r.OK || r.State == nil || r.State.Playback != control.PlaybackPaused || r.State.Track == nil {
		t.Fatalf("state reply = %+v, want the paused snapshot", r)
	}
}

func TestControl_UnknownOpRefused(t *testing.T) {
	p := &stubPlayer{}
	m, _ := newHostedModel(playback.StatePlaying, WithPlayer(p))
	reply := make(chan control.Reply, 1)
	m, cmd := controlUpdate(t, m, controlRequestMsg{req: control.Request{ID: 3, Op: "capture"}, reply: reply})
	if r := <-reply; r.OK || r.Message == "" {
		t.Fatalf("unknown op reply = %+v, want a refusal", r)
	}
	djWakeMessages(cmd)
	if m.playback.state != playback.StatePlaying || p.pauseCalls != 0 || p.playCalls != 0 {
		t.Fatal("an unknown op must not touch playback")
	}
}

func TestControl_NilPublisherIsNoOp(t *testing.T) {
	m := New(context.Background())
	if m.hosted() {
		t.Fatal("a model without a publisher must not be hosted")
	}
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "h1"
	m, _ = controlUpdate(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.control.published {
		t.Fatal("a model without a publisher must not record a publish")
	}
}

func TestControlDispatch_SendsRequestThroughProgram(t *testing.T) {
	var sent tea.Msg
	replies := ControlDispatch(func(msg tea.Msg) { sent = msg })(control.Request{ID: 4, Op: control.OpNext})
	req, ok := sent.(controlRequestMsg)
	if !ok || req.req.Op != control.OpNext {
		t.Fatalf("sent %#v, want a controlRequestMsg for next", sent)
	}
	req.reply <- control.Reply{OK: true}
	if r := <-replies; !r.OK {
		t.Fatalf("reply = %+v, want the TUI's answer", r)
	}
}
