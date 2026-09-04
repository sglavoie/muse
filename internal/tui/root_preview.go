package tui

import (
	"log/slog"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/overlay"
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
		m = m.closePreview()
		m.overlay.previewWasPlaying = false
		return m, tea.Quit, true
	case "esc", "P":
		m, cmd := m.dismissPreview()
		return m, cmd, true
	case "space":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.TogglePause(); err != nil {
				slog.Warn("preview pause toggle failed", "err", err)
				m.statusBar.Notify("Preview pause failed: " + err.Error())
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
	switch m.ActiveView {
	case viewLibrary:
		return m.library.Panel().FocusedTrack()
	case viewQueue:
		filtered := m.queue.FilteredEntries()
		if m.queue.Cursor >= 0 && m.queue.Cursor < len(filtered) {
			e := filtered[m.queue.Cursor]
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
			if t, ok := m.library.Index.FindByHash(e.Hash); ok {
				track.PlayCount = t.PlayCount
			}
			return track, true
		}
	case viewPlaylists:
		track, ok := m.playlists.FocusedTrack()
		if ok && track.Hash != "" {
			if t, found := m.library.Index.FindByHash(track.Hash); found {
				track.PlayCount = t.PlayCount
			}
		}
		return track, ok
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return m.browse.Artists.FocusedTrack()
		case browse.BrowseModeAlbums:
			return m.browse.Albums.FocusedTrack()
		case browse.BrowseModeGenres:
			return m.browse.Genres.FocusedTrack()
		}
	case viewFavorites:
		return m.favorites.FocusedTrack()
	case viewTags:
		track, ok := m.tagsView.FocusedTrackAsLibTrack()
		if ok && track.Hash != "" {
			if t, found := m.library.Index.FindByHash(track.Hash); found {
				track.PlayCount = t.PlayCount
				track.Genre = t.Genre
			}
		}
		return track, ok
	case viewMixes:
		if t := m.mixes.FocusedTrack(); t != nil {
			return *t, true
		}
	case viewInsights:
		if t := m.insights.FocusedTrack(); t != nil {
			return *t, true
		}
	}
	return library.Track{}, false
}

// startPreviewCmd returns a tea.Cmd that launches a PreviewPlayer, loads the
// given track, and returns a previewStartedMsg on success.
func (m Model) startPreviewCmd(track library.Track, request uint64) tea.Cmd {
	ctx := m.appCtx
	runtimeDir := m.runtimeDir
	tagsLine := m.buildTagsLine(track.Hash)
	normalize := m.cfg.NormalizeVolume
	return func() tea.Msg {
		socketPath := filepath.Join(runtimeDir, "muse-preview.sock")
		pp := player.NewPreviewPlayer(ctx, socketPath)
		if err := pp.Start(); err != nil {
			return previewStartErrorMsg{err: err, request: request}
		}
		if normalize {
			if err := pp.SetAudioFilter("lavfi=[loudnorm]"); err != nil {
				slog.Warn("preview: set normalize", "err", err)
			}
		}
		if err := pp.LoadAndPlay(track.Path); err != nil {
			_ = pp.Close()
			return previewStartErrorMsg{err: err, request: request}
		}
		return previewStartedMsg{
			player:  pp,
			request: request,
			overlay: overlay.PreviewOverlay{
				Hash:      track.Hash,
				Artist:    track.Artist,
				Title:     track.Title,
				Album:     track.Album,
				PlayCount: track.PlayCount,
				TagsLine:  tagsLine,
			},
		}
	}
}

// pauseForPreviewCmd orders the main-player pause after every request already
// dispatched by the Update loop. The preview itself is started only when the
// result is handled, after this command confirms the pause took effect.
func (m *Model) pauseForPreviewCmd(track library.Track) tea.Cmd {
	gate := m.playback.requests
	req := gate.dispatchControl(m.playback.requestSeq)
	m.playback.requestSeq = req
	p := m.player
	return func() tea.Msg {
		err, ran := gate.run(req, p.Pause)
		return previewPauseResultMsg{request: req, track: track, err: err, ran: ran}
	}
}

// resumeAfterPreview queues main playback behind every earlier selection and
// transport operation. The optimistic state keeps dismissal behavior
// responsive; the ordinary control result restores it if Play fails.
func (m Model) resumeAfterPreview() (Model, tea.Cmd) {
	wasPlaying := m.overlay.previewWasPlaying
	m.overlay.previewWasPlaying = false
	if !wasPlaying || m.player == nil {
		return m, nil
	}
	before := m.playback.state
	m.playback.state = playback.StatePlaying
	cmd := m.playbackControlCmd(
		playbackControlPlay,
		m.player.Play,
		before,
		m.playback.state,
	)
	return m, cmd
}

func (m Model) closePreview() Model {
	if m.overlay.previewPlayer != nil {
		_ = m.overlay.previewPlayer.Close()
	}
	m.overlay.preview = nil
	m.overlay.previewPlayer = nil
	m.overlay.previewStarting = false
	m.overlay.previewDurationFilled = false
	return m
}

// dismissPreview stops the preview player, clears the overlay, and resumes
// main playback if it was playing before the preview started.
func (m Model) dismissPreview() (Model, tea.Cmd) {
	m = m.closePreview()
	return m.resumeAfterPreview()
}
