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
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// overlayInfo groups all overlay and modal state.
type overlayInfo struct {
	preview           *PreviewOverlay
	previewPlayer     player.Previewer
	previewWasPlaying bool
	nowPlaying        *NowPlayingOverlay
	trackInfo         *TrackInfoOverlay
	metadataEditor    *MetadataEditor
	destPicker        *pltui.DestinationPickerModal
	destPickerTracks  []library.Track
}

// undoInfo groups the fields that track pending destructive-action confirmations
// and the reversible undo slot.
type undoInfo struct {
	slot                undoSlot
	clearQueuePending   bool
	clearQueuePendingAt time.Time
	bulkRemovePending   bool
	bulkRemovePendingAt time.Time
}

// playbackInfo groups the fields that describe the current playback state.
type playbackInfo struct {
	state       PlaybackState
	repeatMode  RepeatMode
	shuffle     bool
	volume      int
	currentHash string
	elapsedSecs float64
	totalSecs   float64
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
	favorites FavoritesView

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

	// watchDegraded is true when the filesystem watcher could not be started
	// (e.g. fd exhaustion). The status bar shows a notice so the user knows
	// to use manual rescan.
	watchDegraded bool

	// libraryRoot and extractor are required for manual rescan. When
	// libraryRoot is empty the rescan hotkey is a no-op.
	libraryRoot string
	extractor   library.MetadataExtractor

	statusBar        StatusBar
	termWidth        int
	termHeight       int
	registry         Registry
	helpVisible      bool           // true while the help overlay is shown
	helpViewport     viewport.Model // scrollable viewport for the help overlay
	helpFiltered     bool           // true when showing only current-view keybindings
	helpSearchFilter filter.State   // search filter state for the help overlay
	prevActiveView   viewID         // the view that was active before help was opened

	// player is the playback backend. It may be nil when playback is not
	// available (e.g. in tests that do not exercise playback).
	player          playPauser
	queueWriter     libtui.QueueAppender // may be nil; set via WithQueueWriter
	reorderer       queueReorderer       // may be nil; set via WithQueueReorderer
	undoer          queueDequeuer        // may be nil; set via WithQueueDequeuer
	sessionWriter   sessionWriter        // may be nil; set via WithSessionWriter
	playlistReader  playlistTrackReader  // may be nil; set via WithPlaylistTrackReader
	playlistWriter  playlistCreator      // may be nil; set via WithPlaylistWriter
	playlistRenamer playlistRenamer      // may be nil; set via WithPlaylistRenamer
	playlistDeleter playlistDeleter      // may be nil; set via WithPlaylistDeleter
	playlistAdder   playlistTrackAdder   // may be nil; set via WithPlaylistAdder
	batchEnqueuer   batchEnqueuer        // may be nil; set via WithBatchEnqueuer
	favoriteReader  favoriteReader       // may be nil; set via WithFavoriteReader
	favoriteWriter  favoriteWriter       // may be nil; set via WithFavoriteWriter
	favoriteHashes  map[string]struct{}  // set of favorited track hashes for highlight

	overlay        overlayInfo     // all overlay and modal state
	metadataWriter metadataUpdater // may be nil; set via WithMetadataWriter

	undo undoInfo // reversible undo slot and pending confirmation flags

	// missingTrackCount tracks how many queue entries were skipped due to
	// missing files during the current playback session. Reset when playback
	// ends or the queue is cleared.
	missingTrackCount int

	playback playbackInfo // current playback state, repeat, shuffle, volume, position

	// runtimeDir is the XDG_RUNTIME_DIR-based directory for ephemeral sockets
	// (e.g. preview player). Set via WithRuntimeDir before tea.NewProgram.
	runtimeDir string

	// styles holds the current lipgloss style set. Initialized to dark-safe
	// defaults in New(); replaced on tea.BackgroundColorMsg once the terminal
	// reports its actual background color.
	styles *styles.Styles
}

// New creates a root Model. All dependencies are injected via functional
// options (With* functions). Only ctx is required as a positional parameter.
//
// Example:
//
//	model := tui.New(ctx,
//	    tui.WithConfigPath(path, true),
//	    tui.WithScanChannel(scanCh),
//	    tui.WithLibraryRoot(root),
//	    tui.WithPlayer(mgr),
//	)
func New(ctx context.Context, opts ...Option) Model {
	initialStyles := styles.NewStyles(true)
	m := Model{
		appCtx:     ctx,
		activeView: viewLibrary,
		library:    LibraryView{fuzzy: library.SahilmFuzzySearcher{}},
		registry:   newRegistry(),
		styles:     &initialStyles,
	}
	for _, opt := range opts {
		opt(&m)
	}
	// Derive scanning state from scanCh after options are applied.
	m.scanning = m.scanCh != nil
	// Pre-load the set of favorited track hashes for row highlighting.
	m.favoriteHashes = loadFavoriteTrackHashes(m.favoriteReader)
	// Sync statusBar fields from model state.
	m.statusBar.configPath = m.configPath
	m.statusBar.configCreated = m.configCreated
	return m
}

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

// WithScanChannel sets the library scan channel. When non-nil, the TUI reads
// incremental track updates from it. Pass the channel returned by library.Scan.
func WithScanChannel(ch <-chan library.Track) Option {
	return func(m *Model) { m.scanCh = ch }
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

// Volume returns the current volume level as an integer percentage.
func (m Model) Volume() int { return m.playback.volume }

// WithPlaybackState pre-sets the playback state.
func WithPlaybackState(state PlaybackState) Option {
	return func(m *Model) { m.playback.state = state }
}

// WithCurrentTrackHash pre-sets the current track hash.
func WithCurrentTrackHash(hash string) Option {
	return func(m *Model) { m.playback.currentHash = hash }
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

// RemoveLibraryTrackByPath removes a track from the in-memory library index
// by its file path. Call this before starting the TUI to evict deleted files
// discovered during the diff scan setup phase.
func (m *Model) RemoveLibraryTrackByPath(path string) {
	m.library.index.RemoveByPath(path)
}

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
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
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
				Source:  e.SourcePlaylist,
			}
		}
		qs.SetQueue(pt)
	}
}

// setUndoSlot replaces the current undo slot and derives the status-bar
// notification from action.description(). cancelPurge may be nil for actions
// that do not schedule a soft-delete purge timer. The caller is responsible
// for closing any existing cancelPurge before calling this (scheduleSoftDelete
// handles that for soft-delete cases; direct callers must do it manually).
func (m *Model) setUndoSlot(action undoAction, cancelPurge chan struct{}) {
	m.undo.slot = undoSlot{
		action:      action,
		expiresAt:   time.Now().Add(undoWindowDuration),
		cancelPurge: cancelPurge,
	}
	m.statusBar.notification = newNotificationFor(action.description(), undoWindowDuration)
}

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
	hash := m.playback.currentHash
	positionMs := int64(m.playback.elapsedSecs * 1000)
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
func countAudioFiles(ctx context.Context, root string) tea.Cmd {
	return func() tea.Msg {
		return library.ScanTotalMsg{Total: library.CountAudioFiles(ctx, root)}
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
	// configCreated notice is now dismissed on the first keypress (see
	// tea.KeyPressMsg handler) instead of an auto-clear timer.
	if m.scanCh != nil {
		cmds = append(cmds, waitForNextBatch(m.scanCh))
		if m.libraryRoot != "" {
			cmds = append(cmds, countAudioFiles(m.appCtx, m.libraryRoot))
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

// setViewDimensions updates the width and height on all primary views.
// height is reduced by 3 to leave room for the status bar and header rows.
func (m *Model) setViewDimensions(width, height int) {
	viewHeight := max(height-3, 0)
	m.library.width = width
	m.library.height = viewHeight
	m.playlists.width = width
	m.playlists.height = viewHeight
	m.queue.width = width
	m.queue.height = viewHeight
	m.artists.width = width
	m.artists.height = viewHeight
	m.albums.width = width
	m.albums.height = viewHeight
	m.favorites.width = width
	m.favorites.height = viewHeight
}

// Update handles incoming messages. Number keys 1/2/3/4 switch the active view;
// Ctrl+c quits. q and Esc are intentionally ignored here — they are reserved
// for in-view navigation.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		m.setViewDimensions(msg.Width, msg.Height)
		if m.helpVisible {
			m.helpViewport.SetWidth(msg.Width)
			extraLines := 0
			if m.helpSearchFilter.Active || m.helpSearchFilter.Text != "" {
				extraLines = 1
			}
			m.helpViewport.SetHeight(max(msg.Height-2-extraLines, 0))
		}

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

	case PlayerDisconnectedMsg:
		m = m.handlePlayerDisconnectedMsg()

	case PlaybackEndedMsg:
		m = m.handlePlaybackEndedMsg()
		if m.playback.state == PlaybackPlaying {
			return m, m.sessionPositionCmd()
		}

	case PauseChangedMsg:
		return m.handlePauseChangedMsg(msg)

	case RemoteActionMsg:
		m, cmd, _ := m.handlePlaybackKeyAction(remoteAction(msg.ActionName))
		return m, cmd

	case previewStartedMsg:
		m = m.handlePreviewStartedMsg(msg)

	case previewStartErrorMsg:
		m = m.handlePreviewStartErrorMsg(msg)

	case trackInfoMsg:
		if msg.err != nil {
			m.statusBar.notification = newNotification("Track info error: " + msg.err.Error())
		} else {
			overlay := newTrackInfoOverlay(msg.path, msg.tags)
			m.overlay.trackInfo = &overlay
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
		m.statusBar.notification = newNotification("Cannot play — file is missing")

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

	case TransientNotifyMsg:
		m.statusBar.notification = newNotification(string(msg))

	// --- Key handling ---

	case tea.KeyPressMsg:
		// Dismiss the first-run config notice on any keypress.
		m.configCreated = false

		if m, cmd, ok := m.handleHelpKeyPress(msg); ok {
			return m, cmd
		}
		if m, cmd, ok := m.handleNowPlayingKeyPress(msg); ok {
			return m, cmd
		}
		if m, cmd, ok := m.handleTrackInfoKeyPress(msg); ok {
			return m, cmd
		}
		if m, cmd, ok := m.handlePreviewKeyPress(msg); ok {
			return m, cmd
		}
		if m, cmd, ok := m.handleMetadataEditorKeyPress(msg); ok {
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
		if m, cmd, ok := m.handleDestPickerKeyPress(msg); ok {
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

	case favoriteToggledMsg:
		m = m.handleFavoriteToggledMsg(msg)

	case favoritesLoadedMsg:
		m = m.handleFavoritesLoadedMsg(msg)

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

// refreshHelpViewport rebuilds the help viewport content from the current
// help state and search filter, then resets the scroll position to the top.
func (m *Model) refreshHelpViewport() {
	sections, title := m.helpState()
	if m.helpSearchFilter.Text != "" {
		sections = searchHelpSections(sections, m.helpSearchFilter.Text)
	}
	m.helpViewport.SetContent(HelpOverlay{}.render(*m.styles, sections, title))
	m.helpViewport.SetYOffset(0)
	extraLines := 0
	if m.helpSearchFilter.Active || m.helpSearchFilter.Text != "" {
		extraLines = 1
	}
	m.helpViewport.SetHeight(max(m.termHeight-2-extraLines, 0))
}

// View renders the active panel and a status bar, returning a tea.View.
// When the help overlay is visible it replaces the body content entirely.
func (m Model) View() tea.View {
	var body string
	if m.helpVisible {
		var filterBar string
		if m.helpSearchFilter.Active {
			filterBar = "Search: > " + m.helpSearchFilter.Text + "\n"
		} else if m.helpSearchFilter.Text != "" {
			filterBar = "Search: " + m.helpSearchFilter.Text + "\n"
		}
		body = filterBar + m.helpViewport.View()
	} else if m.overlay.nowPlaying != nil {
		body = m.overlay.nowPlaying.Render(m.termWidth, m.termHeight, *m.styles)
	} else if m.overlay.trackInfo != nil {
		body = m.overlay.trackInfo.Render(m.termWidth, m.termHeight, *m.styles)
	} else if m.overlay.preview != nil {
		body = m.overlay.preview.Render(m.termWidth, m.termHeight, *m.styles)
	} else if m.overlay.metadataEditor != nil {
		body = m.overlay.metadataEditor.Render(m.termWidth, m.termHeight, *m.styles)
	} else if m.overlay.destPicker != nil {
		body = m.overlay.destPicker.Render(m.termWidth, m.termHeight, *m.styles)
	} else {
		switch m.activeView {
		case viewLibrary:
			body = m.library.render(*m.styles, m.favoriteHashes)
		case viewPlaylists:
			body = m.playlists.render(*m.styles, m.favoriteHashes)
		case viewQueue:
			body = m.queue.render(*m.styles, m.playback.currentHash, m.favoriteHashes)
		case viewArtists:
			body = m.artists.render(*m.styles, m.favoriteHashes)
		case viewAlbums:
			body = m.albums.render(*m.styles, m.favoriteHashes)
		case viewFavorites:
			body = m.favorites.render(*m.styles, m.favoriteHashes)
		}
	}

	// Prepend the centralized title bar when showing a primary view.
	if m.overlay.nowPlaying == nil && m.overlay.trackInfo == nil && m.overlay.preview == nil && m.overlay.metadataEditor == nil && m.overlay.destPicker == nil && !m.helpVisible {
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
	sb.playbackState = m.playback.state
	sb.repeatMode = m.playback.repeatMode
	sb.shuffle = m.playback.shuffle
	sb.configCreated = m.configCreated
	sb.watchDegraded = m.watchDegraded
	sb.queueEmpty = len(m.queue.entries) == 0
	if m.playback.elapsedSecs > 0 || m.playback.totalSecs > 0 {
		sb.elapsed = formatTime(m.playback.elapsedSecs)
		sb.total = formatTime(m.playback.totalSecs)
	}
	return sb.render(m.termWidth, *m.styles)
}

// clearPendingConfirmations resets all two-press confirmation states.
// Call this on any view switch or action that should cancel pending confirmations.
func (m *Model) clearPendingConfirmations() {
	m.undo.clearQueuePending = false
	m.undo.bulkRemovePending = false
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
	if id == viewFavorites && m.favoriteReader != nil {
		a, al, t, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.artistCount = a
			m.favorites.albumCount = al
			m.favorites.trackCount = t
		}
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
	// Favorites actions (root_favorites.go).
	if model, cmd, ok := m.handleFavoriteKeyAction(action); ok {
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
		m.helpSearchFilter = filter.State{}
		m.helpViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(m.termHeight-2))
		m.refreshHelpViewport()
	case actionNextView:
		m = m.switchToView(nextView(m.activeView))
	case actionPrevView:
		m = m.switchToView(prevView(m.activeView))
	case actionQuit:
		m.overlay.nowPlaying = nil
		m.overlay.trackInfo = nil
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
	case actionSwitchFavorites:
		m = m.switchToView(viewFavorites)
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
		case viewFavorites:
			m.favorites = m.favorites.HandleAction(action)
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
		} else if m.activeView == viewFavorites {
			if m.favorites.level != favtui.CategoryLevel {
				m.favorites = m.favorites.toggleSelection(m.favorites.cursor)
			}
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
		case viewFavorites:
			m.favorites = m.favorites.clearSelection()
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
