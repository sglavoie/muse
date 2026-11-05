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

// WithStateObserver adds a sink that hears the same state events a host does,
// without marking the session as hosted: the system media controls (MPRIS on
// Linux) use it. A nil observer is ignored.
func WithStateObserver(p controlPublisher) Option {
	return func(m *Model) {
		if p != nil {
			m.control.observers = append(m.control.observers, p)
		}
	}
}

// controlState is the state-event bookkeeping on the Model. last is the key of
// the state most recently published, so a tick that only moves elapsed does
// not republish. seeks counts the seeks noticed in the current track, so a
// jump in elapsed republishes and media controls can move their position.
type controlState struct {
	publisher controlPublisher
	observers []controlPublisher
	last      controlStateKey
	published bool
	seeks     uint64
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
	seeks    uint64
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
	switch msg.req.Op {
	case control.OpState:
		state := m.controlSnapshot()
		msg.reply <- control.Reply{OK: true, State: &state}
		return m, nil
	case control.OpPlay, control.OpPause:
		// Toggle only when the state differs, so a headphone's pause never
		// resumes a paused track.
		want := msg.req.Op == control.OpPlay
		if m.sleepFading() {
			// Play during the sleep timer's fade means "keep listening": the
			// fade is cancelled and the music stays on. Pause still pauses.
			m = m.cancelSleepTimer("Sleep timer cancelled")
		}
		if (m.playback.state == playback.StatePlaying) == want {
			msg.reply <- control.Reply{OK: true, Message: msg.req.Op}
			return m, nil
		}
		next, cmd, _ := m.handlePlaybackKeyAction(keys.ActionPlayPause)
		msg.reply <- control.Reply{OK: true, Message: msg.req.Op}
		return next, cmd
	case control.OpSeek:
		return m.handleControlSeek(msg)
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

// handleControlSeek moves the current track to the requested position,
// clamped to the track, and answers with the outcome.
func (m Model) handleControlSeek(msg controlRequestMsg) (tea.Model, tea.Cmd) {
	pos := msg.req.Position
	switch {
	case pos == nil || math.IsNaN(*pos) || math.IsInf(*pos, 0):
		msg.reply <- control.Reply{OK: false, Message: "seek needs a position in seconds"}
		return m, nil
	case m.playback.currentHash == "" ||
		(m.playback.state != playback.StatePlaying && m.playback.state != playback.StatePaused):
		msg.reply <- control.Reply{OK: false, Message: "nothing is playing"}
		return m, nil
	}
	target := max(0, *pos)
	if m.playback.totalSecs > 0 {
		target = min(target, m.playback.totalSecs)
	}
	cp, ok := m.player.(commandPlayer)
	if !ok {
		msg.reply <- control.Reply{OK: false, Message: "player unavailable"}
		return m, nil
	}
	if err := cp.SeekAbsolute(target); err != nil {
		msg.reply <- control.Reply{OK: false, Message: "seek failed: " + err.Error()}
		return m, nil
	}
	m.playback.elapsedSecs = target
	m.playback.lastSampleSecs = target
	m.control.seeks++
	msg.reply <- control.Reply{OK: true, Message: msg.req.Op}
	return m, m.sessionPositionCmd()
}

// publishControlState publishes a state event when the comparable state has
// changed since the last one. It is a no-op with no host and no observer.
func (m Model) publishControlState() Model {
	if m.control.publisher == nil && len(m.control.observers) == 0 {
		return m
	}
	state := m.controlSnapshot()
	key := controlKeyFor(state)
	key.seeks = m.control.seeks
	if m.control.published && key == m.control.last {
		return m
	}
	event := control.Event{Event: control.EventState, State: &state}
	if m.control.publisher != nil {
		m.control.publisher.Publish(event)
	}
	for _, o := range m.control.observers {
		o.Publish(event)
	}
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
