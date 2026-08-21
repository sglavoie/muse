package tui

// Sessions: the one panel whose rows do not open tracks directly. A session is
// a thing in its own right — it has a length, a shape, an opener and an order
// it was heard in — so a Sessions row opens the sessions themselves, and a
// session opens its tracks.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/chart"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// insightsSessionHistHeight is how tall the duration histogram stands. It is
// the whole point of the panel header, so it gets more rows than Discovery's.
const insightsSessionHistHeight = 4

// The shape keys a drillSessionsShape row carries.
const (
	insightsShapeSinglePlay = "single-play"
	insightsDietOne         = "one-artist"
	insightsDietFew         = "few-artists"
	insightsDietMany        = "many-artists"
)

// insightsDurationBucket is one column of the session-length histogram.
type insightsDurationBucket struct {
	label string
	// short is the histogram's axis label, which a column's width has to hold.
	short string
	// max is the exclusive upper bound in seconds; 0 means unbounded.
	max int
}

var insightsDurationBuckets = []insightsDurationBucket{
	{"under 15m", "<15m", 15 * 60},
	{"15–30m", "30m", 30 * 60},
	{"30m–1h", "1h", 3600},
	{"1–2h", "2h", 2 * 3600},
	{"2–4h", "4h", 4 * 3600},
	{"4h+", "4h+", 0},
}

// insightsDurationBucketOf files a session length under a bucket. Row building
// and drill filtering both go through it, so a histogram column can never
// disagree with the set of sessions behind it.
func insightsDurationBucketOf(secs int) int {
	for i, b := range insightsDurationBuckets {
		if b.max == 0 || secs < b.max {
			return i
		}
	}
	return len(insightsDurationBuckets) - 1
}

// insightsSessionSecs is a session's wall-clock length, the same measure
// insights.computeSessions puts in SessionStats.Durations.
func insightsSessionSecs(r insights.SessionRow) int {
	return max(int(r.EndedAt-r.StartedAt), 0)
}

// insightsSessionDiet names how wide a session's artist selection was.
func insightsSessionDiet(r insights.SessionRow) string {
	switch {
	case r.DistinctArtists <= 1:
		return insightsDietOne
	case r.DistinctArtists <= 3:
		return insightsDietFew
	default:
		return insightsDietMany
	}
}

// insightsSessionsMatching returns the sessions a Sessions row stands for,
// newest first. The counts in the rows come from here too, so a row's number
// and the list it opens are always the same set.
func insightsSessionsMatching(sessions []insights.SessionRow, kind insightsDrillKind, key string) []insights.SessionRow {
	bucket := -1
	if kind == drillSessionsBucket {
		if idx, err := strconv.Atoi(key); err == nil {
			bucket = idx
		}
	}

	out := make([]insights.SessionRow, 0, len(sessions))
	for i := len(sessions) - 1; i >= 0; i-- {
		r := sessions[i]
		keep := false
		switch kind {
		case drillSessionsAll:
			keep = true
		case drillSessionsBucket:
			keep = bucket >= 0 && insightsDurationBucketOf(insightsSessionSecs(r)) == bucket
		case drillSessionsShape:
			if key == insightsShapeSinglePlay {
				keep = r.Plays <= 1
			} else {
				keep = insightsSessionDiet(r) == key
			}
		case drillSessionsOpener:
			keep = r.OpenerArtist == key
		case drillSessionsCloser:
			keep = r.CloserArtist == key
		}
		if keep {
			out = append(out, r)
		}
	}
	return out
}

func (v InsightsView) sessionRows() []insightsRow {
	ss := v.snap.SessionStats
	if ss.Count == 0 {
		return nil
	}
	rows := make([]insightsRow, 0, len(insightsDurationBuckets)+len(ss.TopOpeners)+len(ss.TopClosers)+5)

	sessionNote := func(n int) string {
		return fmt.Sprintf("%d %s · %.0f%%", n, pluralize(n, "session", "sessions"),
			insightsFraction(n, ss.Count)*100)
	}

	rows = append(rows, insightsRow{
		section:  "Sessions",
		label:    "All sessions",
		note:     fmt.Sprintf("%d %s · %s", ss.Count, pluralize(ss.Count, "session", "sessions"), formatListeningTime(ss.TotalSecs)),
		fraction: 1,
		drill:    drillSessionsAll,
	})

	counts := insightsDurationCounts(ss.Durations)
	peak := 0
	for _, c := range counts {
		peak = max(peak, c)
	}
	for i, b := range insightsDurationBuckets {
		// An empty bucket is a row that opens nothing, so it is left off rather
		// than offered as a drill that dead-ends.
		if counts[i] == 0 {
			continue
		}
		rows = append(rows, insightsRow{
			section:  "Session length",
			label:    b.label,
			note:     sessionNote(counts[i]),
			fraction: insightsFraction(counts[i], peak),
			drill:    drillSessionsBucket,
			key:      strconv.Itoa(i),
		})
	}

	if ss.SinglePlay > 0 {
		rows = append(rows, insightsRow{
			section:  "False starts",
			label:    "One play and out",
			note:     sessionNote(ss.SinglePlay),
			fraction: insightsFraction(ss.SinglePlay, ss.Count),
			drill:    drillSessionsShape,
			key:      insightsShapeSinglePlay,
		})
	}

	// SessionStats counts single-artist sessions but not the split above one, so
	// the diet comes off the raw rows.
	diet := map[string]int{}
	for _, r := range v.sessions {
		diet[insightsSessionDiet(r)]++
	}
	for _, d := range []struct{ key, label string }{
		{insightsDietOne, "One artist only"},
		{insightsDietFew, "Two or three artists"},
		{insightsDietMany, "Four or more artists"},
	} {
		if diet[d.key] == 0 {
			continue
		}
		rows = append(rows, insightsRow{
			section:  "Session diet",
			label:    d.label,
			note:     sessionNote(diet[d.key]),
			fraction: insightsFraction(diet[d.key], ss.Count),
			drill:    drillSessionsShape,
			key:      d.key,
		})
	}

	appendEntities := func(section string, ents []insights.EntityStat, kind insightsDrillKind) {
		top := 0
		for _, e := range ents {
			top = max(top, e.Plays)
		}
		for _, e := range ents {
			rows = append(rows, insightsRow{
				section:  section,
				label:    e.Name,
				note:     fmt.Sprintf("%d %s · %s", e.Plays, pluralize(e.Plays, "session", "sessions"), formatListeningTime(e.ListenedSecs)),
				fraction: insightsFraction(e.Plays, top),
				drill:    kind,
				key:      e.Name,
			})
		}
	}
	appendEntities("Opens with", ss.TopOpeners, drillSessionsOpener)
	appendEntities("Ends with", ss.TopClosers, drillSessionsCloser)

	return rows
}

func insightsDurationCounts(durations []int) []int {
	counts := make([]int, len(insightsDurationBuckets))
	for _, d := range durations {
		counts[insightsDurationBucketOf(d)]++
	}
	return counts
}

func (v InsightsView) sessionsHeaderLines(s styles.Styles) []string {
	ss := v.snap.SessionStats
	head := []string{"", s.Dim.Render(v.headline(panelSessions)), ""}
	if ss.Count == 0 {
		return head
	}

	// The headline already gives the count and the median, so the summary takes
	// the far end of the histogram and says what enter does here, which is the
	// one place in Insights where it does not open tracks.
	tail := []string{
		s.TextSecondary.Render(v.fit(fmt.Sprintf("longest %s · %.1f plays a session · %d false %s",
			formatListeningTime(ss.LongestSecs), ss.AvgPlays,
			ss.SinglePlay, pluralize(ss.SinglePlay, "start", "starts")))),
		s.Dim.Render(v.fit("enter opens the sessions themselves")),
		"",
	}
	if v.compact() {
		return append(head, tail...)
	}

	counts := insightsDurationCounts(ss.Durations)
	bars := make([]chart.VBar, len(insightsDurationBuckets))
	for i, b := range insightsDurationBuckets {
		bars[i] = chart.VBar{Label: b.short, Value: float64(counts[i])}
	}
	hist := chart.VBarChart(bars, chart.VBarOptions{
		Width:  max(v.width, 1),
		Height: insightsSessionHistHeight,
	}, chart.FromStyles(s))
	hist = append(hist, "")

	return insightsFitHeader(v.headerBudget(), head, [][]string{hist}, tail)
}

// --- the session list (level 2) ----------------------------------------------

// sessionBodyRows is how many sessions fit under the list header.
func (v InsightsView) sessionBodyRows() int {
	return max(v.height-len(v.sessionListHeaderLines(styles.Styles{})), 1)
}

// sessionListHeaderLines is the block above the session rows. Rendering it with
// a zero Styles yields the same line count as the styled render, so the sizing
// and the drawing can never drift apart.
func (v InsightsView) sessionListHeaderLines(s styles.Styles) []string {
	n := len(v.sessionList)
	plays, secs := 0, 0
	for _, r := range v.sessionList {
		plays += r.Plays
		secs += r.ListenedSecs
	}
	line := strip.Render(v.width, []string{"Insights", "Sessions", v.sessionLabel},
		strip.SortChunk{}, strip.FilterChunk{},
		strip.CountsChunk{Filtered: n, Total: n}, nil, "esc ←", s)
	if line == "" {
		line = "Sessions"
	}
	return []string{
		line,
		"",
		s.TextSecondary.Render(v.fit(fmt.Sprintf("%d %s · %d %s · %s listened",
			n, pluralize(n, "session", "sessions"),
			plays, pluralize(plays, "play", "plays"), formatListeningTime(secs)))),
		s.Dim.Render(v.fit("enter → tracks · E → replay this session in order")),
		"",
	}
}

func (v InsightsView) renderSessionList(s styles.Styles) string {
	lines := v.sessionListHeaderLines(s)
	if len(v.sessionList) == 0 {
		lines = append(lines, s.Dim.Render("No sessions behind this row."))
	}
	budget := v.sessionBodyRows()
	for i := v.sessionScroll; i < len(v.sessionList) && budget > 0; i++ {
		lines = append(lines, v.renderSessionRow(s, v.sessionList[i], i == v.sessionCursor))
		budget--
	}
	if v.height > 0 && len(lines) > v.height {
		lines = lines[:v.height]
	}
	return strings.Join(lines, "\n")
}

func (v InsightsView) renderSessionRow(s styles.Styles, r insights.SessionRow, focused bool) string {
	cursor := "  "
	if focused {
		cursor = "> "
	}
	width := max(v.width-len(cursor), 1)

	note := fmt.Sprintf("%d %s · %d %s",
		r.Plays, pluralize(r.Plays, "play", "plays"),
		r.DistinctArtists, pluralize(r.DistinctArtists, "artist", "artists"))
	label := fmt.Sprintf("%s · %s · %s",
		time.Unix(r.StartedAt, 0).In(v.location()).Format("2 Jan 15:04"),
		formatListeningTime(insightsSessionSecs(r)),
		insightsSessionArc(r))

	labelW := max(width-len(note)-2, 1)
	body := text.Pad(text.Truncate(label, labelW), labelW) + "  " + note
	if focused {
		return s.Accent.Render(cursor + text.Truncate(ansi.Strip(body), width))
	}
	return s.TextTertiary.Render(cursor) + s.TextPrimary.Render(body)
}

// insightsSessionArc names where a session started and where it ended up. A
// session that never left one artist says so once instead of twice.
func insightsSessionArc(r insights.SessionRow) string {
	switch {
	case r.OpenerArtist == "" && r.CloserArtist == "":
		return "unknown artist"
	case r.OpenerArtist == r.CloserArtist:
		return r.OpenerArtist
	default:
		return r.OpenerArtist + " → " + r.CloserArtist
	}
}
