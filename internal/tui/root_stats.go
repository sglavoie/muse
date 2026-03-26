package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// statsPeriods defines the available listening-stats time windows.
var statsPeriods = []struct {
	label     string
	sinceDays int // 0 = all-time, >0 = last N days, -1 = start of month, -2 = start of year
}{
	{"All Time", 0},
	{"Last 7 Days", 7},
	{"Last 30 Days", 30},
	{"This Month", -1},
	{"This Year", -2},
}

// sinceUnixForPeriod converts a period index to the unix timestamp cutoff.
// Pass 0 for all-time (returns 0).
func sinceUnixForPeriod(periodIdx int) int64 {
	p := statsPeriods[periodIdx]
	now := time.Now()
	switch p.sinceDays {
	case 0:
		return 0
	case -1: // start of current month
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	case -2: // start of current year
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).Unix()
	default:
		return now.AddDate(0, 0, -p.sinceDays).Unix()
	}
}

// StatsPanel holds a snapshot of listening stats for rendering.
type StatsPanel struct {
	stats       db.ListeningStats
	periodLabel string
	err         error
}

// render builds the stats overlay string.
func (p StatsPanel) render(s styles.Styles) string {
	var sb strings.Builder

	title := s.Heading.Render("Listening Stats") + "  " + s.Dim.Render("("+p.periodLabel+")")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	if p.err != nil {
		sb.WriteString(s.Error.Render("Error loading stats: " + p.err.Error()))
		sb.WriteString("\n")
		return sb.String()
	}

	if p.stats.TotalPlays == 0 {
		sb.WriteString(s.Dim.Render("No listening data yet."))
		sb.WriteString("\n")
		return sb.String()
	}

	// Overview
	sb.WriteString(s.Bold.Render("Overview"))
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "  Total plays:    %d\n", p.stats.TotalPlays)
	fmt.Fprintf(&sb, "  Total time:     %s\n", formatListeningTime(p.stats.TotalListenedSecs))
	if p.stats.CurrentStreak > 0 {
		streak := fmt.Sprintf("  Current streak: %d day", p.stats.CurrentStreak)
		if p.stats.CurrentStreak != 1 {
			streak += "s"
		}
		sb.WriteString(streak + "\n")
	}
	sb.WriteString("\n")

	writeTopList(&sb, s, "Top Artists", p.stats.TopArtists)
	writeTopList(&sb, s, "Top Albums", p.stats.TopAlbums)
	writeTopList(&sb, s, "Top Genres", p.stats.TopGenres)
	writeTopList(&sb, s, "Most Played Tracks", p.stats.TopTracks)

	return sb.String()
}

// writeTopList appends a section heading and up to 5 stat rows to sb.
func writeTopList(sb *strings.Builder, s styles.Styles, heading string, entries []db.StatEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString(s.Bold.Render(heading))
	sb.WriteString("\n")
	for i, e := range entries {
		name := e.Name
		if name == "" {
			name = s.Dim.Render("(unknown)")
		}
		fmt.Fprintf(sb, "  %d. %s  %s  %s\n",
			i+1,
			name,
			s.Dim.Render(fmt.Sprintf("×%d", e.PlayCount)),
			s.Dim.Render(formatListeningTime(e.ListenedSecs)),
		)
	}
	sb.WriteString("\n")
}

// formatListeningTime formats a duration in seconds as "Xh Ym" or "Ym" or "Xs".
func formatListeningTime(secs int) string {
	if secs <= 0 {
		return "0s"
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// loadStatsCmd dispatches a background DB read and returns a listeningStatsMsg.
func (m Model) loadStatsCmd() tea.Cmd {
	if m.statsReader == nil {
		return func() tea.Msg {
			return listeningStatsMsg{}
		}
	}
	since := sinceUnixForPeriod(m.statsPeriod)
	return func() tea.Msg {
		stats, err := m.statsReader.ReadListeningStats(since)
		return listeningStatsMsg{stats: stats, err: err}
	}
}

// handleStatsKeyPress handles key events while the stats overlay is active.
// Returns (model, cmd, true) when the key was consumed, (model, nil, false) to
// fall through to global key handling (ctrl+c only).
func (m Model) handleStatsKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.statsVisible {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m, nil, false
	case "esc":
		m.statsVisible = false
		m.statsViewport = viewport.Model{}
		m.activeView = m.statsPrevView
		return m, nil, true
	case "left", "shift+tab":
		if m.statsPeriod > 0 {
			m.statsPeriod--
		} else {
			m.statsPeriod = len(statsPeriods) - 1
		}
		return m, m.loadStatsCmd(), true
	case "right", "tab":
		m.statsPeriod = (m.statsPeriod + 1) % len(statsPeriods)
		return m, m.loadStatsCmd(), true
	default:
		var cmd tea.Cmd
		m.statsViewport, cmd = m.statsViewport.Update(msg)
		return m, cmd, true
	}
}

// renderStatsOverlay renders the stats viewport plus the period hint footer.
func (m Model) renderStatsOverlay() string {
	hint := s(m).Dim.Render("← → to change period · esc to close")
	return m.statsViewport.View() + "\n" + hint
}

// s is a local helper to dereference the styles pointer.
func s(m Model) styles.Styles { return *m.styles }
