package tui

import (
	"log/slog"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// Option configures optional dependencies on the Model. Pass options to New()
// to wire dependencies at construction time rather than via separate Set* calls.
type Option func(*Model)

// WithConfig sets the application config on the Model so that runtime-tunable
// values (volume step, seek offsets, notification durations) are sourced from
// config rather than hardcoded constants.
func WithConfig(cfg config.Config) Option {
	return func(m *Model) { m.cfg = cfg }
}

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

// WithMusicDirHint sets the actionable instruction block shown in the welcome
// overlay when a configured music directory failed validation at startup. Pass
// config.ConfigHint(configPath); an empty string means the directories
// validated and the overlay keeps its plain first-run copy.
//
// partial reports that at least one directory is still usable (see
// config.MusicDirsResult.Degraded), which selects copy that does not claim the
// library is empty while the valid roots are being scanned.
func WithMusicDirHint(hint string, partial bool) Option {
	return func(m *Model) {
		m.musicDirHint = hint
		m.musicDirsPartial = partial
	}
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

// WithDurationAuditChannel sets the channel that delivers corrected durations
// from the background ffprobe audit. When nil, no audit is performed.
func WithDurationAuditChannel(ch <-chan library.DurationFix) Option {
	return func(m *Model) { m.auditCh = ch }
}

// WithLibraryRoots sets the music library root directories. Required for the
// manual rescan hotkey and the parallel file-count goroutine.
func WithLibraryRoots(roots []string) Option {
	return func(m *Model) { m.libraryRoots = roots }
}

// WithExtractor sets the metadata extractor used during manual rescans.
func WithExtractor(e library.MetadataExtractor) Option {
	return func(m *Model) { m.extractor = e }
}

// WithPlayer sets the playback backend. Pass nil to disable playback controls.
// It also hands the Queue view the player's playback timeline when the backend
// exposes one, so the queue panel can show the order rows will actually play in
// rather than the order they are stored in.
func WithPlayer(p playPauser) Option {
	return func(m *Model) {
		m.player = p
		m.queue.Traversal = queueTraversalSource(p)
	}
}

// WithWatchStatusChannel sets the channel that reports filesystem watcher
// availability once the background library pipeline completes its asynchronous
// setup. The TUI drains it once on startup; when the watcher is unavailable it
// marks the status bar degraded and notifies the user to use manual rescan.
func WithWatchStatusChannel(ch <-chan library.WatchStatusMsg) Option {
	return func(m *Model) { m.watchStatusCh = ch }
}

// WithRuntimeDir sets the runtime directory for ephemeral sockets (e.g. preview player).
func WithRuntimeDir(dir string) Option {
	return func(m *Model) { m.runtimeDir = dir }
}

// WithDataDir sets the XDG data directory that contains the SQLite database.
func WithDataDir(dir string) Option {
	return func(m *Model) { m.dataDir = dir }
}

// WithQueueWriter sets the QueueAppender for persisting enqueued tracks.
func WithQueueWriter(w libtui.QueueAppender) Option {
	return func(m *Model) {
		m.queueWriter = w
		if r, ok := w.(queueReorderer); ok && m.reorderer == nil {
			m.reorderer = r
		}
	}
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

// WithDurationWriter sets the writer for persisting track duration edits.
func WithDurationWriter(w durationUpdater) Option {
	return func(m *Model) { m.durationWriter = w }
}

// WithTrackPositionWriter sets the writer for persisting imported track and
// disc numbers.
func WithTrackPositionWriter(w trackPositionUpdater) Option {
	return func(m *Model) { m.trackPositionWriter = w }
}

// WithScanWriter sets the batch writer used to persist tracks discovered during
// a manual rescan (Shift+R) to the database. Without this, rescanned tracks
// exist only in the in-memory index and are lost on restart.
func WithScanWriter(w scanUpsertBatcher, batchSize int) Option {
	return func(m *Model) {
		m.scanWriter = w
		m.upsertBatchSize = batchSize
	}
}

// WithUserEditedReader sets the reader for user-edited track metadata and
// eagerly loads the overlay map so the startup DiffScan path preserves edits.
func WithUserEditedReader(r userEditedReader) Option {
	return func(m *Model) {
		m.userEditedReader = r
		rows, err := r.ReadUserEditedTracks()
		if err != nil {
			slog.Warn("startup: read user-edited tracks", "err", err)
			return
		}
		if len(rows) > 0 {
			overlay := make(map[string]db.UserEditedRow, len(rows))
			for _, row := range rows {
				overlay[row.Hash] = row
			}
			m.userEditedOverlay = overlay
		}
	}
}

// WithSmartPlaylistEvaluator sets the evaluator for smart playlist queries.
func WithSmartPlaylistEvaluator(e smartPlaylistEvaluator) Option {
	return func(m *Model) { m.smartEval = e }
}

// WithPlayCountWriter sets the writer for incrementing track play counts.
func WithPlayCountWriter(w playCountIncrementer) Option {
	return func(m *Model) { m.playCountWriter = w }
}

// WithHistoryRecorder sets the recorder for persisting play history rows.
// When set, it is used instead of playCountWriter for recording plays.
func WithHistoryRecorder(w playHistoryRecorder) Option {
	return func(m *Model) { m.historyRecorder = w }
}

// WithTrackNotifier sets the sink for on-track-change desktop notifications.
// When nil (the default) no banner is ever posted, regardless of the
// notify_on_track_change setting.
func WithTrackNotifier(n trackNotifier) Option {
	return func(m *Model) { m.notifier = n }
}

// WithHistoryReader sets the reader for lazily loading the play history tab.
func WithHistoryReader(r playHistoryReader) Option {
	return func(m *Model) { m.historyReader = r }
}

// WithInsightsReader sets the reader that feeds the Insights view.
func WithInsightsReader(r insightsReader) Option {
	return func(m *Model) { m.insightsReader = r }
}

// WithRecommendationReader sets the reader for Auto-DJ behavioral signals
// (completion rates and session co-occurrences).
func WithRecommendationReader(r recommendationReader) Option {
	return func(m *Model) { m.recReader = r }
}

// WithDismissalReader sets the reader for loading dismissed track hashes.
func WithDismissalReader(r dismissalReader) Option {
	return func(m *Model) { m.dismissalReader = r }
}

// WithDismissalWriter sets the writer for persisting track dismiss/undismiss operations.
func WithDismissalWriter(w dismissalWriter) Option {
	return func(m *Model) { m.dismissalWriter = w }
}

// WithCacheReader sets the reader for loading pre-computed mix memberships
// from recommendation_cache. When set, loadMixCmd tries the cache first and
// falls back to live queries only when the cache is empty.
func WithCacheReader(r cachedMixReader) Option {
	return func(m *Model) { m.cacheReader = r }
}

// WithCacheRebuilder sets the writer for rebuilding recommendation_cache.
// Root fires an async rebuild after every N plays when this is set.
func WithCacheRebuilder(w recommendationCacheRebuilder) Option {
	return func(m *Model) { m.cacheRebuilder = w }
}

// WithSmartPlaylistWriter sets the writer for creating and updating smart playlists.
func WithSmartPlaylistWriter(w smartPlaylistWriter) Option {
	return func(m *Model) { m.smartPlaylistWriter = w }
}

// WithTagReader sets the reader for loading tag data and eagerly populates the
// in-memory tag cache on startup.
func WithTagReader(r tagReader) Option {
	return func(m *Model) {
		m.tagReader = r
		tags, err := r.ReadTags()
		if err != nil {
			slog.Warn("startup: read tags", "err", err)
			// Recorded rather than only logged: an empty tag cache that came
			// from a failed read must not be mistaken for a library with no
			// tags, or an Auto-DJ tag filter would silently resolve against
			// nothing. See Model.tagCacheErr.
			m.tagCacheErr = err
			return
		}
		m.tags = tags
		m.tagNames = make(map[int64]string, len(tags))
		m.tagColors = make(map[int64]string, len(tags))
		for _, t := range tags {
			m.tagNames[t.ID] = t.Name
			m.tagColors[t.ID] = t.Color
		}
	}
}

// WithTagWriter sets the writer for persisting tag mutations (create, rename,
// delete, toggle).
func WithTagWriter(w tagWriter) Option {
	return func(m *Model) { m.tagWriter = w }
}

// WithQueueEntries pre-populates the queue view with entries.
func WithQueueEntries(entries []queue.Entry) Option {
	return func(m *Model) { m.queue.Entries = entries }
}

// WithRepeatMode pre-sets the repeat mode.
func WithRepeatMode(mode playback.RepeatMode) Option {
	return func(m *Model) { m.playback.repeatMode = mode }
}

// WithShuffle pre-sets the shuffle flag.
func WithShuffle(enabled bool) Option {
	return func(m *Model) { m.playback.shuffle = enabled }
}

// WithRadioMode pre-sets the radio mode flag.
func WithRadioMode(enabled bool) Option {
	return func(m *Model) { m.radio.enabled = enabled }
}

// WithDJSession restores the persisted Auto-DJ session: the applied settings
// snapshot, the seed captured when the session started, and the discovery
// rounding carry.
//
// restoreErr is saved state this build refused to use. Pass it through rather
// than dropping it: the DJ then starts stopped on stock settings and the
// failure is reportable, where silently substituting defaults would replace the
// user's restrictive filters with an unrestricted DJ.
func WithDJSession(session db.DJSession, restoreErr error) Option {
	return func(m *Model) { *m = m.djRestore(session, restoreErr) }
}

// WithDJWriter sets the writer that persists Auto-DJ session transitions
// (start, stop, resume, reseed, apply, allocation).
func WithDJWriter(w djSessionWriter) Option {
	return func(m *Model) { m.djWriter = w }
}

// WithDJQueueWriter sets the writer that prepares, publishes, and retires
// Auto-DJ queue batches.
//
// It supersedes the pre-DJ pending-radio path wherever both are injected: rows
// it publishes carry DJ ownership and the planner's reasons, and the discovery
// carry the batch advanced lands in the same transaction, so a crash between
// publishing a batch and recording what it consumed is not representable.
func WithDJQueueWriter(w djQueueWriter) Option {
	return func(m *Model) { m.djQueue = w }
}

// WithRadioRecentHashes seeds the radio recent-history ring buffer, e.g. from
// play history at startup so avoidance of just-heard tracks survives a
// restart. Input is oldest-first; entries beyond RadioRecentCap are dropped
// from the front.
func WithRadioRecentHashes(hashes []string) Option {
	return func(m *Model) {
		m.radio.recentHashes = trimRadioRecent(append([]string(nil), hashes...))
	}
}

// WithVolume pre-sets the volume level.
func WithVolume(v int) Option {
	return func(m *Model) { m.playback.volume = max(0, min(v, 100)) }
}

// WithPlaybackState pre-sets the playback state.
func WithPlaybackState(state playback.State) Option {
	return func(m *Model) { m.playback.state = state }
}

// WithCurrentTrackHash pre-sets the current track hash.
func WithCurrentTrackHash(hash string) Option {
	return func(m *Model) { m.playback.currentHash = hash }
}

// WithCurrentIndex pre-sets the current queue index so that
// FocusNowPlaying works immediately after session restore.
func WithCurrentIndex(index int) Option {
	return func(m *Model) { m.playback.currentIndex = index }
}

// WithStatusBarTrack pre-sets the status bar track metadata so the first
// render after session restore shows artist, title, and playlist source.
func WithStatusBarTrack(artist, title, source string) Option {
	return func(m *Model) {
		m.statusBar.Artist = artist
		m.statusBar.Title = title
		m.statusBar.PlaylistSource = source
	}
}

// WithPlaylists pre-populates the playlists view.
func WithPlaylists(rows []db.PlaylistRow) Option {
	return func(m *Model) { m.playlists.Playlists = rows }
}

// WithLibraryTracks pre-populates the library index from cached tracks.
func WithLibraryTracks(tracks []library.Track) Option {
	return func(m *Model) {
		m.library.Index.BulkLoad(tracks)
		m.scanned = len(tracks)
		m.scanTotal = len(tracks)
	}
}

// WithSessionCount sets the number of times the app has been launched,
// including the current session. The TUI uses this to show one-time tips to
// new users (e.g. command palette hint for the first 3 sessions).
func WithSessionCount(n int) Option {
	return func(m *Model) { m.sessionCount = n }
}

// WithFreshLibrary signals that this is the first-ever library scan (no tracks
// existed before). The TUI uses this to switch to Recently Added mode and set
// the initial sort state so new users see newly scanned tracks immediately.
func WithFreshLibrary(fresh bool) Option {
	return func(m *Model) { m.freshLibrary = fresh }
}
