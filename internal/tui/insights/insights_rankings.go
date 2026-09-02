package insights

// Concentration and Time vs Plays: the two panels built on the same ranking of
// artists. Concentration asks how few names your listening actually comes down
// to; Time vs Plays asks whether pressing play often and spending time with
// somebody are the same thing.

import (
	"fmt"
	"sort"
	"strconv"

	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/chart"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

const (
	// insightsLadderArtistRows caps the top-artists list under the ladder.
	insightsLadderArtistRows = 12
	// insightsRankTableRows caps the side-by-side ordering, which is a
	// comparison of the head of two lists rather than a full listing.
	insightsRankTableRows = 10
	// RutTopArtists is the size of the rut the ! shortcut steers away
	// from: the artists that already hold the top of the time ranking.
	RutTopArtists = 20
)

// --- Concentration -----------------------------------------------------------

func (v View) concentrationRows() []Row {
	r := v.Snap.Rankings
	l := r.Ladder
	if l.TotalArtists == 0 {
		return nil
	}
	rows := make([]Row, 0, 4+insightsLadderArtistRows)

	for _, step := range []struct {
		share   int
		artists int
	}{
		{25, l.Artists25}, {50, l.Artists50}, {75, l.Artists75}, {90, l.Artists90},
	} {
		if step.artists == 0 {
			continue
		}
		rows = append(rows, Row{
			Section: "Repertoire ladder",
			Label:   fmt.Sprintf("%d%% of your time", step.share),
			note: fmt.Sprintf("%d of %d %s", step.artists, l.TotalArtists,
				text.Pluralize(l.TotalArtists, "artist", "artists")),
			fraction: insightsFraction(step.artists, l.TotalArtists),
			Drill:    DrillTopArtists,
			Key:      strconv.Itoa(step.artists),
		})
	}

	peak := 0
	if len(r.ArtistsByHours) > 0 {
		peak = r.ArtistsByHours[0].ListenedSecs
	}
	for i, e := range r.ArtistsByHours {
		if i >= insightsLadderArtistRows {
			break
		}
		rows = append(rows, Row{
			Section:  "Top artists by time",
			Label:    e.Name,
			note:     insightsPlaysNote(e.Plays, e.ListenedSecs),
			fraction: insightsFraction(e.ListenedSecs, peak),
			Drill:    DrillArtistPlays,
			Key:      e.Name,
		})
	}
	return rows
}

func (v View) concentrationHeaderLines(s styles.Styles) []string {
	r := v.Snap.Rankings
	l := r.Ladder
	head := []string{"", s.Dim.Render(v.headline(PanelConcentration)), ""}
	if l.TotalArtists == 0 {
		return head
	}

	// The headline already gives the half-time figure, so the summary takes the
	// far rung of the ladder and names what ! does with it.
	tail := []string{
		s.TextSecondary.Render(v.fit(fmt.Sprintf(
			"%d %s in this window · %d of them cover 90%% of it · ! builds a set from outside the top %d",
			l.TotalArtists, text.Pluralize(l.TotalArtists, "artist", "artists"),
			l.Artists90, RutTopArtists))),
		"",
	}
	if v.compact() {
		return append(head, tail...)
	}

	block := []string{
		s.Dim.Render(v.fit("share of listening time held by your top 5 artists")),
		chart.Ratio(r.TopFiveShare, 1, max(v.width, 1), chart.FromStyles(s)),
		"",
	}
	return insightsFitHeader(v.headerBudget(), head, [][]string{block}, tail)
}

// --- Time vs Plays -----------------------------------------------------------

func (v View) timeVsPlaysRows() []Row {
	r := v.Snap.Rankings
	if len(r.Deltas) == 0 {
		return nil
	}

	byName := make(map[string]insightsdata.EntityStat, len(r.ArtistsByHours))
	for _, e := range r.ArtistsByHours {
		byName[e.Name] = e
	}
	peak := 0
	if len(r.ArtistsByHours) > 0 {
		peak = r.ArtistsByHours[0].ListenedSecs
	}

	moved := make([]insightsdata.RankDelta, 0, len(r.Deltas))
	steady := make([]insightsdata.RankDelta, 0, len(r.Deltas))
	for _, d := range r.Deltas {
		if d.Delta != 0 {
			moved = append(moved, d)
		} else {
			steady = append(steady, d)
		}
	}
	// Biggest mover first: the row that most contradicts the play count is the
	// one the panel exists to show.
	sort.SliceStable(moved, func(i, j int) bool {
		return insightsAbs(moved[i].Delta) > insightsAbs(moved[j].Delta)
	})

	rows := make([]Row, 0, len(r.Deltas))
	add := func(section string, deltas []insightsdata.RankDelta) {
		for _, d := range deltas {
			e := byName[d.Name]
			rows = append(rows, Row{
				Section: section,
				Label:   d.Name,
				note: fmt.Sprintf("plays #%d · hours #%d · %s",
					d.PlayRank, d.HoursRank, insightsDeltaText(d.Delta)),
				fraction: insightsFraction(e.ListenedSecs, peak),
				Drill:    DrillArtistPlays,
				Key:      d.Name,
			})
		}
	}
	add("Moved by hours", moved)
	add("Same either way", steady)
	return rows
}

func (v View) timeVsPlaysHeaderLines(s styles.Styles) []string {
	r := v.Snap.Rankings
	head := []string{"", s.Dim.Render(v.headline(PanelTimeVsPlays)), ""}
	if len(r.Deltas) == 0 {
		return head
	}

	tail := []string{
		s.TextSecondary.Render(v.fit(fmt.Sprintf(
			"%d of %d %s rank differently once time is counted · b opens the focused artist in Browse",
			r.MovedCount, len(r.Deltas), text.Pluralize(len(r.Deltas), "artist", "artists")))),
		"",
	}
	if v.compact() {
		return append(head, tail...)
	}

	// Unlike the other panels' chart blocks, the table shrinks to the room it
	// has rather than being dropped whole: two orderings side by side are the
	// panel itself, and even one rung of them makes the comparison.
	budget := v.headerBudget()
	n := min(budget-len(head)-len(tail)-2, insightsRankTableRows)
	n = min(n, min(len(r.ArtistsByPlays), len(r.ArtistsByHours)))
	if n <= 0 {
		return append(head, tail...)
	}

	deltaOf := make(map[string]int, len(r.Deltas))
	for _, d := range r.Deltas {
		deltaOf[d.Name] = d.Delta
	}

	w := max(v.width, 1)
	left := max(w/2, 1)
	right := max(w-left, 1)
	table := []string{s.BucketHeader.Render(text.Pad("   by plays", left) + text.Pad("   by hours", right))}
	for i := range n {
		e := r.ArtistsByHours[i]
		mark := ""
		// Deltas only covers the head of the play ranking, so an artist that is
		// high on hours alone gets no arrow rather than a "=" it has not earned.
		if d, ok := deltaOf[e.Name]; ok {
			mark = " " + insightsDeltaText(d)
		}
		table = append(table, s.TextSecondary.Render(
			text.Pad(fmt.Sprintf("%2d %s", i+1, r.ArtistsByPlays[i].Name), left)+
				text.Pad(fmt.Sprintf("%2d %s%s", i+1, e.Name, mark), right)))
	}
	table = append(table, "")

	return insightsFitHeader(budget, head, [][]string{table}, tail)
}

// insightsDeltaText renders a rank move as plain text. chart.Delta styles its
// output, and a note handed to the bar renderer has its escapes stripped, so
// the arrow is spelled out here instead.
func insightsDeltaText(delta int) string {
	switch {
	case delta > 0:
		return fmt.Sprintf("▲%d", delta)
	case delta < 0:
		return fmt.Sprintf("▼%d", -delta)
	default:
		return "="
	}
}

func insightsAbs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
