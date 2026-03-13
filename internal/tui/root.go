package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// clearConfigNotice is sent after the transient first-run notification timeout.
type clearConfigNotice struct{}

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
}

// New creates a root Model. logPath is displayed in the status bar on every
// render. When configCreated is true, configPath is also shown as a transient
// first-run notification.
func New(logPath, configPath string, configCreated bool) Model {
	return Model{
		activeView:    viewLibrary,
		logPath:       logPath,
		configPath:    configPath,
		configCreated: configCreated,
	}
}

// configNoticeDuration controls how long the first-run config path notification
// is visible in the status bar before it is automatically dismissed.
const configNoticeDuration = 5 * time.Second

// Init satisfies the tea.Model interface. When this is a first-run session,
// schedules a command to dismiss the transient config path notification after
// configNoticeDuration.
func (m Model) Init() tea.Cmd {
	if m.configCreated {
		return tea.Tick(configNoticeDuration, func(time.Time) tea.Msg {
			return clearConfigNotice{}
		})
	}
	return nil
}

// Update handles incoming messages. Number keys 1/2/3 switch the active view;
// Ctrl+c quits. q and Esc are intentionally ignored here — they are reserved
// for in-view navigation.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearConfigNotice:
		m.configCreated = false
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
	bar := fmt.Sprintf("log: %s", m.logPath)
	if m.configCreated {
		bar += fmt.Sprintf("  |  config created: %s", m.configPath)
	}
	return bar
}
