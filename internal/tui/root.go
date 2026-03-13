package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
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

// positionSyncer is implemented by player.Manager. Root uses it to
// periodically persist the current playback position for crash-resume.
type positionSyncer interface {
	SyncPosition() error
}

// repeatShuffler is an optional extension checked via type assertion. Root
// calls SetRepeat and SetShuffle after toggling the model booleans so that mpv
// is instructed to change its repeat/shuffle mode and the player state stays in
// sync with what the status bar displays.
type repeatShuffler interface {
	SetRepeat(bool) error
	SetShuffle(bool) error
}

// elapsedReader is implemented by player.Manager to provide the current
// elapsed playback position and total track duration. Root queries it on
// each 1-second tick to keep the status bar time display current.
type elapsedReader interface {
	ElapsedPosition() (elapsed, total float64, err error)
}

// tickMsg is dispatched every second by tickCmd. It drives elapsed-time
// display updates and transient-notification expiry checks.
type tickMsg struct{}

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

// PlaylistRenameMsg is dispatched when the user presses 'r' in the playlists
// view to rename the selected playlist.
type PlaylistRenameMsg struct{}

// PlaylistCreateMsg is dispatched when the user presses 'c' in the playlists
// view to create a new playlist.
type PlaylistCreateMsg struct{}

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
	player        playPauser
	playbackState PlaybackState // current play/pause/stopped/idle state
	repeat        bool          // repeat mode enabled
	shuffle       bool          // shuffle mode enabled

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

// configNoticeDuration controls how long the first-run config path notification
// is visible in the status bar before it is automatically dismissed.
const configNoticeDuration = 5 * time.Second

// positionSyncInterval is how often root polls the player for the current
// playback position so that crash-resume can restore to the right offset.
const positionSyncInterval = 5 * time.Second

// syncPositionCmd schedules one position-sync tick.
func syncPositionCmd() tea.Cmd {
	return tea.Tick(positionSyncInterval, func(time.Time) tea.Msg {
		return syncPositionMsg{}
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

	case PlayerStatusMsg:
		m.statusBar.notification = transientNotification{
			msg:       msg.Text,
			expiresAt: time.Now().Add(transientDuration),
		}

	case MissingTrackMsg:
		m.queue.markMissing(msg.Path)
		m.statusBar.handleMissingTrack(msg.Path)

	case PlaybackEndedMsg:
		m.playbackState = PlaybackIdle
		m.elapsedSecs = 0
		m.totalSecs = 0

	case TrackChangedMsg:
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
		m.queue.entries = nil
		m.queue.cursor = 0

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
			}
		case actionSeekForward:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(5)
			}
		case actionSeekBackLong:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(-30)
			}
		case actionSeekForwardLong:
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(30)
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
			m.repeat = !m.repeat
			if rs, ok := m.player.(repeatShuffler); ok {
				_ = rs.SetRepeat(m.repeat)
			}
		case actionShuffleToggle:
			m.shuffle = !m.shuffle
			if rs, ok := m.player.(repeatShuffler); ok {
				_ = rs.SetShuffle(m.shuffle)
			}
		case actionUndo:
			return m, func() tea.Msg { return UndoMsg{} }
		case actionFilterBar:
			return m, func() tea.Msg { return FilterBarMsg{} }
		case actionCursorUp, actionCursorDown:
			// Route cursor movement to the active view. Library cursor is
			// stored on LibraryView; queue cursor is stored on QueueView.
			if m.activeView == viewLibrary {
				m.library = m.library.HandleAction(a)
			} else {
				var cmd tea.Cmd
				m.queue, cmd = m.queue.HandleAction(a)
				return m, cmd
			}
		case actionSelect, actionDequeue, actionBulkSelectionToggle,
			actionClearQueue, actionMoveTrackUp, actionMoveTrackDown:
			// Queue-only actions — pass to QueueView regardless of active view.
			var cmd tea.Cmd
			m.queue, cmd = m.queue.HandleAction(a)
			return m, cmd
		case actionPlaylistRename:
			return m, func() tea.Msg { return PlaylistRenameMsg{} }
		case actionPlaylistCreate:
			return m, func() tea.Msg { return PlaylistCreateMsg{} }
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
	sb.repeat = m.repeat
	sb.shuffle = m.shuffle
	sb.configCreated = m.configCreated
	if m.elapsedSecs > 0 || m.totalSecs > 0 {
		sb.elapsed = formatTime(m.elapsedSecs)
		sb.total = formatTime(m.totalSecs)
	}
	return sb.render(m.termWidth)
}
