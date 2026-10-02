package tui

import (
	"fmt"
	"math"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/control"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// controlPublisher is where a hosted session's state events go. In production
// it is the *control.Server serving --control; it must not block, because
// publishControlState calls it from Update.
type controlPublisher interface {
	Publish(event control.Event)
}

// WithControlPublisher marks the session as hosted and sets where its state
// events go. When nil (the default) nothing is published and the window chrome
// stays terminal-neutral.
func WithControlPublisher(p controlPublisher) Option {
	return func(m *Model) { m.control.publisher = p }
}

// controlState is the hosted-session bookkeeping on the Model. last is the key
// of the state most recently published, so a tick that only moves elapsed does
// not republish.
type controlState struct {
	publisher controlPublisher
	last      controlStateKey
	published bool
}

// controlStateKey is the comparable part of a state event. Elapsed is left out
// on purpose: it moves every tick, and a host extrapolates it between events.
// Duration is kept to whole seconds so a corrected duration of the same length
// does not republish.
type controlStateKey struct {
	playback string
	hash     string
	queueID  int64
	title    string
	artist   string
	album    string
	duration int64
}

// controlRequestMsg carries one host request into Update. reply is buffered,
// so answering never blocks even when the server has given up waiting.
type controlRequestMsg struct {
	req   control.Request
	reply chan<- control.Reply
}

// ControlDispatch adapts a program's Send into the control server's Dispatch,
// so every host request runs through Update like a keystroke would.
func ControlDispatch(send func(tea.Msg)) control.Dispatch {
	return func(req control.Request) <-chan control.Reply {
		reply := make(chan control.Reply, 1)
		send(controlRequestMsg{req: req, reply: reply})
		return reply
	}
}

// hosted reports whether a host drives this session over --control.
func (m Model) hosted() bool {
	return m.control.publisher != nil
}

// handleControlRequest answers one host request. Playback ops share muse.ctrl's
// vocabulary through playback.RemoteAction, so both sockets do exactly what the
// matching key does.
func (m Model) handleControlRequest(msg controlRequestMsg) (tea.Model, tea.Cmd) {
	if msg.req.Op == control.OpState {
		state := m.controlSnapshot()
		msg.reply <- control.Reply{OK: true, State: &state}
		return m, nil
	}
	action := playback.RemoteAction(msg.req.Op)
	if action == keys.ActionNone {
		msg.reply <- control.Reply{OK: false, Message: fmt.Sprintf("unknown op %q", msg.req.Op)}
		return m, nil
	}
	next, cmd, _ := m.handlePlaybackKeyAction(action)
	msg.reply <- control.Reply{OK: true, Message: msg.req.Op}
	return next, cmd
}

// publishControlState publishes a state event when the comparable state has
// changed since the last one. It is a no-op when the session is not hosted.
func (m Model) publishControlState() Model {
	if m.control.publisher == nil {
		return m
	}
	state := m.controlSnapshot()
	key := controlKeyFor(state)
	if m.control.published && key == m.control.last {
		return m
	}
	m.control.publisher.Publish(control.Event{Event: control.EventState, State: &state})
	m.control.last = key
	m.control.published = true
	return m
}

// controlSnapshot reports what is playing. The track is omitted when nothing
// is loaded.
func (m Model) controlSnapshot() control.State {
	state := control.State{
		Playback: controlPlayback(m.playback.state),
		Elapsed:  m.playback.elapsedSecs,
	}
	if m.playback.state == playback.StateIdle || m.playback.currentHash == "" {
		state.Elapsed = 0
		return state
	}
	track := &control.Track{
		ID:       m.playback.currentHash,
		Title:    m.statusBar.Title,
		Artist:   m.statusBar.Artist,
		Album:    m.statusBar.Album,
		Duration: m.playback.totalSecs,
	}
	if e, ok := m.currentControlEntry(); ok {
		track.QueueID = e.ID
		if track.Album == "" {
			track.Album = e.Album
		}
		if track.Duration <= 0 {
			track.Duration = float64(e.DurationMs) / 1000
		}
	}
	state.Track = track
	return state
}

// currentControlEntry returns the queue row of the current track, if the
// current index still names it.
func (m Model) currentControlEntry() (queue.Entry, bool) {
	i := m.playback.currentIndex
	if i < 0 || i >= len(m.queue.Entries) || m.queue.Entries[i].Hash != m.playback.currentHash {
		return queue.Entry{}, false
	}
	return m.queue.Entries[i], true
}

func controlKeyFor(state control.State) controlStateKey {
	key := controlStateKey{playback: state.Playback}
	if t := state.Track; t != nil {
		key.hash = t.ID
		key.queueID = t.QueueID
		key.title = t.Title
		key.artist = t.Artist
		key.album = t.Album
		key.duration = int64(math.Round(t.Duration))
	}
	return key
}

func controlPlayback(s playback.State) string {
	switch s {
	case playback.StatePlaying:
		return control.PlaybackPlaying
	case playback.StatePaused:
		return control.PlaybackPaused
	default:
		return control.PlaybackStopped
	}
}
