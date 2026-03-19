package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
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

// queueDequeuer handles soft-delete, undo, and purge operations for queue
// entries. Root performs a type assertion to detect it; tests use stubs.
// When nil, destructive actions still update the in-memory TUI state but
// skip all database operations.
type queueDequeuer interface {
	DequeueTrackSoft(id int64) error
	UndoDequeue(id int64) error
	ClearQueueSoft() ([]int64, error)
	UndoClearQueue(ids []int64) error
	PurgeDeletedQueueEntries(before int64) error
}

// undoKind distinguishes which type of destructive action is reversible.
type undoKind uint8

const (
	undoNone                         undoKind = iota
	undoKindEnqueue                           // single track added via TrackEnqueuedMsg
	undoKindDequeue                           // single track removed via DequeueTrackMsg
	undoKindClear                             // all tracks removed via ClearQueueMsg
	undoKindDeletePlaylist                    // playlist soft-deleted via actionPlaylistDelete at ListLevel
	undoKindRemovePlaylistTrack               // track soft-removed via actionPlaylistDelete at TrackLevel
	undoKindBatchEnqueue                      // batch of tracks added via actionPlaylistEnqueueAll
	undoKindBulkDequeue                       // multiple selected tracks removed via bulk dequeue
	undoKindBulkRemovePlaylistTracks          // multiple selected tracks removed from playlist via 'a'
	undoKindBulkEditMetadata                  // bulk metadata edit via 'M' with selection
)

// undoSlot holds the reversible state for the most recent destructive queue
// action. Only one slot is maintained; a new action replaces the prior one.
type undoSlot struct {
	kind      undoKind
	expiresAt time.Time

	// cancelPurge is closed when this slot is superseded by a new destructive
	// action or when undo is successfully performed. The time.AfterFunc purge
	// callback checks this channel before running to avoid purging rows that
	// belong to a newer undo slot (e.g. same-second dequeue followed by another
	// dequeue of the same DB row ID after an undo).
	cancelPurge chan struct{}

	// Dequeue fields — valid when kind == undoKindDequeue.
	dequeueID    int64
	dequeueEntry QueueEntry
	dequeueIdx   int

	// Clear fields — valid when kind == undoKindClear.
	clearIDs     []int64
	clearEntries []QueueEntry
	clearTracks  []player.QueueTrack // parallel to clearEntries; used to restore player queue on undo

	// DeletePlaylist fields — valid when kind == undoKindDeletePlaylist.
	deletePlaylistEntry db.PlaylistRow
	deletePlaylistIdx   int

	// RemovePlaylistTrack fields — valid when kind == undoKindRemovePlaylistTrack.
	removeTrackEntry db.PlaylistTrackRow
	removeTrackIdx   int

	// BatchEnqueue fields — valid when kind == undoKindBatchEnqueue.
	batchIDs      []int64 // DB row IDs of all batch-enqueued entries
	batchCount    int     // number of entries added
	batchStartIdx int     // index of the first batch entry in queue.entries

	// BulkDequeue fields — valid when kind == undoKindBulkDequeue.
	bulkDequeueEntries []QueueEntry // removed entries in original order
	bulkDequeueIndices []int        // original indices for re-insertion
	bulkDequeueIDs     []int64      // DB IDs for undo

	// BulkRemovePlaylistTracks fields — valid when kind == undoKindBulkRemovePlaylistTracks.
	bulkRemoveTrackEntries []db.PlaylistTrackRow // removed tracks in original order
	bulkRemoveTrackIndices []int                 // original indices for re-insertion

	// BulkEditMetadata fields — valid when kind == undoKindBulkEditMetadata.
	bulkEditOriginals map[string][3]string // hash → {title, artist, album}
}

// tickMsg is dispatched every second by tickCmd. It drives elapsed-time
// display updates and transient-notification expiry checks.
type tickMsg struct{}

// sessionTickMsg is dispatched every sessionTickInterval by sessionTickCmd.
// On each tick the current playback position is persisted to the session table,
// but only when playback is active (PlaybackPlaying).
type sessionTickMsg struct{}

// syncPositionMsg is dispatched every positionSyncInterval to trigger a
// playback-position update on the player.
type syncPositionMsg struct{}

// clearConfigNotice is sent after the transient first-run notification timeout.
type clearConfigNotice struct{}

// MissingTrackMsg is dispatched by the player when it attempts to play a
// queued track whose file cannot be found. The TUI marks that entry as missing
// in the queue view and displays a transient error in the status bar.
type MissingTrackMsg struct {
	Path string
}

// PlayerStatusMsg carries a transient status string from the player backend
// (e.g. "Connecting to mpv…" or "mpv disconnected, restarting…") into the
// TUI status bar. Wire it by calling mgr.SetStatusFunc with a callback that
// calls p.Send(PlayerStatusMsg{Text: s}).
type PlayerStatusMsg struct {
	Text string
}

// PlayQueueIndexMsg is dispatched by the queue view when the user presses
// Enter on a track. Root handles it by starting playback from that index.
type PlayQueueIndexMsg struct {
	Index int
}

// MissingTrackSelectedMsg is dispatched by the queue view when the user
// presses Enter on a missing track. Root handles it by showing a transient
// notification explaining why playback cannot start.
type MissingTrackSelectedMsg struct{}

// PlaybackEndedMsg is dispatched by the player when auto-advance exhausts the
// queue (no more playable entries). Root handles it by resetting the playback
// state to idle so the space-bar toggle stays synchronized with mpv's state.
type PlaybackEndedMsg struct{}

// RemoteActionMsg is sent when an external signal (SIGUSR1) requests a
// playback action, e.g. from a global keybinding. ActionName maps to an
// action constant via remoteAction(); an empty name defaults to play/pause
// for backward compatibility with bare `kill -USR1`.
type RemoteActionMsg struct{ ActionName string }

// remoteAction maps a subcommand name to the corresponding action constant.
// An empty or unrecognized name defaults to actionPlayPause for backward
// compatibility with bare SIGUSR1.
func remoteAction(name string) action {
	switch name {
	case "toggle-play":
		return actionPlayPause
	case "next":
		return actionNext
	case "previous":
		return actionPrevious
	case "volume-up":
		return actionVolumeUp
	case "volume-down":
		return actionVolumeDown
	default:
		return actionPlayPause
	}
}

// previewStartedMsg is dispatched when the preview player has been launched
// and is ready for playback.
type previewStartedMsg struct {
	player  player.Previewer
	overlay PreviewOverlay
}

// previewStartErrorMsg is dispatched when the preview player fails to start.
type previewStartErrorMsg struct{ err error }

// UndoMsg is dispatched when the user presses 'u'.
type UndoMsg struct{}

// FilterBarMsg is dispatched when the user presses '/' to activate the
// filter bar in the active view.
type FilterBarMsg struct{}

// BatchEnqueuedMsg carries the result of a batch enqueue operation (all tracks
// from a playlist). Root handles it by appending all entries to the queue,
// syncing the player, and setting up the undo slot.
type BatchEnqueuedMsg struct {
	Entries      []QueueEntry
	QueueIDs     []int64
	WasEmpty     bool
	PlaylistName string
	Err          error
}

// DequeueTrackMsg is dispatched when the user presses 'd' in the queue view
// to remove the entry at Index from the playback queue.
type DequeueTrackMsg struct {
	Index int
}

// BulkSelectionToggleMsg is dispatched when the user presses 'x' in the queue
// view to toggle bulk-selection on the entry at Index.
type BulkSelectionToggleMsg struct {
	Index int
}

// ClearQueueMsg is dispatched when the user presses 'C' (Shift+c) in the
// queue view to remove all entries from the playback queue.
type ClearQueueMsg struct{}

// MoveTrackUpMsg is dispatched when the user presses 'K' (Shift+k) in the
// queue view to move the entry at Index one position earlier.
type MoveTrackUpMsg struct {
	Index int
}

// MoveTrackDownMsg is dispatched when the user presses 'J' (Shift+j) in the
// queue view to move the entry at Index one position later.
type MoveTrackDownMsg struct {
	Index int
}

// RepeatMode controls how the queue cycles at the end of play.
type RepeatMode uint8

const (
	RepeatOff RepeatMode = iota // no repeat; stop after the last track
	RepeatAll                   // repeat the entire queue from the beginning
	RepeatOne                   // repeat the current track indefinitely
)

// String returns the session-table value for this RepeatMode. The values match
// the CHECK constraint in the session schema: 'none', 'all', or 'one'.
func (r RepeatMode) String() string {
	switch r {
	case RepeatAll:
		return "all"
	case RepeatOne:
		return "one"
	default:
		return "none"
	}
}

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

// sessionWriter is implemented by db.Writer. Root calls WriteRepeatMode after
// each actionRepeatCycle, WriteShuffle after each actionShuffleToggle, and
// UpdateSessionPosition on pause, seek, and track-change events to persist
// state changes to the session table via the single-writer goroutine.
type sessionWriter interface {
	WriteRepeatMode(mode string) error
	WriteShuffle(enabled bool) error
	UpdateSessionPosition(hash string, positionMs int64) error
}

// PlaybackState represents the current playback status shown in the status bar.
type PlaybackState int

const (
	PlaybackIdle    PlaybackState = iota // no track loaded; no indicator shown
	PlaybackPlaying                      // actively playing; shows ▶
	PlaybackPaused                       // paused; shows ⏸
	PlaybackStopped                      // stopped at track start; shows ■
)

// TrackChangedMsg is dispatched when the active track changes (e.g. via
// auto-advance, Next, or Previous). Root handles it by updating the status bar
// track info and transitioning to the playing state.
type TrackChangedMsg struct {
	Artist         string
	Title          string
	PlaylistSource string // "Queue" or a named playlist name; defaults to "Queue" if empty
	Hash           string // content hash of the new track; used for session persistence
}

// scanBatchSize is the maximum number of tracks collected per batch before
// returning a ScanBatchMsg to the update loop.
const scanBatchSize = 100

// watchBatchMsg carries tracks discovered by the filesystem watcher and paths
// of files deleted/renamed-away. Unlike ScanBatchMsg, receiving this message
// does not reset the library index: modified tracks are upserted by path and
// deleted paths are evicted.
type watchBatchMsg struct {
	tracks  []library.Track
	deleted []string
}

// Model is the root Bubble Tea v2 model. It manages view switching between
// Library, Playlists, Queue, and Artists panels, and renders a persistent status bar.
type Model struct {
	appCtx        context.Context // application-level context for subprocess lifecycle
	activeView    viewID
	configPath    string
	configCreated bool

	library   LibraryView
	playlists PlaylistsView
	queue     QueueView
	artists   ArtistsView
	albums    AlbumsView

	// scanCh is the channel returned by library.Scan. It is nil when no scan
	// is in progress or after the scan has completed.
	scanCh    <-chan library.Track
	scanning  bool
	scanned   int
	scanTotal int // estimated total from the parallel counting goroutine

	// watchCh is the channel returned by library.Watch. It emits WatchEvents
	// (track additions/updates and deletion notifications) and is nil when
	// watching is disabled.
	watchCh <-chan library.WatchEvent

	// libraryRoot and extractor are required for manual rescan. When
	// libraryRoot is empty the rescan hotkey is a no-op.
	libraryRoot string
	extractor   library.MetadataExtractor

	statusBar      StatusBar
	termWidth      int
	termHeight     int
	registry       Registry
	helpVisible    bool           // true while the help overlay is shown
	helpViewport   viewport.Model // scrollable viewport for the help overlay
	helpFiltered   bool           // true when showing only current-view keybindings
	prevActiveView viewID         // the view that was active before help was opened

	// player is the playback backend. It may be nil when playback is not
	// available (e.g. in tests that do not exercise playback).
	player          playPauser
	queueWriter     libtui.QueueAppender // may be nil; set via SetQueueWriter
	reorderer       queueReorderer       // may be nil; set via SetQueueReorderer
	undoer          queueDequeuer        // may be nil; set via SetQueueDequeuer
	sessionWriter   sessionWriter        // may be nil; set via SetSessionWriter
	playlistReader  playlistTrackReader  // may be nil; set via SetPlaylistTrackReader
	playlistWriter  playlistCreator      // may be nil; set via SetPlaylistWriter
	playlistRenamer playlistRenamer      // may be nil; set via SetPlaylistRenamer
	playlistDeleter playlistDeleter      // may be nil; set via SetPlaylistDeleter
	playlistAdder   playlistTrackAdder   // may be nil; set via SetPlaylistAdder
	batchEnqueuer   batchEnqueuer        // may be nil; set via SetBatchEnqueuer

	// preview is non-nil while the preview overlay is active.
	preview           *PreviewOverlay
	previewPlayer     player.Previewer // separate mpv instance for preview
	previewWasPlaying bool             // main player was playing before preview

	// nowPlayingInfo is non-nil while the now-playing info overlay is active.
	nowPlayingInfo *NowPlayingOverlay

	// trackInfo is non-nil while the track info overlay is active.
	trackInfo *TrackInfoOverlay

	// metadataEditor is non-nil while the metadata editor overlay is active.
	metadataEditor *MetadataEditor
	metadataWriter metadataUpdater // may be nil; set via SetMetadataWriter

	// destPicker is non-nil while the destination picker modal is open.
	// Root dispatches all keypresses to the modal while it is open.
	destPicker *pltui.DestinationPickerModal
	// destPickerTracks holds the tracks staged for the pending destination
	// action. Populated when the modal opens; cleared on confirmation or dismiss.
	destPickerTracks []library.Track

	// undoSlot holds the reversible state for the most recent destructive
	// queue action. The slot is cleared after a successful undo or when a new
	// destructive action replaces it before the window expires.
	undoSlot undoSlot

	// missingTrackCount tracks how many queue entries were skipped due to
	// missing files during the current playback session. Reset when playback
	// ends or the queue is cleared.
	missingTrackCount int

	// clearQueuePending is true when the user has pressed 'C' once and is
	// waiting for a second press to confirm clearing the queue.
	clearQueuePending   bool
	clearQueuePendingAt time.Time

	// bulkRemovePending is true when the user has pressed 'a' once in
	// Playlists view with selected tracks and is waiting for a second
	// press to confirm removing them.
	bulkRemovePending   bool
	bulkRemovePendingAt time.Time

	playbackState    PlaybackState // current play/pause/stopped/idle state
	repeatMode       RepeatMode    // repeat mode: off, all, or one
	shuffle          bool          // shuffle mode enabled
	volume           int           // current volume level (0–100)
	currentTrackHash string        // content hash of the track currently loaded in the player

	// elapsedSecs and totalSecs hold the most recently queried playback
	// position and track duration in seconds. Zero values mean no time
	// data is available and suppress the time display in the status bar.
	elapsedSecs float64
	totalSecs   float64

	// runtimeDir is the XDG_RUNTIME_DIR-based directory for ephemeral sockets
	// (e.g. preview player). Set via SetRuntimeDir before tea.NewProgram.
	runtimeDir string

	// styles holds the current lipgloss style set. Initialized to dark-safe
	// defaults in New(); replaced on tea.BackgroundColorMsg once the terminal
	// reports its actual background color.
	styles *styles.Styles
}

// New creates a root Model. configPath is displayed in the status bar on every
// render. When configCreated is true, configPath is also shown as a transient
// first-run notification.
//
// scanCh may be nil when library scanning is not needed (e.g. in tests). Pass
// the channel returned by library.Scan to enable incremental library updates.
//
// watchCh may be nil to disable filesystem-change watching. Pass the channel
// returned by library.Watch to receive incremental updates: new/modified tracks
// are upserted by path and deleted paths are evicted from the index without a
// full rebuild. The context passed to library.Watch must be cancelled when the
// application exits to avoid a goroutine leak.
//
// libraryRoot and extractor enable the manual rescan hotkey. Pass empty string
// and nil to disable it (e.g. in tests that do not exercise rescan).
//
// p is the playback backend. Pass nil to disable playback controls.
func New(ctx context.Context, configPath string, configCreated bool, scanCh <-chan library.Track, watchCh <-chan library.WatchEvent, libraryRoot string, extractor library.MetadataExtractor, p playPauser) Model {
	initialStyles := styles.NewStyles(true)
	return Model{
		appCtx:        ctx,
		activeView:    viewLibrary,
		configPath:    configPath,
		configCreated: configCreated,
		library:       LibraryView{fuzzy: library.SahilmFuzzySearcher{}},
		scanCh:        scanCh,
		scanning:      scanCh != nil,
		watchCh:       watchCh,
		libraryRoot:   libraryRoot,
		extractor:     extractor,
		player:        p,
		registry:      newRegistry(),
		styles:        &initialStyles,
		statusBar: StatusBar{
			configPath:    configPath,
			configCreated: configCreated,
		},
	}
}

// SetQueueEntries pre-populates the queue view with entries loaded from the
// database before the first render. Call this on the Model returned by New
// before passing it to tea.NewProgram, so that the queue view and the player
// Manager share the same index-ordered entries from the start.
func (m *Model) SetQueueEntries(entries []QueueEntry) {
	m.queue.entries = entries
}

// SetQueueWriter sets the QueueAppender used by the library-view Enter handler
// to persist newly enqueued tracks to the database. Call this on the Model
// returned by New before passing it to tea.NewProgram. Passing nil disables
// enqueueing (Enter in the library view becomes a no-op).
func (m *Model) SetQueueWriter(qa libtui.QueueAppender) {
	m.queueWriter = qa
}

// SetQueueReorderer sets the queueReorderer used to persist queue entry
// position swaps in the database. Call this on the Model returned by New
// before passing it to tea.NewProgram. When nil, MoveTrackUpMsg/MoveTrackDownMsg
// still update the TUI slice but skip the database write.
func (m *Model) SetQueueReorderer(r queueReorderer) {
	m.reorderer = r
}

// SetQueueDequeuer sets the queueDequeuer used to soft-delete, undo, and purge
// queue entries in the database. Call this on the Model returned by New before
// passing it to tea.NewProgram. When nil, destructive actions (dequeue, clear)
// still update the TUI state but skip all database operations.
func (m *Model) SetQueueDequeuer(qd queueDequeuer) {
	m.undoer = qd
}

// SetSessionWriter sets the sessionWriter used to persist session-level state
// (such as repeat mode) to the database. Call this on the Model returned by
// New before passing it to tea.NewProgram. When nil, mode changes update
// the in-memory TUI state but skip all database writes.
func (m *Model) SetSessionWriter(sw sessionWriter) {
	m.sessionWriter = sw
}

// SetRepeatMode pre-sets the repeat mode on a newly created Model. Call this
// before passing the model to tea.NewProgram to restore persisted session state.
func (m *Model) SetRepeatMode(mode RepeatMode) {
	m.repeatMode = mode
}

// SetShuffle pre-sets the shuffle flag on a newly created Model. Call this
// before passing the model to tea.NewProgram to restore persisted session state.
func (m *Model) SetShuffle(enabled bool) {
	m.shuffle = enabled
}

// SetVolume pre-sets the volume level on a newly created Model. Call this
// before passing the model to tea.NewProgram to initialise the tracked volume.
func (m *Model) SetVolume(v int) {
	m.volume = clampVolume(v)
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

// SetPlaybackState pre-sets the playback state on a newly created Model. Call
// this with PlaybackPaused when restoring a session that has a non-empty queue,
// so that the status bar and space-bar toggle reflect the correct state on the
// first render.
func (m *Model) SetPlaybackState(state PlaybackState) {
	m.playbackState = state
}

// SetCurrentTrackHash pre-sets the current track hash on a newly created Model.
// Call this when restoring session state so that the first session-position
// write uses the correct hash.
func (m *Model) SetCurrentTrackHash(hash string) {
	m.currentTrackHash = hash
}

// SetPlaylists pre-populates the playlists view with rows loaded from the
// database before the first render. Call this on the Model returned by New
// before passing it to tea.NewProgram so that the playlist list is visible
// immediately on first render.
func (m *Model) SetPlaylists(rows []db.PlaylistRow) {
	m.playlists.playlists = rows
}

// SetLibraryTracks pre-populates the library index from cached tracks loaded
// from the database. Call this on the Model returned by New before passing it
// to tea.NewProgram to show the full library immediately on first render
// without waiting for the scan pipeline to complete.
func (m *Model) SetLibraryTracks(tracks []library.Track) {
	m.library.index.BulkLoad(tracks)
	m.scanned = len(tracks)
	m.scanTotal = len(tracks)
}

// RemoveLibraryTrackByPath removes a track from the in-memory library index
// by its file path. Call this before starting the TUI to evict deleted files
// discovered during the diff scan setup phase.
func (m *Model) RemoveLibraryTrackByPath(path string) {
	m.library.index.RemoveByPath(path)
}

// SetPlaylistTrackReader sets the playlistTrackReader used to load tracks when
// the user opens a playlist. Call this on the Model returned by New before
// passing it to tea.NewProgram. When nil, actionPlaylistOpen is a no-op.
func (m *Model) SetPlaylistTrackReader(r playlistTrackReader) {
	m.playlistReader = r
}

// SetPlaylistWriter sets the playlistCreator used to persist a new playlist
// and refresh the list after creation. Call this on the Model returned by New
// before passing it to tea.NewProgram. When nil, confirming the inline create
// input is a no-op (the field is dismissed without writing to the database).
func (m *Model) SetPlaylistWriter(w playlistCreator) {
	m.playlistWriter = w
}

// SetPlaylistRenamer sets the playlistRenamer used to persist a playlist rename
// and refresh the list after renaming. Call this on the Model returned by New
// before passing it to tea.NewProgram. When nil, confirming the inline rename
// input is a no-op (the field is dismissed without writing to the database).
func (m *Model) SetPlaylistRenamer(r playlistRenamer) {
	m.playlistRenamer = r
}

// SetPlaylistDeleter sets the playlistDeleter used to soft-delete playlists
// and tracks and to undo those deletions within the undo window. Call this on
// the Model returned by New before passing it to tea.NewProgram. When nil,
// destructive playlist actions still update the in-memory TUI state but skip
// all database operations.
func (m *Model) SetPlaylistDeleter(d playlistDeleter) {
	m.playlistDeleter = d
}

// SetPlaylistAdder sets the playlistTrackAdder used to append tracks to a
// playlist when the user confirms the destination picker modal with a playlist
// destination. Call this on the Model returned by New before passing it to
// tea.NewProgram. When nil, the playlist append path of the destination picker
// is a no-op (the modal still opens and navigates but Enter on a playlist does
// nothing).
func (m *Model) SetPlaylistAdder(a playlistTrackAdder) {
	m.playlistAdder = a
}

// SetBatchEnqueuer sets the batchEnqueuer used to batch-insert tracks into the
// queue when the user presses 'E' in the playlists view and to batch soft-delete
// on undo. Call this on the Model returned by New before passing it to
// tea.NewProgram. When nil, 'E' in the playlists view is a no-op.
func (m *Model) SetBatchEnqueuer(be batchEnqueuer) {
	m.batchEnqueuer = be
}

// SetRuntimeDir sets the runtime directory used for ephemeral sockets (e.g.
// the preview player socket). Call this on the Model returned by New before
// passing it to tea.NewProgram. When empty, the preview socket falls back to
// os.TempDir().
func (m *Model) SetRuntimeDir(dir string) {
	m.runtimeDir = dir
}

// undoWindowDuration is the time after a destructive queue action during which
// pressing 'u' will reverse the action. Only a single undo slot is maintained;
// a new destructive action within the window replaces the prior slot.
const undoWindowDuration = 5 * time.Second

// pluralize returns singular when n == 1, plural otherwise.
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// scheduleSoftDelete cancels any in-flight purge timer from the previous undo
// slot, calls softDeleteFn to perform the soft-delete, then schedules a
// time.AfterFunc purge via purgeFn. Returns the cancel channel that the new
// undo slot should store.
func (m *Model) scheduleSoftDelete(softDeleteFn func(), purgeFn func(beforeUnix int64)) chan struct{} {
	if m.undoSlot.cancelPurge != nil {
		close(m.undoSlot.cancelPurge)
	}
	cancel := make(chan struct{})
	softDeleteTime := time.Now().Unix()
	softDeleteFn()
	if purgeFn != nil {
		time.AfterFunc(undoWindowDuration, func() {
			select {
			case <-cancel:
				return
			default:
				purgeFn(softDeleteTime + 1)
			}
		})
	}
	return cancel
}

// syncPlayerQueue rebuilds the player's in-memory queue from the current TUI
// queue entries. Call this after any operation that reorders the TUI queue
// (e.g. MoveTrackUp/Down) so that the player's auto-advance resolves indices
// against the same ordering the user sees.
func (m Model) syncPlayerQueue() {
	if qs, ok := m.player.(playerQueueSetter); ok {
		pt := make([]player.QueueTrack, len(m.queue.entries))
		for i, e := range m.queue.entries {
			pt[i] = player.QueueTrack{
				Path:    e.Path,
				Hash:    e.Hash,
				QueueID: e.ID,
				Artist:  e.Artist,
				Title:   e.Title,
			}
		}
		qs.SetQueue(pt)
	}
}

// setUndoSlot replaces the current undo slot and sets a matching status-bar
// notification that expires at the end of the undo window.
func (m *Model) setUndoSlot(slot undoSlot, notification string) {
	m.undoSlot = slot
	m.statusBar.notification = transientNotification{
		msg:       notification,
		expiresAt: time.Now().Add(undoWindowDuration),
	}
}

// configNoticeDuration controls how long the first-run config path notification
// is visible in the status bar before it is automatically dismissed.
const configNoticeDuration = 5 * time.Second

// positionSyncInterval is how often root polls the player for the current
// playback position so that crash-resume can restore to the right offset.
const positionSyncInterval = 5 * time.Second

// sessionTickInterval is how often the periodic session ticker fires to persist
// the current elapsed position to the session table during uninterrupted playback.
const sessionTickInterval = 10 * time.Second

// syncPositionCmd schedules one position-sync tick.
func syncPositionCmd() tea.Cmd {
	return tea.Tick(positionSyncInterval, func(time.Time) tea.Msg {
		return syncPositionMsg{}
	})
}

// sessionPositionCmd returns a Cmd that writes the current track hash and
// elapsed position (in milliseconds) to the session table. Returns nil when
// no sessionWriter is configured.
func (m Model) sessionPositionCmd() tea.Cmd {
	if m.sessionWriter == nil {
		return nil
	}
	sw := m.sessionWriter
	hash := m.currentTrackHash
	positionMs := int64(m.elapsedSecs * 1000)
	return func() tea.Msg {
		_ = sw.UpdateSessionPosition(hash, positionMs)
		return nil
	}
}

// sessionTickCmd schedules one periodic session-position tick.
func sessionTickCmd() tea.Cmd {
	return tea.Tick(sessionTickInterval, func(time.Time) tea.Msg {
		return sessionTickMsg{}
	})
}

// tickCmd schedules a 1-second tick for elapsed-time display updates.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// countAudioFiles returns a Cmd that runs library.CountAudioFiles in a
// background goroutine and dispatches a ScanTotalMsg when done.
func countAudioFiles(root string) tea.Cmd {
	return func() tea.Msg {
		return library.ScanTotalMsg{Total: library.CountAudioFiles(root)}
	}
}

// waitForNextBatch returns a Cmd that reads up to scanBatchSize tracks from ch
// and dispatches a ScanBatchMsg. When the channel closes with no remaining
// tracks it dispatches ScanDoneMsg instead.
func waitForNextBatch(ch <-chan library.Track) tea.Cmd {
	return func() tea.Msg {
		var batch []library.Track
		for t := range ch {
			batch = append(batch, t)
			if len(batch) >= scanBatchSize {
				return library.ScanBatchMsg{Tracks: batch}
			}
		}
		// Channel closed.
		if len(batch) > 0 {
			return library.ScanBatchMsg{Tracks: batch}
		}
		return library.ScanDoneMsg{}
	}
}

// waitForWatchBatch returns a Cmd that reads up to scanBatchSize events from
// the watcher channel and dispatches a watchBatchMsg carrying both new/updated
// tracks and deleted paths. When the channel closes (context cancelled) it
// returns nil, stopping further scheduling.
func waitForWatchBatch(ch <-chan library.WatchEvent) tea.Cmd {
	return func() tea.Msg {
		var tracks []library.Track
		var deleted []string
		for e := range ch {
			if e.Deleted {
				deleted = append(deleted, e.Path)
			} else {
				tracks = append(tracks, e.Track)
			}
			if len(tracks)+len(deleted) >= scanBatchSize {
				return watchBatchMsg{tracks: tracks, deleted: deleted}
			}
		}
		if len(tracks)+len(deleted) > 0 {
			return watchBatchMsg{tracks: tracks, deleted: deleted}
		}
		return nil // channel closed, stop scheduling
	}
}

// Init satisfies the tea.Model interface. It schedules the config-notice
// dismissal timer when this is a first-run session, kicks off the first
// batch read from the scanner channel when one is present, and starts the
// watcher batch reader when a watch channel is provided.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.configCreated {
		cmds = append(cmds, tea.Tick(configNoticeDuration, func(time.Time) tea.Msg {
			return clearConfigNotice{}
		}))
	}
	if m.scanCh != nil {
		cmds = append(cmds, waitForNextBatch(m.scanCh))
		if m.libraryRoot != "" {
			cmds = append(cmds, countAudioFiles(m.libraryRoot))
		}
	}
	if m.watchCh != nil {
		cmds = append(cmds, waitForWatchBatch(m.watchCh))
	}
	if _, ok := m.player.(positionSyncer); ok {
		cmds = append(cmds, syncPositionCmd())
	}
	cmds = append(cmds, tickCmd())
	cmds = append(cmds, sessionTickCmd())
	cmds = append(cmds, tea.RequestBackgroundColor)
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}

// activeViewContext maps the current activeView to its bindingContext so that
// the registry can check view-local bindings before falling back to global.
func (m Model) activeViewContext() bindingContext {
	ctx := viewToContext(m.activeView)
	if ctx == contextGlobal {
		// A new viewID was added without a corresponding case in viewToContext.
		// Add the case there to ensure view-local keybinding precedence works
		// correctly for the new view. Falling back to contextGlobal would
		// silently bypass view-local shadowing (e.g. playlist 'r' rename vs.
		// global repeat).
		slog.Warn("activeViewContext: unhandled viewID, falling back to global bindings",
			"viewID", int(m.activeView))
	}
	return ctx
}

// Update handles incoming messages. Number keys 1/2/3/4 switch the active view;
// Ctrl+c quits. q and Esc are intentionally ignored here — they are reserved
// for in-view navigation.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		m.library.width = msg.Width
		m.library.height = max(msg.Height-3, 0)
		m.playlists.width = msg.Width
		m.playlists.height = max(msg.Height-3, 0)
		m.queue.width = msg.Width
		m.queue.height = max(msg.Height-3, 0)
		m.artists.width = msg.Width
		m.artists.height = max(msg.Height-3, 0)
		m.albums.width = msg.Width
		m.albums.height = max(msg.Height-3, 0)
		if m.helpVisible {
			m.helpViewport.SetWidth(msg.Width)
			m.helpViewport.SetHeight(max(msg.Height-2, 0))
		}

	case clearConfigNotice:
		m.configCreated = false

	case tea.BackgroundColorMsg:
		s := styles.NewStyles(msg.IsDark())
		m.styles = &s

	// --- Library / scan / watcher messages (root_library.go) ---

	case library.ScanProgressMsg:
		m = m.handleScanProgressMsg(msg)

	case library.ScanTotalMsg:
		m = m.handleScanTotalMsg(msg)

	case library.ScanBatchMsg:
		return m.handleScanBatchMsg(msg)

	case watchBatchMsg:
		return m.handleWatchBatchMsg(msg)

	case library.ScanDoneMsg:
		m = m.handleScanDoneMsg()

	// --- Playback messages (root_playback.go) ---

	case tickMsg:
		return m.handleTickMsg()

	case syncPositionMsg:
		return m.handleSyncPositionMsg()

	case sessionTickMsg:
		return m.handleSessionTickMsg()

	case PlayerStatusMsg:
		m = m.handlePlayerStatusMsg(msg)

	case MissingTrackMsg:
		m = m.handleMissingTrackMsg(msg)

	case PlaybackEndedMsg:
		m = m.handlePlaybackEndedMsg()
		if m.playbackState == PlaybackPlaying {
			return m, m.sessionPositionCmd()
		}

	case RemoteActionMsg:
		m, cmd, _ := m.handlePlaybackKeyAction(remoteAction(msg.ActionName))
		return m, cmd

	case previewStartedMsg:
		m = m.handlePreviewStartedMsg(msg)

	case previewStartErrorMsg:
		m = m.handlePreviewStartErrorMsg(msg)

	case trackInfoMsg:
		if msg.err != nil {
			m.statusBar.notification = transientNotification{
				msg:       "Track info error: " + msg.err.Error(),
				expiresAt: time.Now().Add(transientDuration),
			}
		} else {
			overlay := newTrackInfoOverlay(msg.path, msg.tags)
			m.trackInfo = &overlay
		}

	case TrackChangedMsg:
		return m.handleTrackChangedMsg(msg)

	// --- Filter messages (root_filter.go) ---

	case FilterBarMsg:
		m = m.handleFilterBarMsg()

	// --- Queue messages (root_queue.go) ---

	case libtui.TrackEnqueuedMsg:
		m = m.handleTrackEnqueuedMsg(msg)

	case BatchEnqueuedMsg:
		m = m.handleBatchEnqueuedMsg(msg)

	case PlayQueueIndexMsg:
		m = m.handlePlayQueueIndexMsg(msg)

	case MissingTrackSelectedMsg:
		m.statusBar.notification = transientNotification{
			msg:       "Cannot play — file is missing",
			expiresAt: time.Now().Add(transientDuration),
		}

	case DequeueTrackMsg:
		m = m.handleDequeueTrackMsg(msg)

	case BulkSelectionToggleMsg:
		m = m.handleBulkSelectionToggleMsg(msg)

	case ClearQueueMsg:
		m = m.handleClearQueueMsg()

	case UndoMsg:
		m = m.handleUndo()

	case MoveTrackUpMsg:
		return m.handleMoveTrackUpMsg(msg)

	case MoveTrackDownMsg:
		return m.handleMoveTrackDownMsg(msg)

	// --- Key handling ---

	case tea.KeyPressMsg:
		if m.helpVisible {
			switch msg.String() {
			case "ctrl+c":
				// Fall through to global key handling for quit.
			case "esc":
				m.helpVisible = false
				m.helpViewport = viewport.Model{}
				m.helpFiltered = false
				m.activeView = m.prevActiveView
				return m, nil
			case "?":
				m.helpFiltered = !m.helpFiltered
				sections, title := m.helpState()
				m.helpViewport.SetContent(HelpOverlay{}.render(*m.styles, sections, title))
				m.helpViewport.SetYOffset(0)
				return m, nil
			default:
				var cmd tea.Cmd
				m.helpViewport, cmd = m.helpViewport.Update(msg)
				return m, cmd
			}
		}
		// While the now-playing info overlay is active, swallow all keys
		// except ctrl+c (quit) and esc/I (dismiss).
		if m.nowPlayingInfo != nil {
			switch msg.String() {
			case "ctrl+c":
				m.nowPlayingInfo = nil
				return m, tea.Quit
			case "esc", "I":
				m.nowPlayingInfo = nil
				return m, nil
			default:
				return m, nil
			}
		}
		// While the track info overlay is active, swallow all keys
		// except ctrl+c (quit) and esc/i (dismiss).
		if m.trackInfo != nil {
			switch msg.String() {
			case "ctrl+c":
				m.trackInfo = nil
				return m, tea.Quit
			case "esc", "i":
				m.trackInfo = nil
				return m, nil
			case "y":
				m.statusBar.notification = transientNotification{
					msg:       "Path copied to clipboard",
					expiresAt: time.Now().Add(transientDuration),
				}
				return m, tea.SetClipboard(m.trackInfo.path)
			default:
				return m, nil
			}
		}
		// While the preview overlay is active, swallow all keys except
		// ctrl+c (quit) and esc/P (dismiss preview).
		if m.preview != nil {
			switch msg.String() {
			case "ctrl+c":
				m = m.dismissPreview()
				return m, tea.Quit
			case "esc", "P":
				m = m.dismissPreview()
				return m, nil
			default:
				return m, nil
			}
		}
		// While the metadata editor overlay is active, route all keys to it.
		if m.metadataEditor != nil {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			updated, act, cmd := m.metadataEditor.HandleKey(msg)
			m.metadataEditor = &updated
			switch act {
			case editorCancel:
				m.metadataEditor = nil
				return m, nil
			case editorSave:
				return m.handleMetadataSave()
			}
			return m, cmd
		}
		// Filter bar interception (root_filter.go).
		if model, cmd, ok := m.handleFilterKeyPress(msg); ok {
			return model, cmd
		}
		// Playlist inline input interception (root_playlists.go).
		if model, cmd, ok := m.handlePlaylistCreateInput(msg); ok {
			return model, cmd
		}
		if model, cmd, ok := m.handlePlaylistRenameInput(msg); ok {
			return model, cmd
		}
		if model, cmd, ok := m.handleQueueSaveInput(msg); ok {
			return model, cmd
		}
		// While the destination picker modal is open, route all keypresses to
		// it instead of the normal registry. The modal dispatches its own Cmds
		// (DestinationPickedMsg, DestinationPickerDismissedMsg, etc.) which are
		// handled below in the non-KeyPressMsg cases.
		if m.destPicker != nil {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			updated, cmd := m.destPicker.HandleKey(msg.String())
			m.destPicker = &updated
			return m, cmd
		}
		return m.handleKeyAction(m.registry.resolve(m.activeViewContext(), msg.String()))

	// --- Playlist messages (root_playlists.go) ---

	case playlistTracksLoadedMsg:
		m = m.handlePlaylistTracksLoadedMsg(msg)

	case playlistTracksLoadErrorMsg:
		m = m.handlePlaylistTracksLoadErrorMsg(msg)

	case PlaylistRenameMsg:
		m = m.handlePlaylistRenameMsg()

	case playlistRenamedMsg:
		m = m.handlePlaylistRenamedMsg(msg)

	case playlistRenameErrorMsg:
		m = m.handlePlaylistRenameErrorMsg(msg)

	case PlaylistCreateMsg:
		m = m.handlePlaylistCreateMsg()

	case playlistCreatedMsg:
		return m.handlePlaylistCreatedMsg(msg)

	case playlistCreateErrorMsg:
		m = m.handlePlaylistCreateErrorMsg(msg)

	case queueSavedAsPlaylistMsg:
		m = m.handleQueueSavedAsPlaylistMsg(msg)

	case queueSaveAsPlaylistErrorMsg:
		m = m.handleQueueSaveAsPlaylistErrorMsg(msg)

	case playlistAddErrorMsg:
		m = m.handlePlaylistAddErrorMsg(msg)

	case tracksAddedToPlaylistMsg:
		m = m.handleTracksAddedToPlaylistMsg(msg)

	case metadataEditedMsg:
		// No-op: in-memory state was already updated synchronously in handleMetadataSave.

	case pltui.DestinationPickerDismissedMsg:
		m = m.handleDestinationPickerDismissedMsg()

	case pltui.DestinationPickerCreateMsg:
		m = m.handleDestinationPickerCreateMsg()

	case pltui.DestinationPickedMsg:
		return m.handleDestinationPickedMsg(msg)
	}
	return m, nil
}

// helpState returns the sections and title to display in the help overlay,
// depending on whether the user has toggled view-specific filtering.
func (m Model) helpState() ([]helpSection, string) {
	if !m.helpFiltered {
		return helpSections(), "Keybindings  (? to filter by view)"
	}
	ctx := viewToContext(m.prevActiveView)
	name := viewName(m.prevActiveView)
	return filteredHelpSections(ctx), "Keybindings — " + name + "  (? to show all)"
}

// View renders the active panel and a status bar, returning a tea.View.
// When the help overlay is visible it replaces the body content entirely.
func (m Model) View() tea.View {
	var body string
	if m.helpVisible {
		body = m.helpViewport.View()
	} else if m.nowPlayingInfo != nil {
		body = m.nowPlayingInfo.Render(m.termWidth, m.termHeight, *m.styles)
	} else if m.trackInfo != nil {
		body = m.trackInfo.Render(m.termWidth, m.termHeight, *m.styles)
	} else if m.preview != nil {
		body = m.preview.Render(m.termWidth, m.termHeight, *m.styles)
	} else if m.metadataEditor != nil {
		body = m.metadataEditor.Render(m.termWidth, m.termHeight, *m.styles)
	} else if m.destPicker != nil {
		body = m.destPicker.Render(m.termWidth, m.termHeight, *m.styles)
	} else {
		switch m.activeView {
		case viewLibrary:
			body = m.library.render(*m.styles)
		case viewPlaylists:
			body = m.playlists.render(*m.styles)
		case viewQueue:
			body = m.queue.render(*m.styles, m.currentTrackHash)
		case viewArtists:
			body = m.artists.render(*m.styles)
		case viewAlbums:
			body = m.albums.render(*m.styles)
		}
	}

	// Prepend the centralized title bar when showing a primary view.
	if m.nowPlayingInfo == nil && m.trackInfo == nil && m.preview == nil && m.metadataEditor == nil && m.destPicker == nil && !m.helpVisible {
		body = m.renderTitleBar() + "\n\n" + body
	}

	statusBar := m.renderStatusBar()
	bodyLines := strings.Count(body, "\n") + 1
	if padding := m.termHeight - bodyLines - 1; padding > 0 {
		body += strings.Repeat("\n", padding)
	}
	content := body + "\n" + statusBar
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderTitleBar builds the centralized title bar showing view names with
// chevrons and numbered badges. When the terminal is wide enough all views
// are shown; otherwise only the previous, active, and next views are displayed.
func (m Model) renderTitleBar() string {
	// Compute the width needed to show all tabs:
	// "‹ " + "1 Name1  2 Name2  ...  N NameN" + " ›"
	// Each tab: badge digit (1) + space (1) + name length.
	const chevronWidth = 4 // "‹ " + " ›"
	const badgeWidth = 2   // digit + space before name
	sep := "  "
	fullWidth := chevronWidth
	for i, id := range viewOrder {
		fullWidth += badgeWidth + len(viewName(id))
		if i < len(viewOrder)-1 {
			fullWidth += len(sep)
		}
	}

	if m.termWidth >= fullWidth {
		// Wide layout: show all tabs with numbered badges.
		var parts []string
		for i, id := range viewOrder {
			badge := m.styles.Badge.Render(fmt.Sprintf("%d", i+1))
			name := viewName(id)
			if id == m.activeView {
				parts = append(parts, badge+" "+m.styles.Heading.Render(name))
			} else {
				parts = append(parts, badge+" "+m.styles.Dim.Render(name))
			}
		}
		return m.styles.Dim.Render("‹ ") + strings.Join(parts, sep) + m.styles.Dim.Render(" ›")
	}

	// Narrow layout: previous, active, next (no badges).
	prev := prevView(m.activeView)
	next := nextView(m.activeView)
	return m.styles.Dim.Render("‹ "+viewName(prev)) +
		"  " + m.styles.Heading.Render(viewName(m.activeView)) + "  " +
		m.styles.Dim.Render(viewName(next)+" ›")
}

// renderStatusBar builds the status bar line shown at the bottom of the view.
// It syncs the model-owned playbackState, repeat, shuffle, and configCreated
// into a local StatusBar copy so that the render logic has a complete picture.
func (m Model) renderStatusBar() string {
	sb := m.statusBar
	sb.playbackState = m.playbackState
	sb.repeatMode = m.repeatMode
	sb.shuffle = m.shuffle
	sb.configCreated = m.configCreated
	sb.queueEmpty = len(m.queue.entries) == 0
	if m.elapsedSecs > 0 || m.totalSecs > 0 {
		sb.elapsed = formatTime(m.elapsedSecs)
		sb.total = formatTime(m.totalSecs)
	}
	return sb.render(m.termWidth, *m.styles)
}

// clearPendingConfirmations resets all two-press confirmation states.
// Call this on any view switch or action that should cancel pending confirmations.
func (m *Model) clearPendingConfirmations() {
	m.clearQueuePending = false
	m.bulkRemovePending = false
}

// switchToView transitions to the given view, clearing pending confirmations,
// resetting all sort states, and lazily deriving artist/album lists when needed.
func (m Model) switchToView(id viewID) Model {
	m.clearPendingConfirmations()
	m = m.resetAllSorts()
	m.activeView = id
	if id == viewArtists && len(m.artists.artists) == 0 {
		m.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
	}
	if id == viewAlbums && len(m.albums.albums) == 0 {
		m.albums.albums = albtui.DeriveAllAlbums(m.library.index.Tracks())
	}
	return m
}

// handleKeyAction dispatches a resolved KeyAction to the appropriate handler.
// It is called from the tea.KeyPressMsg case in Update after the registry
// resolves the key string to an action.
func (m Model) handleKeyAction(action action) (tea.Model, tea.Cmd) {
	// Sort actions (root_sort.go).
	if model, cmd, ok := m.handleSortAction(action); ok {
		return model, cmd
	}
	// Metadata editor action (root_metadata.go).
	if model, cmd, ok := m.handleMetadataKeyAction(action); ok {
		return model, cmd
	}
	// Playback actions (root_playback.go).
	if model, cmd, ok := m.handlePlaybackKeyAction(action); ok {
		return model, cmd
	}
	// Library / artist / album actions (root_library.go).
	if model, cmd, ok := m.handleLibraryKeyAction(action); ok {
		return model, cmd
	}
	// Queue actions (root_queue.go).
	if model, cmd, ok := m.handleQueueKeyAction(action); ok {
		return model, cmd
	}

	// Navigation, view-switching, cursor, playlist, and remaining actions.
	switch action {
	case actionHelp:
		m.prevActiveView = m.activeView
		m.helpVisible = true
		m.helpFiltered = false
		m.helpViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(m.termHeight-2))
		sections, title := m.helpState()
		m.helpViewport.SetContent(HelpOverlay{}.render(*m.styles, sections, title))
	case actionNextView:
		m = m.switchToView(nextView(m.activeView))
	case actionPrevView:
		m = m.switchToView(prevView(m.activeView))
	case actionQuit:
		m.nowPlayingInfo = nil
		m.trackInfo = nil
		m = m.dismissPreview()
		return m, tea.Quit
	case actionFilterBar:
		return m, func() tea.Msg { return FilterBarMsg{} }
	case actionSwitchLibrary:
		m = m.switchToView(viewLibrary)
	case actionSwitchPlaylists:
		m = m.switchToView(viewPlaylists)
	case actionSwitchQueue:
		m = m.switchToView(viewQueue)
	case actionSwitchArtists:
		m = m.switchToView(viewArtists)
	case actionSwitchAlbums:
		m = m.switchToView(viewAlbums)
	case actionCursorUp, actionCursorDown,
		actionHalfPageDown, actionHalfPageUp,
		actionFullPageDown, actionFullPageUp,
		actionGoToTop, actionGoToBottom:
		switch m.activeView {
		case viewLibrary:
			m.library = m.library.HandleAction(action)
		case viewPlaylists:
			m.playlists = m.playlists.HandleAction(action)
		case viewArtists:
			m.artists = m.artists.HandleAction(action)
		case viewAlbums:
			m.albums = m.albums.HandleAction(action)
		default:
			var cmd tea.Cmd
			m.queue, cmd = m.queue.HandleAction(action)
			return m, cmd
		}
	case actionBulkSelectionToggle:
		if m.activeView == viewLibrary {
			m.library = m.library.HandleAction(action)
		} else if m.activeView == viewPlaylists {
			m.playlists = m.playlists.HandleAction(action)
		} else {
			var cmd tea.Cmd
			m.queue, cmd = m.queue.HandleAction(action)
			return m, cmd
		}
	case actionClearSelection:
		switch m.activeView {
		case viewLibrary:
			m.library = m.library.ClearSelection()
		case viewQueue:
			for i := range m.queue.entries {
				m.queue.entries[i].Selected = false
			}
		case viewPlaylists:
			m.playlists.selectedTrackIDs = nil
		}
	case actionPlaylistRename:
		return m, func() tea.Msg { return PlaylistRenameMsg{} }
	case actionPlaylistCreate:
		return m, func() tea.Msg { return PlaylistCreateMsg{} }
	case actionPlaylistOpen:
		if m.playlists.atListLevel() && m.playlistReader != nil {
			pid, ok := m.playlists.focusedPlaylistID()
			if ok {
				reader := m.playlistReader
				return m, func() tea.Msg {
					tracks, err := reader.ReadPlaylistTracks(pid)
					if err != nil {
						return playlistTracksLoadErrorMsg{err: err}
					}
					return playlistTracksLoadedMsg{tracks: tracks}
				}
			}
		} else if !m.playlists.atListLevel() && m.queueWriter != nil {
			if cmd := m.playlists.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd
			}
		}
	case actionPlaylistBack:
		if !m.playlists.atListLevel() {
			m.playlists = m.playlists.backToList()
		}
	case actionPlaylistEnqueueAll:
		if m.batchEnqueuer == nil || m.playlistReader == nil {
			break
		}
		var pid int64
		var playlistName string
		if m.playlists.atListLevel() {
			pl, ok := m.playlists.focusedPlaylist()
			if !ok {
				break
			}
			pid = pl.ID
			playlistName = pl.Name
		} else {
			if m.playlists.listCursor < 0 || m.playlists.listCursor >= len(m.playlists.playlists) {
				break
			}
			pid = m.playlists.playlists[m.playlists.listCursor].ID
			playlistName = m.playlists.playlists[m.playlists.listCursor].Name
		}
		reader := m.playlistReader
		be := m.batchEnqueuer
		wasEmpty := len(m.queue.entries) == 0
		return m, func() tea.Msg {
			rows, err := reader.ReadPlaylistTracks(pid)
			if err != nil {
				return BatchEnqueuedMsg{Err: err}
			}
			if len(rows) == 0 {
				return BatchEnqueuedMsg{PlaylistName: playlistName}
			}
			tracks := make([]library.Track, len(rows))
			for i, r := range rows {
				tracks[i] = library.Track{
					Hash:     r.Hash,
					Path:     r.Path,
					Artist:   r.Artist,
					Title:    r.Title,
					Album:    r.Album,
					Duration: float64(r.DurationMs) / 1000,
				}
			}
			ids, err := be.AppendTracks(tracks, playlistName)
			if err != nil {
				return BatchEnqueuedMsg{Err: err}
			}
			return BatchEnqueuedMsg{
				Entries:      newQueueEntries(tracks, ids, playlistName),
				QueueIDs:     ids,
				WasEmpty:     wasEmpty,
				PlaylistName: playlistName,
			}
		}
	case actionPlaylistDelete:
		return m.handlePlaylistDelete()
	}
	return m, nil
}
