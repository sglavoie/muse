package insights

// Rhythm, Momentum and Discovery: the time-series family. All three answer a
// "when" question, and all three obey the same rule as Coverage — every row is
// a query with a shape, so each one names the tracks that enter opens.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
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
	// monthRows caps the month lists — Discovery's backlog, and the
	// monthly trend rows — so a long history stays a briefing.
	monthRows = 24
	// insightsPunchcardHours is the number of columns in a day.
	insightsPunchcardHours = 24
)

// GenreHours is one genre's play count per local hour: the row the
// Rhythm affinity grid draws and the shape "you only reach for jazz after
// 22:00" is visible in.
type GenreHours struct {
	genre string
	hours [insightsPunchcardHours]int
}

// BacklogMonth counts the never-played tracks that entered the library
// in one local month, so Discovery can say what the backlog is made of rather
// than only how big it is.
type BacklogMonth struct {
	key    string // "2006-01", the drill key
	label  string
	tracks int
	secs   float64
}

// BacklogMonths groups the never-played tracks by the local month they entered
// the library, most recent first. Tracks with no recorded add date are left out
// rather than piled into the epoch; the Discovery body carries a total row so
// they stay reachable.
func BacklogMonths(tracks []library.Track, loc *time.Location) []BacklogMonth {
	byKey := map[string]*BacklogMonth{}
	for _, t := range tracks {
		if t.PlayCount > 0 || t.CreatedAt <= 0 {
			continue
		}
		added := time.Unix(t.CreatedAt, 0).In(loc)
		key := BucketKey(insightsdata.ResolutionMonthly, added)
		b := byKey[key]
		if b == nil {
			b = &BacklogMonth{key: key, label: added.Format("January 2006")}
			byKey[key] = b
		}
		b.tracks++
		b.secs += t.Duration
	}

	out := make([]BacklogMonth, 0, len(byKey))
	for _, b := range byKey {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key > out[j].key })
	return out[:min(len(out), monthRows)]
}

func insightsFraction(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return float64(part) / float64(whole)
}

// BucketKey is the key every trend row drills on. Each form is sortable
// so a string compare and a chronological compare agree, and each is distinct
// per resolution so a key minted at one bucket size can never resolve at
// another.
func BucketKey(res insightsdata.Resolution, t time.Time) string {
	switch res {
	case insightsdata.ResolutionDaily:
		return t.Format("2006-01-02")
	case insightsdata.ResolutionWeekly:
		// ISO year rather than calendar year: the week holding 1 January can
		// belong to the previous one, and "2026-W01" for both would merge two
		// bars into one drill.
		y, w := t.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", y, w)
	default:
		return t.Format("2006-01")
	}
}

// BucketLabel names a bucket in a row.
func BucketLabel(res insightsdata.Resolution, t time.Time) string {
	switch res {
	case insightsdata.ResolutionDaily:
		return t.Format("Mon 2 Jan 2006")
	case insightsdata.ResolutionWeekly:
		return "Week of " + t.Format("2 Jan 2006")
	default:
		return t.Format("January 2006")
	}
}

// insightsBucketShort names a bucket under a bar, where the column is only a
// few cells wide.
func insightsBucketShort(res insightsdata.Resolution, t time.Time) string {
	switch res {
	case insightsdata.ResolutionDaily:
		return t.Format("02")
	case insightsdata.ResolutionWeekly:
		_, w := t.ISOWeek()
		return fmt.Sprintf("W%02d", w)
	default:
		return t.Format("Jan")
	}
}

// insightsTrendSection is the section header the trend rows sit under.
func insightsTrendSection(res insightsdata.Resolution) string {
	switch res {
	case insightsdata.ResolutionDaily:
		return "Day by day"
	case insightsdata.ResolutionWeekly:
		return "Week by week"
	default:
		return "Month by month"
	}
}

// insightsTrendRowCap scales the row cap with the bucket size so every
// resolution lists a comparable stretch of history: a flat cap would make the
// daily view reach back only three weeks.
func insightsTrendRowCap(res insightsdata.Resolution) int {
	switch res {
	case insightsdata.ResolutionDaily:
		return 90
	case insightsdata.ResolutionWeekly:
		return 52
	default:
		return monthRows
	}
}

// fit clamps a prose line to the view width. Chart output is already exactly as
// wide as it was asked to be; a summary sentence is not, and one overlong line
// is enough to overflow the frame.
func (v View) fit(line string) string {
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

func (v View) rhythmRows() []Row {
	r := v.Snap.Rhythm
	rows := make([]Row, 0, insightsPunchcardHours+7+insightsGenreRows)

	peakHour := 0
	for _, h := range r.Hours {
		peakHour = max(peakHour, h.Plays)
	}
	for _, h := range r.Hours {
		if h.Plays == 0 {
			continue
		}
		rows = append(rows, Row{
			Section:  "Hour of day",
			Label:    fmt.Sprintf("%02d:00", h.Hour),
			note:     insightsPlaysNote(h.Plays, h.ListenedSecs),
			fraction: insightsFraction(h.Plays, peakHour),
			Drill:    DrillHour,
			Key:      strconv.Itoa(h.Hour),
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
		rows = append(rows, Row{
			Section:  "Weekday",
			Label:    d.Weekday.String(),
			note:     note,
			fraction: insightsFraction(d.Plays, peakDay),
			Drill:    DrillWeekday,
			Key:      strconv.Itoa(int(d.Weekday)),
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
			rows = append(rows, Row{
				Section:  "Genre affinity",
				Label:    g.Canonical,
				note:     insightsPlaysNote(g.Plays, g.ListenedSecs),
				fraction: insightsFraction(g.Plays, r.TotalPlays),
				Drill:    DrillRhythmGenre,
				Key:      g.Canonical,
			})
		}
	}
	return rows
}

func (v View) rhythmHeaderLines(s styles.Styles) []string {
	r := v.Snap.Rhythm
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
	if !r.GenresUnclean && len(v.GenreHours) > 0 {
		grid := make([]chart.HeatRow, 0, insightsGenreRows)
		for i, g := range v.GenreHours {
			if i >= insightsGenreRows {
				break
			}
			cells := make([]chart.HeatCell, insightsPunchcardHours)
			for h, n := range g.hours {
				cells[h] = chart.HeatCell{Value: float64(n), Present: true}
			}
			grid = append(grid, chart.HeatRow{Label: g.genre, Cells: cells})
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
