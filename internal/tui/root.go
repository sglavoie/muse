package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/layout"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// overlayInfo groups all overlay and modal state.
type overlayInfo struct {
	preview               *PreviewOverlay
	previewPlayer         player.Previewer
	previewWasPlaying     bool
	previewDurationFilled bool
	nowPlaying            *NowPlayingOverlay
	trackInfo             *TrackInfoOverlay
	metadataEditor        *MetadataEditor
	destPicker            *pltui.DestinationPickerModal
	destPickerTracks      []library.Track
	goTo                  *GoToModal
	settings              *SettingsOverlay
	smartRuleEditor       *SmartRuleEditor
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
	state        PlaybackState
	repeatMode   RepeatMode
	shuffle      bool
	volume       int
	currentHash  string
	currentIndex     int // index into queue entries; -1 = nothing playing
	elapsedSecs      float64
	totalSecs        float64
	stopAfterCurrent bool // true when "stop after current track" is armed
}

// Model is the root Bubble Tea v2 model. It manages view switching between
// Library, Playlists, Queue, and Artists panels, and renders a persistent status bar.
type Model struct {
	appCtx         context.Context // application-level context for subprocess lifecycle
	activeView     viewID
	cfg            config.Config
	configPath     string
	configCreated  bool
	configWarnings []string

	library       LibraryView
	recentlyAdded LibraryView
	playlists     PlaylistsView
	queue         QueueView
	artists       ArtistsView
	albums        AlbumsView
	favorites     FavoritesView
	genres        GenresView

	// scanCh is the channel returned by library.Scan. It is nil when no scan
	// is in progress or after the scan has completed.
	scanCh <-chan library.Track
	// scanErrCh carries non-fatal walk errors from the initial diff scan.
	// It is already closed when Init() runs; drained once on startup.
	scanErrCh <-chan library.ScanErrorMsg
	scanning  bool
	scanned   int
	scanTotal int // estimated total from the parallel counting goroutine

	// userEditedReader queries the DB for user-edited tracks so that
	// rescan and watcher flows can preserve edited metadata in-memory.
	userEditedReader userEditedReader
	// userEditedOverlay caches user-edited metadata keyed by hash.
	// Built at startup and before rescan; consulted when inserting
	// scanned/watched tracks into the in-memory index.
	userEditedOverlay map[string]db.UserEditedRow
	// rescanSnapshot caches created_at and duration for every track in the
	// library index just before a full rescan clears it. Consulted by
	// preserveTrackMetadata so in-memory values survive the rescan.
	rescanSnapshot map[string]rescanEntry

	// watchCh is the channel returned by library.Watch. It emits WatchEvents
	// (track additions/updates and deletion notifications) and is nil when
	// watching is disabled.
	watchCh <-chan library.WatchEvent

	// watchDegraded is true when the filesystem watcher could not be started
	// (e.g. fd exhaustion). The status bar shows a notice so the user knows
	// to use manual rescan.
	watchDegraded bool

	// libraryRoots and extractor are required for manual rescan. When
	// libraryRoots is empty the rescan hotkey is a no-op.
	libraryRoots []string
	extractor    library.MetadataExtractor

	statusBar        StatusBar
	termWidth        int
	termHeight       int
	registry         Registry
	helpVisible      bool           // true while the help overlay is shown
	helpViewport     viewport.Model // scrollable viewport for the help overlay
	helpFiltered     bool           // true when showing only current-view keybindings
	helpSearchFilter filter.State   // search filter state for the help overlay
	prevActiveView   viewID         // the view that was active before help was opened

	debugVisible  bool           // true while the debug panel is shown
	debugViewport viewport.Model // scrollable viewport for the debug panel
	debugPrevView viewID         // the view that was active before debug was opened

	// player is the playback backend. It may be nil when playback is not
	// available (e.g. in tests that do not exercise playback).
	player              playPauser
	queueWriter         libtui.QueueAppender   // may be nil; set via WithQueueWriter
	reorderer           queueReorderer         // may be nil; set via WithQueueReorderer
	undoer              queueDequeuer          // may be nil; set via WithQueueDequeuer
	sessionWriter       sessionWriter          // may be nil; set via WithSessionWriter
	playlistReader      playlistTrackReader    // may be nil; set via WithPlaylistTrackReader
	playlistWriter      playlistCreator        // may be nil; set via WithPlaylistWriter
	playlistRenamer     playlistRenamer        // may be nil; set via WithPlaylistRenamer
	playlistDeleter     playlistDeleter        // may be nil; set via WithPlaylistDeleter
	playlistAdder       playlistTrackAdder     // may be nil; set via WithPlaylistAdder
	batchEnqueuer       batchEnqueuer          // may be nil; set via WithBatchEnqueuer
	favoriteReader      favoriteReader         // may be nil; set via WithFavoriteReader
	favoriteWriter      favoriteWriter         // may be nil; set via WithFavoriteWriter
	favoriteHashes      map[string]struct{}    // set of favorited track hashes for highlight
	favoriteArtists     map[string]struct{}    // set of favorited artist names for highlight
	favoriteAlbums      map[string]struct{}    // set of favorited album names for highlight
	smartEval           smartPlaylistEvaluator // may be nil; set via WithSmartPlaylistEvaluator
	smartPlaylistWriter smartPlaylistWriter    // may be nil; set via WithSmartPlaylistWriter
	playCountWriter     playCountIncrementer   // may be nil; set via WithPlayCountWriter
	historyRecorder     playHistoryRecorder    // may be nil; set via WithHistoryRecorder
	statsReader         listeningStatsReader   // may be nil; set via WithStatsReader

	statsVisible  bool           // true while the listening stats overlay is shown
	statsViewport viewport.Model // scrollable viewport for the stats overlay
	statsPrevView viewID         // the view that was active before stats was opened
	statsPeriod   int            // index into statsPeriods slice

	overlay         overlayInfo       // all overlay and modal state
	metadataWriter  metadataUpdater   // may be nil; set via WithMetadataWriter
	durationWriter  durationUpdater   // may be nil; set via WithDurationWriter
	scanWriter      scanUpsertBatcher // may be nil; set via WithScanWriter
	upsertBatchSize int               // batch size for scan DB writes; set via WithScanWriter

	undo       undoInfo      // reversible undo slot and pending confirmation flags
	yankBuffer []QueueEntry // clipboard for queue yank/paste; persists until next yank

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
//	    tui.WithLibraryRoots(roots),
//	    tui.WithPlayer(mgr),
//	)
func New(ctx context.Context, opts ...Option) Model {
	initialStyles := styles.NewStyles(true)
	m := Model{
		appCtx:        ctx,
		activeView:    viewLibrary,
		cfg:           config.Defaults(),
		library:       LibraryView{fuzzy: library.SahilmFuzzySearcher{}},
		playback:      playbackInfo{currentIndex: -1},
		recentlyAdded: newRecentlyAddedView(),
		registry:      newRegistry(),
		styles:        &initialStyles,
	}
	for _, opt := range opts {
		opt(&m)
	}
	// Derive scanning state from scanCh after options are applied.
	m.scanning = m.scanCh != nil
	// Pre-load the set of favorited track hashes for row highlighting.
	m.favoriteHashes = loadFavoriteTrackHashes(m.favoriteReader)
	m.favoriteArtists = loadFavoriteArtists(m.favoriteReader)
	m.favoriteAlbums = loadFavoriteAlbums(m.favoriteReader)
	m = m.syncRecentlyAddedFromLibrary()
	// Sync statusBar fields from model state.
	m.statusBar.configPath = m.configPath
	m.statusBar.configCreated = m.configCreated
	if len(m.configWarnings) > 0 {
		m.statusBar.notification = newNotificationFor(
			"Config: "+strings.Join(m.configWarnings, "; "),
			10*time.Second,
		)
	}
	return m
}

// Volume returns the current volume level as an integer percentage.
func (m Model) Volume() int { return m.playback.volume }

// PlaybackCurrentIndex returns the current queue index for the playing track.
func (m Model) PlaybackCurrentIndex() int { return m.playback.currentIndex }

// StatusBarArtist returns the current status bar artist string.
func (m Model) StatusBarArtist() string { return m.statusBar.artist }

// StatusBarTitle returns the current status bar title string.
func (m Model) StatusBarTitle() string { return m.statusBar.title }

// StatusBarPlaylistSource returns the current status bar playlist source string.
func (m Model) StatusBarPlaylistSource() string { return m.statusBar.playlistSource }

// RemoveLibraryTrackByPath removes a track from the in-memory library index
// by its file path. Call this before starting the TUI to evict deleted files
// discovered during the diff scan setup phase.
func (m *Model) RemoveLibraryTrackByPath(path string) {
	m.library.index.RemoveByPath(path)
	updated := m.syncRecentlyAddedFromLibrary()
	m.recentlyAdded = updated.recentlyAdded
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

// schedulePurge sets up a deferred purge of soft-deleted queue entries.
// It closes any existing cancel channel on the slot, creates a new one,
// and schedules purgeFn to run after undoWindowDuration.
func schedulePurge(slot *undoSlot, purgeFn func(int64) error) {
	if slot.cancelPurge != nil {
		close(slot.cancelPurge)
	}
	cancel := make(chan struct{})
	slot.cancelPurge = cancel
	softDeleteTime := time.Now().Unix()
	time.AfterFunc(undoWindowDuration, func() {
		select {
		case <-cancel:
			return
		default:
			if err := purgeFn(softDeleteTime + 1); err != nil {
				slog.Warn("purge failed", "err", err)
			}
		}
	})
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
		_ = sw.UpdateSessionPosition(hash, positionMs) // periodic persistence, next tick will retry
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
func countAudioFiles(ctx context.Context, roots []string, excludedPaths []string) tea.Cmd {
	return func() tea.Msg {
		return library.ScanTotalMsg{Total: library.CountAudioFiles(ctx, roots, excludedPaths)}
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

// waitForScanErr returns a Cmd that reads a single ScanErrorMsg from ch and
// dispatches it. The channel is already closed when Init runs so this Cmd
// terminates quickly; it is re-scheduled only while items remain.
func waitForScanErr(ch <-chan library.ScanErrorMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil // channel exhausted
		}
		return msg
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
		if len(m.libraryRoots) > 0 {
			cmds = append(cmds, countAudioFiles(m.appCtx, m.libraryRoots, m.cfg.ExcludedPaths))
		}
	}
	if m.scanErrCh != nil {
		cmds = append(cmds, waitForScanErr(m.scanErrCh))
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
	if cmd := evalSmartPlaylistCountsCmd(m.playlists.playlists, m.smartEval); cmd != nil {
		cmds = append(cmds, cmd)
	}
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

// resizable is implemented by every primary view that tracks its own
// terminal dimensions.
type resizable interface {
	setSize(w, h int)
}

// setViewDimensions updates the width and height on all primary views.
// height is reduced by 3 to leave room for the status bar and header rows.
func (m *Model) setViewDimensions(width, height int) {
	viewHeight := max(height-3, 0)
	for _, v := range []resizable{&m.library, &m.recentlyAdded, &m.playlists, &m.queue, &m.artists, &m.albums, &m.favorites, &m.genres} {
		v.setSize(width, viewHeight)
	}
}

// clampAllScrollOffsets adjusts the scroll offset for every view so that the
// cursor remains visible after a terminal resize. Without this, shrinking the
// terminal can leave the cursor below the visible viewport until the user
// presses a cursor key.
func (m *Model) clampAllScrollOffsets() {
	// Library: reserve 1 line for column header.
	libMaxRows := layout.VisibleRows(m.library.height, 1)
	m.library.scrollOffset = adjustScroll(m.library.cursor, m.library.scrollOffset, libMaxRows)

	// Queue: reserve 2 lines for column header + bottom margin.
	queueMaxRows := m.queue.visibleRows()
	m.queue.scrollOffset = adjustScroll(m.queue.cursor, m.queue.scrollOffset, queueMaxRows)

	// Playlists: reserve depends on level.
	plReserve := 1
	if m.playlists.level == pltui.TrackLevel {
		plReserve = 4
	}
	plMaxRows := layout.VisibleRows(m.playlists.height, plReserve)
	m.playlists.scrollOffset = adjustScroll(m.playlists.cursor, m.playlists.scrollOffset, plMaxRows)

	// Artists: reserve depends on level and filter/sort state.
	artReserve := artistsVisibleReserve(m.artists)
	artMaxRows := layout.VisibleRows(m.artists.height, artReserve)
	m.artists.scrollOffset = adjustScroll(m.artists.cursor, m.artists.scrollOffset, artMaxRows)

	// Albums: reserve depends on level and filter/sort state.
	albReserve := albumsVisibleReserve(m.albums)
	albMaxRows := layout.VisibleRows(m.albums.height, albReserve)
	m.albums.scrollOffset = adjustScroll(m.albums.cursor, m.albums.scrollOffset, albMaxRows)

	// Favorites: reserve depends on level, category, filter, and sort state.
	favReserve := favoritesVisibleReserve(m.favorites)
	favMaxRows := layout.VisibleRows(m.favorites.height, favReserve)
	m.favorites.scrollOffset = adjustScroll(m.favorites.cursor, m.favorites.scrollOffset, favMaxRows)

	// Genres: reserve depends on level and filter/sort state.
	genReserve := genresVisibleReserve(m.genres)
	genMaxRows := layout.VisibleRows(m.genres.height, genReserve)
	m.genres.scrollOffset = adjustScroll(m.genres.cursor, m.genres.scrollOffset, genMaxRows)
}

// Update handles incoming messages. Number keys 1/2/3/4 switch the active view;
// Ctrl+c quits. q and Esc are intentionally ignored here — they are reserved
// for in-view navigation.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.handleWindowSizeMsg(msg)

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

	case library.ScanErrorMsg:
		m.statusBar.notification = newNotification(fmt.Sprintf("Scan error: %v", msg.Err))
		if m.scanErrCh != nil {
			return m, waitForScanErr(m.scanErrCh)
		}

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
		var cmd tea.Cmd
		m, cmd = m.handlePlaybackEndedMsg()
		if m.playback.state == PlaybackPlaying {
			return m, tea.Batch(cmd, m.sessionPositionCmd())
		}
		if cmd != nil {
			return m, cmd
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
			m.statusBar.notification = newNotification("Track info failed: " + msg.err.Error())
		} else {
			overlay := newTrackInfoOverlay(msg.path, msg.tags, msg.playCount)
			m.overlay.trackInfo = &overlay
		}

	case TrackChangedMsg:
		return m.handleTrackChangedMsg(msg)

	// --- Filter messages (root_filter.go) ---

	case FilterBarMsg:
		return m.handleFilterBarMsg()

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

	case QueuePastedMsg:
		m = m.handleQueuePastedMsg(msg)

	case TransientNotifyMsg:
		m.statusBar.notification = newNotification(string(msg))

	// --- Key handling ---

	case settingsSavedMsg:
		m = m.handleSettingsSavedMsg(msg)

	case tea.KeyPressMsg:
		return m.handleKeyPressMsg(msg)

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

	case smartPlaylistCreatedMsg:
		return m.handleSmartPlaylistCreatedMsg(msg)

	case smartPlaylistEditedMsg:
		return m.handleSmartPlaylistEditedMsg(msg)

	case smartPlaylistCountsUpdatedMsg:
		m = m.handleSmartPlaylistCountsUpdatedMsg(msg)

	case listeningStatsMsg:
		panel := StatsPanel{
			stats:       msg.stats,
			periodLabel: statsPeriods[m.statsPeriod].label,
			err:         msg.err,
		}
		m.statsViewport.SetContent(panel.render(*m.styles))
		m.statsViewport.SetYOffset(0)

	case favoriteToggledMsg:
		m = m.handleFavoriteToggledMsg(msg)

	case favoritesLoadedMsg:
		m = m.handleFavoritesLoadedMsg(msg)

	case metadataEditedMsg:
		// No-op: in-memory state was already updated synchronously in handleMetadataSave.

	case GoToDismissedMsg:
		m.overlay.goTo = nil

	case GoToPickedMsg:
		m.overlay.goTo = nil
		switch msg.Kind {
		case goToArtist:
			m = m.jumpToArtist(msg.Value)
		case goToAlbum:
			m = m.jumpToAlbum(msg.Value)
		case goToGenre:
			m = m.jumpToGenre(msg.Value)
		case goToPlaylist:
			m = m.jumpToPlaylist(msg.Value)
		}

	case pltui.DestinationPickerDismissedMsg:
		m = m.handleDestinationPickerDismissedMsg()

	case pltui.DestinationPickerCreateMsg:
		m = m.handleDestinationPickerCreateMsg()

	case pltui.DestinationPickedMsg:
		return m.handleDestinationPickedMsg(msg)

	default:
		return m.handleUnhandledMsg(msg)
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
	if id == viewGenres && len(m.genres.genres) == 0 {
		m.genres.genres = gentui.DeriveGenres(m.library.index.Tracks())
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
	// Playlist actions (root_playlists.go).
	if model, cmd, ok := m.handlePlaylistKeyAction(action); ok {
		return model, cmd
	}
	// Navigation actions (root_navigation.go).
	if model, cmd, ok := m.handleNavigationAction(action); ok {
		return model, cmd
	}

	// View-switching and remaining actions.
	switch action {
	case actionSettings:
		return m.handleSettingsOpen()
	case actionDebug:
		m.debugPrevView = m.activeView
		m.debugVisible = true
		m.debugViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(max(m.termHeight-2, 0)))
		panel := collectDebugInfo(m)
		m.debugViewport.SetContent(panel.render(*m.styles))
		m.debugViewport.SetYOffset(0)
	case actionStats:
		m.statsPrevView = m.activeView
		m.statsVisible = true
		m.statsPeriod = 0
		m.statsViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(max(m.termHeight-3, 0)))
		m.statsViewport.SetYOffset(0)
		return m, m.loadStatsCmd()
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
	case actionSwitchRecentlyAdded:
		m = m.switchToView(viewRecentlyAdded)
	case actionSwitchGenres:
		m = m.switchToView(viewGenres)
	case actionGoTo:
		ctx := m.focusedGoToContext()
		if ctx.found {
			options := m.buildGoToOptions(ctx)
			if len(options) > 0 {
				modal := NewGoToModal(options)
				m.overlay.goTo = &modal
			}
		}
	}
	return m, nil
}
