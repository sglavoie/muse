package insights

import (
	"strings"
	"testing"
	"time"

	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// panelsTestLoc is fixed so a bucket's hour, weekday and month cannot depend on
// where the test runs.
func panelsTestLoc() *time.Location { return time.UTC }

// at builds a UTC instant for the fixed 2026 dates these tests bucket against.
func at(month time.Month, day, hour int) time.Time {
	return time.Date(2026, month, day, hour, 30, 0, 0, panelsTestLoc())
}

func sectionLabels(rows []Row, section string) []string {
	var out []string
	for _, r := range rows {
		if r.Section == section {
			out = append(out, r.Label)
		}
	}
	return out
}

func TestRhythmRowsListEveryWeekdayIncludingSilentOnes(t *testing.T) {
	var r insightsdata.Rhythm
	for i := range r.Weekdays {
		r.Weekdays[i].Weekday = time.Weekday(i)
	}
	r.Weekdays[time.Monday].Plays = 4
	r.Hours[9] = insightsdata.HourBucket{Hour: 9, Plays: 4, ListenedSecs: 600}
	r.TotalPlays = 4

	rows := View{Panel: panelRhythm, Snap: insightsdata.Snapshot{Rhythm: r}}.Rows()

	if got := sectionLabels(rows, "Weekday"); len(got) != 7 {
		t.Errorf("want all 7 weekdays listed, got %v", got)
	}
	// Only the hours that recorded something become rows; the silent ones are
	// the punchcard's gaps, not 23 empty lines of body.
	if got := sectionLabels(rows, "Hour of day"); len(got) != 1 || got[0] != "09:00" {
		t.Errorf("want only the 09:00 hour row, got %v", got)
	}
}

func TestRhythmRowsHideGenreAffinityWhenGenresCollide(t *testing.T) {
	genres := []insightsdata.GenreGroup{{Canonical: "Ambient", Plays: 3, Variants: []string{"Ambient", "ambient"}}}
	snap := insightsdata.Snapshot{Rhythm: insightsdata.Rhythm{Genres: genres, TotalPlays: 3}}

	snap.Rhythm.GenresUnclean = false
	clean := View{Panel: panelRhythm, Snap: snap}.Rows()
	if got := sectionLabels(clean, "Genre affinity"); len(got) != 1 {
		t.Fatalf("clean genres should produce an affinity row, got %v", got)
	}

	snap.Rhythm.GenresUnclean = true
	dirty := View{Panel: panelRhythm, Snap: snap}.Rows()
	if got := sectionLabels(dirty, "Genre affinity"); got != nil {
		t.Errorf("colliding genres should withhold the affinity rows, got %v", got)
	}
}

func TestRhythmHeaderExplainsTheWithheldAffinityEvenWhenCompact(t *testing.T) {
	snap := insightsdata.Snapshot{Rhythm: insightsdata.Rhythm{TotalPlays: 3, GenresUnclean: true}}
	s := styles.NewStyles(false)

	// The gate outlives the grid: at 60 columns there is no heatmap to hide, but
	// the rows are withheld just the same, so the reason still has to show.
	for _, w := range []int{60, 100} {
		v := View{Panel: panelRhythm, Snap: snap, width: w, Height: 30, Loc: panelsTestLoc()}
		if !strings.Contains(strings.Join(v.PanelHeaderLines(s), "\n"), "genre names collide") {
			t.Errorf("width %d: header should explain why affinity is hidden", w)
		}
	}
}

func TestMomentumRowsRunNewestFirstAndNameSilentMonths(t *testing.T) {
	snap := insightsdata.Snapshot{Trends: insightsdata.Trends{Buckets: []insightsdata.TrendBucket{
		{Start: at(time.June, 1, 0), Plays: 2, ListenedSecs: 600},
		{Start: at(time.July, 1, 0)},
		{Start: at(time.August, 1, 0), Plays: 5, ListenedSecs: 900},
	}}}

	rows := View{Panel: PanelMomentum, Snap: snap, Loc: panelsTestLoc()}.Rows()

	want := []string{"August 2026", "July 2026", "June 2026"}
	if got := sectionLabels(rows, "Month by month"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("want %v newest first, got %v", want, got)
	}
	if rows[1].note != "silent" {
		t.Errorf("a month with no plays should read as silent, got %q", rows[1].note)
	}
	if rows[0].Key != "2026-08" {
		t.Errorf("drill key should be the sortable month, got %q", rows[0].Key)
	}
}

func TestDiscoveryRowsLeadTheBacklogWithItsTotal(t *testing.T) {
	v := View{
		Panel: PanelDiscovery,
		Snap: insightsdata.Snapshot{
			Trends:   insightsdata.Trends{Buckets: []insightsdata.TrendBucket{{Start: at(time.August, 1, 0), FirstPlays: 2}}},
			Coverage: insightsdata.Coverage{UnheardTracks: 5, UnheardSecs: 900},
		},
		Backlog: []BacklogMonth{{key: "2026-08", label: "August 2026", tracks: 2, secs: 400}},
		Loc:     panelsTestLoc(),
	}

	rows := v.Rows()

	if got := sectionLabels(rows, "First heard"); len(got) != 1 || got[0] != "August 2026" {
		t.Errorf("want one first-heard month row, got %v", got)
	}
	backlog := sectionLabels(rows, "Added but never played")
	if len(backlog) != 2 || backlog[0] != "Everything unheard" {
		t.Fatalf("backlog should lead with its total, got %v", backlog)
	}
	// The per-month rows only cover tracks whose add date was recorded, so the
	// total row is the only way the rest of the backlog stays reachable.
	for _, r := range rows {
		if r.Label == "Everything unheard" && r.Drill != DrillUnheardAll {
			t.Errorf("total row should drill to the whole unheard set, got %v", r.Drill)
		}
	}
}

func TestBacklogMonths(t *testing.T) {
	loc := panelsTestLoc()
	tracks := []library.Track{
		{Hash: "a", Duration: 100, CreatedAt: at(time.August, 3, 9).Unix()},
		{Hash: "b", Duration: 200, CreatedAt: at(time.August, 20, 9).Unix()},
		{Hash: "c", Duration: 300, CreatedAt: at(time.June, 1, 9).Unix()},
		{Hash: "heard", Duration: 400, CreatedAt: at(time.June, 1, 9).Unix(), PlayCount: 3},
		{Hash: "no-add-date", Duration: 500},
	}

	got := BacklogMonths(tracks, loc)

	if len(got) != 2 {
		t.Fatalf("want two add-months, got %+v", got)
	}
	if got[0].key != "2026-08" || got[0].tracks != 2 || got[0].secs != 300 {
		t.Errorf("August bucket wrong: %+v", got[0])
	}
	if got[1].key != "2026-06" || got[1].tracks != 1 {
		t.Errorf("June bucket should exclude the heard track: %+v", got[1])
	}
	if got[0].label != "August 2026" {
		t.Errorf("label should be readable, got %q", got[0].label)
	}
}

func TestInsightsHourGenreFold(t *testing.T) {
	genreOf := map[string][]string{"a": {"Ambient", "Drone"}, "b": {"ambient"}}
	plays := []insightsdata.PlayRow{
		{Hash: "a", StartAt: at(time.August, 3, 9).Unix()},
		{Hash: "a", StartAt: at(time.August, 4, 9).Unix()},
		{Hash: "b", StartAt: at(time.August, 5, 22).Unix()},
	}

	got := HourGenreFold(plays, func(h string) []string { return genreOf[h] }, panelsTestLoc())

	if len(got) != 2 {
		t.Fatalf("want Ambient and Drone rows, got %+v", got)
	}
	// "ambient" folds into "Ambient" the same way the aggregate above the grid
	// folds it, so a row and its heading cannot disagree on what one genre is.
	if got[0].genre != "Ambient" || got[0].hours[9] != 2 || got[0].hours[22] != 1 {
		t.Errorf("Ambient should lead with 2 plays at 09h and 1 at 22h, got %+v", got[0])
	}
	if got[1].genre != "Drone" || got[1].hours[9] != 2 {
		t.Errorf("Drone row wrong: %+v", got[1])
	}
}

func TestInsightsFitHeaderDropsWholeBlocks(t *testing.T) {
	head := []string{"h1", "h2"}
	tail := []string{"t1"}
	big := []string{"b1", "b2", "b3"}
	small := []string{"s1"}

	if got := insightsFitHeader(10, head, [][]string{big, small}, tail); len(got) != 7 {
		t.Errorf("everything fits: want 7 lines, got %v", got)
	}
	// A budget that fits neither block still keeps the prose either side.
	if got := insightsFitHeader(3, head, [][]string{big, small}, tail); strings.Join(got, ",") != "h1,h2,t1" {
		t.Errorf("tight budget should keep only head and tail, got %v", got)
	}
	// The first block is dropped whole rather than clipped, and the lesser block
	// behind it goes too: showing the heatmap without the punchcard that carries
	// its hour scale would invert the priority the caller expressed.
	got := insightsFitHeader(4, head, [][]string{big, small}, tail)
	if strings.Join(got, ",") != "h1,h2,t1" {
		t.Errorf("a block that does not fit should end the run, got %v", got)
	}
	// Reversing the order proves the cutoff is positional, not size-based.
	if got := insightsFitHeader(4, head, [][]string{small, big}, tail); strings.Join(got, ",") != "h1,h2,s1,t1" {
		t.Errorf("want the small block kept when it comes first, got %v", got)
	}
}

func TestInsightsHeaderBudgetLeavesRoomForBody(t *testing.T) {
	tall := View{width: 100, Height: 30}
	if got := tall.headerBudget(); got != 30-1-len(insightsPanelSpecs)-insightsMinBodyRows {
		t.Errorf("tall budget: got %d", got)
	}
	// At 60x15 the briefing list is dropped, so the header gets those lines back.
	short := View{width: 60, Height: 15}
	if short.briefingFits() {
		t.Fatal("precondition: the briefing list should not fit at 60x15")
	}
	if got, want := short.headerBudget(), 15-1-insightsMinBodyRows; got != want {
		t.Errorf("short budget: want %d, got %d", want, got)
	}
	// Even a terminal with no room at all keeps the header's blank line and
	// headline rather than going negative.
	if got := (View{width: 60, Height: 3}).headerBudget(); got != insightsMinHeaderLines {
		t.Errorf("floor: want %d, got %d", insightsMinHeaderLines, got)
	}
}
