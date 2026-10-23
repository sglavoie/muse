package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

func (m Model) handleWatchStatus(msg library.WatchStatusMsg) Model {
	// The background library pipeline reports watcher availability exactly
	// once. On failure the watcher never delivers live change events, so
	// mark the status bar degraded and tell the user to rely on manual
	// rescan. A setup error and a graceful fd-exhaustion fallback are
	// distinguished so the notification matches what actually happened.
	if !msg.Available {
		m.watchDegraded = true
		if msg.Err != nil {
			m.statusBar.NotifyError("File watcher failed to start; use manual rescan (ctrl+r)")
		} else {
			m.statusBar.NotifyError("File watcher unavailable; use manual rescan (ctrl+r)")
		}
	}

	return m
}

func (m Model) handleTrackInfoResult(msg overlay.TrackInfoMsg) Model {
	current := m.overlay.trackInfo
	if current == nil || msg.Request != current.Request || msg.Path != current.Path {
		return m
	}
	if msg.Err != nil {
		m.statusBar.NotifyError("File details unavailable: " + msg.Err.Error())
	}
	info := current.WithFileResult(msg)
	m.overlay.trackInfo = &info

	return m
}

func (m Model) handleGoToPicked(msg overlay.GoToPickedMsg) Model {
	m.overlay.goTo = nil
	switch msg.Kind {
	case overlay.GoToArtist:
		m = m.jumpToArtist(msg.Value)
	case overlay.GoToAlbum:
		m = m.jumpToAlbum(msg.Value)
	case overlay.GoToGenre:
		m = m.jumpToGenre(msg.Value)
	case overlay.GoToPlaylist:
		m = m.jumpToPlaylist(msg.Value)
	}

	return m
}

func (m Model) handlePalettePicked(msg palette.CommandPalettePickedMsg) (tea.Model, tea.Cmd) {
	m.overlay.commandPalette = nil
	// In a fixture-backed harness run, gate key/text-driven palette
	// selections through the same allowlist as direct command actions so a
	// scenario cannot execute a side-effectful command (Finder, clipboard,
	// open-directory) by opening the palette and pressing enter.
	if m.fixtureSafeCommands && !IsSafeDirectCommand(msg.Command.Key()) {
		name := msg.Command.Name
		return m, func() tea.Msg {
			return statusbar.TransientNotifyMsg(fmt.Sprintf("Command %q blocked in fixture mode", name))
		}
	}
	if msg.Command.Exec != nil {
		return m, msg.Command.Exec()
	}

	return m, nil
}
