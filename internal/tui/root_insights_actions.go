package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// insightsAntiRutCount is how many unheard tracks the anti-rut shortcut pulls.
// Small enough to be a nudge out of a rut rather than a replacement queue.
const insightsAntiRutCount = 25

// handleInsightsOpen drills one level in: a briefing row opens its tracks, a
// Sessions row opens the sessions themselves, and a session opens its tracks.
// A track list has nothing deeper to open, so there enter enqueues the
// selection the way it does in Library — which is what the selection-mode
// footer advertises.
func (m Model) handleInsightsOpen() (Model, tea.Cmd) {
	if m.activeView != viewInsights {
		return m, nil
	}
	switch m.insights.level {
	case insightsLevelBriefing:
		row := m.insights.focusedRow()
		if row == nil || row.drill == drillNone {
			return m, nil
		}
		if row.drill.opensSessions() {
			sessions := insightsSessionsMatching(m.insights.sessions, row.drill, row.key)
			if len(sessions) == 0 {
				m.statusBar.Notify("No sessions behind this row")
				return m, nil
			}
			m.insights.openSessionList(row.label, sessions)
			return m, nil
		}
		tracks := m.insightsDrillTracks(*row)
		if len(tracks) == 0 {
			m.statusBar.Notify("Nothing behind this row")
			return m, nil
		}
		m.insights.openDrill(row.label, tracks)
	case insightsLevelSessions:
		tracks, label := m.insightsSessionActionTracks()
		if len(tracks) == 0 {
			m.statusBar.Notify("Nothing behind this session")
			return m, nil
		}
		m.insights.openDrill(label, tracks)
	case insightsLevelTracks, insightsLevelSessionTracks:
		return m.insightsEnqueueSelection()
	}
	return m, nil
}

// insightsEnqueueSelection appends the bulk-selected tracks — or the focused
// one when nothing is selected — to the queue, then drops the selection the way
// Library's enter does. A refused enqueue keeps the selection so the key can be
// pressed again once a queue is available.
func (m Model) insightsEnqueueSelection() (Model, tea.Cmd) {
	m, cmd := m.insightsEnqueue(m.insightsBulkTracks(), m.insights.drillLabel)
	if cmd == nil {
		return m, nil
	}
	m = exitVisualMode(m)
	m.insights.trackView.SelectedHashes = nil
	return m, cmd
}

// insightsBulkTracks returns what the selection-aware actions operate on at a
// drilled-in track level: the bulk-selected rows in display order, or the
// focused row when nothing is selected. A session's repeats are kept, since its
// track list is a replay rather than a set.
func (m Model) insightsBulkTracks() []library.Track {
	if !m.insights.atTracks() {
		return nil
	}
	order := m.insights.trackView.BaseTracks()
	if m.insights.trackView.SortedTracks != nil {
		order = m.insights.trackView.SortedTracks
	}
	if len(m.insights.trackView.SelectedHashes) > 0 {
		return collectSelectedTracks(order, m.insights.trackView.SelectedHashes)
	}
	if t := m.insights.focusedTrack(); t != nil {
		return []library.Track{*t}
	}
	return nil
}

// handleInsightsBack steps out one level.
func (m Model) handleInsightsBack() Model {
	if m.activeView != viewInsights || m.insights.level == insightsLevelBriefing {
		return m
	}
	// Stepping out replaces the list visual mode anchored into, so the range
	// has nothing left to grow over.
	m = exitVisualMode(m)
	m.insights.back()
	m.insights.clampCursor()
	return m
}

// insightsActionTracks returns the tracks the batch actions operate on: the
// drilled-in list at a track level, the focused session at the session level,
// or the focused row's set on the briefing. The label is what notifications and
// the playlist name use.
func (m Model) insightsActionTracks() ([]library.Track, string) {
	if m.insights.atTracks() {
		return m.insights.trackView.SourceTracks, m.insights.drillLabel
	}
	if m.insights.level == insightsLevelSessions {
		return m.insightsSessionActionTracks()
	}
	row := m.insights.focusedRow()
	if row == nil || row.drill == drillNone {
		return nil, ""
	}
	return m.insightsDrillTracks(*row), row.label
}

// insightsSessionActionTracks returns the focused session in the order it was
// heard, so E replays it rather than approximating it.
func (m Model) insightsSessionActionTracks() ([]library.Track, string) {
	s := m.insights.focusedSession()
	if s == nil {
		return nil, ""
	}
	label := time.Unix(s.StartedAt, 0).In(m.insights.location()).Format("2 Jan 15:04")
	return m.insightsSessionTracks(s.ID), "Session " + label
}

// handleInsightsEnqueueAll appends every track behind the current row (or the
// whole drilled-in list) to the queue.
func (m Model) handleInsightsEnqueueAll() (Model, tea.Cmd) {
	if m.activeView != viewInsights {
		return m, nil
	}
	tracks, label := m.insightsActionTracks()
	return m.insightsEnqueue(tracks, label)
}

// handleInsightsAntiRut enqueues a slice of never-played tracks drawn from the
// dustiest shelves, so the rut can be broken without leaving the briefing. On
// Concentration the rut is a different one — the handful of artists that hold
// the listening time — so there it pulls from outside that group instead.
func (m Model) handleInsightsAntiRut() (Model, tea.Cmd) {
	if m.activeView != viewInsights {
		return m, nil
	}
	if m.insights.panel == panelConcentration {
		tracks := m.insightsRutBreakerTracks(insightsAntiRutCount)
		if len(tracks) == 0 {
			// A library smaller than the rut has no outside, which is a different
			// answer from "nothing to enqueue" and worth saying out loud.
			m.statusBar.Notify(fmt.Sprintf("Your whole library is inside your top %d artists", insightsRutTopArtists))
			return m, nil
		}
		return m.insightsEnqueue(tracks, fmt.Sprintf("Outside your top %d artists", insightsRutTopArtists))
	}
	return m.insightsEnqueue(m.insightsCoverageAntiRutTracks(insightsAntiRutCount), "Never played")
}

func (m Model) insightsEnqueue(tracks []library.Track, label string) (Model, tea.Cmd) {
	if len(tracks) == 0 {
		m.statusBar.Notify("Nothing to enqueue")
		return m, nil
	}
	be := m.batchEnqueuer
	if be == nil {
		m.statusBar.Notify("Enqueue not available")
		return m, nil
	}
	wasEmpty := len(m.queue.entries) == 0
	return m, func() tea.Msg {
		ids, err := be.AppendTracks(tracks, label)
		if err != nil {
			return BatchEnqueuedMsg{Err: fmt.Errorf("enqueue insights: %w", err)}
		}
		return BatchEnqueuedMsg{
			Entries:      newQueueEntries(tracks, ids, label),
			QueueIDs:     ids,
			WasEmpty:     wasEmpty,
			PlaylistName: label,
		}
	}
}

// handleInsightsRadioSeed starts radio seeded from the aggregate metadata of
// the focused row's tracks. With radio already on, or nothing to seed from, it
// falls back to the plain toggle.
func (m Model) handleInsightsRadioSeed() (Model, tea.Cmd) {
	if m.radio.enabled {
		tm, cmd, _ := m.radioToggle()
		return tm.(Model), cmd
	}
	tracks, label := m.insightsActionTracks()
	if len(tracks) == 0 {
		tm, cmd, _ := m.radioToggle()
		return tm.(Model), cmd
	}
	seed := buildMixSeed(tracks, m.trackTags)
	m.radio.syntheticSeed = &seed
	m.radio.enabled = true
	m.radio.seedHash = ""
	if m.sessionWriter != nil {
		if err := m.sessionWriter.WriteRadioMode(true); err != nil {
			slog.Warn("session: write radio mode", "err", err)
		}
	}
	m.statusBar.PushMsg("Radio: seeded from "+label, time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, statusbar.NotifInfo)
	cmd := m.radioMaybeEnqueueCmd()
	if cmd != nil {
		m.radio.enqueuePending = true
	}
	return m, cmd
}

// handleInsightsJumpBrowse opens the focused row's artist in Browse, so a bar
// in the briefing leads back to the shelf it measures.
func (m Model) handleInsightsJumpBrowse() Model {
	if m.activeView != viewInsights {
		return m
	}
	artist := ""
	switch {
	case m.insights.atTracks():
		if t := m.insights.focusedTrack(); t != nil {
			artist = t.Artist
		}
	case m.insights.level == insightsLevelSessions:
		if s := m.insights.focusedSession(); s != nil {
			artist = s.OpenerArtist
		}
	default:
		row := m.insights.focusedRow()
		if row == nil {
			return m
		}
		switch row.drill {
		case drillNone:
			return m
		case drillUnheardArtist, drillArtistPlays, drillSessionsOpener, drillSessionsCloser:
			artist = row.key
		case drillDormant:
			if len(m.insights.dormant) > 0 {
				artist = m.insights.dormant[0].Artist
			}
		default:
			// Every remaining row stands for a set rather than for one name, so
			// b lands on the artist the set opens with.
			if tracks := m.insightsDrillTracks(*row); len(tracks) > 0 {
				artist = tracks[0].Artist
			}
		}
	}
	if artist == "" {
		m.statusBar.Notify("No artist to jump to")
		return m
	}
	return m.jumpToArtist(artist)
}

// handleInsightsSavePlaylist saves the drilled-in tracks as a static playlist.
func (m Model) handleInsightsSavePlaylist() (Model, tea.Cmd) {
	if m.activeView != viewInsights || !m.insights.atTracks() {
		return m, nil
	}
	tracks := m.insights.trackView.SourceTracks
	if len(tracks) == 0 {
		m.statusBar.Notify("No tracks to save")
		return m, nil
	}
	pw := m.playlistWriter
	if pw == nil {
		m.statusBar.Notify("Playlist writer not available")
		return m, nil
	}
	pa := m.playlistAdder
	name := "Insights: " + m.insights.drillLabel
	hashes := make([]string, len(tracks))
	for i, t := range tracks {
		hashes[i] = t.Hash
	}
	count := len(hashes)
	return m, func() tea.Msg {
		id, err := pw.CreatePlaylist(name)
		if err != nil {
			return mixSaveAsPlaylistErrorMsg{err: err}
		}
		if pa != nil {
			if err := pa.AddTracksToPlaylist(id, hashes); err != nil {
				return mixSaveAsPlaylistErrorMsg{err: err}
			}
		}
		rows, err := pw.ReadPlaylists()
		if err != nil {
			return mixSaveAsPlaylistErrorMsg{err: err}
		}
		return mixSavedAsPlaylistMsg{playlists: rows, playlistName: name, count: count}
	}
}
