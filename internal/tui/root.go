package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// clearConfigNotice is sent after the transient first-run notification timeout.
type clearConfigNotice struct{}

// scanBatchSize is the maximum number of tracks collected per batch before
// returning a ScanBatchMsg to the update loop.
const scanBatchSize = 100

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
	scanCh  <-chan library.Track
	scanning bool
	scanned  int

	// libraryRoot and extractor are required for manual rescan. When
	// libraryRoot is empty the rescan hotkey is a no-op.
	libraryRoot string
	extractor   library.MetadataExtractor

	statusBar StatusBar
}

// New creates a root Model. logPath is displayed in the status bar on every
// render. When configCreated is true, configPath is also shown as a transient
// first-run notification.
//
// scanCh may be nil when library scanning is not needed (e.g. in tests). Pass
// the channel returned by library.Scan to enable incremental library updates.
//
// libraryRoot and extractor enable the manual rescan hotkey. Pass empty string
// and nil to disable it (e.g. in tests that do not exercise rescan).
func New(logPath, configPath string, configCreated bool, scanCh <-chan library.Track, libraryRoot string, extractor library.MetadataExtractor) Model {
	return Model{
		activeView:    viewLibrary,
		logPath:       logPath,
		configPath:    configPath,
		configCreated: configCreated,
		scanCh:        scanCh,
		scanning:      scanCh != nil,
		libraryRoot:   libraryRoot,
		extractor:     extractor,
		statusBar: StatusBar{
			logPath:       logPath,
			configPath:    configPath,
			configCreated: configCreated,
		},
	}
}

// configNoticeDuration controls how long the first-run config path notification
// is visible in the status bar before it is automatically dismissed.
const configNoticeDuration = 5 * time.Second

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

// Init satisfies the tea.Model interface. It schedules the config-notice
// dismissal timer when this is a first-run session, and kicks off the first
// batch read from the scanner channel when one is present.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.configCreated {
		cmds = append(cmds, tea.Tick(configNoticeDuration, func(time.Time) tea.Msg {
			return clearConfigNotice{}
		}))
	}
	if m.scanCh != nil {
		cmds = append(cmds, waitForNextBatch(m.scanCh))
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
	case clearConfigNotice:
		m.configCreated = false

	case library.ScanProgressMsg:
		m.statusBar.handleScanProgress(msg)

	case library.ScanBatchMsg:
		for _, t := range msg.Tracks {
			m.library.index.Insert(t)
		}
		m.scanned += len(msg.Tracks)
		if m.scanCh != nil {
			return m, waitForNextBatch(m.scanCh)
		}

	case library.ScanDoneMsg:
		m.scanning = false
		m.scanCh = nil
		m.statusBar.handleScanDone()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
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
				return m, waitForNextBatch(ch)
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
func (m Model) renderStatusBar() string {
	return m.statusBar.render()
}
