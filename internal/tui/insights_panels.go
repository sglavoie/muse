package tui

// Rhythm, Momentum and Discovery: the time-series family. All three answer a
// "when" question, and all three obey the same rule as Coverage — every row is
// a query with a shape, so each one names the tracks that enter opens.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/chart"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

const (
	// insightsPunchcardLabelW is the gutter the Rhythm punchcard leaves empty so
	// its hour columns sit directly above the genre grid's.
	insightsPunchcardLabelW = 10
	// insightsGenreRows caps both the affinity grid and its row list: past a
	// handful of genres the grid stops being readable at a glance.
	insightsGenreRows = 5
	// insightsMonthRows caps the month lists so a long history stays a briefing.
	insightsMonthRows = 24
	// insightsPunchcardHours is the number of columns in a day.
	insightsPunchcardHours = 24
)

// insightsGenreHours is one genre's play count per local hour: the row the
// Rhythm affinity grid draws and the shape "you only reach for jazz after
// 22:00" is visible in.
type insightsGenreHours struct {
	Genre string
	Hours [insightsPunchcardHours]int
}

// insightsBacklogMonth counts the never-played tracks that entered the library
// in one local month, so Discovery can say what the backlog is made of rather
// than only how big it is.
type insightsBacklogMonth struct {
	Key    string // "2006-01", the drill key
	Label  string
	Tracks int
	Secs   float64
}

func insightsFraction(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return float64(part) / float64(whole)
}

// insightsMonthKey is the bucket key every month row drills on. It is the
// sortable form so a string compare and a chronological compare agree.
func insightsMonthKey(t time.Time) string { return t.Format("2006-01") }

// fit clamps a prose line to the view width. Chart output is already exactly as
// wide as it was asked to be; a summary sentence is not, and one overlong line
// is enough to overflow the frame.
func (v InsightsView) fit(line string) string {
	if v.width <= 0 {
		return line
	}
	return text.Truncate(line, v.width)
}

func insightsPlaysNote(plays, secs int) string {
	return fmt.Sprintf("%d %s · %s", plays, pluralize(plays, "play", "plays"), formatListeningTime(secs))
}

// insightsFitHeader assembles a panel header from a mandatory prologue and
// epilogue plus optional chart blocks. Blocks are listed most important first
// and taken whole — never half a chart — until one does not fit, at which point
// the rest go too: keeping a lesser block after dropping a greater one would
// invert the order the caller put them in.
func insightsFitHeader(budget int, head []string, blocks [][]string, tail []string) []string {
	out := append([]string(nil), head...)
	used := len(head) + len(tail)
	for _, b := range blocks {
		if used+len(b) > budget {
			break
		}
		out = append(out, b...)
		used += len(b)
	}
	return append(out, tail...)
}

// --- Rhythm ------------------------------------------------------------------

func (v InsightsView) rhythmRows() []insightsRow {
	r := v.snap.Rhythm
	rows := make([]insightsRow, 0, insightsPunchcardHours+7+insightsGenreRows)

	peakHour := 0
	for _, h := range r.Hours {
		peakHour = max(peakHour, h.Plays)
	}
	for _, h := range r.Hours {
		if h.Plays == 0 {
			continue
		}
		rows = append(rows, insightsRow{
			section:  "Hour of day",
			label:    fmt.Sprintf("%02d:00", h.Hour),
			note:     insightsPlaysNote(h.Plays, h.ListenedSecs),
			fraction: insightsFraction(h.Plays, peakHour),
			drill:    drillHour,
			key:      strconv.Itoa(h.Hour),
		})
	}

	peakDay := 0
	for _, d := range r.Weekdays {
		peakDay = max(peakDay, d.Plays)
	}
	// Every weekday is listed, silent ones included: a dead Sunday is the most
	// interesting thing the weekday breakdown has to say, and dropping the empty
	// rows would hide exactly that.
	for _, d := range r.Weekdays {
		note := "silent"
		if d.Plays > 0 {
			note = fmt.Sprintf("%d %s · %.0f%%", d.Plays, pluralize(d.Plays, "play", "plays"), d.Share*100)
		}
		rows = append(rows, insightsRow{
			section:  "Weekday",
			label:    d.Weekday.String(),
			note:     note,
			fraction: insightsFraction(d.Plays, peakDay),
			drill:    drillWeekday,
			key:      strconv.Itoa(int(d.Weekday)),
		})
	}

	// The affinity rows stay hidden while genre spellings collide: an
	// "Ambient"/"ambient" split would show one shelf as two rows, and the drill
	// behind each would understate what was actually played.
	if !r.GenresUnclean {
		for i, g := range r.Genres {
			if i >= insightsGenreRows {
				break
			}
			rows = append(rows, insightsRow{
				section:  "Genre affinity",
				label:    g.Canonical,
				note:     insightsPlaysNote(g.Plays, g.ListenedSecs),
				fraction: insightsFraction(g.Plays, r.TotalPlays),
				drill:    drillRhythmGenre,
				key:      g.Canonical,
			})
		}
	}
	return rows
}

func (v InsightsView) rhythmHeaderLines(s styles.Styles) []string {
	r := v.snap.Rhythm
	head := []string{"", s.Dim.Render(v.headline(panelRhythm)), ""}
	if r.TotalPlays == 0 {
		return head
	}

	// The headline already names the peak hour and the busiest weekday, so the
	// summary takes the other end: what the punchcard's gaps are made of.
	tail := []string{s.TextSecondary.Render(v.fit(fmt.Sprintf(
		"%d %s in rotation · quietest %s with %d %s",
		r.DistinctTracks, pluralize(r.DistinctTracks, "track", "tracks"),
		r.BottomWeekday.Weekday, r.BottomWeekday.Plays,
		pluralize(r.BottomWeekday.Plays, "play", "plays"))))}
	// The banner is part of the tail rather than of the chart block because the
	// gate outlives the grid: at compact widths there is no grid to hide, but the
	// affinity rows are withheld just the same and the reason still has to show.
	if r.GenresUnclean {
		tail = append(tail, s.Warning.Render(v.fit(
			"Genre affinity hidden: genre names collide (Data Health)")))
	}
	tail = append(tail, "")

	if v.compact() {
		return append(head, tail...)
	}

	pal := chart.FromStyles(s)
	gutter := insightsPunchcardLabelW
	colW := max((max(v.width, 1)-gutter-1)/insightsPunchcardHours, 1)
	gridW := colW * insightsPunchcardHours
	pad := strings.Repeat(" ", gutter+1)

	bars := make([]chart.VBar, insightsPunchcardHours)
	for i, h := range r.Hours {
		bars[i] = chart.VBar{Value: float64(h.Plays)}
	}
	punchcard := make([]string, 0, 5)
	for _, line := range chart.VBarChart(bars, chart.VBarOptions{Width: gridW, Height: 3, ColWidth: colW}, pal) {
		punchcard = append(punchcard, pad+line)
	}
	punchcard = append(punchcard,
		pad+chart.Scale([]string{"00", "06", "12", "18", "23"}, gridW, pal), "")

	var affinity []string
	if !r.GenresUnclean && len(v.genreHours) > 0 {
		grid := make([]chart.HeatRow, 0, insightsGenreRows)
		for i, g := range v.genreHours {
			if i >= insightsGenreRows {
				break
			}
			cells := make([]chart.HeatCell, insightsPunchcardHours)
			for h, n := range g.Hours {
				cells[h] = chart.HeatCell{Value: float64(n), Present: true}
			}
			grid = append(grid, chart.HeatRow{Label: g.Genre, Cells: cells})
		}
		affinity = append(affinity, chart.Heatmap(grid, chart.HeatmapOptions{
			Width:      gutter + 1 + gridW,
			LabelWidth: gutter,
			CellWidth:  colW,
		}, pal)...)
		affinity = append(affinity, "")
	}

	return insightsFitHeader(v.headerBudget(), head, [][]string{punchcard, affinity}, tail)
}

// --- Momentum ----------------------------------------------------------------

func (v InsightsView) momentumRows() []insightsRow {
	months := v.snap.Trends.Months
	peak := 0
	for _, b := range months {
		peak = max(peak, b.ListenedSecs)
	}

	rows := make([]insightsRow, 0, min(len(months), insightsMonthRows))
	// Newest first: the question Momentum answers is about the recent end, and
	// putting it on top means the answer needs no scrolling.
	for i := len(months) - 1; i >= 0 && len(rows) < insightsMonthRows; i-- {
		b := months[i]
		note := "silent"
		if b.Plays > 0 {
			note = insightsPlaysNote(b.Plays, b.ListenedSecs)
		}
		rows = append(rows, insightsRow{
			section:  "Month by month",
			label:    fmt.Sprintf("%s %d", b.Month, b.Year),
			note:     note,
			fraction: insightsFraction(b.ListenedSecs, peak),
			drill:    drillMonth,
			key:      insightsMonthKey(time.Date(b.Year, b.Month, 1, 0, 0, 0, 0, v.location())),
		})
	}
	return rows
}

func (v InsightsView) momentumHeaderLines(s styles.Styles) []string {
	t := v.snap.Trends
	head := []string{"", s.Dim.Render(v.headline(panelMomentum)), ""}
	if len(t.Months) == 0 {
		return head
	}

	pal := chart.FromStyles(s)
	w := max(v.width, 1)

	// The headline already compares this month to the average, so the summary
	// names the high-water mark instead — the month a dry spell is measured off.
	best := t.Months[0]
	for _, b := range t.Months {
		if b.ListenedSecs > best.ListenedSecs {
			best = b
		}
	}
	summary := fmt.Sprintf("best month %s %d at %s · enter replays a month in order",
		best.Month, best.Year, formatListeningTime(best.ListenedSecs))
	if t.AvgMonthlySecs > 0 {
		summary = chart.Trend(t.CurrentVsAvg-1, pal) + " " + summary
	}
	tail := []string{s.TextSecondary.Render(v.fit(summary))}
	if t.LongestDry.Days > 0 {
		tail = append(tail, s.Dim.Render(v.fit(fmt.Sprintf("longest silence %d days, %s – %s · %d dry %s in all",
			t.LongestDry.Days, t.LongestDry.Start.Format("2 Jan 2006"), t.LongestDry.End.Format("2 Jan 2006"),
			len(t.DrySpells), pluralize(len(t.DrySpells), "spell", "spells")))))
	}
	tail = append(tail, "")

	if v.compact() {
		return append(head, tail...)
	}

	bars := make([]chart.VBar, 0, len(t.Months))
	for i, b := range t.Months {
		bars = append(bars, chart.VBar{
			Label: b.Month.String()[:3],
			Value: float64(b.ListenedSecs),
			// The current month is still filling up, so it is drawn muted rather
			// than read as a finished month that came up short.
			Muted: i == len(t.Months)-1,
		})
	}
	monthly := chart.VBarChart(bars, chart.VBarOptions{Width: w, Height: 4}, pal)
	monthly = append(monthly, "")

	// The calendar is where a dry spell stops being a statistic: the run of
	// blank cells is the silence itself.
	var calendar []string
	if days := v.snap.Calendar.Days; len(days) > 0 {
		cells := make([]chart.HeatCell, len(days))
		for i, d := range days {
			cells[i] = chart.HeatCell{Value: float64(d.Plays), Present: true}
		}
		calendar = chart.Calendar(cells, chart.CalendarOptions{
			Width:        w,
			FirstWeekday: days[0].Date.Weekday(),
		}, pal)
		calendar = append(calendar, "")
	}

	return insightsFitHeader(v.headerBudget(), head, [][]string{monthly, calendar}, tail)
}

// --- Discovery ---------------------------------------------------------------

func (v InsightsView) discoveryRows() []insightsRow {
	months := v.snap.Trends.Months
	peak := 0
	for _, b := range months {
		peak = max(peak, b.FirstPlays)
	}

	rows := make([]insightsRow, 0, insightsMonthRows+len(v.backlog)+1)
	for i := len(months) - 1; i >= 0 && len(rows) < insightsMonthRows; i-- {
		b := months[i]
		if b.FirstPlays == 0 {
			continue
		}
		rows = append(rows, insightsRow{
			section: "First heard",
			label:   fmt.Sprintf("%s %d", b.Month, b.Year),
			note: fmt.Sprintf("%d new %s", b.FirstPlays,
				pluralize(b.FirstPlays, "track", "tracks")),
			fraction: insightsFraction(b.FirstPlays, peak),
			drill:    drillFirstPlaysMonth,
			key:      insightsMonthKey(time.Date(b.Year, b.Month, 1, 0, 0, 0, 0, v.location())),
		})
	}

	// The backlog leads with its total: the per-month rows below only cover
	// tracks whose add date was recorded, so the whole set needs a row of its
	// own to stay reachable.
	c := v.snap.Coverage
	if c.UnheardTracks > 0 {
		rows = append(rows, insightsRow{
			section: "Added but never played",
			label:   "Everything unheard",
			note: fmt.Sprintf("%s %s · %s", formatCount(c.UnheardTracks),
				pluralize(c.UnheardTracks, "track", "tracks"), formatListeningTime(int(c.UnheardSecs))),
			fraction: 1,
			drill:    drillUnheardAll,
		})
	}
	peakBacklog := 0
	for _, b := range v.backlog {
		peakBacklog = max(peakBacklog, b.Tracks)
	}
	for _, b := range v.backlog {
		rows = append(rows, insightsRow{
			section: "Added but never played",
			label:   b.Label,
			note: fmt.Sprintf("%d %s · %s", b.Tracks,
				pluralize(b.Tracks, "track", "tracks"), formatListeningTime(int(b.Secs))),
			fraction: insightsFraction(b.Tracks, peakBacklog),
			drill:    drillAddedNeverPlayed,
			key:      b.Key,
		})
	}
	return rows
}

func (v InsightsView) discoveryHeaderLines(s styles.Styles) []string {
	t := v.snap.Trends
	head := []string{"", s.Dim.Render(v.headline(panelDiscovery)), ""}
	if len(t.Months) == 0 {
		return head
	}

	pal := chart.FromStyles(s)
	w := max(v.width, 1)

	tail := []string{
		s.TextSecondary.Render(v.fit(fmt.Sprintf("%.1f new tracks a month on average · %s still unheard",
			t.AvgMonthlyFirstPlays, formatCount(v.snap.Coverage.UnheardTracks)))),
		"",
	}
	if v.compact() {
		return append(head, tail...)
	}

	bars := make([]chart.VBar, 0, len(t.Months))
	for i, b := range t.Months {
		bars = append(bars, chart.VBar{
			Label: b.Month.String()[:3],
			Value: float64(b.FirstPlays),
			Muted: i == len(t.Months)-1,
		})
	}
	block := chart.VBarChart(bars, chart.VBarOptions{Width: w, Height: 3}, pal)
	block = append(block, "")

	return insightsFitHeader(v.headerBudget(), head, [][]string{block}, tail)
}

// --- shared folding ----------------------------------------------------------

// insightsHourGenreFold folds a window's plays into per-genre hourly counts,
// heaviest genre first. It takes the genre from the library rather than from
// insights.GenreRow because that aggregate has already summed the hours away.
func insightsHourGenreFold(plays []insights.PlayRow, genreOf func(hash string) []string, loc *time.Location) []insightsGenreHours {
	type acc struct {
		row   insightsGenreHours
		plays int
	}
	byGenre := map[string]*acc{}
	for _, p := range plays {
		hour := time.Unix(p.StartAt, 0).In(loc).Hour()
		for _, g := range genreOf(p.Hash) {
			key := insightsGenreKey(g)
			if key == "" {
				continue
			}
			a := byGenre[key]
			if a == nil {
				a = &acc{row: insightsGenreHours{Genre: g}}
				byGenre[key] = a
			}
			a.row.Hours[hour]++
			a.plays++
		}
	}

	accs := make([]*acc, 0, len(byGenre))
	for _, a := range byGenre {
		accs = append(accs, a)
	}
	sort.Slice(accs, func(i, j int) bool {
		if accs[i].plays != accs[j].plays {
			return accs[i].plays > accs[j].plays
		}
		return accs[i].row.Genre < accs[j].row.Genre
	})

	out := make([]insightsGenreHours, 0, len(accs))
	for _, a := range accs {
		out = append(out, a.row)
	}
	return out
}

// insightsGenreKey folds a stored genre string the same way insights.foldGenres
// does, so a row and the aggregate above it agree on what counts as one genre.
func insightsGenreKey(g string) string { return strings.ToLower(strings.TrimSpace(g)) }
