package insights

import (
	"strings"
	"testing"

	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// rankingsFixture is four artists whose two orderings deliberately disagree:
// Dana is played often but briefly, Carol rarely but at length.
func rankingsFixture() insightsdata.Rankings {
	return insightsdata.Rankings{
		ArtistsByPlays: []insightsdata.EntityStat{
			{Name: "Alice", Plays: 40, ListenedSecs: 9000, Rank: 1},
			{Name: "Dana", Plays: 30, ListenedSecs: 1200, Rank: 2},
			{Name: "Bob", Plays: 20, ListenedSecs: 6000, Rank: 3},
			{Name: "Carol", Plays: 5, ListenedSecs: 4000, Rank: 4},
		},
		ArtistsByHours: []insightsdata.EntityStat{
			{Name: "Alice", Plays: 40, ListenedSecs: 9000, Rank: 1},
			{Name: "Bob", Plays: 20, ListenedSecs: 6000, Rank: 2},
			{Name: "Carol", Plays: 5, ListenedSecs: 4000, Rank: 3},
			{Name: "Dana", Plays: 30, ListenedSecs: 1200, Rank: 4},
		},
		Deltas: []insightsdata.RankDelta{
			{Name: "Alice", PlayRank: 1, HoursRank: 1, Delta: 0},
			{Name: "Dana", PlayRank: 2, HoursRank: 4, Delta: -2},
			{Name: "Bob", PlayRank: 3, HoursRank: 2, Delta: 1},
			{Name: "Carol", PlayRank: 4, HoursRank: 3, Delta: 1},
		},
		MovedCount:   3,
		TopFiveShare: 0.75,
		Ladder:       insightsdata.Ladder{TotalArtists: 40, Artists25: 1, Artists50: 3, Artists75: 8, Artists90: 17},
	}
}

func rankingsFixtureView(panel Panel) View {
	return View{
		Panel:  panel,
		width:  100,
		Height: 30,
		Loc:    panelsTestLoc(),
		Snap:   insightsdata.Snapshot{Rankings: rankingsFixture()},
	}
}

func TestConcentrationRowsBuildTheRepertoireLadder(t *testing.T) {
	rows := rankingsFixtureView(PanelConcentration).Rows()

	want := []string{"25% of your time", "50% of your time", "75% of your time", "90% of your time"}
	got := sectionLabels(rows, "Repertoire ladder")
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("want the four rungs, got %v", got)
	}
	// The rung's key is the artist count it stands for, so opening it resolves to
	// exactly the group the number names.
	for _, r := range rows {
		if r.Section != "Repertoire ladder" {
			continue
		}
		if r.Drill != DrillTopArtists {
			t.Errorf("rung %q should drill into its top artists, got %v", r.Label, r.Drill)
		}
	}
	if rows[1].Key != "3" || !strings.Contains(rows[1].note, "3 of 40 artists") {
		t.Errorf("the half-time rung should name 3 of 40 artists, got key %q note %q", rows[1].Key, rows[1].note)
	}
	if got := sectionLabels(rows, "Top artists by time"); strings.Join(got, ",") != "Alice,Bob,Carol,Dana" {
		t.Errorf("top artists should run in hours order, got %v", got)
	}
}

func TestConcentrationRowsSkipEmptyRungsAndEmptyWindows(t *testing.T) {
	v := rankingsFixtureView(PanelConcentration)
	v.Snap.Rankings.Ladder.Artists25 = 0

	// A rung nobody reaches would open an empty set, so it is left off rather
	// than offered as a drill that dead-ends.
	if got := sectionLabels(v.Rows(), "Repertoire ladder"); len(got) != 3 || got[0] != "50% of your time" {
		t.Errorf("want the 25%% rung dropped, got %v", got)
	}

	v.Snap.Rankings = insightsdata.Rankings{}
	if rows := v.Rows(); rows != nil {
		t.Errorf("no artists should produce no rows, got %v", rows)
	}
}

func TestConcentrationHeaderNamesTheRutTheShortcutAvoids(t *testing.T) {
	v := rankingsFixtureView(PanelConcentration)
	s := styles.NewStyles(false)

	header := strings.Join(v.concentrationHeaderLines(s), "\n")
	for _, want := range []string{"40 artists in this window", "17 of them cover 90%", "outside the top 20"} {
		if !strings.Contains(header, want) {
			t.Errorf("header should contain %q, got:\n%s", want, header)
		}
	}

	// Narrow drops the ratio bar and truncates the summary to the width, but the
	// ladder's far rung still has to reach the reader.
	v.SetSize(60, 30)
	if compact := strings.Join(v.concentrationHeaderLines(s), "\n"); !strings.Contains(compact, "17 of them cover 90%") {
		t.Errorf("compact header should keep the summary, got:\n%s", compact)
	}
}

func TestTimeVsPlaysRowsLeadWithTheBiggestMover(t *testing.T) {
	rows := rankingsFixtureView(PanelTimeVsPlays).Rows()

	// Biggest mover first: the row that most contradicts the play count is the
	// one the panel exists to show.
	if got := sectionLabels(rows, "Moved by hours"); strings.Join(got, ",") != "Dana,Bob,Carol" {
		t.Errorf("want Dana's two-place fall first, got %v", got)
	}
	if got := sectionLabels(rows, "Same either way"); strings.Join(got, ",") != "Alice" {
		t.Errorf("want only Alice unmoved, got %v", got)
	}
	if !strings.Contains(rows[0].note, "plays #2 · hours #4 · ▼2") {
		t.Errorf("Dana's note should spell out both ranks and the move, got %q", rows[0].note)
	}
	if rows[0].Drill != DrillArtistPlays || rows[0].Key != "Dana" {
		t.Errorf("a mover should open its own plays, got %v/%q", rows[0].Drill, rows[0].Key)
	}
}

func TestInsightsDeltaText(t *testing.T) {
	// chart.SafeText strips escapes from a bar's note, so the arrow has to be
	// plain text rather than a styled chart.Delta.
	cases := map[int]string{3: "▲3", -3: "▼3", 0: "="}
	for delta, want := range cases {
		if got := insightsDeltaText(delta); got != want {
			t.Errorf("%d: want %q, got %q", delta, want, got)
		}
	}
}

func TestTimeVsPlaysHeaderShrinksRatherThanVanishing(t *testing.T) {
	v := rankingsFixtureView(PanelTimeVsPlays)
	s := styles.NewStyles(false)

	full := v.timeVsPlaysHeaderLines(s)
	joined := strings.Join(full, "\n")
	for _, want := range []string{"by plays", "by hours", "▼2", "3 of 4 artists rank differently"} {
		if !strings.Contains(joined, want) {
			t.Errorf("header should contain %q, got:\n%s", want, joined)
		}
	}

	// Unlike the other panels' chart blocks the table is the panel itself, so a
	// tight budget shortens it instead of dropping it whole.
	v.SetSize(100, 15)
	if b := v.headerBudget(); b >= len(full) {
		t.Fatalf("precondition: budget %d should be tighter than the full header's %d lines", b, len(full))
	}
	tight := v.timeVsPlaysHeaderLines(s)
	if len(tight) >= len(full) {
		t.Errorf("tight header should be shorter: %d vs %d lines", len(tight), len(full))
	}
	if !strings.Contains(strings.Join(tight, "\n"), "by hours") {
		t.Errorf("tight header should keep the comparison, got:\n%s", strings.Join(tight, "\n"))
	}
}

func TestTimeVsPlaysHeaderMarksOnlyTheComparedHead(t *testing.T) {
	v := rankingsFixtureView(PanelTimeVsPlays)
	// Deltas only covers the head of the play ranking, so an artist high on hours
	// alone gets no arrow rather than a "=" it has not earned.
	v.Snap.Rankings.Deltas = v.Snap.Rankings.Deltas[:1]

	lines := v.timeVsPlaysHeaderLines(styles.NewStyles(false))
	var bob string
	for _, l := range lines {
		if strings.Contains(l, " 2 Bob") {
			bob = l
		}
	}
	if bob == "" {
		t.Fatalf("expected a Bob row in the hours column, got:\n%s", strings.Join(lines, "\n"))
	}
	if strings.ContainsAny(bob, "▲▼=") {
		t.Errorf("Bob is outside the compared head and should carry no mark, got %q", bob)
	}
}

func TestTimeVsPlaysEmptyWindow(t *testing.T) {
	v := View{Panel: PanelTimeVsPlays, width: 100, Height: 30}

	if rows := v.Rows(); rows != nil {
		t.Errorf("no deltas should produce no rows, got %v", rows)
	}
	if got := strings.Join(v.timeVsPlaysHeaderLines(styles.NewStyles(false)), "\n"); !strings.Contains(got, "no artists in this window") {
		t.Errorf("empty header should say so, got:\n%s", got)
	}
}
