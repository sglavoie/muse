package tui

import (
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	instui "github.com/sglavoie/muse/internal/tui/insights"
)

// insightsDormantDays is how long a track must have gone unplayed, measured
// from the most recent play in the whole history, before it counts as dormant.
const insightsDormantDays = 180

// insightsDormantTopN caps the dormant list so the Coverage body stays a
// briefing rather than a dump of the library.
const insightsDormantTopN = 10

// insightsLoadedMsg carries a completed Insights computation. gen is echoed
// back from the request so a reply for an abandoned window cannot overwrite a
// newer one. When err is set every other field is zero: a snapshot folded from
// readers that failed would be plausible and wrong, so none is computed.
type insightsLoadedMsg struct {
	gen           int
	err           error
	snap          insights.Snapshot
	genreCoverage []insights.ShelfStat
	dormant       []library.Track
	plays         []insights.PlayRow
	sessions      []insights.SessionRow
	timeline      []insights.TimelineRow
	genreHours    []instui.GenreHours
	backlog       []instui.BacklogMonth
	health        []instui.HealthRow
	loc           *time.Location
}

// insightsPendingWrites counts the database writes an invalidated Insights memo
// is still waiting on. Undo and rescan deletion both change memory first and
// persist afterwards on a goroutine that reports back to nobody, so this is
// reached through a pointer: the write clears itself without owning the Model it
// was launched from. A nil tracker is a Model that never starts one.
type insightsPendingWrites struct{ inflight atomic.Int64 }

func (w *insightsPendingWrites) begin() {
	if w != nil {
		w.inflight.Add(1)
	}
}

func (w *insightsPendingWrites) done() {
	if w != nil {
		w.inflight.Add(-1)
	}
}

func (w *insightsPendingWrites) busy() bool { return w != nil && w.inflight.Load() > 0 }

// insightsMaybeLoadCmd returns a load command when the view has no data yet or
// its memo has been invalidated. Returns nil when no reader is wired or the
// snapshot is already current.
func (m *Model) insightsMaybeLoadCmd() tea.Cmd {
	if m.insightsReader == nil || m.insights.Loading {
		return nil
	}
	// A read that failed is not retried on its own: the active refresh rides the
	// 1 Hz tick, so a database that keeps failing would become one failed read
	// per second. Entering the view and changing the data both clear loadErr.
	if m.insights.LoadErr != nil {
		return nil
	}
	if m.insights.Loaded && !m.insights.Stale {
		return nil
	}
	return m.insightsLoadCmd()
}

// insightsEnterCmd is the load a deliberate entry to the view issues. Entering
// is the retry for a failed read, so it clears the error the automatic refresh
// path refuses to retry past.
func (m *Model) insightsEnterCmd() tea.Cmd {
	m.insights.LoadErr = nil
	return m.insightsMaybeLoadCmd()
}

// insightsInvalidate marks the memo superseded by a change to the underlying
// data. The generation bump strands any load already in flight: its reply was
// computed before the change and would install numbers that are already wrong.
// A change that reached the database is also fresh evidence that reading it can
// work again, so a previous failure stops blocking the next load.
func (m *Model) insightsInvalidate() {
	m.insights.Gen++
	m.insights.Loading = false
	m.insights.Stale = true
	m.insights.LoadErr = nil
}

// insightsActiveRefreshCmd reloads an invalidated snapshot the user is looking
// at right now. Invalidation on its own only arms the next view entry, so
// without this the panels would sit on numbers the user just changed.
func (m *Model) insightsActiveRefreshCmd() tea.Cmd {
	if m.ActiveView != viewInsights {
		return nil
	}
	return m.insightsMaybeLoadCmd()
}

// insightsLoadCmd reads every Insights source for the active window and folds
// them into a Snapshot off the UI goroutine.
func (m *Model) insightsLoadCmd() tea.Cmd {
	r := m.insightsReader
	if r == nil {
		return nil
	}
	// Reading while a change is half-made would fold pre-change rows into a
	// snapshot that then looks current forever, because neither the undo nor the
	// rescan-deletion write announces its completion. Staying stale is what keeps
	// the 1 Hz refresh asking until every write has succeeded or failed.
	if m.insightsPending.busy() {
		m.insights.Stale = true
		return nil
	}

	m.insights.Gen++
	m.insights.Loading = true

	gen := m.insights.Gen
	windowIdx := m.insights.WindowIdx
	res := m.insights.Resolution()
	mx := m.cfg.Mixes
	// Snapshot the index on the UI goroutine; the closure below runs off it.
	tracks := m.library.Index.Tracks()

	return func() tea.Msg {
		now := time.Now()
		loc := time.Local
		since := instui.WindowSince(windowIdx, now, loc)

		// Every reader feeds a panel that is drawn unconditionally, so a failed
		// one leaves a zero-valued input that renders as a real measurement.
		// Collect them all, then refuse the whole snapshot if any failed.
		var errs []error
		failed := func(what string, err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", what, err))
			}
		}

		plays, err := r.ReadInsightPlays(since)
		failed("plays", err)
		genres, err := r.ReadInsightGenres(since)
		failed("genres", err)
		sessions, err := r.ReadInsightSessions(since)
		failed("sessions", err)
		coverage, err := r.ReadInsightCoverage()
		failed("coverage", err)
		timeline, err := r.ReadInsightTimeline()
		failed("timeline", err)
		quality, err := r.ReadInsightQuality(mx.DeepCutsMinPlays, mx.DeepCutsMaxPlays, mx.DeepCutsMinCompletion)
		failed("quality", err)
		unverified, err := r.ReadUnverifiedDurations()
		failed("unverified durations", err)

		if err := errors.Join(errs...); err != nil {
			slog.Warn("insights: load", "err", err)
			return insightsLoadedMsg{gen: gen, err: err}
		}

		unverifiedHashes := make([]string, len(unverified))
		for i, u := range unverified {
			unverifiedHashes[i] = u.Hash
		}

		snap := insights.Compute(insights.Input{
			Now:         now,
			Loc:         loc,
			Since:       since,
			WindowLabel: instui.Windows[windowIdx].Label,
			Resolution:  res,
			Plays:       plays,
			Genres:      genres,
			Sessions:    sessions,
			Coverage:    coverage,
			Timeline:    timeline,
			Quality:     quality,
		})

		genreOf := make(map[string][]string, len(tracks))
		for _, t := range tracks {
			genreOf[t.Hash] = library.SplitGenres(t.Genre)
		}

		return insightsLoadedMsg{
			gen:           gen,
			snap:          snap,
			genreCoverage: genreCoverageShelves(tracks),
			dormant:       dormantTracks(tracks, snap.LastPlayAt),
			plays:         plays,
			sessions:      sessions,
			timeline:      timeline,
			genreHours:    instui.HourGenreFold(plays, func(h string) []string { return genreOf[h] }, loc),
			backlog:       instui.BacklogMonths(tracks, loc),
			health:        instui.BuildHealth(snap.Quality, tracks, unverifiedHashes, mx.DeepCutsMinPlays, mx.DeepCutsMaxPlays),
			loc:           loc,
		}
	}
}

// handleInsightsLoadedMsg installs a snapshot, discarding replies that a newer
// request has already superseded.
func (m Model) handleInsightsLoadedMsg(msg insightsLoadedMsg) Model {
	if msg.gen != m.insights.Gen {
		return m
	}
	m.insights.Loading = false
	if msg.err != nil {
		// Nothing is installed: the panels keep the last numbers that were whole,
		// or say they have none. stale stays set so the retry has something to do.
		m.insights.LoadErr = msg.err
		m.insights.Stale = true
		if m.insights.Loaded {
			m.statusBar.NotifyError("Insights: refresh failed, showing the last complete figures")
		} else {
			m.statusBar.NotifyError("Insights: could not read your listening history")
		}
		return m
	}
	m.insights.LoadErr = nil
	m.insights.Snap = msg.snap
	m.insights.GenreCoverage = msg.genreCoverage
	m.insights.Dormant = msg.dormant
	m.insights.Plays = msg.plays
	m.insights.Sessions = msg.sessions
	m.insights.Timeline = msg.timeline
	m.insights.GenreHours = msg.genreHours
	m.insights.Backlog = msg.backlog
	m.insights.Health = msg.health
	m.insights.Loc = msg.loc
	m.insights.Loaded = true
	m.insights.Stale = false
	m.insights.ClampCursor()
	return m
}

// handleInsightsPanelCycle moves the active panel. Panels read from the same
// snapshot, so no reload is needed.
func (m Model) handleInsightsPanelCycle(delta int) Model {
	m.insights = m.insights.ShiftPanel(delta)
	m.insights.ClampCursor()
	return m
}

// handleInsightsWindowCycle moves whichever control the active panel actually
// honours — the time window, or the bucket size for the panels whose span is
// always all-time — and reloads. A fixed panel says so rather than reloading
// identical numbers under a new label.
func (m Model) handleInsightsWindowCycle(delta int) (Model, tea.Cmd) {
	switch m.insights.Spec().Class {
	case instui.ClassFixed:
		m.statusBar.Notify("This panel is always all-time")
		return m, nil
	case instui.ClassResolution:
		m.insights = m.insights.ShiftResolution(delta)
	default:
		m.insights = m.insights.ShiftWindow(delta)
	}
	return m, m.insightsLoadCmd()
}
