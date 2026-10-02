// Package control is the inbound half of a hosted muse session: a Unix socket
// a host such as muse.app connects to, so it can ask the running TUI to act and
// hear what the TUI reports, instead of reading the terminal back and typing
// keys into it.
//
// The package owns the wire and the connections alone. It never touches the
// player: every request is handed to the TUI through the Dispatch function, so
// a request runs through the same Update a keystroke would.
//
// The protocol is newline-delimited JSON in both directions. A host writes
// Request lines; muse writes one Reply per request, carrying the request's id,
// and Event lines whenever it has something to report. Every connection hears
// every event. A new connection hears the hello first, then the latest state.
// Once muse begins quitting it refuses further requests and says so with a
// quitting event; the socket reaching EOF afterwards means shutdown finished.
//
// This is a port of tt's internal/control, trimmed to muse's vocabulary.
package control

// ProtocolVersion is the wire format version a hello event announces. Bump it
// on any change a host could misread.
const ProtocolVersion = 1

// The operations a Request may name.
const (
	OpTogglePlay = "toggle-play"
	OpNext       = "next"
	OpPrevious   = "previous"
	OpVolumeUp   = "volume-up"
	OpVolumeDown = "volume-down"
	// OpState asks for the current state, answered in the Reply itself.
	OpState = "state"
)

// The events muse reports.
const (
	// EventHello names the muse on the other end; always a connection's first line.
	EventHello = "hello"
	// EventState carries the playback state whenever it changes.
	EventState = "state"
	// EventNotify carries a notification a host should show, such as a track change.
	EventNotify = "notify"
	// EventQuitting says muse has begun shutting down and refuses new requests.
	EventQuitting = "quitting"
)

// The values State.Playback takes.
const (
	PlaybackPlaying = "playing"
	PlaybackPaused  = "paused"
	PlaybackStopped = "stopped"
)

// Request is one line a host writes.
type Request struct {
	ID int    `json:"id"`
	Op string `json:"op"`
}

// Reply answers the Request whose ID it carries. Message is what muse said: the
// outcome on success, the refusal otherwise. State answers OpState.
type Reply struct {
	ID      int    `json:"reply"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	State   *State `json:"state,omitempty"`
}

// Hello identifies the muse a host is talking to.
type Hello struct {
	PID      int    `json:"pid"`
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
}

// State is what is playing. Track is absent when nothing is loaded; Elapsed is
// in seconds.
type State struct {
	Playback string  `json:"playback"`
	Track    *Track  `json:"track,omitempty"`
	Elapsed  float64 `json:"elapsed"`
}

// Track is the loaded track. ID is the track's content hash and QueueID its
// queue row, so a host can tell a repeat of the same track from a new entry.
// Duration is in seconds.
type Track struct {
	ID       string  `json:"id"`
	QueueID  int64   `json:"queueId"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Duration float64 `json:"duration"`
}

// Notification is one banner a host should show.
type Notification struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Body     string `json:"body,omitempty"`
}

// Event is one line muse writes unprompted. The field matching Event is set.
type Event struct {
	Event  string        `json:"event"`
	Hello  *Hello        `json:"hello,omitempty"`
	State  *State        `json:"state,omitempty"`
	Notify *Notification `json:"notify,omitempty"`
}
