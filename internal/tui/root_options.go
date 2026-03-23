package tui

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// Option configures optional dependencies on the Model. Pass options to New()
// to wire dependencies at construction time rather than via separate Set* calls.
type Option func(*Model)

// WithConfigPath sets the config file path and whether it was newly created.
// The path is displayed in the status bar; configCreated triggers a transient
// first-run notification.
func WithConfigPath(path string, created bool) Option {
	return func(m *Model) {
		m.configPath = path
		m.configCreated = created
	}
}

// WithConfigWarnings sets config validation warnings to display as a TUI
// notification on startup. Pass the warnings returned by config.Load.
func WithConfigWarnings(warnings []string) Option {
	return func(m *Model) { m.configWarnings = warnings }
}

// WithScanChannel sets the library scan channel. When non-nil, the TUI reads
// incremental track updates from it. Pass the channel returned by library.Scan.
func WithScanChannel(ch <-chan library.Track) Option {
	return func(m *Model) { m.scanCh = ch }
}

// WithScanErrChannel sets the channel that carries non-fatal walk errors from
// the initial diff scan. The channel must already be closed before Init() runs
// (LoadLibrary guarantees this). The TUI drains it once on startup and shows
// each error as a status bar notification.
func WithScanErrChannel(ch <-chan library.ScanErrorMsg) Option {
	return func(m *Model) { m.scanErrCh = ch }
}

// WithWatchChannel sets the filesystem watch channel. When non-nil, the TUI
// receives incremental updates (new/modified/deleted tracks) from it.
func WithWatchChannel(ch <-chan library.WatchEvent) Option {
	return func(m *Model) { m.watchCh = ch }
}

// WithLibraryRoot sets the music library root directory. Required for the
// manual rescan hotkey and the parallel file-count goroutine.
func WithLibraryRoot(root string) Option {
	return func(m *Model) { m.libraryRoot = root }
}

// WithExtractor sets the metadata extractor used during manual rescans.
func WithExtractor(e library.MetadataExtractor) Option {
	return func(m *Model) { m.extractor = e }
}

// WithPlayer sets the playback backend. Pass nil to disable playback controls.
func WithPlayer(p playPauser) Option {
	return func(m *Model) { m.player = p }
}

// WithWatchDegraded marks the filesystem watcher as degraded. The status bar
// shows a notice so the user knows to use manual rescan.
func WithWatchDegraded(degraded bool) Option {
	return func(m *Model) { m.watchDegraded = degraded }
}

// WithRuntimeDir sets the runtime directory for ephemeral sockets (e.g. preview player).
func WithRuntimeDir(dir string) Option {
	return func(m *Model) { m.runtimeDir = dir }
}

// WithQueueWriter sets the QueueAppender for persisting enqueued tracks.
func WithQueueWriter(w libtui.QueueAppender) Option {
	return func(m *Model) { m.queueWriter = w }
}

// WithQueueDequeuer sets the dequeuer for soft-delete, undo, and purge of queue entries.
func WithQueueDequeuer(d queueDequeuer) Option {
	return func(m *Model) { m.undoer = d }
}

// WithSessionWriter sets the writer for persisting session-level state (repeat mode, position).
func WithSessionWriter(sw sessionWriter) Option {
	return func(m *Model) { m.sessionWriter = sw }
}

// WithPlaylistTrackReader sets the reader for loading playlist tracks.
func WithPlaylistTrackReader(r playlistTrackReader) Option {
	return func(m *Model) { m.playlistReader = r }
}

// WithPlaylistWriter sets the creator for new playlists.
func WithPlaylistWriter(w playlistCreator) Option {
	return func(m *Model) { m.playlistWriter = w }
}

// WithPlaylistRenamer sets the renamer for playlists.
func WithPlaylistRenamer(r playlistRenamer) Option {
	return func(m *Model) { m.playlistRenamer = r }
}

// WithPlaylistDeleter sets the deleter for soft-deleting playlists and tracks.
func WithPlaylistDeleter(d playlistDeleter) Option {
	return func(m *Model) { m.playlistDeleter = d }
}

// WithPlaylistAdder sets the adder for appending tracks to playlists.
func WithPlaylistAdder(a playlistTrackAdder) Option {
	return func(m *Model) { m.playlistAdder = a }
}

// WithQueueReorderer sets the reorderer for persisting queue position swaps.
func WithQueueReorderer(r queueReorderer) Option {
	return func(m *Model) { m.reorderer = r }
}

// WithBatchEnqueuer sets the batch enqueuer for bulk track insertion.
func WithBatchEnqueuer(be batchEnqueuer) Option {
	return func(m *Model) { m.batchEnqueuer = be }
}

// WithFavoriteReader sets the reader for loading favorites from the database.
func WithFavoriteReader(r favoriteReader) Option {
	return func(m *Model) { m.favoriteReader = r }
}

// WithFavoriteWriter sets the writer for persisting favorite add/remove operations.
func WithFavoriteWriter(w favoriteWriter) Option {
	return func(m *Model) { m.favoriteWriter = w }
}

// WithMetadataWriter sets the writer for persisting metadata edits.
func WithMetadataWriter(w metadataUpdater) Option {
	return func(m *Model) { m.metadataWriter = w }
}

// WithQueueEntries pre-populates the queue view with entries.
func WithQueueEntries(entries []QueueEntry) Option {
	return func(m *Model) { m.queue.entries = entries }
}

// WithRepeatMode pre-sets the repeat mode.
func WithRepeatMode(mode RepeatMode) Option {
	return func(m *Model) { m.playback.repeatMode = mode }
}

// WithShuffle pre-sets the shuffle flag.
func WithShuffle(enabled bool) Option {
	return func(m *Model) { m.playback.shuffle = enabled }
}

// WithVolume pre-sets the volume level.
func WithVolume(v int) Option {
	return func(m *Model) { m.playback.volume = clampVolume(v) }
}

// WithPlaybackState pre-sets the playback state.
func WithPlaybackState(state PlaybackState) Option {
	return func(m *Model) { m.playback.state = state }
}

// WithCurrentTrackHash pre-sets the current track hash.
func WithCurrentTrackHash(hash string) Option {
	return func(m *Model) { m.playback.currentHash = hash }
}

// WithStatusBarTrack pre-sets the status bar track metadata so the first
// render after session restore shows artist, title, and playlist source.
func WithStatusBarTrack(artist, title, source string) Option {
	return func(m *Model) {
		m.statusBar.artist = artist
		m.statusBar.title = title
		m.statusBar.playlistSource = source
	}
}

// WithPlaylists pre-populates the playlists view.
func WithPlaylists(rows []db.PlaylistRow) Option {
	return func(m *Model) { m.playlists.playlists = rows }
}

// WithLibraryTracks pre-populates the library index from cached tracks.
func WithLibraryTracks(tracks []library.Track) Option {
	return func(m *Model) {
		m.library.index.BulkLoad(tracks)
		m.scanned = len(tracks)
		m.scanTotal = len(tracks)
	}
}

// clampVolume restricts v to the [0, 100] range.
func clampVolume(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
