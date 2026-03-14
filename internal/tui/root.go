package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
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
	undoNone                  undoKind = iota
	undoKindDequeue                    // single track removed via DequeueTrackMsg
	undoKindClear                      // all tracks removed via ClearQueueMsg
	undoKindDeletePlaylist             // playlist soft-deleted via actionPlaylistDelete at ListLevel
	undoKindRemovePlaylistTrack        // track soft-removed via actionPlaylistDelete at TrackLevel
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

// UndoMsg is dispatched when the user presses 'u'.
type UndoMsg struct{}

// FilterBarMsg is dispatched when the user presses '/' to activate the
// filter bar in the active view.
type FilterBarMsg struct{}

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

// tracksAddedToPlaylistMsg is dispatched after a successful AddTracksToPlaylist
// call. It carries the refreshed playlist list (for updating track counts) and
// metadata for the status-bar notification.
type tracksAddedToPlaylistMsg struct {
	playlists    []db.PlaylistRow
	playlistName string
	count        int
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
// Library, Playlists, and Queue panels, and renders a persistent status bar.
type Model struct {
	activeView    viewID
	logPath       string
	configPath    string
	configCreated bool

	library   LibraryView
	playlists PlaylistsView
	queue     QueueView

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

	statusBar       StatusBar
	termWidth       int
	termHeight      int
	registry        Registry
	helpVisible     bool   // true while the help overlay is shown
	prevActiveView  viewID // the view that was active before help was opened

	// player is the playback backend. It may be nil when playback is not
	// available (e.g. in tests that do not exercise playback).
	player          playPauser
	queueWriter     libtui.QueueAppender    // may be nil; set via SetQueueWriter
	undoer          queueDequeuer           // may be nil; set via SetQueueDequeuer
	sessionWriter   sessionWriter           // may be nil; set via SetSessionWriter
	playlistReader  playlistTrackReader     // may be nil; set via SetPlaylistTrackReader
	playlistWriter  playlistCreator         // may be nil; set via SetPlaylistWriter
	playlistRenamer playlistRenamer         // may be nil; set via SetPlaylistRenamer
	playlistDeleter  playlistDeleter         // may be nil; set via SetPlaylistDeleter
	playlistAdder    playlistTrackAdder      // may be nil; set via SetPlaylistAdder

	// destPicker is non-nil while the destination picker modal is open.
	// Root dispatches all keypresses to the modal while it is open.
	destPicker       *pltui.DestinationPickerModal
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
	currentTrackHash string        // content hash of the track currently loaded in the player

	// elapsedSecs and totalSecs hold the most recently queried playback
	// position and track duration in seconds. Zero values mean no time
	// data is available and suppress the time display in the status bar.
	elapsedSecs float64
	totalSecs   float64

}

// New creates a root Model. logPath is displayed in the status bar on every
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
func New(logPath, configPath string, configCreated bool, scanCh <-chan library.Track, watchCh <-chan library.WatchEvent, libraryRoot string, extractor library.MetadataExtractor, p playPauser) Model {
	return Model{
		activeView:    viewLibrary,
		logPath:       logPath,
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
		statusBar: StatusBar{
			logPath:       logPath,
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

// undoWindowDuration is the time after a destructive queue action during which
// pressing 'u' will reverse the action. Only a single undo slot is maintained;
// a new destructive action within the window replaces the prior slot.
const undoWindowDuration = 5 * time.Second

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
	default:
		// A new viewID was added without a corresponding case here. Add the
		// case above to ensure view-local keybinding precedence works correctly
		// for the new view. Falling back to contextGlobal would silently bypass
		// view-local shadowing (e.g. playlist 'r' rename vs. global repeat).
		panic("activeViewContext: unhandled viewID — add a case for the new view")
	}
}

// Update handles incoming messages. Number keys 1/2/3 switch the active view;
// Ctrl+c quits. q and Esc are intentionally ignored here — they are reserved
// for in-view navigation.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		m.library.width = msg.Width
		m.library.height = msg.Height
		m.playlists.width = msg.Width
		m.playlists.height = msg.Height
		m.queue.width = msg.Width
		m.queue.height = msg.Height

	case clearConfigNotice:
		m.configCreated = false

	case library.ScanProgressMsg:
		m.statusBar.handleScanProgress(msg)

	case library.ScanTotalMsg:
		m.scanTotal = msg.Total
		if m.scanning {
			m.statusBar.handleScanProgress(library.ScanProgressMsg{Scanned: m.scanned, Total: m.scanTotal})
		}

	case library.ScanBatchMsg:
		for _, t := range msg.Tracks {
			m.library.index.Insert(t)
		}
		m.scanned += len(msg.Tracks)
		m.statusBar.handleScanProgress(library.ScanProgressMsg{Scanned: m.scanned, Total: m.scanTotal})
		if m.scanCh != nil {
			return m, waitForNextBatch(m.scanCh)
		}

	case watchBatchMsg:
		for _, path := range msg.deleted {
			m.library.index.RemoveByPath(path)
		}
		for _, t := range msg.tracks {
			m.library.index.UpsertByPath(t.Path, t)
		}
		if m.watchCh != nil {
			return m, waitForWatchBatch(m.watchCh)
		}

	case library.ScanDoneMsg:
		m.scanning = false
		m.scanCh = nil
		// The last scan-progress notification expires naturally via the tick.

	case tickMsg:
		if er, ok := m.player.(elapsedReader); ok {
			if elapsed, total, err := er.ElapsedPosition(); err == nil {
				m.elapsedSecs = elapsed
				m.totalSecs = total
			}
		}
		if !m.statusBar.notification.expiresAt.IsZero() && time.Now().After(m.statusBar.notification.expiresAt) {
			m.statusBar.notification = transientNotification{}
		}
		return m, tickCmd()

	case syncPositionMsg:
		if ps, ok := m.player.(positionSyncer); ok {
			_ = ps.SyncPosition()
		}
		return m, syncPositionCmd()

	case sessionTickMsg:
		var cmd tea.Cmd
		if m.playbackState == PlaybackPlaying {
			cmd = m.sessionPositionCmd()
		}
		return m, tea.Batch(sessionTickCmd(), cmd)

	case PlayerStatusMsg:
		m.statusBar.notification = transientNotification{
			msg:       msg.Text,
			expiresAt: time.Now().Add(transientDuration),
		}

	case MissingTrackMsg:
		m.queue.markMissing(msg.Path)
		m.statusBar.handleMissingTrack(msg.Path)

	case FilterBarMsg:
		m.library = m.library.ActivateFilter()

	case PlaybackEndedMsg:
		m.playbackState = PlaybackIdle
		m.elapsedSecs = 0
		m.totalSecs = 0
		// PlaybackEndedMsg already implies the queue was exhausted (mpv
		// auto-advance found no more playable entries). Checking queue length
		// here would always be false because entries are never removed from the
		// TUI slice during normal play-through.
		if m.repeatMode == RepeatOff {
			m.statusBar.notification = transientNotification{
				msg:       "Queue finished",
				expiresAt: time.Now().Add(transientDuration),
			}
		}

	case TrackChangedMsg:
		m.currentTrackHash = msg.Hash
		m.statusBar.artist = msg.Artist
		m.statusBar.title = msg.Title
		source := msg.PlaylistSource
		if source == "" {
			source = "Queue"
		}
		m.statusBar.playlistSource = source
		m.playbackState = PlaybackPlaying
		m.elapsedSecs = 0
		m.totalSecs = 0
		return m, m.sessionPositionCmd()

	case libtui.TrackEnqueuedMsg:
		if msg.Err != nil {
			break
		}
		display := msg.Track.Artist + " – " + msg.Track.Title
		if msg.Track.Artist == "" && msg.Track.Title == "" {
			display = msg.Track.Path
		} else if msg.Track.Artist == "" {
			display = msg.Track.Title
		} else if msg.Track.Title == "" {
			display = msg.Track.Artist
		}
		m.queue.entries = append(m.queue.entries, QueueEntry{
			ID:         msg.QueueID,
			Path:       msg.Track.Path,
			Hash:       msg.Track.Hash,
			Display:    display,
			Artist:     msg.Track.Artist,
			Title:      msg.Track.Title,
			DurationMs: int64(msg.Track.Duration * 1000),
		})
		// Keep the player's in-memory queue in sync.
		if pa, ok := m.player.(playerQueueAppender); ok {
			pa.AppendToQueue(player.QueueTrack{
				Path:    msg.Track.Path,
				Hash:    msg.Track.Hash,
				QueueID: msg.QueueID,
				Artist:  msg.Track.Artist,
				Title:   msg.Track.Title,
			})
		}
		// Trigger immediate playback when the queue was previously empty and
		// nothing is playing.
		if msg.WasEmpty && m.playbackState == PlaybackIdle && m.player != nil {
			if err := m.player.PlayFromIndex(0); err == nil {
				m.playbackState = PlaybackPlaying
				m.statusBar.artist = msg.Track.Artist
				m.statusBar.title = msg.Track.Title
				m.statusBar.playlistSource = "Queue"
			}
		}

	case PlayQueueIndexMsg:
		if m.player != nil {
			if err := m.player.PlayFromIndex(msg.Index); err == nil {
				m.playbackState = PlaybackPlaying
				if msg.Index < len(m.queue.entries) {
					e := m.queue.entries[msg.Index]
					m.statusBar.artist = e.Artist
					m.statusBar.title = e.Title
				}
				m.statusBar.playlistSource = "Queue"
			}
		}

	case DequeueTrackMsg:
		if msg.Index >= 0 && msg.Index < len(m.queue.entries) {
			entry := m.queue.entries[msg.Index]
			// Cancel any in-flight purge timer from the previous undo slot
			// before replacing it. Without this, the old timer could fire
			// against rows soft-deleted in the same Unix second and delete DB
			// rows that now belong to the new slot.
			if m.undoSlot.cancelPurge != nil {
				close(m.undoSlot.cancelPurge)
			}
			cancel := make(chan struct{})
			softDeleteTime := time.Now().Unix()
			if m.undoer != nil {
				_ = m.undoer.DequeueTrackSoft(entry.ID)
				undoer := m.undoer
				time.AfterFunc(undoWindowDuration, func() {
					select {
					case <-cancel:
						return // slot superseded; skip purge
					default:
						_ = undoer.PurgeDeletedQueueEntries(softDeleteTime + 1)
					}
				})
			}
			m.undoSlot = undoSlot{
				kind:         undoKindDequeue,
				expiresAt:    time.Now().Add(undoWindowDuration),
				cancelPurge:  cancel,
				dequeueID:    entry.ID,
				dequeueEntry: entry,
				dequeueIdx:   msg.Index,
			}
			m.statusBar.notification = transientNotification{
				msg:       "Track removed — press u to undo",
				expiresAt: time.Now().Add(undoWindowDuration),
			}
			m.queue.entries = append(m.queue.entries[:msg.Index], m.queue.entries[msg.Index+1:]...)
			if m.queue.cursor >= len(m.queue.entries) && m.queue.cursor > 0 {
				m.queue.cursor = len(m.queue.entries) - 1
			}
		}

	case BulkSelectionToggleMsg:
		if msg.Index >= 0 && msg.Index < len(m.queue.entries) {
			m.queue.entries[msg.Index].Selected = !m.queue.entries[msg.Index].Selected
		}

	case ClearQueueMsg:
		savedEntries := make([]QueueEntry, len(m.queue.entries))
		copy(savedEntries, m.queue.entries)
		// Build parallel player tracks from saved entries for undo restore.
		savedTracks := make([]player.QueueTrack, len(savedEntries))
		for i, e := range savedEntries {
			savedTracks[i] = player.QueueTrack{
				Path:    e.Path,
				Hash:    e.Hash,
				QueueID: e.ID,
				Artist:  e.Artist,
				Title:   e.Title,
			}
		}
		// Cancel any in-flight purge timer from the previous undo slot before
		// replacing it (same-second race guard as in DequeueTrackMsg).
		if m.undoSlot.cancelPurge != nil {
			close(m.undoSlot.cancelPurge)
		}
		cancel := make(chan struct{})
		var clearIDs []int64
		softDeleteTime := time.Now().Unix()
		if m.undoer != nil {
			ids, _ := m.undoer.ClearQueueSoft()
			clearIDs = ids
			undoer := m.undoer
			time.AfterFunc(undoWindowDuration, func() {
				select {
				case <-cancel:
					return // slot superseded; skip purge
				default:
					_ = undoer.PurgeDeletedQueueEntries(softDeleteTime + 1)
				}
			})
		}
		m.undoSlot = undoSlot{
			kind:         undoKindClear,
			expiresAt:    time.Now().Add(undoWindowDuration),
			cancelPurge:  cancel,
			clearIDs:     clearIDs,
			clearEntries: savedEntries,
			clearTracks:  savedTracks,
		}
		m.statusBar.notification = transientNotification{
			msg:       "Queue cleared — press u to undo",
			expiresAt: time.Now().Add(undoWindowDuration),
		}
		m.queue.entries = nil
		m.queue.cursor = 0
		// Stop active playback — requirement req-019ce3a80fa97003a43d90cf5e7c7a3f.
		if cp, ok := m.player.(commandPlayer); ok {
			_ = cp.Stop()
			m.playbackState = PlaybackIdle
			m.elapsedSecs = 0
			m.totalSecs = 0
		}
		// Clear the player's in-memory queue so it stays in sync with the TUI.
		if qs, ok := m.player.(playerQueueSetter); ok {
			qs.SetQueue(nil)
		}

	case UndoMsg:
		if m.undoSlot.kind != undoNone && time.Now().Before(m.undoSlot.expiresAt) {
			switch m.undoSlot.kind {
			case undoKindDequeue:
				if m.undoer != nil {
					_ = m.undoer.UndoDequeue(m.undoSlot.dequeueID)
				}
				// Reinsert the entry at its original position in the TUI slice.
				idx := min(m.undoSlot.dequeueIdx, len(m.queue.entries))
				entries := make([]QueueEntry, len(m.queue.entries)+1)
				copy(entries, m.queue.entries[:idx])
				entries[idx] = m.undoSlot.dequeueEntry
				copy(entries[idx+1:], m.queue.entries[idx:])
				m.queue.entries = entries
				m.queue.cursor = idx
			case undoKindClear:
				if m.undoer != nil {
					_ = m.undoer.UndoClearQueue(m.undoSlot.clearIDs)
				}
				m.queue.entries = m.undoSlot.clearEntries
				// Restore the player's in-memory queue to match the restored TUI entries.
				if qs, ok := m.player.(playerQueueSetter); ok {
					qs.SetQueue(m.undoSlot.clearTracks)
				}
				case undoKindDeletePlaylist:
					if m.playlistDeleter != nil {
						_ = m.playlistDeleter.UndoDeletePlaylist(m.undoSlot.deletePlaylistEntry.ID)
					}
					// Reinsert the playlist at its original position in the TUI slice.
					idx := min(m.undoSlot.deletePlaylistIdx, len(m.playlists.playlists))
					playlists := make([]db.PlaylistRow, len(m.playlists.playlists)+1)
					copy(playlists, m.playlists.playlists[:idx])
					playlists[idx] = m.undoSlot.deletePlaylistEntry
					copy(playlists[idx+1:], m.playlists.playlists[idx:])
					m.playlists.playlists = playlists
					m.playlists.cursor = idx
				case undoKindRemovePlaylistTrack:
					if m.playlistDeleter != nil {
						_ = m.playlistDeleter.UndoRemovePlaylistTrack(m.undoSlot.removeTrackEntry.ID)
					}
					// Reinsert the track at its original position in the TUI slice.
					idx := min(m.undoSlot.removeTrackIdx, len(m.playlists.tracks))
					tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks)+1)
					copy(tracks, m.playlists.tracks[:idx])
					tracks[idx] = m.undoSlot.removeTrackEntry
					copy(tracks[idx+1:], m.playlists.tracks[idx:])
					m.playlists.tracks = tracks
					m.playlists.cursor = idx
			}
			// Cancel the background purge — the undo restored deleted_at to
			// NULL, so a stale purge would have been a no-op anyway, but
			// cancelling is cleaner and avoids any same-second edge cases.
			if m.undoSlot.cancelPurge != nil {
				close(m.undoSlot.cancelPurge)
			}
			m.undoSlot = undoSlot{}
		}

	case MoveTrackUpMsg:
		if msg.Index > 0 && msg.Index < len(m.queue.entries) {
			m.queue.entries[msg.Index-1], m.queue.entries[msg.Index] = m.queue.entries[msg.Index], m.queue.entries[msg.Index-1]
			m.queue.cursor = msg.Index - 1
		}

	case MoveTrackDownMsg:
		if msg.Index >= 0 && msg.Index < len(m.queue.entries)-1 {
			m.queue.entries[msg.Index], m.queue.entries[msg.Index+1] = m.queue.entries[msg.Index+1], m.queue.entries[msg.Index]
			m.queue.cursor = msg.Index + 1
		}

	case tea.KeyPressMsg:
		if m.helpVisible {
			if msg.String() == "esc" {
				m.helpVisible = false
				m.activeView = m.prevActiveView
			}
			return m, nil
		}
		// When the library filter bar is active, intercept all keypresses
		// before the registry so that typing populates the filter text
		// instead of triggering normal keybindings.
		if m.library.filterActive && m.activeView == viewLibrary {
			switch msg.String() {
			case "esc":
				m.library = m.library.DeactivateFilter()
			case "backspace":
				m.library = m.library.BackspaceFilter()
			default:
				if msg.Text != "" {
					for _, ch := range msg.Text {
						m.library = m.library.AppendFilterChar(ch)
					}
				}
			}
			return m, nil
		}
		// When the inline playlist create input is active, intercept all
		// keypresses before the registry.
		if m.playlists.isCreating() && m.activeView == viewPlaylists {
			switch msg.String() {
			case "esc":
				m.playlists = m.playlists.cancelCreate()
			case "enter":
				if !m.playlists.createInputIsEmpty() {
					name := m.playlists.createInputValue()
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
					// Reject duplicate names (case-sensitive) with a transient hint.
					// Skip the playlist being renamed so a no-op Enter is treated as cancel.
					for _, p := range m.playlists.playlists {
						if p.ID != pl.ID && p.Name == name {
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
		// While the destination picker modal is open, route all keypresses to
		// it instead of the normal registry. The modal dispatches its own Cmds
		// (DestinationPickedMsg, DestinationPickerDismissedMsg, etc.) which are
		// handled below in the non-KeyPressMsg cases.
		if m.destPicker != nil {
			updated, cmd := m.destPicker.HandleKey(msg.String())
			m.destPicker = &updated
			return m, cmd
		}
		a := m.registry.resolve(m.activeViewContext(), msg.String())
		switch a {
		case actionHelp:
			m.prevActiveView = m.activeView
			m.helpVisible = true
		case actionQuit:
			return m, tea.Quit
		case actionPlayPause:
			if m.player != nil {
				if m.playbackState == PlaybackPlaying {
					_ = m.player.Pause()
					m.playbackState = PlaybackPaused
					return m, m.sessionPositionCmd()
				} else if len(m.queue.entries) == 0 {
					m.statusBar.notification = transientNotification{
						msg:       "Queue is empty — enqueue tracks from Library (key 1)",
						expiresAt: time.Now().Add(transientDuration),
					}
				} else {
					_ = m.player.Play()
					m.playbackState = PlaybackPlaying
				}
			}
		case actionStop:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.Stop()
				m.playbackState = PlaybackStopped
				m.elapsedSecs = 0
				m.totalSecs = 0
			}
		case actionNext:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.Next()
			}
		case actionPrevious:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.Previous()
			}
		case actionSeekBack:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(-5)
				return m, m.sessionPositionCmd()
			}
		case actionSeekForward:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(5)
				return m, m.sessionPositionCmd()
			}
		case actionSeekBackLong:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(-30)
				return m, m.sessionPositionCmd()
			}
		case actionSeekForwardLong:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(30)
				return m, m.sessionPositionCmd()
			}
		case actionVolumeUp:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SetVolumeRelative(5)
			}
		case actionVolumeDown:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SetVolumeRelative(-5)
			}
		case actionSwitchLibrary:
			m.activeView = viewLibrary
		case actionSwitchPlaylists:
			m.activeView = viewPlaylists
		case actionSwitchQueue:
			m.activeView = viewQueue
		case actionRescan:
			if m.libraryRoot != "" && m.extractor != nil && !m.scanning {
				ch := library.Scan(m.libraryRoot, m.extractor)
				m.library.index = library.LibraryIndex{}
				m.scanCh = ch
				m.scanning = true
				m.scanned = 0
				m.scanTotal = 0
				return m, tea.Batch(waitForNextBatch(ch), countAudioFiles(m.libraryRoot))
			}
		case actionRepeatCycle:
			switch m.repeatMode {
			case RepeatOff:
				m.repeatMode = RepeatAll
			case RepeatAll:
				m.repeatMode = RepeatOne
			case RepeatOne:
				m.repeatMode = RepeatOff
			}
			if rs, ok := m.player.(repeatShuffler); ok {
				_ = rs.SetRepeat(m.repeatMode.String())
			}
			if m.sessionWriter != nil {
				_ = m.sessionWriter.WriteRepeatMode(m.repeatMode.String())
			}
		case actionShuffleToggle:
			m.shuffle = !m.shuffle
			if rs, ok := m.player.(repeatShuffler); ok {
				_ = rs.SetShuffle(m.shuffle)
			}
			if m.sessionWriter != nil {
				_ = m.sessionWriter.WriteShuffle(m.shuffle)
			}
		case actionUndo:
			return m, func() tea.Msg { return UndoMsg{} }
		case actionFilterBar:
			return m, func() tea.Msg { return FilterBarMsg{} }
		case actionLibraryEnqueue:
			if m.queueWriter != nil {
				return m, m.library.EnqueueFocused(m.queueWriter, len(m.queue.entries))
			}
		case actionBulkAction:
			if m.activeView == viewLibrary {
				var tracks []library.Track
				if len(m.library.selectedHashes) > 0 {
					// Collect all bulk-selected tracks in index order.
					for _, t := range m.library.index.Tracks() {
						if _, ok := m.library.selectedHashes[t.Hash]; ok {
							tracks = append(tracks, t)
						}
					}
				} else {
					// No bulk selection: use the focused track.
					if t, ok := m.library.view().FocusedTrack(); ok {
						tracks = []library.Track{t}
					}
				}
				if len(tracks) > 0 {
					modal := pltui.NewDestinationPicker(m.playlists.playlists)
					m.destPicker = &modal
					m.destPickerTracks = tracks
				}
			}
		case actionCursorUp, actionCursorDown:
			// Route cursor movement to the active view.
			switch m.activeView {
			case viewLibrary:
				m.library = m.library.HandleAction(a)
			case viewPlaylists:
				m.playlists = m.playlists.HandleAction(a)
			default:
				var cmd tea.Cmd
				m.queue, cmd = m.queue.HandleAction(a)
				return m, cmd
			}
		case actionBulkSelectionToggle:
			// Route to the active view: library, playlists (TrackLevel), or queue.
			if m.activeView == viewLibrary {
				m.library = m.library.HandleAction(a)
			} else if m.activeView == viewPlaylists {
				m.playlists = m.playlists.HandleAction(a)
			} else {
				var cmd tea.Cmd
				m.queue, cmd = m.queue.HandleAction(a)
				return m, cmd
			}
		case actionSelect, actionDequeue,
			actionClearQueue, actionMoveTrackUp, actionMoveTrackDown:
			// Queue-only actions — pass to QueueView regardless of active view.
			var cmd tea.Cmd
			m.queue, cmd = m.queue.HandleAction(a)
			return m, cmd
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
		case actionAddToPlaylist:
			// At TrackLevel with a non-empty bulk selection: remove all selected
			// tracks from the playlist, then clear the selection markers.
			if !m.playlists.atListLevel() && m.playlists.hasTrackSelection() {
				if m.playlistDeleter != nil {
					for _, t := range m.playlists.tracks {
						if _, ok := m.playlists.selectedTrackIDs[t.ID]; ok {
							_ = m.playlistDeleter.RemovePlaylistTrackSoft(t.ID)
						}
					}
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
				n := len(m.playlists.selectedTrackIDs)
				m.playlists = m.playlists.clearTrackSelection()
				noun := "tracks"
				if n == 1 {
					noun = "track"
				}
				m.statusBar.notification = transientNotification{
					msg:       fmt.Sprintf("%d %s removed from playlist", n, noun),
					expiresAt: time.Now().Add(transientDuration),
				}
			}
		case actionPlaylistDelete:
			if m.playlists.atListLevel() {
				// Soft-delete the focused playlist.
				if len(m.playlists.playlists) == 0 {
					break
				}
				idx := m.playlists.cursor
				entry := m.playlists.playlists[idx]
				if m.undoSlot.cancelPurge != nil {
					close(m.undoSlot.cancelPurge)
				}
				cancel := make(chan struct{})
				softDeleteTime := time.Now().Unix()
				if m.playlistDeleter != nil {
					_ = m.playlistDeleter.DeletePlaylistSoft(entry.ID)
					pd := m.playlistDeleter
					time.AfterFunc(undoWindowDuration, func() {
						select {
						case <-cancel:
							return
						default:
							_ = pd.PurgeDeletedPlaylists(softDeleteTime + 1)
						}
					})
				}
				m.undoSlot = undoSlot{
					kind:                undoKindDeletePlaylist,
					expiresAt:           time.Now().Add(undoWindowDuration),
					cancelPurge:         cancel,
					deletePlaylistEntry: entry,
					deletePlaylistIdx:   idx,
				}
				m.statusBar.notification = transientNotification{
					msg:       "Playlist deleted — press u to undo",
					expiresAt: time.Now().Add(undoWindowDuration),
				}
				m.playlists.playlists = append(m.playlists.playlists[:idx], m.playlists.playlists[idx+1:]...)
				if m.playlists.cursor >= len(m.playlists.playlists) && m.playlists.cursor > 0 {
					m.playlists.cursor = len(m.playlists.playlists) - 1
				}
			} else {
				// Soft-remove the focused track from the open playlist.
				if len(m.playlists.tracks) == 0 {
					break
				}
				idx := m.playlists.cursor
				entry := m.playlists.tracks[idx]
				if m.undoSlot.cancelPurge != nil {
					close(m.undoSlot.cancelPurge)
				}
				cancel := make(chan struct{})
				softDeleteTime := time.Now().Unix()
				if m.playlistDeleter != nil {
					_ = m.playlistDeleter.RemovePlaylistTrackSoft(entry.ID)
					pd := m.playlistDeleter
					time.AfterFunc(undoWindowDuration, func() {
						select {
						case <-cancel:
							return
						default:
							_ = pd.PurgeDeletedPlaylistTracks(softDeleteTime + 1)
						}
					})
				}
				m.undoSlot = undoSlot{
					kind:             undoKindRemovePlaylistTrack,
					expiresAt:        time.Now().Add(undoWindowDuration),
					cancelPurge:      cancel,
					removeTrackEntry: entry,
					removeTrackIdx:   idx,
				}
				m.statusBar.notification = transientNotification{
					msg:       "Track removed — press u to undo",
					expiresAt: time.Now().Add(undoWindowDuration),
				}
				m.playlists.tracks = append(m.playlists.tracks[:idx], m.playlists.tracks[idx+1:]...)
				if m.playlists.cursor >= len(m.playlists.tracks) && m.playlists.cursor > 0 {
					m.playlists.cursor = len(m.playlists.tracks) - 1
				}
			}
		}
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

	case playlistCreateErrorMsg:
		m.playlists = m.playlists.cancelCreate()
		m.statusBar.notification = transientNotification{
			msg:       "Failed to create playlist: " + msg.err.Error(),
			expiresAt: time.Now().Add(transientDuration),
		}

	case tracksAddedToPlaylistMsg:
		if msg.playlists != nil {
			m.playlists.playlists = msg.playlists
		}
		noun := "track"
		if msg.count != 1 {
			noun = "tracks"
		}
		m.statusBar.notification = transientNotification{
			msg:       fmt.Sprintf("%d %s added to %s", msg.count, noun, msg.playlistName),
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
						id, err := qa.AppendTrack(track)
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
							return nil
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

// View renders the active panel and a status bar, returning a tea.View.
// When the help overlay is visible it replaces the body content entirely.
func (m Model) View() tea.View {
	var body string
	if m.helpVisible {
		body = HelpOverlay{logPath: m.logPath}.render()
	} else {
		switch m.activeView {
		case viewLibrary:
			body = m.library.render()
			if m.destPicker != nil {
				body += "\n" + m.destPicker.Render(m.termWidth, m.termHeight)
			}
		case viewPlaylists:
			body = m.playlists.render()
		case viewQueue:
			body = m.queue.render()
		}
	}

	content := body + "\n\n" + m.renderStatusBar()
	return tea.NewView(content)
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
	return sb.render(m.termWidth)
}
