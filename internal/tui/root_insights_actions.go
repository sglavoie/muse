package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/library"
	instui "github.com/sglavoie/muse/internal/tui/insights"
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
	switch m.insights.Level {
	case instui.LevelBriefing:
		row := m.insights.FocusedRow()
		if row == nil || row.Drill == instui.DrillNone {
			return m, nil
		}
		if row.Drill.OpensSessions() {
			sessions := instui.SessionsMatching(m.insights.Sessions, row.Drill, row.Key)
			if len(sessions) == 0 {
				m.statusBar.Notify("No sessions behind this row")
				return m, nil
			}
			m.insights.OpenSessionList(row.Label, sessions)
			return m, nil
		}
		tracks := m.insightsDrillTracks(*row)
		if len(tracks) == 0 {
			m.statusBar.Notify("Nothing behind this row")
			return m, nil
		}
		m.insights.OpenDrill(row.Label, tracks)
	case instui.LevelSessions:
		tracks, label := m.insightsSessionActionTracks()
		if len(tracks) == 0 {
			m.statusBar.Notify("Nothing behind this session")
			return m, nil
		}
		m.insights.OpenDrill(label, tracks)
	case instui.LevelTracks, instui.LevelSessionTracks:
		return m.insightsEnqueueSelection()
	}
	return m, nil
}

// insightsEnqueueSelection appends the bulk-selected tracks — or the focused
// one when nothing is selected — to the queue, then drops the selection the way
// Library's enter does. A refused enqueue keeps the selection so the key can be
// pressed again once a queue is available.
func (m Model) insightsEnqueueSelection() (Model, tea.Cmd) {
	m, cmd := m.insightsEnqueue(m.insightsBulkTracks(), m.insights.DrillLabel)
	if cmd == nil {
		return m, nil
	}
	m = exitVisualMode(m)
	m.insights.TrackView.SelectedHashes = nil
	return m, cmd
}

// insightsBulkTracks returns what the selection-aware actions operate on at a
// drilled-in track level: the bulk-selected rows in display order, or the
// focused row when nothing is selected. A session's repeats are kept, since its
// track list is a replay rather than a set.
func (m Model) insightsBulkTracks() []library.Track {
	if !m.insights.AtTracks() {
		return nil
	}
	order := m.insights.TrackView.BaseTracks()
	if m.insights.TrackView.SortedTracks != nil {
		order = m.insights.TrackView.SortedTracks
	}
	if len(m.insights.TrackView.SelectedHashes) > 0 {
		return collectSelectedTracks(order, m.insights.TrackView.SelectedHashes)
	}
	if t := m.insights.FocusedTrack(); t != nil {
		return []library.Track{*t}
	}
	return nil
}

// handleInsightsBack steps out one level.
func (m Model) handleInsightsBack() Model {
	if m.activeView != viewInsights || m.insights.Level == instui.LevelBriefing {
		return m
	}
	// Stepping out replaces the list visual mode anchored into, so the range
	// has nothing left to grow over.
	m = exitVisualMode(m)
	m.insights.Back()
	m.insights.ClampCursor()
	return m
}

// insightsActionTracks returns the tracks the batch actions operate on: the
// drilled-in list at a track level, the focused session at the session level,
// or the focused row's set on the briefing. The label is what notifications and
// the playlist name use.
func (m Model) insightsActionTracks() ([]library.Track, string) {
	if m.insights.AtTracks() {
		return m.insights.TrackView.SourceTracks, m.insights.DrillLabel
	}
	if m.insights.Level == instui.LevelSessions {
		return m.insightsSessionActionTracks()
	}
	row := m.insights.FocusedRow()
	if row == nil || row.Drill == instui.DrillNone {
		return nil, ""
	}
	return m.insightsDrillTracks(*row), row.Label
}

// insightsSessionActionTracks returns the focused session in the order it was
// heard, so E replays it rather than approximating it.
func (m Model) insightsSessionActionTracks() ([]library.Track, string) {
	s := m.insights.FocusedSession()
	if s == nil {
		return nil, ""
	}
	label := time.Unix(s.StartedAt, 0).In(m.insights.Location()).Format("2 Jan 15:04")
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
	if m.insights.Panel == instui.PanelConcentration {
		tracks := m.insightsRutBreakerTracks(insightsAntiRutCount)
		if len(tracks) == 0 {
			// A library smaller than the rut has no outside, which is a different
			// answer from "nothing to enqueue" and worth saying out loud.
			m.statusBar.Notify(fmt.Sprintf("Your whole library is inside your top %d artists", instui.RutTopArtists))
			return m, nil
		}
		return m.insightsEnqueue(tracks, fmt.Sprintf("Outside your top %d artists", instui.RutTopArtists))
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
	wasEmpty := len(m.queue.Entries) == 0
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
	case m.insights.AtTracks():
		if t := m.insights.FocusedTrack(); t != nil {
			artist = t.Artist
		}
	case m.insights.Level == instui.LevelSessions:
		if s := m.insights.FocusedSession(); s != nil {
			artist = s.OpenerArtist
		}
	default:
		row := m.insights.FocusedRow()
		if row == nil {
			return m
		}
		switch row.Drill {
		case instui.DrillNone:
			return m
		case instui.DrillUnheardArtist, instui.DrillArtistPlays, instui.DrillSessionsOpener, instui.DrillSessionsCloser:
			artist = row.Key
		case instui.DrillDormant:
			if len(m.insights.Dormant) > 0 {
				artist = m.insights.Dormant[0].Artist
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
	if m.activeView != viewInsights || !m.insights.AtTracks() {
		return m, nil
	}
	tracks := m.insights.TrackView.SourceTracks
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
	name := "Insights: " + m.insights.DrillLabel
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
