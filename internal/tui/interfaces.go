package tui

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
)

// playPauser is the subset of the player interface used by the root model for
// global play/pause toggling and queue-index playback. Keeping it narrow makes
// test injection simple.
type playPauser interface {
	Play() error
	Pause() error
	PlayFromIndex(int) error
}

// commandPlayer is the optional extended interface for full playback control.
// Root checks for it via type assertion so that test stubs only need to
// implement playPauser.
type commandPlayer interface {
	Stop() error
	SeekAbsolute(seconds float64) error
	SeekRelative(seconds float64) error
	SetVolumeRelative(delta float64) error
	Next() error
	Previous() error
}

// playerQueueAppender is an optional interface for appending a single entry to
// the player's in-memory queue without replacing the whole queue. Root checks
// for it via type assertion after a track is enqueued from the library view.
type playerQueueAppender interface {
	AppendToQueue(player.QueueTrack)
}

// playerQueueSetter is an optional interface for replacing the player's entire
// in-memory queue. Root checks for it via type assertion on ClearQueueMsg (to
// clear the player queue) and UndoMsg for undoKindClear (to restore it), keeping
// the player and TUI queue lists in sync after destructive clear operations.
type playerQueueSetter interface {
	SetQueue([]player.QueueTrack)
}

// playerQueueRemover is an optional interface for removing a single entry from
// the player's in-memory queue by index. Root checks for it via type assertion
// in the DequeueTrackMsg handler to keep the player queue in sync with the TUI
// queue after a track is dequeued.
type playerQueueRemover interface {
	RemoveFromQueue(index int) error
}

// playerIndexSetter is an optional interface for updating the player's current
// queue index after the TUI reorders or deduplicates the queue. Without this,
// the player's auto-advance uses a stale index causing skipped or repeated
// tracks after shuffle, dedup, move, or paste operations.
type playerIndexSetter interface {
	SetCurrentIndex(index int)
}

// positionSyncer is implemented by player.Manager. Root uses it to
// periodically persist the current playback position for crash-resume.
type positionSyncer interface {
	SyncPosition() error
}

// repeatShuffler is an optional extension checked via type assertion. Root
// calls SetRepeat and SetShuffle after cycling the model modes so that mpv is
// instructed to change its repeat/shuffle mode and the player state stays in
// sync with what the status bar displays. SetRepeat receives the string form
// of RepeatMode ("none", "all", or "one") to avoid a circular import with the
// player package.
type repeatShuffler interface {
	SetRepeat(mode string) error
	SetShuffle(bool) error
}

// queueCycleRestarter is an optional interface exposing RestartQueueCycle for
// repeat-all queue exhaustion. When shuffle is on it builds a fresh permutation
// (avoiding an immediate replay of the just-played track); when off it restarts
// sequentially from index zero. Root checks for it via type assertion so that
// lightweight test players implementing only playPauser fall back to
// PlayFromIndex(0).
type queueCycleRestarter interface {
	RestartQueueCycle() error
}

// queueTraversalReader is an optional interface exposing the player-owned
// playback timeline. Root checks for it via type assertion wherever a decision
// depends on how much of the queue is still ahead: the visible queue index says
// nothing about that under shuffle, where traversal order is Go-owned and
// unrelated to the durable row order. Lightweight test players implementing only
// playPauser fall back to visible-index arithmetic.
type queueTraversalReader interface {
	QueueTraversal() player.QueueTraversal
}

// queueExtensionResumer is an optional interface exposing
// ResumeAfterQueueExtension, which continues the traversal already in progress
// after the queue grew and reports the queue index that actually started. Root
// checks for it via type assertion on every natural-exhaustion resume so that a
// completed shuffle cycle is not reanchored — PlayFromIndex, the fallback for
// players lacking the extension, would start a fresh permutation and replay rows
// the cycle already visited.
type queueExtensionResumer interface {
	ResumeAfterQueueExtension() (int, error)
}

// stopAfterToggler enables or disables "stop after current track" mode in
// the player. Root checks for it via type assertion so that test stubs only
// need to implement playPauser.
type stopAfterToggler interface {
	SetStopAfterCurrent(bool)
	StopAfterCurrent() bool
}

// elapsedReader is implemented by player.Manager to provide the current
// elapsed playback position and total track duration. Root queries it on
// each 1-second tick to keep the status bar time display current.
type elapsedReader interface {
	ElapsedPosition() (elapsed, total float64, err error)
}

// queueReorderer persists adjacent-item swaps for the queue order. Root uses
// it when MoveTrackUpMsg or MoveTrackDownMsg is processed. When nil, the TUI
// slice is still swapped but the change is not persisted to the database.
type queueReorderer interface {
	ReorderQueueTrack(id int64, direction int) error
}

// queueShuffler persists a full queue reorder (e.g. Fisher-Yates shuffle).
// Root checks for it via type assertion on the reorderer field. When nil, the
// in-memory shuffle is still applied but not written to the database.
type queueShuffler interface {
	SetQueueOrder(ids []int64) error
}

// queueDequeuer is the db.QueueDequeuer interface. Aliased here to keep
// the tui package's dependency on db narrow and explicit.
type queueDequeuer = db.QueueDequeuer

// playlistTrackReader loads playlist tracks from the database. Root uses it
// when the user opens a playlist to transition to TrackLevel. When nil,
// actionPlaylistOpen is a no-op.
type playlistTrackReader interface {
	ReadPlaylistTracks(playlistID int64) ([]db.PlaylistTrackRow, error)
}

// playlistCreator creates a new playlist and re-reads the full list. Root uses
// it when the user confirms the inline create input. When nil, confirming the
// input is a no-op (the field is dismissed without persisting).
type playlistCreator interface {
	CreatePlaylist(name string) (int64, error)
	ReadPlaylists() ([]db.PlaylistRow, error)
}

// playlistRenamer renames a playlist and re-reads the full list. Root uses it
// when the user confirms the inline rename input. When nil, confirming the
// input is a no-op (the field is dismissed without persisting).
type playlistRenamer interface {
	RenamePlaylist(id int64, name string) error
	ReadPlaylists() ([]db.PlaylistRow, error)
}

// playlistDeleter handles soft-delete, undo, and purge operations for
// playlists and their tracks. Root performs a type assertion to detect it;
// tests use stubs. When nil, destructive playlist actions still update the
// in-memory TUI state but skip all database operations.
type playlistDeleter interface {
	DeletePlaylistSoft(id int64) error
	UndoDeletePlaylist(id int64) error
	PurgeDeletedPlaylists(before int64) error
	RemovePlaylistTrackSoft(id int64) error
	UndoRemovePlaylistTrack(id int64) error
	PurgeDeletedPlaylistTracks(before int64) error
}

// batchEnqueuer supports batch enqueue operations and their undo. Root uses it
// when the user presses 'E' in the playlists view to enqueue all tracks from a
// playlist, and when undoing a batch enqueue. When nil, the 'E' keybinding is
// a no-op.
type batchEnqueuer interface {
	AppendTracks(tracks []library.Track, sourcePlaylist string) ([]int64, error)
	DequeueTracksSoft(ids []int64) error
}

// playlistShuffler persists a full playlist track reorder (e.g. Fisher-Yates
// shuffle). Root checks for it via type assertion on the playlistDeleter field.
// When nil, the in-memory shuffle is still applied but not written to the database.
type playlistShuffler interface {
	SetPlaylistOrder(playlistID int64, ids []int64) error
}

// playlistTrackReorderer persists adjacent-item swaps for playlist track order.
// Root checks for it via type assertion on the playlistDeleter field.
type playlistTrackReorderer interface {
	ReorderPlaylistTrack(playlistID, trackID int64, direction int) error
}

// playlistTrackAdder appends tracks to a playlist by hash. Root uses it when
// the user confirms a destination-picker modal selection pointing to a playlist.
// When nil, the playlist append path of the destination picker is a no-op.
type playlistTrackAdder interface {
	AddTracksToPlaylist(playlistID int64, hashes []string) error
}

// sessionWriter is the db.SessionWriter interface. Aliased here to keep
// the tui package's dependency on db narrow and explicit.
type sessionWriter = db.SessionWriter

// metadataUpdater persists user-edited metadata to the database.
type metadataUpdater interface {
	UpdateTrackMetadata(hash, title, artist, album, genre string) error
}

// durationUpdater persists a manually edited track duration to the database.
type durationUpdater interface {
	UpdateTrackDuration(hash string, durationMs int64) error
}

// trackPositionUpdater persists an imported track/disc number to the database.
type trackPositionUpdater interface {
	UpdateTrackPosition(hash string, trackNumber, discNumber int) error
}

// favoriteReader loads favorites from the database.
type favoriteReader interface {
	ReadFavoritesByType(typ string) ([]db.FavoriteRow, error)
	ReadFavoriteCounts() (artists, albums, tracks int, err error)
	IsFavorite(typ, value string) (bool, error)
}

// favoriteWriter is the db.FavoriteWriter interface. Aliased here to keep
// the tui package's dependency on db narrow and explicit.
type favoriteWriter = db.FavoriteWriter

// audioNormalizer enables or disables real-time EBU R128 volume normalization
// via the player's audio filter chain. Root checks for it via type assertion
// on settings save and is not required by test stubs.
type audioNormalizer interface {
	SetNormalize(enabled bool) error
}

// gaplessToggler enables or disables gapless playback via the player.
// Root checks for it via type assertion on settings save.
type gaplessToggler interface {
	SetGapless(enabled bool) error
}

// equalizerSetter enables or disables the 10-band parametric equalizer via the
// player. Root checks for it via type assertion on settings save and live
// preview.
type equalizerSetter interface {
	SetEqualizer(enabled bool, bands [10]int) error
}

// stereoWideSetter enables or disables stereo widening via the player.
// Root checks for it via type assertion on settings save and live preview.
type stereoWideSetter interface {
	SetStereoWide(enabled bool, amount int) error
}

// smartPlaylistEvaluator evaluates a SmartPlaylistQuery against the database
// and returns the matching tracks. Root uses it when a smart playlist is
// opened via actionPlaylistOpen. When nil, opening a smart playlist is a no-op.
type smartPlaylistEvaluator interface {
	EvalSmartPlaylist(db.SmartPlaylistQuery) ([]db.PlaylistTrackRow, error)
}

// playCountIncrementer increments a track's play count in the database.
// Root calls it on TrackChangedMsg when the previous track hash is non-empty.
// When nil, play counts are not persisted.
type playCountIncrementer interface {
	IncrementPlayCount(hash string) error
}

// debugInfoProvider is implemented by player.Manager to expose diagnostic
// properties for the debug panel. Root checks for it via type assertion so that
// test stubs do not need to implement it.
type debugInfoProvider interface {
	DebugInfo() (map[string]string, error)
}

// smartPlaylistWriter creates and updates smart playlists in the database.
// Root uses it when the SmartRuleEditor commits a new or edited smart playlist.
// When nil, the commit is a no-op.
type smartPlaylistWriter interface {
	CreateSmartPlaylist(name, queryJSON string) (int64, error)
	UpdateSmartPlaylistQuery(id int64, name, queryJSON string) error
	ReadPlaylists() ([]db.PlaylistRow, error)
}

// playHistoryRecorder records a listen to the play_events table.
// Root calls it instead of playCountIncrementer when available, because a
// counted RecordListen atomically increments play_count AND sets
// last_played_at.
type playHistoryRecorder interface {
	RecordListen(rec db.PlayRecord) error
}

// playHistoryReader loads raw play history rows from the database. Root uses
// it to lazily populate the History tab (view 9) on first switch.
type playHistoryReader interface {
	ReadPlayHistory(limit int) ([]library.Track, error)
}

// insightsReader supplies the raw rows the Insights view aggregates. Every
// method is a pure read; all the shaping happens in internal/insights.
type insightsReader interface {
	ReadInsightPlays(sinceUnix int64) ([]insights.PlayRow, error)
	ReadInsightGenres(sinceUnix int64) ([]insights.GenreRow, error)
	ReadInsightSessions(sinceUnix int64) ([]insights.SessionRow, error)
	ReadInsightCoverage() ([]insights.CoverageRow, error)
	ReadInsightTimeline() ([]insights.TimelineRow, error)
	ReadInsightQuality(deepCutsMinPlays, deepCutsMaxPlays int, deepCutsMinCompletion float64) (insights.QualityCounts, error)
	// ReadUnverifiedDurations backs the Data Health row for durations that were
	// taken from tags and never checked against the file.
	ReadUnverifiedDurations() ([]db.UnverifiedTrack, error)
}

// scanUpsertBatcher batches library track upserts during a rescan so that
// newly discovered tracks are persisted to the database.
type scanUpsertBatcher interface {
	UpsertBatch([]library.Track) error
}

// trackMissingMarker persists a track's missing flag by hash. The manual
// rescan path type-asserts m.scanWriter to it so that a file which vanished
// between scans is marked missing durably, mirroring the live watcher's
// deletion handling. Optional: when the scan writer does not implement it, the
// rescan still reconciles in-memory state but skips the durable mark.
type trackMissingMarker interface {
	MarkTrackMissing(hash string) error
}

// userEditedReader reads user-edited track metadata from the database.
// Used to build an overlay map so that rescan and watcher flows preserve
// manually edited metadata in the in-memory index.
type userEditedReader interface {
	ReadUserEditedTracks() ([]db.UserEditedRow, error)
}

// tagReader loads tag data from the database. Root uses it to populate the
// in-memory tag cache on startup and to refresh it after any mutation.
type tagReader interface {
	ReadTags() ([]db.TagRow, error)
	ReadTagsForTrack(hash string) ([]db.TagRow, error)
	ReadTrackHashesByTag(tagID int64) ([]string, error)
	ReadTracksByTag(tagID int64) ([]db.PlaylistTrackRow, error)
	ReadTagCounts() (map[int64]int, error)
}

// recommendationReader provides recommendation mix queries and behavioral
// signals used by MixesView and Auto-DJ scoring.
type recommendationReader interface {
	HasSufficientHistory(minCount int) (bool, error)
	ReadRediscoverMix(minPlays, ageDays, limit int) ([]library.Track, error)
	ReadHiddenGemsMix(maxPlays, topArtists, topGenres, limit int) ([]library.Track, error)
	ReadOnRepeatMix(minPlays, windowDays, limit int) ([]library.Track, error)
	ReadDeepCutsMix(minPlays, maxPlays int, minCompletion float64, limit int) ([]library.Track, error)
	ReadNeverPlayedMix(limit int) ([]library.Track, error)
	ReadCompletionRates(hashes []string) (map[string]float64, error)
	ReadSessionCoOccurrences(seedHash string, limit int) ([]db.SessionCoOccurrence, error)
	ReadHourGenreAffinities() ([]db.HourGenreAffinity, error)
	ReadAllMixProgress() (map[db.MixID]db.MixProgress, error)
}

// dismissalReader loads the set of dismissed track hashes from the database.
// Root uses it on startup to pre-populate the in-memory dismissedHashes set.
type dismissalReader interface {
	ReadDismissedTrackHashes() (map[string]struct{}, error)
}

// dismissalWriter is the db.DismissalWriter interface. Aliased here to keep
// the tui package's dependency on db narrow and explicit.
type dismissalWriter = db.DismissalWriter

// cachedMixReader reads pre-computed mix memberships from the
// recommendation_cache table. Root uses it as a fast path in loadMixCmd and
// loadMixCountsCmd, falling back to live queries when the cache is empty.
type cachedMixReader interface {
	ReadCachedMix(mixID, limit int) ([]library.Track, error)
}

// recommendationCacheRebuilder pre-computes mix memberships and writes them to
// recommendation_cache. Root fires this asynchronously after every N plays.
type recommendationCacheRebuilder interface {
	RebuildRecommendationCache(entries []db.CacheEntry) error
}

// tagWriter persists tag mutations to the database. Root uses it when the
// user creates, renames, deletes, or toggles tags from the tag picker overlay.
type tagWriter interface {
	CreateTag(name, color string) (int64, error)
	RenameTag(id int64, name string) error
	UpdateTagColor(id int64, color string) error
	DeleteTagSoft(id int64) error
	DeleteTagIfEmpty(tagID int64) (bool, error)
	PurgeDeletedTags(before int64) error
	UndoDeleteTag(id int64) error
	AddTrackTag(hash string, tagID int64) error
	RemoveTrackTag(hash string, tagID int64) error
	RemoveAllTrackTags(hash string) error
	AddTrackTagBulk(hashes []string, tagID int64) error
	RemoveTrackTagBulk(hashes []string, tagID int64) error
	MergeTags(sourceID, destID int64) error
}

// trackNotifier posts a desktop notification for a newly started track. Root
// calls it from handleTrackChangedMsg only when the incoming hash differs from
// the previous one and cfg.NotifyOnTrackChange is on, so mpv reconnects and
// session restore never re-banner the track already on screen.
type trackNotifier interface {
	NotifyTrack(title, artist, album string) error
}
