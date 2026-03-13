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

// syncPositionMsg is dispatched every positionSyncInterval to trigger a
// playback-position update on the player.
type syncPositionMsg struct{}

// clearConfigNotice is sent after the transient first-run notification timeout.
type clearConfigNotice struct{}

// clearPlayerStatus is sent after the transient player status message timeout
// to automatically dismiss messages like "Connecting to mpv…".
// gen is a generation counter: only the most recently scheduled clear fires.
type clearPlayerStatus struct{ gen int }

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

	statusBar StatusBar
	termWidth int

	// player is the playback backend. It may be nil when playback is not
	// available (e.g. in tests that do not exercise playback).
	player        playPauser
	playbackState PlaybackState // current play/pause/stopped/idle state
	repeat        bool          // repeat mode enabled
	shuffle       bool          // shuffle mode enabled

	// statusBarGen is incremented each time a transient right message is set.
	// clearPlayerStatus messages carry the generation at which they were
	// scheduled; only a matching generation actually clears the field,
	// preventing an earlier timer from erasing a newer message.
	statusBarGen int
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
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}

// Update handles incoming messages. Number keys 1/2/3 switch the active view;
// Ctrl+c quits. q and Esc are intentionally ignored here — they are reserved
// for in-view navigation.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width

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
		m.statusBar.handleScanDone()

	case syncPositionMsg:
		if ps, ok := m.player.(positionSyncer); ok {
			_ = ps.SyncPosition()
		}
		return m, syncPositionCmd()

	case clearPlayerStatus:
		if msg.gen == m.statusBarGen {
			m.statusBar.transientRight = ""
		}

	case PlayerStatusMsg:
		m.statusBarGen++
		gen := m.statusBarGen
		m.statusBar.transientRight = msg.Text
		return m, tea.Tick(4*time.Second, func(time.Time) tea.Msg { return clearPlayerStatus{gen: gen} })

	case MissingTrackMsg:
		m.queue.markMissing(msg.Path)
		m.statusBar.handleMissingTrack(msg.Path)
		m.statusBarGen++
		gen := m.statusBarGen
		return m, tea.Tick(4*time.Second, func(time.Time) tea.Msg { return clearPlayerStatus{gen: gen} })

	case PlaybackEndedMsg:
		m.playbackState = PlaybackIdle

	case TrackChangedMsg:
		m.statusBar.artist = msg.Artist
		m.statusBar.title = msg.Title
		source := msg.PlaylistSource
		if source == "" {
			source = "Queue"
		}
		m.statusBar.playlistSource = source
		m.playbackState = PlaybackPlaying

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

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "up", "down", "enter":
			if m.activeView == viewQueue {
				var cmd tea.Cmd
				m.queue, cmd = m.queue.Update(msg)
				return m, cmd
			}
		case "space":
			if m.player != nil {
				if m.playbackState == PlaybackPlaying {
					_ = m.player.Pause()
					m.playbackState = PlaybackPaused
				} else {
					_ = m.player.Play()
					m.playbackState = PlaybackPlaying
				}
			}
		case "s":
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.Stop()
				m.playbackState = PlaybackStopped
			}
		case "n":
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.Next()
			}
		case "p":
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.Previous()
			}
		case "left":
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(-5)
			}
		case "right":
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(5)
			}
		case "shift+left":
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(-30)
			}
		case "shift+right":
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SeekRelative(30)
			}
		case "+":
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SetVolumeRelative(5)
			}
		case "-":
			if cp, ok := m.player.(commandPlayer); ok {
				_ = cp.SetVolumeRelative(-5)
			}
		case "1":
			m.activeView = viewLibrary
		case "2":
			m.activeView = viewPlaylists
		case "3":
			m.activeView = viewQueue
		case "r":
			if m.libraryRoot != "" && m.extractor != nil && !m.scanning {
				ch := library.Scan(m.libraryRoot, m.extractor)
				m.library.index = library.LibraryIndex{}
				m.scanCh = ch
				m.scanning = true
				m.scanned = 0
				m.scanTotal = 0
				return m, tea.Batch(waitForNextBatch(ch), countAudioFiles(m.libraryRoot))
			}
		}
	}
	return m, nil
}

// View renders the active panel and a status bar, returning a tea.View.
func (m Model) View() tea.View {
	var body string
	switch m.activeView {
	case viewLibrary:
		body = m.library.render()
	case viewPlaylists:
		body = m.playlists.render()
	case viewQueue:
		body = m.queue.render()
	}

	content := body + "\n\n" + m.renderStatusBar()
	return tea.NewView(content)
}

// renderStatusBar builds the status bar line shown at the bottom of the view.
// It syncs the model-owned playbackState, repeat, and shuffle into a local
// StatusBar copy so that the render logic has a complete picture.
func (m Model) renderStatusBar() string {
	sb := m.statusBar
	sb.playbackState = m.playbackState
	sb.repeat = m.repeat
	sb.shuffle = m.shuffle
	return sb.render(m.termWidth)
}
