package insights

// The drill levels: entering a row's track list, entering the sessions a
// Sessions row stands for, entering one session's tracks, and stepping back out
// again. The focused* accessors are how the rest of the view asks what is
// selected right now, whichever level that happens to mean.

import (
	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// AtTracks reports whether the embedded library.View is what is on screen — true
// both for a row's track list and for the tracks of one session.
func (v View) AtTracks() bool {
	return v.Level == LevelTracks || v.Level == LevelSessionTracks
}

// FocusedRow returns the row under the cursor, or nil when the panel has none.
func (v View) FocusedRow() *Row {
	rows := v.Rows()
	if v.Cursor < 0 || v.Cursor >= len(rows) {
		return nil
	}
	return &rows[v.Cursor]
}

// FocusedTrack returns the focused track at drill level, or nil otherwise.
func (v *View) FocusedTrack() *library.Track {
	if !v.AtTracks() {
		return nil
	}
	track, ok := v.TrackView.Panel().FocusedTrack()
	if !ok {
		return nil
	}
	return &track
}

// FocusedSession returns the session under the cursor in the session list.
func (v View) FocusedSession() *insightsdata.SessionRow {
	if v.Level != LevelSessions ||
		v.sessionCursor < 0 || v.sessionCursor >= len(v.SessionList) {
		return nil
	}
	return &v.SessionList[v.sessionCursor]
}

// OpenDrill enters the track level with the given set.
func (v *View) OpenDrill(label string, tracks []library.Track) {
	if v.Level == LevelSessions {
		v.Level = LevelSessionTracks
	} else {
		v.Level = LevelTracks
	}
	v.DrillLabel = label
	v.TrackView = libtui.View{
		SourceTracks: tracks,
		Fuzzy:        library.SahilmFuzzySearcher{},
		Width:        v.width,
		Height:       v.Height,
	}
}

// OpenSessionList enters the session level with the sessions a Sessions row
// stands for.
func (v *View) OpenSessionList(label string, rows []insightsdata.SessionRow) {
	v.Level = LevelSessions
	v.SessionLabel = label
	v.SessionList = rows
	v.sessionCursor = 0
	v.sessionScroll = 0
}

// Back steps out one level: a session's tracks return to the session list,
// anything else returns to the briefing.
func (v *View) Back() {
	if v.Level == LevelSessionTracks {
		v.Level = LevelSessions
		v.DrillLabel = ""
		v.TrackView = libtui.View{}
		return
	}
	v.Level = LevelBriefing
	v.DrillLabel = ""
	v.TrackView = libtui.View{}
	v.SessionLabel = ""
	v.SessionList = nil
	v.sessionCursor = 0
	v.sessionScroll = 0
}
