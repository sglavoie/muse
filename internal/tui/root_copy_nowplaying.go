package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/desktop"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/statusbar"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (m Model) copyNowPlayingCommand() palette.PaletteCommand {
	label := m.nowPlayingCopyText()
	enabled := label != "" && (m.playback.state == playback.StatePlaying || m.playback.state == playback.StatePaused)
	return palette.PaletteCommand{
		ID: "playback.copy.info", Name: "Copy now-playing info", Category: "file",
		Detail: label, Enabled: func() bool { return enabled }, DisabledReason: "Play a track first",
		Exec: func() tea.Cmd {
			if !enabled {
				return func() tea.Msg { return statusbar.TransientNotifyMsg("No track playing") }
			}
			return desktop.CopyToClipboard(m.appCtx, label)
		},
	}
}

func (m Model) nowPlayingCopyText() string {
	if m.playback.currentHash == "" {
		return ""
	}
	if track, ok := m.library.Index.FindByHash(m.playback.currentHash); ok {
		return text.TrackDisplay(track.Artist, track.Title, track.Path)
	}
	return text.TrackDisplay(m.statusBar.Artist, m.statusBar.Title, "")
}
