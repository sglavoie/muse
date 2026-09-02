package tui

// The drill levels: entering a row's track list, entering the sessions a
// Sessions row stands for, entering one session's tracks, and stepping back out
// again. The focused* accessors are how the rest of the view asks what is
// selected right now, whichever level that happens to mean.

import (
	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// atTracks reports whether the embedded library.View is what is on screen — true
// both for a row's track list and for the tracks of one session.
func (v InsightsView) atTracks() bool {
	return v.level == insightsLevelTracks || v.level == insightsLevelSessionTracks
}

// focusedRow returns the row under the cursor, or nil when the panel has none.
func (v InsightsView) focusedRow() *insightsRow {
	rows := v.rows()
	if v.cursor < 0 || v.cursor >= len(rows) {
		return nil
	}
	return &rows[v.cursor]
}

// focusedTrack returns the focused track at drill level, or nil otherwise.
func (v *InsightsView) focusedTrack() *library.Track {
	if !v.atTracks() {
		return nil
	}
	track, ok := v.trackView.Panel().FocusedTrack()
	if !ok {
		return nil
	}
	return &track
}

// focusedSession returns the session under the cursor in the session list.
func (v InsightsView) focusedSession() *insights.SessionRow {
	if v.level != insightsLevelSessions ||
		v.sessionCursor < 0 || v.sessionCursor >= len(v.sessionList) {
		return nil
	}
	return &v.sessionList[v.sessionCursor]
}

// openDrill enters the track level with the given set.
func (v *InsightsView) openDrill(label string, tracks []library.Track) {
	if v.level == insightsLevelSessions {
		v.level = insightsLevelSessionTracks
	} else {
		v.level = insightsLevelTracks
	}
	v.drillLabel = label
	v.trackView = libtui.View{
		SourceTracks: tracks,
		Fuzzy:        library.SahilmFuzzySearcher{},
		Width:        v.width,
		Height:       v.height,
	}
}

// openSessionList enters the session level with the sessions a Sessions row
// stands for.
func (v *InsightsView) openSessionList(label string, rows []insights.SessionRow) {
	v.level = insightsLevelSessions
	v.sessionLabel = label
	v.sessionList = rows
	v.sessionCursor = 0
	v.sessionScroll = 0
}

// back steps out one level: a session's tracks return to the session list,
// anything else returns to the briefing.
func (v *InsightsView) back() {
	if v.level == insightsLevelSessionTracks {
		v.level = insightsLevelSessions
		v.drillLabel = ""
		v.trackView = libtui.View{}
		return
	}
	v.level = insightsLevelBriefing
	v.drillLabel = ""
	v.trackView = libtui.View{}
	v.sessionLabel = ""
	v.sessionList = nil
	v.sessionCursor = 0
	v.sessionScroll = 0
}
