package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
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
	appCtx         context.Context // application-level context for subprocess lifecycle
	activeView     viewID
	configPath     string
	configCreated  bool
	configWarnings []string

	library   LibraryView
	playlists PlaylistsView
	queue     QueueView
	artists   ArtistsView
	albums    AlbumsView
	favorites FavoritesView

	// scanCh is the channel returned by library.Scan. It is nil when no scan
	// is in progress or after the scan has completed.
	scanCh    <-chan library.Track
	// scanErrCh carries non-fatal walk errors from the initial diff scan.
	// It is already closed when Init() runs; drained once on startup.
	scanErrCh <-chan library.ScanErrorMsg
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
		if m.libraryRoot != "" {
			cmds = append(cmds, countAudioFiles(m.appCtx, m.libraryRoot))
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
	for _, v := range []resizable{&m.library, &m.playlists, &m.queue, &m.artists, &m.albums, &m.favorites} {
		v.setSize(width, viewHeight)
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
	case actionJumpToArtist:
		artist, _ := m.focusedTrackMeta()
		if artist != "" {
			m = m.jumpToArtist(artist)
		}
	case actionJumpToAlbum:
		_, album := m.focusedTrackMeta()
		if album != "" {
			m = m.jumpToAlbum(album)
		}
	}
	return m, nil
}
