package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/chart"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// location is the zone every row buckets in. It falls back to the load path's
// own default rather than to UTC, so a view built without a snapshot still
// files a play under the hour the user heard it.
func (v InsightsView) location() *time.Location {
	if v.loc == nil {
		return time.Local
	}
	return v.loc
}

// coverageRows builds the Coverage body: dusty shelves, albums nobody has ever
// opened, per-genre coverage, and tracks that were heard once and then dropped.
func (v InsightsView) coverageRows() []insightsRow {
	c := v.snap.Coverage
	rows := make([]insightsRow, 0, 40)

	var untouched, dusty []insights.ShelfStat
	for _, a := range c.DustyAlbums {
		if a.Unheard == a.Tracks {
			untouched = append(untouched, a)
		} else {
			dusty = append(dusty, a)
		}
	}

	appendShelves := func(section string, shelves []insights.ShelfStat, kind insightsDrillKind) {
		for _, s := range shelves {
			rows = append(rows, insightsRow{
				section:  section,
				label:    s.Name,
				note:     fmt.Sprintf("%d/%d unheard", s.Unheard, s.Tracks),
				fraction: shelfFraction(s),
				drill:    kind,
				key:      s.Name,
			})
		}
	}

	appendShelves("Dusty artists", c.DustyArtists, drillUnheardArtist)
	appendShelves("Untouched albums", untouched, drillUnheardAlbum)
	appendShelves("Dusty albums", dusty, drillUnheardAlbum)
	appendShelves("Genre coverage", v.genreCoverage, drillUnheardGenre)

	for _, t := range v.dormant {
		rows = append(rows, insightsRow{
			section: "Dormant tracks",
			label:   fmt.Sprintf("%s — %s", t.Artist, t.Title),
			note:    "last " + relativeDayLabel(t.LastPlayedAt, v.snap.LastPlayAt),
			drill:   drillDormant,
			key:     t.Hash,
		})
	}
	return rows
}

func shelfFraction(s insights.ShelfStat) float64 {
	if s.Tracks <= 0 {
		return 0
	}
	return float64(s.Unheard) / float64(s.Tracks)
}

// relativeDayLabel renders how long before ref a timestamp falls. ref is the
// snapshot's most recent play rather than the wall clock so the label stays
// stable between renders.
func relativeDayLabel(at, ref int64) string {
	if at <= 0 {
		return "never"
	}
	if ref <= at {
		return "today"
	}
	days := int((ref - at) / 86400)
	switch {
	case days <= 0:
		return "today"
	case days < 60:
		return fmt.Sprintf("%dd ago", days)
	default:
		return fmt.Sprintf("%dmo ago", days/30)
	}
}

// render draws the Insights view: the briefing plus the active panel at level
// 0, a session list at level 2, and an embedded track list at the two levels
// that end in tracks.
func (v InsightsView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	if v.atTracks() {
		if len(v.trackView.sourceTracks) == 0 {
			return s.Dim.Render("Nothing behind this row.")
		}
		return v.trackView.render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
	}
	if v.level == insightsLevelSessions {
		return v.renderSessionList(s)
	}

	if !v.loaded {
		if v.loading {
			return s.Dim.Render("Reading your listening history…")
		}
		if v.loadErr != nil {
			return s.RenderEmptyCard(v.width, v.height, "Could not read your listening history", []styles.EmptyAction{
				{Key: "8", Label: "retry"},
				{Key: "?", Label: "help"},
			})
		}
		return s.Dim.Render("Insights are not available: no history reader is wired.")
	}
	if v.empty() {
		return s.RenderEmptyCard(v.width, v.height, "Nothing to measure yet", []styles.EmptyAction{
			{Key: "1", Label: "go to library"},
			{Key: "?", Label: "help"},
		})
	}

	var b strings.Builder
	b.WriteString(v.renderStrip(s))
	b.WriteString("\n")
	if v.briefingFits() {
		b.WriteString(v.renderBriefing(s))
		b.WriteString("\n")
	}
	b.WriteString(v.renderPanel(s))

	// The header block has a floor it cannot shrink past, so at extreme heights
	// it can still outgrow the frame. Clamp rather than let it push the footer
	// off the screen.
	out := strings.Split(b.String(), "\n")
	if v.height > 0 && len(out) > v.height {
		out = out[:v.height]
	}
	return strings.Join(out, "\n")
}

func (v InsightsView) briefingFits() bool {
	return v.height >= insightsBriefingMinHeight
}

// headerHeight counts every line drawn above the panel's scrollable rows.
func (v InsightsView) headerHeight() int {
	h := 1 // state strip
	if v.briefingFits() {
		h += len(insightsPanelSpecs)
	}
	// Rendering the header with a zero Styles yields the same line count as the
	// styled render, so the two can never drift apart.
	h += len(v.panelHeaderLines(styles.Styles{}))
	return h
}

func (v InsightsView) renderStrip(s styles.Styles) string {
	spec := v.panelSpec()
	summary := fmt.Sprintf("%s plays · %s · %d %s",
		formatCount(v.snap.TotalPlays),
		text.FormatListeningTime(v.snap.TotalSecs),
		v.snap.SpanDays, text.Pluralize(v.snap.SpanDays, "day", "days"))
	line := strip.Render(v.width, []string{"Insights", summary},
		strip.SortChunk{}, strip.FilterChunk{}, strip.CountsChunk{}, nil,
		v.windowHint(), s)
	if line == "" {
		return spec.name
	}
	return line
}

// windowHint is the right-edge affordance that names the active window and,
// for panels that do not honour it, says so instead of silently ignoring w.
func (v InsightsView) windowHint() string {
	// A failed read leaves every panel on figures that are not current, so the
	// strip says so rather than letting the window label imply they match it.
	// The hint is a "key label" pair, so the retry key leads.
	if v.loadErr != nil {
		return "8 retry · read failed"
	}
	switch v.panelSpec().class {
	case classFixed:
		return "all-time (fixed)"
	case classResolution:
		return "resolution: " + v.resolution().Unit() + " ‹w›"
	default:
		return v.window().label + " ‹w›"
	}
}

func (v InsightsView) renderBriefing(s styles.Styles) string {
	lines := make([]string, 0, len(insightsPanelSpecs))
	labelW := 0
	for _, spec := range insightsPanelSpecs {
		labelW = max(labelW, len(spec.name))
	}
	for _, spec := range insightsPanelSpecs {
		cursor := "  "
		if spec.id == v.panel {
			cursor = "> "
		}
		head := cursor + text.Pad(spec.name, labelW) + "  "
		tail := v.headline(spec.id)
		if v.width > 0 {
			tail = text.Truncate(tail, max(v.width-len(head), 1))
		}
		if spec.id == v.panel {
			lines = append(lines, s.Accent.Render(head+tail))
		} else {
			lines = append(lines, s.TextPrimary.Render(head)+s.TextTertiary.Render(tail))
		}
	}
	return strings.Join(lines, "\n")
}

// headline is the one-line answer a panel gives. Every headline is computed
// from live data even when the panel's expanded body is not built yet.
func (v InsightsView) headline(p insightsPanel) string {
	snap := v.snap
	switch p {
	case panelCoverage:
		c := snap.Coverage
		return fmt.Sprintf("%s of %s tracks never played · %s unheard",
			formatCount(c.UnheardTracks), formatCount(c.TotalTracks),
			text.FormatListeningTime(int(c.UnheardSecs)))
	case panelRhythm:
		r := snap.Rhythm
		if r.TotalPlays == 0 {
			return "no plays in this window"
		}
		return fmt.Sprintf("%ss %.0f%% of plays · peak %02d:00 · %d quiet %s",
			r.TopWeekday.Weekday, r.TopWeekday.Share*100, r.PeakHour,
			len(r.QuietHours), text.Pluralize(len(r.QuietHours), "hour", "hours"))
	case panelMomentum:
		t := snap.Trends
		if t.AvgSecs <= 0 {
			return "not enough completed " + t.Resolution.Period() + "s to compare"
		}
		return fmt.Sprintf("%s %s — %.0f%% of your %s %s average",
			text.FormatListeningTime(t.Current.ListenedSecs), t.Resolution.Current(),
			t.CurrentVsAvg*100, text.FormatListeningTime(int(t.AvgSecs)), t.Resolution.Unit())
	case panelConcentration:
		l := snap.Rankings.Ladder
		if l.TotalArtists == 0 {
			return "no artists in this window"
		}
		return fmt.Sprintf("%d %s = half your listening time · top-5 %.0f%%",
			l.Artists50, text.Pluralize(l.Artists50, "artist", "artists"),
			snap.Rankings.TopFiveShare*100)
	case panelTimeVsPlays:
		r := snap.Rankings
		if len(r.Deltas) == 0 {
			return "no artists in this window"
		}
		// Deltas covers the compared head of the two orderings, not every artist:
		// "moves 9 of your top 15" is only true when 15 is the size of that head.
		return fmt.Sprintf("ranking by hours moves %d of your top %d artists",
			r.MovedCount, len(r.Deltas))
	case panelSessions:
		ss := snap.SessionStats
		if ss.Count == 0 {
			return "no sessions in this window"
		}
		return fmt.Sprintf("%d sessions · median %s · %.0f%% single-artist",
			ss.Count, text.FormatListeningTime(ss.MedianSecs), ss.SingleArtistShare*100)
	case panelDiscovery:
		t := snap.Trends
		return fmt.Sprintf("%d first-time %s %s · %.0f%% are repeats",
			t.CurrentFirstPlays, text.Pluralize(t.CurrentFirstPlays, "track", "tracks"),
			t.Resolution.Current(), t.RepeatShare*100)
	case panelDataHealth:
		// The count is of rows rather than of snapshot issues: a finding with no
		// track list behind it is stated in the header prose instead, and a
		// headline that counted those would promise rows that are not there.
		switch n := len(v.health); {
		case n == 0 && len(snap.Quality.Issues) == 0:
			return "nothing distorting the panels above"
		case n == 0:
			return snap.Quality.Issues[0].Detail
		default:
			return fmt.Sprintf("%d %s · %s %s affected",
				n, text.Pluralize(n, "finding", "findings"),
				formatCount(v.healthAffectedTracks()),
				text.Pluralize(v.healthAffectedTracks(), "track", "tracks"))
		}
	}
	return ""
}

// panelHeaderLines is the static block a panel draws above its scrollable
// rows, trimmed so it can never crowd the body out of a short terminal.
func (v InsightsView) panelHeaderLines(s styles.Styles) []string {
	var lines []string
	switch v.panel {
	case panelCoverage:
		lines = v.coverageHeaderLines(s)
	case panelRhythm:
		lines = v.rhythmHeaderLines(s)
	case panelMomentum:
		lines = v.momentumHeaderLines(s)
	case panelConcentration:
		lines = v.concentrationHeaderLines(s)
	case panelTimeVsPlays:
		lines = v.timeVsPlaysHeaderLines(s)
	case panelSessions:
		lines = v.sessionsHeaderLines(s)
	case panelDiscovery:
		lines = v.discoveryHeaderLines(s)
	case panelDataHealth:
		lines = v.dataHealthHeaderLines(s)
	default:
		lines = []string{"", s.Dim.Render(v.headline(v.panel))}
	}
	if b := v.headerBudget(); len(lines) > b {
		lines = lines[:b]
	}
	return lines
}

// headerBudget is how many lines a panel header may occupy. A header that eats
// the whole frame turns a drillable panel into a picture, so the body's floor
// is reserved before the header gets anything.
func (v InsightsView) headerBudget() int {
	used := 1 // state strip
	if v.briefingFits() {
		used += len(insightsPanelSpecs)
	}
	return max(v.height-used-insightsMinBodyRows, insightsMinHeaderLines)
}

func (v InsightsView) coverageHeaderLines(s styles.Styles) []string {
	c := v.snap.Coverage
	w := max(v.width, 1)
	pal := chart.FromStyles(s)

	lines := []string{
		"",
		s.TextPrimary.Render(fmt.Sprintf("In %d %s you played %s tracks. You own %s.",
			v.snap.SpanDays, text.Pluralize(v.snap.SpanDays, "day", "days"),
			formatCount(c.HeardTracks), formatCount(c.TotalTracks))),
		"",
	}
	if !v.compact() {
		lines = append(lines, chart.Ratio(float64(c.HeardTracks), float64(c.TotalTracks), w, pal))
	}
	lines = append(lines,
		s.TextSecondary.Render(fmt.Sprintf("heard %s · unheard %s · %s unheard",
			formatCount(c.HeardTracks), formatCount(c.UnheardTracks),
			text.FormatListeningTime(int(c.UnheardSecs)))),
		"",
	)
	if v.height >= insightsProseMinHeight {
		for _, l := range strings.Split(ansi.Wrap(v.coverageReadLine(), w, " "), "\n") {
			lines = append(lines, s.Dim.Render(l))
		}
		lines = append(lines, "")
	}
	return lines
}

// coverageReadLine is the cross-reference that makes Coverage land: the size of
// the unheard part measured against everything actually listened to.
func (v InsightsView) coverageReadLine() string {
	c := v.snap.Coverage
	if c.ListenedSecs <= 0 {
		return "Nothing listened to yet — every track below is still unopened."
	}
	sized := fmt.Sprintf("That unheard part is %.0f%% the size of everything you have listened to.",
		c.UnheardVsHeard*100)
	if c.MonthsToFinish <= 0 {
		return sized + " Not enough first-play history to project a finish date."
	}
	return sized + fmt.Sprintf(" At the current discovery rate you would finish it in %s.",
		formatMonths(c.MonthsToFinish))
}

func (v InsightsView) renderPanel(s styles.Styles) string {
	lines := append([]string(nil), v.panelHeaderLines(s)...)

	rows := v.rows()
	if len(rows) == 0 {
		if v.panel == panelCoverage {
			lines = append(lines, s.Dim.Render("Every shelf in your library has been opened."))
		}
		return strings.Join(lines, "\n")
	}

	// Section headers occupy body lines too, so the budget is spent as lines are
	// emitted rather than sliced off the row list up front.
	budget := v.bodyRows()
	section := ""
	for i := v.scrollOffset; i < len(rows) && budget > 0; i++ {
		row := rows[i]
		if row.section != section {
			section = row.section
			lines = append(lines, s.BucketHeader.Render(section))
			budget--
			if budget == 0 {
				break
			}
		}
		lines = append(lines, v.renderRow(s, row, i == v.cursor))
		budget--
	}
	return strings.Join(lines, "\n")
}

func (v InsightsView) renderRow(s styles.Styles, row insightsRow, focused bool) string {
	cursor := "  "
	if focused {
		cursor = "> "
	}
	width := max(v.width-len(cursor), 1)

	var body string
	if v.compact() || row.drill == drillDormant {
		labelW := max(width-len(row.note)-2, 1)
		body = text.Pad(text.Truncate(row.label, labelW), labelW) + "  " + row.note
	} else {
		bars := chart.BarList([]chart.BarRow{{Label: row.label, Value: row.fraction, Note: row.note}},
			chart.BarListOptions{Width: width, Max: 1}, chart.FromStyles(s))
		if len(bars) > 0 {
			body = bars[0]
		}
	}
	if focused {
		// The bar is already styled; strip it so the focus background covers the
		// whole row instead of being punched through by the bar's own colours.
		return s.Accent.Render(cursor + text.Truncate(ansi.Strip(body), width))
	}
	return s.TextTertiary.Render(cursor) + body
}

// formatMonths renders a fractional month count as years and months.
func formatMonths(months float64) string {
	total := int(months + 0.5)
	if total <= 0 {
		return "under a month"
	}
	years, rest := total/12, total%12
	switch {
	case years > 0 && rest > 0:
		return fmt.Sprintf("%d %s %d %s", years, text.Pluralize(years, "year", "years"), rest, text.Pluralize(rest, "month", "months"))
	case years > 0:
		return fmt.Sprintf("%d %s", years, text.Pluralize(years, "year", "years"))
	default:
		return fmt.Sprintf("%d %s", rest, text.Pluralize(rest, "month", "months"))
	}
}
