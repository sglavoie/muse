package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/chart"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// --- Momentum ----------------------------------------------------------------

func (v InsightsView) momentumRows() []insightsRow {
	t := v.snap.Trends
	peak := 0
	for _, b := range t.Buckets {
		peak = max(peak, b.ListenedSecs)
	}

	rowCap := insightsTrendRowCap(t.Resolution)
	rows := make([]insightsRow, 0, min(len(t.Buckets), rowCap))
	// Newest first: the question Momentum answers is about the recent end, and
	// putting it on top means the answer needs no scrolling.
	for i := len(t.Buckets) - 1; i >= 0 && len(rows) < rowCap; i-- {
		b := t.Buckets[i]
		note := "silent"
		if b.Plays > 0 {
			note = insightsPlaysNote(b.Plays, b.ListenedSecs)
		}
		rows = append(rows, insightsRow{
			section:  insightsTrendSection(t.Resolution),
			label:    insightsBucketLabel(t.Resolution, b.Start),
			note:     note,
			fraction: insightsFraction(b.ListenedSecs, peak),
			drill:    drillTrendBucket,
			key:      insightsBucketKey(t.Resolution, b.Start),
		})
	}
	return rows
}

func (v InsightsView) momentumHeaderLines(s styles.Styles) []string {
	t := v.snap.Trends
	head := []string{"", s.Dim.Render(v.headline(panelMomentum)), ""}
	if len(t.Buckets) == 0 {
		return head
	}

	pal := chart.FromStyles(s)
	w := max(v.width, 1)

	// The headline already compares the current bucket to the average, so the
	// summary names the high-water mark instead — what a dry spell is measured
	// off.
	best := t.Buckets[0]
	for _, b := range t.Buckets {
		if b.ListenedSecs > best.ListenedSecs {
			best = b
		}
	}
	// "best:" rather than "best month": the weekly and daily labels already name
	// their own grain, and the replay clause below names it for the monthly one.
	summary := fmt.Sprintf("best: %s at %s · enter replays a %s in order",
		insightsBucketLabel(t.Resolution, best.Start),
		text.FormatListeningTime(best.ListenedSecs), t.Resolution.Period())
	if t.AvgSecs > 0 {
		summary = chart.Trend(t.CurrentVsAvg-1, pal) + " " + summary
	}
	tail := []string{s.TextSecondary.Render(v.fit(summary))}
	if t.LongestDry.Days > 0 {
		tail = append(tail, s.Dim.Render(v.fit(fmt.Sprintf("longest silence %d days, %s – %s · %d dry %s in all",
			t.LongestDry.Days, t.LongestDry.Start.Format("2 Jan 2006"), t.LongestDry.End.Format("2 Jan 2006"),
			len(t.DrySpells), text.Pluralize(len(t.DrySpells), "spell", "spells")))))
	}
	tail = append(tail, "")

	if v.compact() {
		return append(head, tail...)
	}

	bars := make([]chart.VBar, 0, len(t.Buckets))
	for i, b := range t.Buckets {
		bars = append(bars, chart.VBar{
			Label: insightsBucketShort(t.Resolution, b.Start),
			Value: float64(b.ListenedSecs),
			// The current bucket is still filling up, so it is drawn muted rather
			// than read as a finished one that came up short.
			Muted: i == len(t.Buckets)-1,
		})
	}
	series := chart.VBarChart(bars, chart.VBarOptions{Width: w, Height: 4}, pal)
	series = append(series, "")

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

	return insightsFitHeader(v.headerBudget(), head, [][]string{series, calendar}, tail)
}

// --- Discovery ---------------------------------------------------------------

func (v InsightsView) discoveryRows() []insightsRow {
	t := v.snap.Trends
	peak := 0
	for _, b := range t.Buckets {
		peak = max(peak, b.FirstPlays)
	}

	rowCap := insightsTrendRowCap(t.Resolution)
	rows := make([]insightsRow, 0, rowCap+len(v.backlog)+1)
	for i := len(t.Buckets) - 1; i >= 0 && len(rows) < rowCap; i-- {
		b := t.Buckets[i]
		if b.FirstPlays == 0 {
			continue
		}
		rows = append(rows, insightsRow{
			section: "First heard",
			label:   insightsBucketLabel(t.Resolution, b.Start),
			note: fmt.Sprintf("%d new %s", b.FirstPlays,
				text.Pluralize(b.FirstPlays, "track", "tracks")),
			fraction: insightsFraction(b.FirstPlays, peak),
			drill:    drillFirstPlaysBucket,
			key:      insightsBucketKey(t.Resolution, b.Start),
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
				text.Pluralize(c.UnheardTracks, "track", "tracks"), text.FormatListeningTime(int(c.UnheardSecs))),
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
				text.Pluralize(b.Tracks, "track", "tracks"), text.FormatListeningTime(int(b.Secs))),
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
	if len(t.Buckets) == 0 {
		return head
	}

	pal := chart.FromStyles(s)
	w := max(v.width, 1)

	tail := []string{
		s.TextSecondary.Render(v.fit(fmt.Sprintf("%.1f new tracks a %s on average · %s still unheard",
			t.AvgFirstPlays, t.Resolution.Period(), formatCount(v.snap.Coverage.UnheardTracks)))),
		"",
	}
	if v.compact() {
		return append(head, tail...)
	}

	bars := make([]chart.VBar, 0, len(t.Buckets))
	for i, b := range t.Buckets {
		bars = append(bars, chart.VBar{
			Label: insightsBucketShort(t.Resolution, b.Start),
			Value: float64(b.FirstPlays),
			Muted: i == len(t.Buckets)-1,
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
