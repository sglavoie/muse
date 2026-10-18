package insights

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/chart"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Location is the zone every row buckets in. It falls back to the load path's
// own default rather than to UTC, so a view built without a snapshot still
// files a play under the hour the user heard it.
func (v View) Location() *time.Location {
	if v.Loc == nil {
		return time.Local
	}
	return v.Loc
}

// coverageRows builds the Coverage body: dusty shelves, albums nobody has ever
// opened, per-genre coverage, and tracks that were heard once and then dropped.
func (v View) coverageRows() []Row {
	c := v.Snap.Coverage
	rows := make([]Row, 0, 40)
	albumKeys := make([]string, 0, len(c.DustyAlbums))
	for _, album := range c.DustyAlbums {
		albumKeys = append(albumKeys, album.Name)
	}
	albumLabels := library.AlbumDisplayLabels(albumKeys)

	var untouched, dusty []insightsdata.ShelfStat
	for _, a := range c.DustyAlbums {
		if a.Unheard == a.Tracks {
			untouched = append(untouched, a)
		} else {
			dusty = append(dusty, a)
		}
	}

	appendShelves := func(section string, shelves []insightsdata.ShelfStat, kind DrillKind) {
		for _, s := range shelves {
			label := s.Name
			if kind == DrillUnheardAlbum {
				label = albumLabels[s.Name]
			}
			rows = append(rows, Row{
				Section:  section,
				Label:    label,
				note:     fmt.Sprintf("%d/%d unheard", s.Unheard, s.Tracks),
				fraction: shelfFraction(s),
				Drill:    kind,
				Key:      s.Name,
			})
		}
	}

	appendShelves("Dusty artists", c.DustyArtists, DrillUnheardArtist)
	appendShelves("Untouched albums", untouched, DrillUnheardAlbum)
	appendShelves("Dusty albums", dusty, DrillUnheardAlbum)
	appendShelves("Genre coverage", v.GenreCoverage, DrillUnheardGenre)

	for _, t := range v.Dormant {
		rows = append(rows, Row{
			Section: "Dormant tracks",
			Label:   fmt.Sprintf("%s — %s", t.Artist, t.Title),
			note:    "last " + relativeDayLabel(t.LastPlayedAt, v.Snap.LastPlayAt),
			Drill:   DrillDormant,
			Key:     t.Hash,
		})
	}
	return rows
}

func shelfFraction(s insightsdata.ShelfStat) float64 {
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

// Render draws the Insights view: the briefing plus the active panel at level
// 0, a session list at level 2, and an embedded track list at the two levels
// that end in tracks.
func (v View) Render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	if v.AtTracks() {
		if len(v.TrackView.SourceTracks) == 0 {
			return s.Dim.Render("Nothing behind this row.")
		}
		return v.TrackView.Render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
	}
	if v.Level == LevelSessions {
		return v.renderSessionList(s)
	}

	if !v.Loaded {
		if v.Loading {
			return s.Dim.Render("Reading your listening history…")
		}
		if v.LoadErr != nil {
			return s.RenderEmptyCard(v.width, v.Height, "Could not read your listening history", []styles.EmptyAction{
				{Key: "8", Label: "retry"},
				{Key: "?", Label: "help"},
			})
		}
		return s.Dim.Render("Insights are not available: no history reader is wired.")
	}
	if v.Empty() {
		return s.RenderEmptyCard(v.width, v.Height, "Nothing to measure yet", []styles.EmptyAction{
			{Key: "1", Label: "go to library"},
			{Key: "?", Label: "help"},
		})
	}

	var b strings.Builder
	b.WriteString(v.renderStrip(s))
	b.WriteString("\n")
	if v.briefingFits() {
		b.WriteString(v.renderBriefing(s))
	} else {
		b.WriteString(v.renderPanelSelector(s))
	}
	b.WriteString("\n")
	b.WriteString(v.renderPanel(s))

	// The header block has a floor it cannot shrink past, so at extreme heights
	// it can still outgrow the frame. Clamp rather than let it push the footer
	// off the screen.
	out := strings.Split(b.String(), "\n")
	if v.Height > 0 && len(out) > v.Height {
		out = out[:v.Height]
	}
	return strings.Join(out, "\n")
}

func (v View) briefingFits() bool {
	return v.Height >= insightsBriefingMinHeight
}

// headerHeight counts every line drawn above the panel's scrollable rows.
func (v View) headerHeight() int {
	h := 1 // state strip
	if v.briefingFits() {
		h += len(insightsPanelSpecs)
	} else {
		h++
	}
	// Rendering the header with a zero Styles yields the same line count as the
	// styled render, so the two can never drift apart.
	h += len(v.PanelHeaderLines(styles.Styles{}))
	return h
}

func (v View) renderStrip(s styles.Styles) string {
	spec := v.Spec()
	summary := fmt.Sprintf("%s %s · %s · %d %s",
		text.FormatCount(v.Snap.TotalPlays), text.Pluralize(v.Snap.TotalPlays, "play", "plays"),
		text.FormatListeningTime(v.Snap.TotalSecs),
		v.Snap.SpanDays, text.Pluralize(v.Snap.SpanDays, "day", "days"))
	line := strip.Render(v.width, []string{"Insights", summary},
		strip.SortChunk{}, strip.FilterChunk{}, strip.CountsChunk{}, nil,
		v.WindowHint(), s)
	if line == "" {
		return spec.Name
	}
	return line
}

// WindowHint is the right-edge affordance that names the active window and,
// for panels that do not honour it, says so instead of silently ignoring w.
func (v View) WindowHint() string {
	// A failed read leaves every panel on figures that are not current, so the
	// strip says so rather than letting the window label imply they match it.
	// The hint is a "key label" pair, so the retry key leads.
	if v.LoadErr != nil {
		return "8 retry · read failed"
	}
	switch v.Spec().Class {
	case ClassFixed:
		return "all-time (fixed)"
	case ClassResolution:
		return "resolution: " + v.Resolution().Unit() + " ‹w›"
	default:
		return v.window().Label + " ‹w›"
	}
}

func (v View) renderBriefing(s styles.Styles) string {
	lines := make([]string, 0, len(insightsPanelSpecs))
	labelW := 0
	for _, spec := range insightsPanelSpecs {
		labelW = max(labelW, len(spec.Name))
	}
	for _, spec := range insightsPanelSpecs {
		label := spec.Name
		if spec.id == v.Panel {
			label = "[" + label + "]"
		}
		head := text.Pad(label, labelW+2) + "  "
		tail := v.headline(spec.id)
		if v.width > 0 {
			tail = text.Truncate(tail, max(v.width-len(head), 1))
		}
		if spec.id == v.Panel {
			lines = append(lines, s.Accent.Render(head+tail))
		} else {
			lines = append(lines, s.TextPrimary.Render(head)+s.TextTertiary.Render(tail))
		}
	}
	return strings.Join(lines, "\n")
}

// headline is the one-line answer a panel gives. Every headline is computed
// from live data even when the panel's expanded body is not built yet.
func (v View) headline(p Panel) string {
	snap := v.Snap
	switch p {
	case PanelCoverage:
		c := snap.Coverage
		return fmt.Sprintf("%s of %s tracks never played · %s unheard",
			text.FormatCount(c.UnheardTracks), text.FormatCount(c.TotalTracks),
			text.FormatListeningTime(int(c.UnheardSecs)))
	case panelRhythm:
		r := snap.Rhythm
		if r.TotalPlays == 0 {
			return "no plays in this window"
		}
		return fmt.Sprintf("%ss %.0f%% of plays · peak %02d:00 · %d quiet %s",
			r.TopWeekday.Weekday, r.TopWeekday.Share*100, r.PeakHour,
			len(r.QuietHours), text.Pluralize(len(r.QuietHours), "hour", "hours"))
	case PanelMomentum:
		t := snap.Trends
		if t.AvgSecs <= 0 {
			return "not enough completed " + t.Resolution.Period() + "s to compare"
		}
		return fmt.Sprintf("%s %s — %.0f%% of your %s %s average",
			text.FormatListeningTime(t.Current.ListenedSecs), t.Resolution.Current(),
			t.CurrentVsAvg*100, text.FormatListeningTime(int(t.AvgSecs)), t.Resolution.Unit())
	case PanelConcentration:
		l := snap.Rankings.Ladder
		if l.TotalArtists == 0 {
			return "no artists in this window"
		}
		return fmt.Sprintf("%d %s = half your listening time · top-5 %.0f%%",
			l.Artists50, text.Pluralize(l.Artists50, "artist", "artists"),
			snap.Rankings.TopFiveShare*100)
	case PanelTimeVsPlays:
		r := snap.Rankings
		if len(r.Deltas) == 0 {
			return "no artists in this window"
		}
		// Deltas covers the compared head of the two orderings, not every artist:
		// "moves 9 of your top 15" is only true when 15 is the size of that head.
		return fmt.Sprintf("ranking by hours moves %d of your top %d artists",
			r.MovedCount, len(r.Deltas))
	case PanelSessions:
		ss := snap.SessionStats
		if ss.Count == 0 {
			return "no sessions in this window"
		}
		return fmt.Sprintf("%d %s · median %s · %.0f%% single-artist",
			ss.Count, text.Pluralize(ss.Count, "session", "sessions"),
			text.FormatListeningTime(ss.MedianSecs), ss.SingleArtistShare*100)
	case PanelDiscovery:
		t := snap.Trends
		return fmt.Sprintf("%d first-time %s %s · %.0f%% are repeats",
			t.CurrentFirstPlays, text.Pluralize(t.CurrentFirstPlays, "track", "tracks"),
			t.Resolution.Current(), t.RepeatShare*100)
	case PanelDataHealth:
		// The count is of rows rather than of snapshot issues: a finding with no
		// track list behind it is stated in the header prose instead, and a
		// headline that counted those would promise rows that are not there.
		switch n := len(v.Health); {
		case n == 0 && len(snap.Quality.Issues) == 0:
			return "nothing distorting the panels above"
		case n == 0:
			return snap.Quality.Issues[0].Detail
		default:
			return fmt.Sprintf("%d %s · %s %s affected",
				n, text.Pluralize(n, "finding", "findings"),
				text.FormatCount(v.healthAffectedTracks()),
				text.Pluralize(v.healthAffectedTracks(), "track", "tracks"))
		}
	}
	return ""
}

// PanelHeaderLines is the static block a panel draws above its scrollable
// rows, trimmed so it can never crowd the body out of a short terminal.
func (v View) PanelHeaderLines(s styles.Styles) []string {
	var lines []string
	switch v.Panel {
	case PanelCoverage:
		lines = v.coverageHeaderLines(s)
	case panelRhythm:
		lines = v.rhythmHeaderLines(s)
	case PanelMomentum:
		lines = v.momentumHeaderLines(s)
	case PanelConcentration:
		lines = v.concentrationHeaderLines(s)
	case PanelTimeVsPlays:
		lines = v.timeVsPlaysHeaderLines(s)
	case PanelSessions:
		lines = v.sessionsHeaderLines(s)
	case PanelDiscovery:
		lines = v.discoveryHeaderLines(s)
	case PanelDataHealth:
		lines = v.dataHealthHeaderLines(s)
	default:
		lines = []string{"", s.Dim.Render(v.headline(v.Panel))}
	}
	if b := v.headerBudget(); len(lines) > b {
		lines = lines[:b]
	}
	return lines
}

// headerBudget is how many lines a panel header may occupy. A header that eats
// the whole frame turns a drillable panel into a picture, so the body's floor
// is reserved before the header gets anything.
func (v View) headerBudget() int {
	used := 1 // state strip
	if v.briefingFits() {
		used += len(insightsPanelSpecs)
	} else {
		used++
	}
	return max(v.Height-used-insightsMinBodyRows, insightsMinHeaderLines)
}

func (v View) coverageHeaderLines(s styles.Styles) []string {
	c := v.Snap.Coverage
	w := max(v.width, 1)
	pal := chart.FromStyles(s)

	lines := []string{
		"",
		s.TextPrimary.Render(fmt.Sprintf("In %d %s you played %s %s. You own %s.",
			v.Snap.SpanDays, text.Pluralize(v.Snap.SpanDays, "day", "days"),
			text.FormatCount(c.HeardTracks), text.Pluralize(c.HeardTracks, "track", "tracks"),
			text.FormatCount(c.TotalTracks))),
		"",
	}
	if !v.compact() {
		lines = append(lines, chart.Ratio(float64(c.HeardTracks), float64(c.TotalTracks), w, pal))
	}
	lines = append(lines,
		s.TextSecondary.Render(fmt.Sprintf("heard %s · unheard %s · %s unheard",
			text.FormatCount(c.HeardTracks), text.FormatCount(c.UnheardTracks),
			text.FormatListeningTime(int(c.UnheardSecs)))),
		"",
	)
	if v.Height >= insightsProseMinHeight {
		for _, l := range strings.Split(ansi.Wrap(v.coverageReadLine(), w, " "), "\n") {
			lines = append(lines, s.Dim.Render(l))
		}
		lines = append(lines, "")
	}
	return lines
}

// coverageReadLine is the cross-reference that makes Coverage land: the size of
// the unheard part measured against everything actually listened to.
func (v View) coverageReadLine() string {
	c := v.Snap.Coverage
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

func (v View) renderPanel(s styles.Styles) string {
	lines := append([]string(nil), v.PanelHeaderLines(s)...)

	rows := v.Rows()
	if len(rows) == 0 {
		if v.Panel == PanelCoverage {
			lines = append(lines, s.Dim.Render("Every shelf in your library has been opened."))
		}
		return strings.Join(lines, "\n")
	}

	// Section headers occupy body lines too, so the budget is spent as lines are
	// emitted rather than sliced off the row list up front.
	budget := v.BodyRows()
	section := ""
	for i := v.ScrollOffset; i < len(rows) && budget > 0; i++ {
		row := rows[i]
		if row.Section != section {
			if budget < 2 {
				break // Keep each heading with its first row.
			}
			section = row.Section
			lines = append(lines, s.BucketHeader.Render(section))
			budget--
			if budget == 0 {
				break
			}
		}
		lines = append(lines, v.renderRow(s, row, i == v.Cursor))
		budget--
	}
	return strings.Join(lines, "\n")
}

func (v View) renderRow(s styles.Styles, row Row, focused bool) string {
	cursor := text.FocusMarker(focused)
	width := max(v.width-2, 1)

	var body string
	switch {
	case row.Drill == DrillHealth:
		note := row.note
		if index, err := strconv.Atoi(row.Key); err == nil && index >= 0 && index < len(v.Health) && width >= 78 {
			note += " · " + v.Health[index].ActionHint()
		}
		labelW := max(width-ansi.StringWidth(note)-2, 1)
		body = text.Pad(text.Truncate(row.Label, labelW), labelW) + "  " + note
	case v.compact() || row.Drill == DrillDormant:
		labelW := max(width-len(row.note)-2, 1)
		body = text.Pad(text.Truncate(row.Label, labelW), labelW) + "  " + row.note
	default:
		bars := chart.BarList([]chart.BarRow{{Label: row.Label, Value: row.fraction, Note: row.note}},
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

// renderPanelSelector leaves the scrollable content room in ordinary terminals.
func (v View) renderPanelSelector(s styles.Styles) string {
	position := 1
	for i, spec := range insightsPanelSpecs {
		if spec.id == v.Panel {
			position = i + 1
			break
		}
	}
	label := fmt.Sprintf("‹ %s · %d/%d ›  `/~ panel", v.Spec().Name, position, len(insightsPanelSpecs))
	return s.Heading.Render(text.Truncate(label, v.width))
}
