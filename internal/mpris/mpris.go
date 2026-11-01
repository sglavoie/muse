// Package mpris publishes a muse TUI session on the D-Bus session bus as an
// MPRIS 2 media player (https://specifications.freedesktop.org/mpris-spec/),
// so media keys, headset buttons, desktop media widgets and playerctl can see
// what is playing and control it.
//
// It speaks the same vocabulary as a hosted session's control socket: player
// methods become control.Request values handed to the TUI's control.Dispatch,
// so they run through Update like a keystroke would, and the TUI's state
// events (tui.WithStateObserver) drive the properties this package reports.
package mpris

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/sglavoie/muse/internal/control"
	"github.com/sglavoie/muse/internal/safego"
)

// D-Bus names from the MPRIS 2 specification.
const (
	objectPath     = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	busNamePrefix  = "org.mpris.MediaPlayer2."
	ifaceRoot      = "org.mpris.MediaPlayer2"
	ifacePlayer    = "org.mpris.MediaPlayer2.Player"
	ifaceProps     = "org.freedesktop.DBus.Properties"
	noTrack        = dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack")
	trackPathRoot  = "/io/github/sglavoie/muse/track/"
	errNotSupport  = "org.mpris.MediaPlayer2.Error.NotSupported"
	errUnknownProp = "org.freedesktop.DBus.Error.UnknownProperty"
	errUnknownIfc  = "org.freedesktop.DBus.Error.UnknownInterface"
	errReadOnly    = "org.freedesktop.DBus.Error.PropertyReadOnly"
	errFailed      = "org.freedesktop.DBus.Error.Failed"
)

// identity is the player name media widgets show.
const identity = "muse"

// replyTimeout bounds how long a method call waits for the TUI to answer.
const replyTimeout = 2 * time.Second

// seekTolerance is how far a state's elapsed may drift from where steady
// playback would have put it before it counts as a seek and Seeked is emitted.
const seekTolerance = 1.5

// emitter sends one D-Bus signal from the player's object path.
type emitter func(name string, values ...any) error

// Service is one exported MPRIS player. Publish feeds it the TUI's state
// events; method calls reach the TUI through dispatch once Serve has run.
type Service struct {
	emit   emitter
	now    func() time.Time
	closer func() error

	mu       sync.Mutex
	dispatch control.Dispatch
	state    control.State
	stateAt  time.Time
	// emitted is what clients were last told, so a new state signals only
	// the properties that changed.
	emitted map[string]dbus.Variant
	// seekPending says the latest state jumped, so flush emits Seeked.
	seekPending bool
	// busName is the name claimed on the bus, for logging.
	busName string

	wake      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// newService returns a service that emits through emit. Start wires one to a
// real bus; tests pass a recorder.
func newService(dispatch control.Dispatch, emit emitter, now func() time.Time) *Service {
	s := &Service{
		dispatch: dispatch,
		emit:     emit,
		now:      now,
		state:    control.State{Playback: control.PlaybackStopped},
		stateAt:  now(),
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	s.emitted = s.playerProps()
	return s
}

// run emits signals for the latest state whenever Publish wakes it, so the
// TUI's Update never waits on the bus.
func (s *Service) run() {
	s.wg.Add(1)
	safego.Go("mpris-emitter", func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.done:
				return
			case <-s.wake:
				s.flush()
			}
		}
	})
}

// Publish implements the TUI's state observer. It never blocks.
func (s *Service) Publish(event control.Event) {
	if event.Event != control.EventState || event.State == nil {
		return
	}
	s.mu.Lock()
	if s.isSeek(*event.State) {
		s.seekPending = true
	}
	s.state = *event.State
	s.stateAt = s.now()
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// isSeek reports whether next jumps away from where the current track should
// be by now. The caller holds s.mu.
func (s *Service) isSeek(next control.State) bool {
	if s.state.Track == nil || next.Track == nil || trackPath(s.state.Track) != trackPath(next.Track) {
		return false
	}
	return math.Abs(next.Elapsed-s.extrapolate()) > seekTolerance
}

// extrapolate is the current position in seconds: the last reported elapsed,
// advanced by the time since while playing. The caller holds s.mu.
func (s *Service) extrapolate() float64 {
	pos := s.state.Elapsed
	if s.state.Playback == control.PlaybackPlaying {
		pos += s.now().Sub(s.stateAt).Seconds()
	}
	if t := s.state.Track; t != nil && t.Duration > 0 {
		pos = min(pos, t.Duration)
	}
	return max(0, pos)
}

// position is Position's value: microseconds into the current track.
func (s *Service) position() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(math.Round(s.extrapolate() * 1e6))
}

// flush emits PropertiesChanged for every player property that differs from
// what clients were last told.
func (s *Service) flush() {
	props := s.playerProps()
	changed := map[string]dbus.Variant{}
	for name, v := range props {
		if old, ok := s.emitted[name]; !ok || !variantEqual(old, v) {
			changed[name] = v
		}
	}
	s.emitted = props
	if len(changed) > 0 {
		if err := s.emit(ifaceProps+".PropertiesChanged", ifacePlayer, changed, []string{}); err != nil {
			slog.Debug("mpris: emit PropertiesChanged", "err", err)
		}
	}
	s.mu.Lock()
	seeked := s.seekPending
	s.seekPending = false
	s.mu.Unlock()
	if seeked {
		if err := s.emit(ifacePlayer+".Seeked", s.position()); err != nil {
			slog.Debug("mpris: emit Seeked", "err", err)
		}
	}
}

// variantEqual compares two property values by their D-Bus rendering, which
// covers the maps and slices Metadata holds.
func variantEqual(a, b dbus.Variant) bool {
	return a.Signature() == b.Signature() && a.String() == b.String()
}

// playerProps is the org.mpris.MediaPlayer2.Player properties that signal
// changes. Position is left out: the spec has clients poll it and hear
// Seeked when it jumps.
func (s *Service) playerProps() map[string]dbus.Variant {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	canSeek := st.Track != nil && st.Track.Duration > 0 && st.Playback != control.PlaybackStopped
	return map[string]dbus.Variant{
		"PlaybackStatus": dbus.MakeVariant(playbackStatus(st.Playback)),
		"Metadata":       dbus.MakeVariant(metadata(st.Track)),
		"Rate":           dbus.MakeVariant(1.0),
		"MinimumRate":    dbus.MakeVariant(1.0),
		"MaximumRate":    dbus.MakeVariant(1.0),
		"CanGoNext":      dbus.MakeVariant(true),
		"CanGoPrevious":  dbus.MakeVariant(true),
		"CanPlay":        dbus.MakeVariant(true),
		"CanPause":       dbus.MakeVariant(true),
		"CanSeek":        dbus.MakeVariant(canSeek),
		"CanControl":     dbus.MakeVariant(true),
	}
}

// rootProps is the org.mpris.MediaPlayer2 properties; they never change.
func rootProps() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"CanQuit":             dbus.MakeVariant(false),
		"CanRaise":            dbus.MakeVariant(false),
		"HasTrackList":        dbus.MakeVariant(false),
		"Identity":            dbus.MakeVariant(identity),
		"SupportedUriSchemes": dbus.MakeVariant([]string{}),
		"SupportedMimeTypes":  dbus.MakeVariant([]string{}),
	}
}

func playbackStatus(p string) string {
	switch p {
	case control.PlaybackPlaying:
		return "Playing"
	case control.PlaybackPaused:
		return "Paused"
	default:
		return "Stopped"
	}
}

// trackPath is the track's mpris:trackid: its queue row, so a repeat of the
// same song is a new track, else its hash.
func trackPath(t *control.Track) dbus.ObjectPath {
	switch {
	case t == nil:
		return noTrack
	case t.QueueID > 0:
		return dbus.ObjectPath(fmt.Sprintf("%sq%d", trackPathRoot, t.QueueID))
	case t.ID != "" && dbus.ObjectPath(trackPathRoot+"h"+t.ID).IsValid():
		return dbus.ObjectPath(trackPathRoot + "h" + t.ID)
	default:
		return dbus.ObjectPath(trackPathRoot + "current")
	}
}

// metadata is the Metadata map for t; with nothing loaded it holds only the
// NoTrack id, as the spec asks.
func metadata(t *control.Track) map[string]dbus.Variant {
	md := map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(trackPath(t))}
	if t == nil {
		return md
	}
	if t.Title != "" {
		md["xesam:title"] = dbus.MakeVariant(t.Title)
	}
	if t.Artist != "" {
		md["xesam:artist"] = dbus.MakeVariant([]string{t.Artist})
	}
	if t.Album != "" {
		md["xesam:album"] = dbus.MakeVariant(t.Album)
	}
	if t.Duration > 0 {
		md["mpris:length"] = dbus.MakeVariant(int64(math.Round(t.Duration * 1e6)))
	}
	return md
}

// Serve starts answering method calls through dispatch and signalling state
// changes. Until it runs, calls are refused and state is only remembered, so
// the service can be handed to the TUI as an observer before the program that
// dispatches requests exists.
func (s *Service) Serve(dispatch control.Dispatch) {
	s.mu.Lock()
	s.dispatch = dispatch
	s.mu.Unlock()
	s.run()
}

// request hands one op to the TUI and waits briefly for its answer.
func (s *Service) request(req control.Request) *dbus.Error {
	select {
	case <-s.done:
		return dbus.NewError(errFailed, []any{"muse is quitting"})
	default:
	}
	s.mu.Lock()
	dispatch := s.dispatch
	s.mu.Unlock()
	if dispatch == nil {
		return dbus.NewError(errFailed, []any{"muse is starting"})
	}
	select {
	case r := <-dispatch(req):
		if !r.OK {
			return dbus.NewError(errFailed, []any{r.Message})
		}
		return nil
	case <-time.After(replyTimeout):
		return dbus.NewError(errFailed, []any{"muse did not answer"})
	case <-s.done:
		return dbus.NewError(errFailed, []any{"muse is quitting"})
	}
}

func (s *Service) op(name string) *dbus.Error { return s.request(control.Request{Op: name}) }

func (s *Service) seekTo(seconds float64) *dbus.Error {
	return s.request(control.Request{Op: control.OpSeek, Position: &seconds})
}

// Close stops the emitter and releases the bus. It is safe to call twice.
func (s *Service) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		s.wg.Wait()
		if s.closer != nil {
			err = s.closer()
		}
	})
	return err
}

// --- Exported D-Bus interfaces ------------------------------------------------
//
// Each interface is its own type, so godbus exports exactly the methods that
// interface defines.

// rootIface is org.mpris.MediaPlayer2. muse runs in a terminal it cannot
// raise, and quitting is left to the TUI, so both methods do nothing, as
// CanRaise and CanQuit say.
type rootIface struct{ s *Service }

func (rootIface) Raise() *dbus.Error { return nil }
func (rootIface) Quit() *dbus.Error  { return nil }

// playerIface is org.mpris.MediaPlayer2.Player.
type playerIface struct{ s *Service }

func (p playerIface) Next() *dbus.Error      { return p.s.op(control.OpNext) }
func (p playerIface) Previous() *dbus.Error  { return p.s.op(control.OpPrevious) }
func (p playerIface) Pause() *dbus.Error     { return p.s.op(control.OpPause) }
func (p playerIface) PlayPause() *dbus.Error { return p.s.op(control.OpTogglePlay) }
func (p playerIface) Play() *dbus.Error      { return p.s.op(control.OpPlay) }

// Stop pauses: muse keeps its place in the queue rather than unloading it.
func (p playerIface) Stop() *dbus.Error { return p.s.op(control.OpPause) }

// SeekBy is the D-Bus Seek method, renamed because go vet reserves Seek for
// io.Seeker. It moves by offset microseconds. Past the end it skips to the
// next track, as the spec asks; before the start it seeks to the start.
func (p playerIface) SeekBy(offset int64) *dbus.Error {
	p.s.mu.Lock()
	track := p.s.state.Track
	target := p.s.extrapolate() + float64(offset)/1e6
	p.s.mu.Unlock()
	if track == nil {
		return nil
	}
	if track.Duration > 0 && target >= track.Duration {
		return p.s.op(control.OpNext)
	}
	return p.s.seekTo(max(0, target))
}

// SetPosition seeks to position microseconds, ignored unless trackID is the
// current track and the position lies within it.
func (p playerIface) SetPosition(trackID dbus.ObjectPath, position int64) *dbus.Error {
	p.s.mu.Lock()
	track := p.s.state.Track
	p.s.mu.Unlock()
	if track == nil || trackID != trackPath(track) || position < 0 {
		return nil
	}
	seconds := float64(position) / 1e6
	if track.Duration > 0 && seconds > track.Duration {
		return nil
	}
	return p.s.seekTo(seconds)
}

func (playerIface) OpenUri(string) *dbus.Error {
	return dbus.NewError(errNotSupport, []any{"muse plays from its own library"})
}

// propsIface is org.freedesktop.DBus.Properties for both MPRIS interfaces.
type propsIface struct{ s *Service }

func (p propsIface) Get(iface, name string) (dbus.Variant, *dbus.Error) {
	all, derr := p.GetAll(iface)
	if derr != nil {
		return dbus.Variant{}, derr
	}
	v, ok := all[name]
	if !ok {
		return dbus.Variant{}, dbus.NewError(errUnknownProp, []any{fmt.Sprintf("no property %s.%s", iface, name)})
	}
	return v, nil
}

func (p propsIface) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	switch iface {
	case ifaceRoot:
		return rootProps(), nil
	case ifacePlayer:
		props := p.s.playerProps()
		props["Position"] = dbus.MakeVariant(p.s.position())
		return props, nil
	default:
		return nil, dbus.NewError(errUnknownIfc, []any{fmt.Sprintf("no interface %s", iface)})
	}
}

// Set refuses every write: muse exposes no writable property (rate, volume,
// loop and shuffle are not offered).
func (p propsIface) Set(iface, name string, _ dbus.Variant) *dbus.Error {
	if _, derr := p.Get(iface, name); derr != nil {
		return derr
	}
	return dbus.NewError(errReadOnly, []any{fmt.Sprintf("%s.%s is read-only", iface, name)})
}

// ErrUnavailable means there is no session bus to publish on, as over SSH or
// on a system without D-Bus. Callers treat it as "no media controls".
var ErrUnavailable = errors.New("mpris: no D-Bus session bus")

// BusName is the D-Bus name the player claimed.
func (s *Service) BusName() string { return s.busName }
