package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/desktop"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
)

// handleTrackInfoKeyPress handles key events while the track info overlay is
// active. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleTrackInfoKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.trackInfo == nil {
		return m, nil, false
	}
	if next, cmd, ok := m.handleInfoTransport(msg); ok {
		return next, cmd, true
	}
	switch msg.String() {
	case "ctrl+c":
		m.overlay.trackInfo = nil
		return m.markQuitting(), tea.Quit, true
	case "esc", "i":
		m.overlay.trackInfo = nil
		return m, nil, true
	case "y":
		if m.overlay.trackInfo.Path == "" {
			return m, nil, true
		}
		if m.fixtureSafeCommands {
			m.statusBar.Notify("Clipboard copy blocked in fixture mode")
			return m, nil, true
		}
		return m, desktop.CopyToClipboard(m.appCtx, m.overlay.trackInfo.Path), true
	default:
		next := *m.overlay.trackInfo
		next.Scroll(msg.String(), m.termWidth, max(m.termHeight-3, 1))
		m.overlay.trackInfo = &next
		return m, nil, true
	}
}

// Open from the index before probing the file. A request identity prevents a
// late probe from reopening a dismissed panel or replacing a newer inspection.
func (m Model) openTrackInfo(track library.Track) (tea.Model, tea.Cmd, bool) {
	if indexed, ok := m.library.Index.FindByHash(track.Hash); ok {
		track = indexed
	}
	var tags []db.TagRow
	if m.tagReader != nil && track.Hash != "" {
		tags, _ = m.tagReader.ReadTagsForTrack(track.Hash)
	}
	m.overlay.trackInfoRequest++
	request := m.overlay.trackInfoRequest
	status := "Loading file details…"
	if track.Path == "" {
		status = "File details unavailable: track path unknown"
	} else if m.fixtureSafeCommands {
		status = "File details unavailable in fixture mode"
	}
	info := overlay.NewLibraryTrackInfoOverlay(track, tags, request, status)
	m.overlay.trackInfo = &info
	if track.Path == "" || m.fixtureSafeCommands {
		return m, nil, true
	}
	return m, overlay.TrackInfoCmd(m.appCtx, track.Path, request), true
}

// Missing queue files still have useful metadata and a path worth inspecting.
func (m Model) focusedTrackForInspection() (library.Track, bool) {
	if m.ActiveView != viewQueue {
		return m.focusedTrackForPreview()
	}
	entries := m.queue.FilteredEntries()
	if m.queue.Cursor < 0 || m.queue.Cursor >= len(entries) {
		return library.Track{}, false
	}
	e := entries[m.queue.Cursor]
	return library.Track{Hash: e.Hash, Path: e.Path, Title: e.Title, Artist: e.Artist,
		Album: e.Album, AlbumArtist: e.AlbumArtist, Duration: float64(e.DurationMs) / 1000}, true
}
