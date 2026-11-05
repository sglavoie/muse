package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/recap"
	"github.com/sglavoie/muse/internal/tui/chart"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// recapCard builds the current page. Every builder gets the text column's
// width and the body's height and drops its least important lines first.
func (m Model) recapCard(st styles.Styles, now time.Time) recapCard {
	o := m.overlay.recap
	width, height := m.recapBodySize()
	if o.recapFeatureHash() != "" {
		return m.recapCoverCard(m.recapFeatureCover(), st, width, height)
	}
	w := recapPanelWidth(width, 0)
	count := m.recapProgress(now, recapCountDuration)
	r := o.data
	switch o.currentPage() {
	case recapPageTotals:
		return recapCard{lines: recapTotalsLines(r, st, height, count), center: true}
	case recapPageArtists:
		return recapCard{lines: m.recapArtistsLines(r, st, w, height)}
	case recapPageTracks:
		return recapCard{lines: recapTracksLines(r, st, w, height)}
	case recapPageClock:
		return recapCard{lines: recapClockLines(r, st, w, height)}
	case recapPageStreak:
		return recapCard{lines: recapStreakLines(r, st, w, height, count, m.recapNow()), center: true}
	case recapPageGenres:
		return recapCard{lines: recapGenreLines(r, st, w, height)}
	case recapPageSummary:
		return recapCard{lines: recapSummaryLines(r, st, w, height)}
	}
	return recapCard{lines: recapEmptyLines(r, st), center: true}
}

// recapCoverCard builds a page that features one album or song beside its
// cover; feature is nil when the screen has no room for the cover.
func (m Model) recapCoverCard(feature *recapCover, st styles.Styles, width, height int) recapCard {
	o := m.overlay.recap
	coverCols := 0
	if feature != nil {
		coverCols = feature.cols
	}
	w := recapPanelWidth(width, coverCols)
	r := o.data
	var lines []string
	switch o.currentPage() {
	case recapPageAlbum:
		lines = m.recapAlbumLines(r, st, w, height)
	case recapPageDiscovery:
		lines = recapDiscoveryLines(r, st, w, height)
	default:
		lines = recapRepeatLines(r, st, w, height)
	}
	return recapCard{lines: lines, cover: feature}
}

// recapSections joins sections with blank lines, in priority order, keeping
// the first and as many of the rest as fit in height.
func recapSections(height int, sections ...[]string) []string {
	var lines []string
	for i, s := range sections {
		if len(s) == 0 {
			continue
		}
		need := len(s)
		if len(lines) > 0 {
			need++
		}
		if i > 0 && len(lines)+need > height {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, s...)
	}
	return lines[:min(len(lines), height)]
}

func recapCount(n int, singular, plural string) string {
	return text.FormatCount(n) + " " + text.Pluralize(n, singular, plural)
}

// recapScale is n counted up to progress p.
func recapScale(n int, p float64) int {
	return int(math.Round(float64(n) * p))
}

func recapPeriodName(p recap.Period) string {
	if p.Rolling() {
		return "The last 12 months"
	}
	return p.Label()
}

func recapPreviousName(p recap.Period) string {
	if p.Rolling() {
		return "the 12 months before"
	}
	return p.Previous().Label()
}

// recapDate formats a day, leaving the year out when it is the period's.
func recapDate(d time.Time, p recap.Period) string {
	if !p.Rolling() && d.Year() == p.Year {
		return d.Format("Jan 2")
	}
	return d.Format("Jan 2, 2006")
}

func recapEmptyLines(r recap.Recap, st styles.Styles) []string {
	what := "Nothing played in " + r.Period.Label() + " yet"
	if r.Period.Rolling() {
		what = "Nothing played in the last 12 months"
	}
	return []string{
		st.TextPrimary.Bold(true).Render(what),
		"",
		st.Dim.Render("Play some music and your recap fills in."),
		st.Dim.Render("[ and ] switch to another year."),
	}
}

func recapTotalsLines(r recap.Recap, st styles.Styles, height int, p float64) []string {
	t := r.Totals
	head := []string{st.Dim.Render(recapPeriodName(r.Period) + " in muse")}
	big := []string{
		st.AccentMark.Bold(true).Render(text.FormatListeningTime(recapScale(t.Secs, p))),
		st.Dim.Render("of listening"),
	}
	counts := []string{
		st.TextPrimary.Render(recapCount(recapScale(t.Plays, p), "play", "plays") + " · " + recapCount(recapScale(t.Tracks, p), "track", "tracks")),
		st.TextPrimary.Render(recapCount(recapScale(t.Artists, p), "artist", "artists") + " · " + recapCount(recapScale(t.Albums, p), "album", "albums")),
		st.TextSecondary.Render("Music on " + recapCount(recapScale(t.Days, p), "day", "days")),
	}
	return recapSections(height, head, big, counts, []string{recapComparison(r, st)})
}

// recapComparison sets the period's listening time against the one before.
func recapComparison(r recap.Recap, st styles.Styles) string {
	prev := recapPreviousName(r.Period)
	switch {
	case r.Previous.Plays > 0:
	case r.FirstPeriod && r.Period.Rolling():
		return st.TextSecondary.Render("Your first 12 months in muse")
	case r.FirstPeriod:
		return st.TextSecondary.Render("Your first year in muse")
	default:
		return st.Dim.Render("Nothing played in " + prev + " to compare with")
	}
	cur, before := float64(r.Totals.Secs), float64(r.Previous.Secs)
	if before <= 0 {
		cur, before = float64(r.Totals.Plays), float64(r.Previous.Plays)
	}
	change := (cur - before) / before
	pct := int(math.Round(math.Abs(change) * 100))
	switch {
	case pct == 0:
		return st.TextSecondary.Render("About as much as " + prev)
	case change > 0:
		return st.Success.Render(fmt.Sprintf("▲ %d%% more than %s", pct, prev)) + st.Dim.Render(" ("+text.FormatListeningTime(r.Previous.Secs)+")")
	default:
		return st.Warning.Render(fmt.Sprintf("▼ %d%% less than %s", pct, prev)) + st.Dim.Render(" ("+text.FormatListeningTime(r.Previous.Secs)+")")
	}
}

// recapArtistsLines lists the top artists, each beside a thumbnail of their
// most-played album when the screen has room for one.
func (m Model) recapArtistsLines(r recap.Recap, st styles.Styles, width, height int) []string {
	lines := []string{st.Dim.Render("Top artists"), ""}
	rows := m.recapThumbRows()
	covers := map[string]recapCover{}
	for _, c := range m.recapCovers() {
		covers[c.track.Hash] = c
	}
	for i, a := range r.TopArtists {
		name := recapRankName(i, a.Name, st)
		stats := recapCount(a.Plays, "play", "plays") + " · " + text.FormatListeningTime(a.Secs)
		if rows == 0 {
			lines = append(lines, recapRanked(i, st)+name+st.Dim.Render("  "+stats))
			continue
		}
		info := []string{recapRanked(i, st) + name, "   " + st.Dim.Render(stats)}
		if a.Album != "" {
			info = append(info, "   "+st.AlbumText.Render(a.Album))
		}
		if rows == 2 && len(info) == 3 {
			info = []string{info[0], "   " + st.Dim.Render(stats+" · ") + st.AlbumText.Render(a.Album)}
		}
		thumb := make([]string, rows)
		if c, ok := covers[a.Hash]; ok {
			thumb = strings.Split(m.recapCoverArt(c, st), "\n")
		}
		if i > 0 {
			lines = append(lines, "")
		}
		for j := range rows {
			lines = append(lines, recapLineOr(thumb, j, 2*rows)+"  "+recapLineOr(info, j, 0))
		}
	}
	return recapFit(lines, width, height)
}

func recapRanked(i int, st styles.Styles) string {
	return st.TextTertiary.Render(fmt.Sprintf("%d  ", i+1))
}

func recapRankName(i int, name string, st styles.Styles) string {
	if i == 0 {
		return st.AccentMark.Bold(true).Render(name)
	}
	return st.TextPrimary.Bold(true).Render(name)
}

// recapFit truncates every line to width and keeps the first height.
func recapFit(lines []string, width, height int) []string {
	lines = lines[:min(len(lines), height)]
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "…")
	}
	return lines
}

// recapTracksLines lists the top tracks with a bar of their plays against
// the top one, or one line each on a short screen.
func recapTracksLines(r recap.Recap, st styles.Styles, width, height int) []string {
	lines := []string{st.Dim.Render("Top tracks"), ""}
	twoLine := height >= 2+2*len(r.TopTracks)
	top := float64(r.TopTracks[0].Plays)
	for i, t := range r.TopTracks {
		title := recapRanked(i, st) + recapRankName(i, t.Name, st)
		plays := recapCount(t.Plays, "play", "plays")
		if !twoLine {
			lines = append(lines, title+st.Dim.Render(" · "+t.Artist+"  "+plays))
			continue
		}
		lines = append(lines, title+st.Dim.Render(" · "+t.Artist))
		bar := max(4, width-3-ansi.StringWidth(plays)-1)
		lines = append(lines, "   "+chart.BarTrack(float64(t.Plays)/top, bar, chart.FromStyles(st))+" "+st.TextSecondary.Render(plays))
	}
	return recapFit(lines, width, height)
}

// recapFeatureLines is a cover page's text: a kicker, the featured name, and
// what made it stand out.
func recapFeatureLines(st styles.Styles, width, height int, kicker, name, artist string, details ...string) []string {
	head := []string{st.Dim.Render(kicker)}
	title := []string{}
	for _, l := range wrapRecap(name, width, 2) {
		title = append(title, st.AccentMark.Bold(true).Render(l))
	}
	if artist != "" {
		title = append(title, st.TextPrimary.Render(text.Truncate(artist, width)))
	}
	about := make([]string, 0, len(details))
	for _, d := range details {
		for _, l := range wrapRecap(d, width, 2) {
			if l != "" {
				about = append(about, st.TextSecondary.Render(l))
			}
		}
	}
	return recapSections(height, head, title, about)
}

func wrapRecap(value string, width, limit int) []string {
	if width <= 0 {
		return nil
	}
	lines := strings.Split(ansi.Wrap(value, width, " "), "\n")
	if len(lines) > limit {
		lines[limit-1] = text.Truncate(strings.Join(lines[limit-1:], " "), width)
		lines = lines[:limit]
	}
	return lines
}

func (m Model) recapAlbumLines(r recap.Recap, st styles.Styles, width, height int) []string {
	a := r.TopAlbum
	kicker := "Album of the year"
	if r.Period.Rolling() {
		kicker = "Album of the last 12 months"
	}
	most := ""
	if t, ok := m.library.Index.FindByHash(a.Hash); ok && t.Title != "" {
		most = "Most played: " + t.Title
	}
	return recapFeatureLines(st, width, height, kicker, a.Name, a.Artist,
		recapCount(a.Plays, "play", "plays")+" · "+text.FormatListeningTime(a.Secs), most)
}

func recapDiscoveryLines(r recap.Recap, st styles.Styles, width, height int) []string {
	d := r.Discovery
	others := ""
	if d.NewTracks > 1 {
		others = "One of " + recapCount(d.NewTracks, "track", "tracks") + " you heard for the first time"
	}
	return recapFeatureLines(st, width, height, "Biggest discovery", d.Name, d.Artist,
		"First heard "+recapDate(d.FirstHeard, r.Period)+" · "+recapCount(d.Plays, "play", "plays")+" since", others)
}

func recapRepeatLines(r recap.Recap, st styles.Styles, width, height int) []string {
	d := r.RepeatDay
	return recapFeatureLines(st, width, height, "On repeat · "+d.Date.Format("Monday, Jan 2, 2006"), d.Name, d.Artist,
		fmt.Sprintf("Played %d times that day", d.Plays))
}

// recapClockLines draws the listening clock: plays by hour of the day as
// columns, then by weekday as bars when there is room.
func recapClockLines(r recap.Recap, st styles.Styles, width, height int) []string {
	c := r.Clock
	head := []string{
		st.Dim.Render("Listening clock"),
		"",
		st.TextPrimary.Render(fmt.Sprintf("Most at %02d:00", c.PeakHour)) + st.Dim.Render(" · ") +
			st.TextPrimary.Render("busiest on "+c.PeakWeekday.String()+"s"),
	}
	p := chart.FromStyles(st)
	colW := max(1, min(3, width/24))
	// The hour columns need a blank row, at least two rows and their scale;
	// the weekday bars a blank row and seven more.
	room := height - len(head)
	withWeek := room >= 2+2+8
	chartHeight := min(6, room-2)
	if withWeek {
		chartHeight = min(6, room-2-8)
	}
	var hours []string
	if chartHeight >= 2 {
		bars := make([]chart.VBar, 24)
		for h, n := range c.Hours {
			bars[h] = chart.VBar{Value: float64(n)}
		}
		hours = chart.VBarChart(bars, chart.VBarOptions{Width: 24 * colW, Height: chartHeight, ColWidth: colW}, p)
		hours = append(hours, chart.Scale([]string{"0:00", "6:00", "12:00", "18:00", "23:00"}, 24*colW, p))
	}
	var week []string
	if withWeek {
		rows := make([]chart.BarRow, 0, 7)
		for _, d := range recap.MondayFirst {
			rows = append(rows, chart.BarRow{Label: d.String()[:3], Value: float64(c.Weekdays[d]), Note: text.FormatCount(c.Weekdays[d])})
		}
		week = chart.BarList(rows, chart.BarListOptions{Width: min(width, 24*colW)}, p)
	}
	return recapSections(height, head, hours, week)
}

// recapStreakLines shows the longest run of listening days and, with room,
// the whole period as a calendar.
func recapStreakLines(r recap.Recap, st styles.Styles, width, height int, p float64, now time.Time) []string {
	s := r.Streak
	head := []string{st.Dim.Render("Longest streak")}
	big := []string{
		st.AccentMark.Bold(true).Render(recapCount(recapScale(s.Days, p), "day", "days") + " in a row"),
		st.TextSecondary.Render(recapDate(s.Start, r.Period) + " – " + recapDate(s.End, r.Period)),
	}
	loc := r.Period.Start.Location()
	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	cells := make([]chart.HeatCell, len(r.Daily))
	elapsed := 0
	for i, d := range r.Daily {
		present := !d.Date.After(today)
		if present {
			elapsed++
		}
		cells[i] = chart.HeatCell{Value: float64(d.Plays), Present: present}
	}
	share := []string{st.Dim.Render(fmt.Sprintf("Music on %s of %s", recapCount(r.Totals.Days, "day", "days"), text.FormatCount(elapsed)))}
	var cal []string
	if len(cells) > 0 {
		weeks := (int(r.Daily[0].Date.Weekday()) + len(cells) + 6) / 7
		cal = chart.Calendar(cells, chart.CalendarOptions{
			Width:        min(width, 4+weeks),
			FirstWeekday: r.Daily[0].Date.Weekday(),
		}, chart.FromStyles(st))
	}
	return recapSections(height, head, big, share, cal)
}

// recapGenreLines shares the period out by genre.
func recapGenreLines(r recap.Recap, st styles.Styles, width, height int) []string {
	rows := make([]chart.BarRow, len(r.Genres))
	for i, g := range r.Genres {
		rows[i] = chart.BarRow{Label: g.Name, Value: g.Share, Note: fmt.Sprintf("%d%%", int(math.Round(g.Share*100)))}
	}
	bars := chart.BarList(rows, chart.BarListOptions{Width: min(width, 48), Max: r.Genres[0].Share}, chart.FromStyles(st))
	return recapSections(height, []string{st.Dim.Render("Genre mix")}, bars)
}

// recapSummaryLines fits the whole story on one screen.
func recapSummaryLines(r recap.Recap, st styles.Styles, width, height int) []string {
	t := r.Totals
	head := []string{
		st.AccentMark.Bold(true).Render(recapPeriodName(r.Period) + " in muse"),
		st.TextPrimary.Render(text.FormatListeningTime(t.Secs) + " · " + recapCount(t.Plays, "play", "plays") + " · " +
			recapCount(t.Tracks, "track", "tracks") + " · " + recapCount(t.Artists, "artist", "artists")),
	}
	rows := recapSummaryRows(r)
	labelW := 0
	for _, row := range rows {
		labelW = max(labelW, ansi.StringWidth(row[0]))
	}
	body := make([]string, 0, len(rows))
	for _, row := range rows {
		label := st.Dim.Render(text.Pad(row[0], labelW+2))
		body = append(body, label+st.TextPrimary.Render(text.Truncate(row[1], max(width-labelW-2, 1))))
	}
	save := []string{st.Dim.Render("S saves your top " + recapCount(len(r.Playlist), "track", "tracks") + " as a playlist")}
	if len(head)+len(body) >= height {
		return recapFit(append(head, body...), width, height)
	}
	return recapFit(recapSections(height, head, body, save), width, height)
}

// recapSummaryRows are the summary's label/value pairs, most important
// first, leaving out what the period has nothing for.
func recapSummaryRows(r recap.Recap) [][2]string {
	var rows [][2]string
	add := func(label, value string) {
		if value != "" {
			rows = append(rows, [2]string{label, value})
		}
	}
	if len(r.TopArtists) > 0 {
		add("Top artist", r.TopArtists[0].Name)
	}
	if len(r.TopTracks) > 0 {
		add("Top track", recapTrackName(r.TopTracks[0]))
	}
	add("Top album", r.TopAlbum.Name)
	add("Prime time", fmt.Sprintf("%02d:00 · %ss", r.Clock.PeakHour, r.Clock.PeakWeekday))
	if r.Streak.Days > 0 {
		add("Streak", recapCount(r.Streak.Days, "day", "days"))
	}
	if r.Discovery.Hash != "" {
		add("Discovery", recapTrackName(r.Discovery.Entry))
	}
	if r.RepeatDay.Hash != "" {
		add("On repeat", fmt.Sprintf("%s ×%d", r.RepeatDay.Name, r.RepeatDay.Plays))
	}
	var genres []string
	for _, g := range r.Genres[:min(3, len(r.Genres))] {
		genres = append(genres, fmt.Sprintf("%s %d%%", g.Name, int(math.Round(g.Share*100))))
	}
	add("Genres", strings.Join(genres, " · "))
	return rows
}

func recapTrackName(e recap.Entry) string {
	if e.Artist == "" {
		return e.Name
	}
	return e.Name + " · " + e.Artist
}
