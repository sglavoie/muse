package tui

// Rhythm, Momentum and Discovery: the time-series family. All three answer a
// "when" question, and all three obey the same rule as Coverage — every row is
// a query with a shape, so each one names the tracks that enter opens.

import (
	"fmt"
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
	// insightsMonthRows caps the month lists — Discovery's backlog, and the
	// monthly trend rows — so a long history stays a briefing.
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

// insightsBucketKey is the key every trend row drills on. Each form is sortable
// so a string compare and a chronological compare agree, and each is distinct
// per resolution so a key minted at one bucket size can never resolve at
// another.
func insightsBucketKey(res insights.Resolution, t time.Time) string {
	switch res {
	case insights.ResolutionDaily:
		return t.Format("2006-01-02")
	case insights.ResolutionWeekly:
		// ISO year rather than calendar year: the week holding 1 January can
		// belong to the previous one, and "2026-W01" for both would merge two
		// bars into one drill.
		y, w := t.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", y, w)
	default:
		return t.Format("2006-01")
	}
}

// insightsBucketLabel names a bucket in a row.
func insightsBucketLabel(res insights.Resolution, t time.Time) string {
	switch res {
	case insights.ResolutionDaily:
		return t.Format("Mon 2 Jan 2006")
	case insights.ResolutionWeekly:
		return "Week of " + t.Format("2 Jan 2006")
	default:
		return t.Format("January 2006")
	}
}

// insightsBucketShort names a bucket under a bar, where the column is only a
// few cells wide.
func insightsBucketShort(res insights.Resolution, t time.Time) string {
	switch res {
	case insights.ResolutionDaily:
		return t.Format("02")
	case insights.ResolutionWeekly:
		_, w := t.ISOWeek()
		return fmt.Sprintf("W%02d", w)
	default:
		return t.Format("Jan")
	}
}

// insightsTrendSection is the section header the trend rows sit under.
func insightsTrendSection(res insights.Resolution) string {
	switch res {
	case insights.ResolutionDaily:
		return "Day by day"
	case insights.ResolutionWeekly:
		return "Week by week"
	default:
		return "Month by month"
	}
}

// insightsTrendRowCap scales the row cap with the bucket size so every
// resolution lists a comparable stretch of history: a flat cap would make the
// daily view reach back only three weeks.
func insightsTrendRowCap(res insights.Resolution) int {
	switch res {
	case insights.ResolutionDaily:
		return 90
	case insights.ResolutionWeekly:
		return 52
	default:
		return insightsMonthRows
	}
}

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
	return fmt.Sprintf("%d %s · %s", plays, text.Pluralize(plays, "play", "plays"), text.FormatListeningTime(secs))
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
			note = fmt.Sprintf("%d %s · %.0f%%", d.Plays, text.Pluralize(d.Plays, "play", "plays"), d.Share*100)
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
		r.DistinctTracks, text.Pluralize(r.DistinctTracks, "track", "tracks"),
		r.BottomWeekday.Weekday, r.BottomWeekday.Plays,
		text.Pluralize(r.BottomWeekday.Plays, "play", "plays"))))}
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
