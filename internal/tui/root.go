package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	undoNone                    undoKind = iota
	undoKindEnqueue                      // single track added via TrackEnqueuedMsg
	undoKindDequeue                      // single track removed via DequeueTrackMsg
	undoKindClear                        // all tracks removed via ClearQueueMsg
	undoKindDeletePlaylist               // playlist soft-deleted via actionPlaylistDelete at ListLevel
	undoKindRemovePlaylistTrack          // track soft-removed via actionPlaylistDelete at TrackLevel
	undoKindBatchEnqueue                 // batch of tracks added via actionPlaylistEnqueueAll
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

// PlaybackEndedMsg is dispatched by the player when auto-advance exhausts the
// queue (no more playable entries). Root handles it by resetting the playback
// state to idle so the space-bar toggle stays synchronized with mpv's state.
type PlaybackEndedMsg struct{}

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

// playlistTracksLoadedMsg carries the result of a background ReadPlaylistTracks
// call initiated by actionPlaylistOpen. Root handles it by transitioning the
// playlists view to TrackLevel with the loaded rows.
type playlistTracksLoadedMsg struct {
	tracks []db.PlaylistTrackRow
}

// PlaylistRenameMsg is dispatched when the user presses 'r' in the playlists
// view to rename the selected playlist.
type PlaylistRenameMsg struct{}

// playlistRenamedMsg is dispatched after a successful RenamePlaylist call.
// It carries the ID of the renamed playlist so root can reposition the cursor,
// and the refreshed playlist list so root can update the in-memory state.
type playlistRenamedMsg struct {
	id        int64
	playlists []db.PlaylistRow
}

// playlistRenameErrorMsg is dispatched when RenamePlaylist returns an error.
// Root uses it to cancel the inline rename input and notify the user.
type playlistRenameErrorMsg struct {
	err error
}

// PlaylistCreateMsg is dispatched when the user presses 'c' in the playlists
// view to create a new playlist.
type PlaylistCreateMsg struct{}

// playlistCreatedMsg is dispatched after a successful CreatePlaylist call.
// It carries the refreshed playlist list and the ID of the newly created
// entry so that root can position the cursor on it by ID.
type playlistCreatedMsg struct {
	id        int64
	playlists []db.PlaylistRow
}

// playlistCreateErrorMsg is dispatched when CreatePlaylist returns an error.
// Root uses it to cancel the inline create input and notify the user.
type playlistCreateErrorMsg struct {
	err error
}

// playlistAddErrorMsg is dispatched when AddTracksToPlaylist returns an error.
// Root uses it to display a transient error notification in the status bar.
type playlistAddErrorMsg struct {
	err error
}

// tracksAddedToPlaylistMsg is dispatched after a successful AddTracksToPlaylist
// call. It carries the refreshed playlist list (for updating track counts) and
// metadata for the status-bar notification.
type tracksAddedToPlaylistMsg struct {
	playlists    []db.PlaylistRow
	playlistName string
	count        int
}

// queueSavedAsPlaylistMsg is dispatched after successfully saving the queue as
// a new playlist. It carries the refreshed playlist list and metadata for the
// status-bar notification.
type queueSavedAsPlaylistMsg struct {
	playlists     []db.PlaylistRow
	playlistName  string
	count         int
	fromSelection bool // true when only bulk-selected entries were saved
}

// queueSaveAsPlaylistErrorMsg is dispatched when saving the queue as a
// playlist fails. Root cancels the inline input and shows an error notification.
type queueSaveAsPlaylistErrorMsg struct {
	err error
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
	helpVisible    bool   // true while the help overlay is shown
	helpScroll     int    // scroll offset (in lines) for the help overlay
	prevActiveView viewID // the view that was active before help was opened

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
	switch m.activeView {
	case viewLibrary:
		return contextLibrary
	case viewPlaylists:
		return contextPlaylists
	case viewQueue:
		return contextQueue
	case viewArtists:
		return contextArtists
	case viewAlbums:
		return contextAlbums
	default:
		// A new viewID was added without a corresponding case here. Add the
		// case above to ensure view-local keybinding precedence works correctly
		// for the new view. Falling back to contextGlobal would silently bypass
		// view-local shadowing (e.g. playlist 'r' rename vs. global repeat).
		slog.Warn("activeViewContext: unhandled viewID, falling back to global bindings",
			"viewID", int(m.activeView))
		return contextGlobal
	}
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
		m.library.height = msg.Height - 3
		m.playlists.width = msg.Width
		m.playlists.height = msg.Height - 3
		m.queue.width = msg.Width
		m.queue.height = msg.Height - 3
		m.artists.width = msg.Width
		m.artists.height = msg.Height - 3
		m.albums.width = msg.Width
		m.albums.height = msg.Height - 3

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

	case previewStartedMsg:
		m = m.handlePreviewStartedMsg(msg)

	case previewStartErrorMsg:
		m = m.handlePreviewStartErrorMsg(msg)

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
				m.helpScroll = 0
				m.activeView = m.prevActiveView
				return m, nil
			case "j", "down":
				m.helpScroll++
				m.helpScroll = m.clampHelpScroll()
				return m, nil
			case "k", "up":
				m.helpScroll--
				m.helpScroll = m.clampHelpScroll()
				return m, nil
			case "ctrl+d":
				m.helpScroll += m.termHeight / 2
				m.helpScroll = m.clampHelpScroll()
				return m, nil
			case "ctrl+u":
				m.helpScroll -= m.termHeight / 2
				m.helpScroll = m.clampHelpScroll()
				return m, nil
			default:
				return m, nil
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
		// Filter bar interception (root_filter.go).
		if model, cmd, ok := m.handleFilterKeyPress(msg); ok {
			return model, cmd
		}
		// When the inline playlist create input is active, intercept all
		// keypresses before the registry.
		if m.playlists.isCreating() && m.activeView == viewPlaylists {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.playlists = m.playlists.cancelCreate()
			case "enter":
				if !m.playlists.createInputIsEmpty() {
					name := m.playlists.createInputValue()
					// Reject duplicate names (case-insensitive, matching DB COLLATE NOCASE ordering).
					for _, p := range m.playlists.playlists {
						if strings.EqualFold(p.Name, name) {
							m.statusBar.notification = transientNotification{
								msg:       "A playlist with that name already exists",
								expiresAt: time.Now().Add(transientDuration),
							}
							return m, nil
						}
					}
					if m.playlistWriter != nil {
						pw := m.playlistWriter
						return m, func() tea.Msg {
							id, err := pw.CreatePlaylist(name)
							if err != nil {
								return playlistCreateErrorMsg{err: err}
							}
							rows, err := pw.ReadPlaylists()
							if err != nil {
								return playlistCreateErrorMsg{err: err}
							}
							return playlistCreatedMsg{id: id, playlists: rows}
						}
					}
					// No writer: dismiss the input without persisting.
					m.playlists = m.playlists.cancelCreate()
				}
				// Empty/whitespace input: keep the field active (no-op).
			case "backspace":
				m.playlists = m.playlists.backspaceCreate()
			default:
				if msg.Text != "" {
					for _, ch := range msg.Text {
						m.playlists = m.playlists.appendCreateChar(ch)
					}
				}
			}
			return m, nil
		}
		// When the inline playlist rename input is active, intercept all
		// keypresses before the registry.
		if m.playlists.isRenaming() && m.activeView == viewPlaylists {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.playlists = m.playlists.cancelRename()
			case "enter":
				if !m.playlists.renameInputIsEmpty() {
					name := m.playlists.renameInputValue()
					pl, ok := m.playlists.focusedPlaylist()
					if !ok {
						m.playlists = m.playlists.cancelRename()
						return m, nil
					}
					// Reject duplicate names (case-insensitive, matching DB COLLATE NOCASE ordering).
					// Skip the playlist being renamed so a no-op Enter is treated as cancel.
					for _, p := range m.playlists.playlists {
						if p.ID != pl.ID && strings.EqualFold(p.Name, name) {
							m.statusBar.notification = transientNotification{
								msg:       "A playlist with that name already exists",
								expiresAt: time.Now().Add(transientDuration),
							}
							return m, nil
						}
					}
					if m.playlistRenamer != nil {
						pr := m.playlistRenamer
						id := pl.ID
						return m, func() tea.Msg {
							if err := pr.RenamePlaylist(id, name); err != nil {
								return playlistRenameErrorMsg{err: err}
							}
							rows, err := pr.ReadPlaylists()
							if err != nil {
								return playlistRenameErrorMsg{err: err}
							}
							return playlistRenamedMsg{id: id, playlists: rows}
						}
					}
					// No renamer: dismiss the input without persisting.
					m.playlists = m.playlists.cancelRename()
				}
				// Empty/whitespace input: keep the field active (no-op).
			case "backspace":
				m.playlists = m.playlists.backspaceRename()
			default:
				if msg.Text != "" {
					for _, ch := range msg.Text {
						m.playlists = m.playlists.appendRenameChar(ch)
					}
				}
			}
			return m, nil
		}
		// When the inline save-as-playlist input is active, intercept all
		// keypresses before the registry.
		if m.queue.isSavingAsPlaylist() && m.activeView == viewQueue {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.queue = m.queue.cancelSaveAsPlaylist()
			case "enter":
				if !m.queue.saveInputIsEmpty() {
					name := m.queue.saveInputValue()
					// Reject duplicate names (case-insensitive).
					for _, p := range m.playlists.playlists {
						if strings.EqualFold(p.Name, name) {
							m.statusBar.notification = transientNotification{
								msg:       "A playlist with that name already exists",
								expiresAt: time.Now().Add(transientDuration),
							}
							return m, nil
						}
					}
					if m.playlistWriter != nil {
						pw := m.playlistWriter
						pa := m.playlistAdder
						// Capture hashes at dispatch time: selected entries only, or all.
						var hashes []string
						if m.queue.savingFromSelection {
							hashes = m.queue.selectedHashes()
						} else {
							hashes = make([]string, len(m.queue.entries))
							for i, e := range m.queue.entries {
								hashes[i] = e.Hash
							}
						}
						count := len(hashes)
						fromSelection := m.queue.savingFromSelection
						return m, func() tea.Msg {
							id, err := pw.CreatePlaylist(name)
							if err != nil {
								return queueSaveAsPlaylistErrorMsg{err: err}
							}
							if pa != nil && len(hashes) > 0 {
								if err := pa.AddTracksToPlaylist(id, hashes); err != nil {
									return queueSaveAsPlaylistErrorMsg{err: err}
								}
							}
							rows, err := pw.ReadPlaylists()
							if err != nil {
								return queueSaveAsPlaylistErrorMsg{err: err}
							}
							return queueSavedAsPlaylistMsg{playlists: rows, playlistName: name, count: count, fromSelection: fromSelection}
						}
					}
					// No writer: dismiss the input without persisting.
					m.queue = m.queue.cancelSaveAsPlaylist()
				}
				// Empty/whitespace input: keep the field active (no-op).
			case "backspace":
				m.queue = m.queue.backspaceSave()
			default:
				if msg.Text != "" {
					for _, ch := range msg.Text {
						m.queue = m.queue.appendSaveChar(ch)
					}
				}
			}
			return m, nil
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

	// --- Playlist messages (remain in root.go) ---

	case playlistTracksLoadedMsg:
		m.playlists = m.playlists.openTracks(msg.tracks)

	case PlaylistRenameMsg:
		if m.playlists.atListLevel() {
			if pl, ok := m.playlists.focusedPlaylist(); ok {
				m.playlists = m.playlists.startRename(pl.Name)
			}
		}

	case playlistRenamedMsg:
		m.playlists.playlists = msg.playlists
		m.playlists = m.playlists.cancelRename()
		for i, p := range msg.playlists {
			if p.ID == msg.id {
				m.playlists.cursor = i
				break
			}
		}
		m.playlists = m.playlists.ensureCursorVisible()

	case playlistRenameErrorMsg:
		m.playlists = m.playlists.cancelRename()
		m.statusBar.notification = transientNotification{
			msg:       "Failed to rename playlist: " + msg.err.Error(),
			expiresAt: time.Now().Add(transientDuration),
		}

	case PlaylistCreateMsg:
		if m.playlists.atListLevel() {
			m.playlists = m.playlists.startCreate()
		}

	case playlistCreatedMsg:
		m.playlists.playlists = msg.playlists
		m.playlists = m.playlists.cancelCreate()
		for i, p := range msg.playlists {
			if p.ID == msg.id {
				m.playlists.cursor = i
				break
			}
		}
		m.playlists = m.playlists.ensureCursorVisible()

	case playlistCreateErrorMsg:
		m.playlists = m.playlists.cancelCreate()
		m.statusBar.notification = transientNotification{
			msg:       "Failed to create playlist: " + msg.err.Error(),
			expiresAt: time.Now().Add(transientDuration),
		}

	case queueSavedAsPlaylistMsg:
		m.queue = m.queue.cancelSaveAsPlaylist()
		m.playlists.playlists = msg.playlists
		source := "queue"
		if msg.fromSelection {
			source = "selection"
		}
		m.statusBar.notification = transientNotification{
			msg:       fmt.Sprintf("Saved %s as %q (%d %s)", source, msg.playlistName, msg.count, pluralize(msg.count, "track", "tracks")),
			expiresAt: time.Now().Add(transientDuration),
		}

	case queueSaveAsPlaylistErrorMsg:
		m.queue = m.queue.cancelSaveAsPlaylist()
		m.statusBar.notification = transientNotification{
			msg:       "Failed to save queue as playlist: " + msg.err.Error(),
			expiresAt: time.Now().Add(transientDuration),
		}

	case playlistAddErrorMsg:
		m.statusBar.notification = transientNotification{
			msg:       "Failed to add tracks: " + msg.err.Error(),
			expiresAt: time.Now().Add(transientDuration),
		}

	case tracksAddedToPlaylistMsg:
		if msg.playlists != nil {
			m.playlists.playlists = msg.playlists
		}
		m.statusBar.notification = transientNotification{
			msg:       fmt.Sprintf("%d %s added to %s", msg.count, pluralize(msg.count, "track", "tracks"), msg.playlistName),
			expiresAt: time.Now().Add(transientDuration),
		}

	case pltui.DestinationPickerDismissedMsg:
		m.destPicker = nil
		m.destPickerTracks = nil

	case pltui.DestinationPickerCreateMsg:
		// User selected "New playlist..." — dismiss the picker, switch to the
		// playlists view, and activate the inline create input.
		//
		// Two-step flow (intentional): destPickerTracks is discarded here rather
		// than forwarded to playlistCreatedMsg. After the playlist is created the
		// user must re-open the destination picker ('a') to add the tracks. This
		// matches the requirement text (ac-876649c360cab6be4ab38dee626495c7) which
		// only specifies "an inline prompt offers to create a new playlist" with no
		// obligation to auto-add the staged tracks. Forwarding them would require
		// complex state threading across the create flow with no requirement backing.
		m.destPicker = nil
		m.destPickerTracks = nil
		m.library = m.library.ClearSelection()
		m.activeView = viewPlaylists
		m.playlists = m.playlists.startCreate()

	case pltui.DestinationPickedMsg:
		tracks := m.destPickerTracks
		m.destPicker = nil
		m.destPickerTracks = nil
		m.library = m.library.ClearSelection()
		if msg.IsQueue {
			// Append each track to the play queue.
			if m.queueWriter != nil {
				queueLen := len(m.queue.entries)
				var cmds []tea.Cmd
				for i, t := range tracks {
					track := t
					wasEmpty := queueLen+i == 0
					qa := m.queueWriter
					cmds = append(cmds, func() tea.Msg {
						id, err := qa.AppendTrack(track, "")
						return libtui.TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty}
					})
				}
				if len(cmds) > 0 {
					return m, tea.Batch(cmds...)
				}
			}
		} else {
			// Append tracks to the named playlist.
			for _, p := range m.playlists.playlists {
				if p.Name == msg.PlaylistName {
					if m.playlistAdder == nil {
						break
					}
					pid := p.ID
					pa := m.playlistAdder
					pr := m.playlistRenamer
					pw := m.playlistWriter
					var hashes []string
					for _, t := range tracks {
						hashes = append(hashes, t.Hash)
					}
					playlistName := p.Name
					count := len(hashes)
					return m, func() tea.Msg {
						if err := pa.AddTracksToPlaylist(pid, hashes); err != nil {
							return playlistAddErrorMsg{err: err}
						}
						var rows []db.PlaylistRow
						if pr != nil {
							rows, _ = pr.ReadPlaylists()
						} else if pw != nil {
							rows, _ = pw.ReadPlaylists()
						}
						return tracksAddedToPlaylistMsg{playlists: rows, playlistName: playlistName, count: count}
					}
				}
			}
		}
	}
	return m, nil
}

// clampHelpScroll constrains helpScroll to [0, maxScroll] based on the
// terminal height and the total number of help lines.
func (m Model) clampHelpScroll() int {
	total := helpLineCount()
	visible := m.termHeight - 2 // reserve space for the status bar gap
	if visible <= 0 || visible >= total {
		return 0
	}
	maxScroll := total - visible
	s := m.helpScroll
	if s > maxScroll {
		s = maxScroll
	}
	if s < 0 {
		s = 0
	}
	return s
}

// View renders the active panel and a status bar, returning a tea.View.
// When the help overlay is visible it replaces the body content entirely.
func (m Model) View() tea.View {
	var body string
	if m.helpVisible {
		full := HelpOverlay{}.render(*m.styles)
		lines := strings.Split(full, "\n")
		visible := m.termHeight - 2
		if visible <= 0 || visible >= len(lines) {
			body = full
		} else {
			end := m.helpScroll + visible
			if end > len(lines) {
				end = len(lines)
			}
			body = strings.Join(lines[m.helpScroll:end], "\n")
		}
	} else if m.nowPlayingInfo != nil {
		body = m.nowPlayingInfo.Render(m.termWidth, m.termHeight, *m.styles)
	} else if m.preview != nil {
		body = m.preview.Render(m.termWidth, m.termHeight, *m.styles)
	} else {
		switch m.activeView {
		case viewLibrary:
			if m.destPicker != nil {
				body = m.destPicker.Render(m.termWidth, m.termHeight, *m.styles)
			} else {
				body = m.library.render(*m.styles)
			}
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

	content := body + "\n\n" + m.renderStatusBar()
	v := tea.NewView(content)
	v.AltScreen = true
	return v
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
	if m.elapsedSecs > 0 || m.totalSecs > 0 {
		sb.elapsed = formatTime(m.elapsedSecs)
		sb.total = formatTime(m.totalSecs)
	}
	return sb.render(m.termWidth, *m.styles)
}

// handleKeyAction dispatches a resolved KeyAction to the appropriate handler.
// It is called from the tea.KeyPressMsg case in Update after the registry
// resolves the key string to an action.
func (m Model) handleKeyAction(action action) (tea.Model, tea.Cmd) {
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
		m.helpScroll = 0
	case actionNextView:
		switch m.activeView {
		case viewLibrary:
			m.activeView = viewPlaylists
		case viewPlaylists:
			m.activeView = viewQueue
		case viewQueue:
			m.activeView = viewArtists
		case viewArtists:
			m.activeView = viewAlbums
		case viewAlbums:
			m.activeView = viewLibrary
		}
		if m.activeView == viewArtists && len(m.artists.artists) == 0 {
			m.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
		}
		if m.activeView == viewAlbums && len(m.albums.albums) == 0 {
			m.albums.albums = albtui.DeriveAllAlbums(m.library.index.Tracks())
		}
	case actionQuit:
		m.nowPlayingInfo = nil
		m = m.dismissPreview()
		return m, tea.Quit
	case actionFilterBar:
		return m, func() tea.Msg { return FilterBarMsg{} }
	case actionSwitchLibrary:
		m.activeView = viewLibrary
	case actionSwitchPlaylists:
		m.activeView = viewPlaylists
	case actionSwitchQueue:
		m.activeView = viewQueue
	case actionSwitchArtists:
		m.activeView = viewArtists
		if len(m.artists.artists) == 0 {
			m.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
		}
	case actionSwitchAlbums:
		m.activeView = viewAlbums
		if len(m.albums.albums) == 0 {
			m.albums.albums = albtui.DeriveAllAlbums(m.library.index.Tracks())
		}
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
					tracks, _ := reader.ReadPlaylistTracks(pid)
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
					Duration: float64(r.DurationMs) / 1000,
				}
			}
			ids, err := be.AppendTracks(tracks, playlistName)
			if err != nil {
				return BatchEnqueuedMsg{Err: err}
			}
			entries := make([]QueueEntry, len(tracks))
			for i, t := range tracks {
				display := t.Artist + " – " + t.Title
				if t.Artist == "" && t.Title == "" {
					display = t.Path
				} else if t.Artist == "" {
					display = t.Title
				} else if t.Title == "" {
					display = t.Artist
				}
				entries[i] = QueueEntry{
					ID:             ids[i],
					Path:           t.Path,
					Hash:           t.Hash,
					Display:        display,
					Artist:         t.Artist,
					Title:          t.Title,
					DurationMs:     int64(t.Duration * 1000),
					SourcePlaylist: playlistName,
				}
			}
			return BatchEnqueuedMsg{
				Entries:      entries,
				QueueIDs:     ids,
				WasEmpty:     wasEmpty,
				PlaylistName: playlistName,
			}
		}
	case actionBulkRemovePlaylistTracks:
		if !m.playlists.atListLevel() && m.playlists.hasTrackSelection() {
			softDeleteTime := time.Now().Unix()
			if m.playlistDeleter != nil {
				pd := m.playlistDeleter
				for _, t := range m.playlists.tracks {
					if _, ok := m.playlists.selectedTrackIDs[t.ID]; ok {
						_ = pd.RemovePlaylistTrackSoft(t.ID)
					}
				}
				time.AfterFunc(undoWindowDuration, func() {
					_ = pd.PurgeDeletedPlaylistTracks(softDeleteTime + 1)
				})
			}
			var remaining []db.PlaylistTrackRow
			for _, t := range m.playlists.tracks {
				if _, ok := m.playlists.selectedTrackIDs[t.ID]; !ok {
					remaining = append(remaining, t)
				}
			}
			m.playlists.tracks = remaining
			if m.playlists.cursor >= len(m.playlists.tracks) && m.playlists.cursor > 0 {
				m.playlists.cursor = len(m.playlists.tracks) - 1
			}
			if nt := len(m.playlists.tracks); nt > 0 {
				maxOffset := nt - 1
				if m.playlists.scrollOffset > maxOffset {
					m.playlists.scrollOffset = maxOffset
				}
			} else {
				m.playlists.scrollOffset = 0
			}
			n := len(m.playlists.selectedTrackIDs)
			m.playlists = m.playlists.clearTrackSelection()
			m.statusBar.notification = transientNotification{
				msg:       fmt.Sprintf("%d %s removed from playlist", n, pluralize(n, "track", "tracks")),
				expiresAt: time.Now().Add(transientDuration),
			}
		}
	case actionPlaylistDelete:
		if m.playlists.atListLevel() {
			if len(m.playlists.playlists) == 0 {
				break
			}
			idx := m.playlists.cursor
			if idx < 0 || idx >= len(m.playlists.playlists) {
				break
			}
			entry := m.playlists.playlists[idx]
			pd := m.playlistDeleter
			cancel := m.scheduleSoftDelete(
				func() {
					if pd != nil {
						_ = pd.DeletePlaylistSoft(entry.ID)
					}
				},
				func(before int64) {
					if pd != nil {
						_ = pd.PurgeDeletedPlaylists(before)
					}
				},
			)
			m.setUndoSlot(undoSlot{
				kind:                undoKindDeletePlaylist,
				expiresAt:           time.Now().Add(undoWindowDuration),
				cancelPurge:         cancel,
				deletePlaylistEntry: entry,
				deletePlaylistIdx:   idx,
			}, "Playlist deleted — press u to undo")
			m.playlists.playlists = append(m.playlists.playlists[:idx], m.playlists.playlists[idx+1:]...)
			if m.playlists.cursor >= len(m.playlists.playlists) && m.playlists.cursor > 0 {
				m.playlists.cursor = len(m.playlists.playlists) - 1
			}
			if n := len(m.playlists.playlists); n > 0 {
				maxOffset := n - 1
				if m.playlists.scrollOffset > maxOffset {
					m.playlists.scrollOffset = maxOffset
				}
			} else {
				m.playlists.scrollOffset = 0
			}
		} else {
			if len(m.playlists.tracks) == 0 {
				break
			}
			idx := m.playlists.cursor
			if idx < 0 || idx >= len(m.playlists.tracks) {
				break
			}
			entry := m.playlists.tracks[idx]
			pd := m.playlistDeleter
			cancel := m.scheduleSoftDelete(
				func() {
					if pd != nil {
						_ = pd.RemovePlaylistTrackSoft(entry.ID)
					}
				},
				func(before int64) {
					if pd != nil {
						_ = pd.PurgeDeletedPlaylistTracks(before)
					}
				},
			)
			m.setUndoSlot(undoSlot{
				kind:             undoKindRemovePlaylistTrack,
				expiresAt:        time.Now().Add(undoWindowDuration),
				cancelPurge:      cancel,
				removeTrackEntry: entry,
				removeTrackIdx:   idx,
			}, "Track removed — press u to undo")
			m.playlists.tracks = append(m.playlists.tracks[:idx], m.playlists.tracks[idx+1:]...)
			if m.playlists.cursor >= len(m.playlists.tracks) && m.playlists.cursor > 0 {
				m.playlists.cursor = len(m.playlists.tracks) - 1
			}
			if n := len(m.playlists.tracks); n > 0 {
				maxOffset := n - 1
				if m.playlists.scrollOffset > maxOffset {
					m.playlists.scrollOffset = maxOffset
				}
			} else {
				m.playlists.scrollOffset = 0
			}
		}
	}
	return m, nil
}
