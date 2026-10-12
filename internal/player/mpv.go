package player

// baseFlags are always prepended to the mpv command line. They keep mpv in
// headless audio-only mode so it never registers with the macOS window server
// (which would create a Dock icon and app-switcher entry). --volume-max=100
// caps mpv's volume at the 0-100 range muse tracks and displays; mpv's own
// default of 130 would let relative "add volume" steps climb past it.
var baseFlags = []string{"--no-video", "--no-terminal", "--really-quiet", "--volume-max=100"}

// launchArgs returns the full mpv argument list: idle mode and the IPC socket,
// then baseFlags, then any user-supplied extra flags.
func launchArgs(socketPath string, extra []string) []string {
	args := append([]string{"--idle=yes", "--input-ipc-server=" + socketPath}, baseFlags...)
	return append(args, extra...)
}

// Compile-time check that Manager implements Player.
var _ Player = (*Manager)(nil)

// noopMarker is a MissingMarker that silently discards all calls. It is used
// when PlayFromIndex is called before a real MissingMarker has been wired in.
type noopMarker struct{}

func (noopMarker) MarkTrackMissing(_ string) error     { return nil }
func (noopMarker) MarkQueueEntryMissing(_ int64) error { return nil }

// EventCallbacks bundles the optional notification callbacks that the Manager
// fires in response to mpv events. All fields are optional; nil callbacks are
// silently skipped. Pass the struct to SetEventCallbacks once all callbacks are
// wired — the call also unblocks the event reader.
type EventCallbacks struct {
	// OnStatus is called with transient status messages (e.g. "Connecting to mpv…").
	OnStatus func(string)
	// OnMissingFile is called with the file path when a missing track is detected.
	OnMissingFile func(string)
	// OnPlaybackEnded is called when auto-advance exhausts the queue.
	OnPlaybackEnded func()
	// OnTrackChanged is called when a new track starts (mpv "start-file" event).
	// It receives the content hash, artist, title, playlist source, queue index,
	// and the queue row id of the occurrence that started.
	//
	// queueID is what separates two queue rows holding the same track: the hash
	// and the metadata are identical for both, and the index describes the queue
	// as it was when mpv started the file rather than as the receiver sees it. It
	// is zero for an occurrence that is not persisted, which is the only case in
	// which duplicates remain indistinguishable.
	OnTrackChanged func(hash, artist, title, source string, index int, queueID int64)
	// OnPauseChanged is called when mpv's pause property changes.
	OnPauseChanged func(paused bool)
	// OnDisconnected is called at the start of handleRestart, before reconnection begins.
	OnDisconnected func()
}
