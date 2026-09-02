package tui

import (
	"log/slog"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// handleNowPlayingKeyPress handles key events while the now-playing overlay is
// active. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleNowPlayingKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.nowPlaying == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		m.overlay.nowPlaying = nil
		return m, tea.Quit, true
	case "esc", "I":
		m.overlay.nowPlaying = nil
		return m, nil, true
	default:
		return m, nil, true
	}
}

// handlePreviewKeyPress handles key events while the preview overlay is active.
// Returns (model, cmd, true) when the key was consumed.
func (m Model) handlePreviewKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.preview == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		m = m.dismissPreview()
		return m, tea.Quit, true
	case "esc", "P":
		m = m.dismissPreview()
		return m, nil, true
	case "space":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.TogglePause(); err != nil {
				slog.Warn("preview pause toggle failed", "err", err)
				m.statusBar.notify("Preview pause failed: " + err.Error())
			}
		}
		return m, nil, true
	case "left":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.SeekRelative(-m.cfg.SeekOffsetShort); err != nil {
				slog.Warn("preview seek failed", "offset", -m.cfg.SeekOffsetShort, "err", err)
			}
		}
		return m, nil, true
	case "right":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.SeekRelative(m.cfg.SeekOffsetShort); err != nil {
				slog.Warn("preview seek failed", "offset", m.cfg.SeekOffsetShort, "err", err)
			}
		}
		return m, nil, true
	case "shift+left":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.SeekRelative(-m.cfg.SeekOffsetLong); err != nil {
				slog.Warn("preview seek failed", "offset", -m.cfg.SeekOffsetLong, "err", err)
			}
		}
		return m, nil, true
	case "shift+right":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.SeekRelative(m.cfg.SeekOffsetLong); err != nil {
				slog.Warn("preview seek failed", "offset", m.cfg.SeekOffsetLong, "err", err)
			}
		}
		return m, nil, true
	default:
		return m, nil, true
	}
}

// focusedTrackForPreview returns the track under the cursor in whichever view
// is active, or false when no track is focused.
func (m Model) focusedTrackForPreview() (library.Track, bool) {
	switch m.activeView {
	case viewLibrary:
		return m.library.view().FocusedTrack()
	case viewQueue:
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			e := filtered[m.queue.cursor]
			if e.Missing {
				return library.Track{}, false
			}
			track := library.Track{
				Hash:     e.Hash,
				Path:     e.Path,
				Artist:   e.Artist,
				Title:    e.Title,
				Album:    e.Album,
				Duration: float64(e.DurationMs) / 1000,
			}
			if t, ok := m.library.index.FindByHash(e.Hash); ok {
				track.PlayCount = t.PlayCount
			}
			return track, true
		}
	case viewPlaylists:
		track, ok := m.playlists.focusedTrack()
		if ok && track.Hash != "" {
			if t, found := m.library.index.FindByHash(track.Hash); found {
				track.PlayCount = t.PlayCount
			}
		}
		return track, ok
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			return m.browse.artists.focusedTrack()
		case BrowseModeAlbums:
			return m.browse.albums.focusedTrack()
		case BrowseModeGenres:
			return m.browse.genres.focusedTrack()
		}
	case viewFavorites:
		return m.favorites.focusedTrack()
	case viewTags:
		track, ok := m.tagsView.focusedTrackAsLibTrack()
		if ok && track.Hash != "" {
			if t, found := m.library.index.FindByHash(track.Hash); found {
				track.PlayCount = t.PlayCount
				track.Genre = t.Genre
			}
		}
		return track, ok
	case viewMixes:
		if t := m.mixes.focusedTrack(); t != nil {
			return *t, true
		}
	case viewInsights:
		if t := m.insights.focusedTrack(); t != nil {
			return *t, true
		}
	}
	return library.Track{}, false
}

// startPreviewCmd returns a tea.Cmd that launches a PreviewPlayer, loads the
// given track, and returns a previewStartedMsg on success.
func (m Model) startPreviewCmd(track library.Track) tea.Cmd {
	ctx := m.appCtx
	runtimeDir := m.runtimeDir
	tagsLine := m.buildTagsLine(track.Hash)
	normalize := m.cfg.NormalizeVolume
	return func() tea.Msg {
		socketPath := filepath.Join(runtimeDir, "muse-preview.sock")
		pp := player.NewPreviewPlayer(ctx, socketPath)
		if err := pp.Start(); err != nil {
			return previewStartErrorMsg{err: err}
		}
		if normalize {
			if err := pp.SetAudioFilter("lavfi=[loudnorm]"); err != nil {
				slog.Warn("preview: set normalize", "err", err)
			}
		}
		if err := pp.LoadAndPlay(track.Path); err != nil {
			_ = pp.Close()
			return previewStartErrorMsg{err: err}
		}
		return previewStartedMsg{
			player: pp,
			overlay: PreviewOverlay{
				hash:      track.Hash,
				artist:    track.Artist,
				title:     track.Title,
				album:     track.Album,
				playCount: track.PlayCount,
				tagsLine:  tagsLine,
			},
		}
	}
}

// dismissPreview stops the preview player, clears the overlay, and resumes
// main playback if it was playing before the preview started.
func (m Model) dismissPreview() Model {
	if m.overlay.previewPlayer != nil {
		_ = m.overlay.previewPlayer.Close()
	}
	m.overlay.preview = nil
	m.overlay.previewPlayer = nil
	m.overlay.previewDurationFilled = false
	if m.overlay.previewWasPlaying && m.player != nil {
		if err := m.player.Play(); err != nil {
			slog.Warn("player resume after preview failed", "err", err)
		} else {
			m.playback.state = playback.StatePlaying
		}
	}
	m.overlay.previewWasPlaying = false
	return m
}
