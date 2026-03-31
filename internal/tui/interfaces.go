package tui

import (
	"github.com/sglavoie/muse/internal/db"
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

// playHistoryRecorder records a completed play to the play_history table.
// Root calls it instead of playCountIncrementer when available, because
// RecordPlay atomically increments play_count AND sets last_played_at.
type playHistoryRecorder interface {
	RecordPlay(hash string, playedAt int64, listenedSecs int) error
}

// listeningStatsReader queries aggregated listening statistics from the
// play_history table. Root uses it to populate the stats overlay (Session 3).
type listeningStatsReader interface {
	ReadListeningStats(sinceUnix int64) (db.ListeningStats, error)
}

// scanUpsertBatcher batches library track upserts during a rescan so that
// newly discovered tracks are persisted to the database.
type scanUpsertBatcher interface {
	UpsertBatch([]library.Track) error
}

// userEditedReader reads user-edited track metadata from the database.
// Used to build an overlay map so that rescan and watcher flows preserve
// manually edited metadata in the in-memory index.
type userEditedReader interface {
	ReadUserEditedTracks() ([]db.UserEditedRow, error)
}
