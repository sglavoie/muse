package tui

import (
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// Capture the song shown in the palette detail, even if playback advances
// before the command is delivered.
type browseNowPlayingMsg struct{ hash string }

func (m Model) nowPlayingBrowseOptions(hash string) []overlay.GoToOption {
	track, ok := m.library.Index.FindByHash(hash)
	if !ok || hash == "" {
		return nil
	}
	// Library offers every destination; the focused Browse level must not
	// suppress destinations belonging to a different, currently playing song.
	contextModel := m
	contextModel.ActiveView = viewLibrary
	return contextModel.buildGoToOptions(goToContext{
		artist: track.Artist, album: track.Album, genre: track.Genre, found: true,
	})
}

func (m Model) browseNowPlayingCommand() palette.PaletteCommand {
	hash := m.playback.currentHash
	enabled := m.playback.state == playback.StatePlaying || m.playback.state == playback.StatePaused
	reason := "Play a track first"
	if enabled && len(m.nowPlayingBrowseOptions(hash)) == 0 {
		enabled = false
		reason = "Playing track has no library destinations"
	}
	command := paletteMessageCmd("playback.browse", "nav", "Browse now-playing…", "", browseNowPlayingMsg{hash})
	command.Detail = m.nowPlayingCopyText()
	command.Enabled = func() bool { return enabled }
	command.DisabledReason = reason
	return command
}

func (m Model) browseNowPlaying(hash string) Model {
	options := m.nowPlayingBrowseOptions(hash)
	if len(options) == 0 {
		m.statusBar.Notify("No library destinations for this track")
		return m
	}
	modal := overlay.NewGoToModal(options)
	m.overlay.goTo = &modal
	return m
}
