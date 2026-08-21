package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/insights"
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

func sectionLabels(rows []insightsRow, section string) []string {
	var out []string
	for _, r := range rows {
		if r.section == section {
			out = append(out, r.label)
		}
	}
	return out
}

func TestRhythmRowsListEveryWeekdayIncludingSilentOnes(t *testing.T) {
	var r insights.Rhythm
	for i := range r.Weekdays {
		r.Weekdays[i].Weekday = time.Weekday(i)
	}
	r.Weekdays[time.Monday].Plays = 4
	r.Hours[9] = insights.HourBucket{Hour: 9, Plays: 4, ListenedSecs: 600}
	r.TotalPlays = 4

	rows := InsightsView{panel: panelRhythm, snap: insights.Snapshot{Rhythm: r}}.rows()

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
	genres := []insights.GenreGroup{{Canonical: "Ambient", Plays: 3, Variants: []string{"Ambient", "ambient"}}}
	snap := insights.Snapshot{Rhythm: insights.Rhythm{Genres: genres, TotalPlays: 3}}

	snap.Rhythm.GenresUnclean = false
	clean := InsightsView{panel: panelRhythm, snap: snap}.rows()
	if got := sectionLabels(clean, "Genre affinity"); len(got) != 1 {
		t.Fatalf("clean genres should produce an affinity row, got %v", got)
	}

	snap.Rhythm.GenresUnclean = true
	dirty := InsightsView{panel: panelRhythm, snap: snap}.rows()
	if got := sectionLabels(dirty, "Genre affinity"); got != nil {
		t.Errorf("colliding genres should withhold the affinity rows, got %v", got)
	}
}

func TestRhythmHeaderExplainsTheWithheldAffinityEvenWhenCompact(t *testing.T) {
	snap := insights.Snapshot{Rhythm: insights.Rhythm{TotalPlays: 3, GenresUnclean: true}}
	s := styles.NewStyles(false)

	// The gate outlives the grid: at 60 columns there is no heatmap to hide, but
	// the rows are withheld just the same, so the reason still has to show.
	for _, w := range []int{60, 100} {
		v := InsightsView{panel: panelRhythm, snap: snap, width: w, height: 30, loc: panelsTestLoc()}
		if !strings.Contains(strings.Join(v.panelHeaderLines(s), "\n"), "genre names collide") {
			t.Errorf("width %d: header should explain why affinity is hidden", w)
		}
	}
}

func TestMomentumRowsRunNewestFirstAndNameSilentMonths(t *testing.T) {
	snap := insights.Snapshot{Trends: insights.Trends{Buckets: []insights.TrendBucket{
		{Start: at(time.June, 1, 0), Plays: 2, ListenedSecs: 600},
		{Start: at(time.July, 1, 0)},
		{Start: at(time.August, 1, 0), Plays: 5, ListenedSecs: 900},
	}}}

	rows := InsightsView{panel: panelMomentum, snap: snap, loc: panelsTestLoc()}.rows()

	want := []string{"August 2026", "July 2026", "June 2026"}
	if got := sectionLabels(rows, "Month by month"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("want %v newest first, got %v", want, got)
	}
	if rows[1].note != "silent" {
		t.Errorf("a month with no plays should read as silent, got %q", rows[1].note)
	}
	if rows[0].key != "2026-08" {
		t.Errorf("drill key should be the sortable month, got %q", rows[0].key)
	}
}

func TestDiscoveryRowsLeadTheBacklogWithItsTotal(t *testing.T) {
	v := InsightsView{
		panel: panelDiscovery,
		snap: insights.Snapshot{
			Trends:   insights.Trends{Buckets: []insights.TrendBucket{{Start: at(time.August, 1, 0), FirstPlays: 2}}},
			Coverage: insights.Coverage{UnheardTracks: 5, UnheardSecs: 900},
		},
		backlog: []insightsBacklogMonth{{Key: "2026-08", Label: "August 2026", Tracks: 2, Secs: 400}},
		loc:     panelsTestLoc(),
	}

	rows := v.rows()

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
		if r.label == "Everything unheard" && r.drill != drillUnheardAll {
			t.Errorf("total row should drill to the whole unheard set, got %v", r.drill)
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

	got := backlogMonths(tracks, loc)

	if len(got) != 2 {
		t.Fatalf("want two add-months, got %+v", got)
	}
	if got[0].Key != "2026-08" || got[0].Tracks != 2 || got[0].Secs != 300 {
		t.Errorf("August bucket wrong: %+v", got[0])
	}
	if got[1].Key != "2026-06" || got[1].Tracks != 1 {
		t.Errorf("June bucket should exclude the heard track: %+v", got[1])
	}
	if got[0].Label != "August 2026" {
		t.Errorf("label should be readable, got %q", got[0].Label)
	}
}

func TestInsightsHourGenreFold(t *testing.T) {
	genreOf := map[string][]string{"a": {"Ambient", "Drone"}, "b": {"ambient"}}
	plays := []insights.PlayRow{
		{Hash: "a", StartAt: at(time.August, 3, 9).Unix()},
		{Hash: "a", StartAt: at(time.August, 4, 9).Unix()},
		{Hash: "b", StartAt: at(time.August, 5, 22).Unix()},
	}

	got := insightsHourGenreFold(plays, func(h string) []string { return genreOf[h] }, panelsTestLoc())

	if len(got) != 2 {
		t.Fatalf("want Ambient and Drone rows, got %+v", got)
	}
	// "ambient" folds into "Ambient" the same way the aggregate above the grid
	// folds it, so a row and its heading cannot disagree on what one genre is.
	if got[0].Genre != "Ambient" || got[0].Hours[9] != 2 || got[0].Hours[22] != 1 {
		t.Errorf("Ambient should lead with 2 plays at 09h and 1 at 22h, got %+v", got[0])
	}
	if got[1].Genre != "Drone" || got[1].Hours[9] != 2 {
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
	tall := InsightsView{width: 100, height: 30}
	if got := tall.headerBudget(); got != 30-1-len(insightsPanelSpecs)-insightsMinBodyRows {
		t.Errorf("tall budget: got %d", got)
	}
	// At 60x15 the briefing list is dropped, so the header gets those lines back.
	short := InsightsView{width: 60, height: 15}
	if short.briefingFits() {
		t.Fatal("precondition: the briefing list should not fit at 60x15")
	}
	if got, want := short.headerBudget(), 15-1-insightsMinBodyRows; got != want {
		t.Errorf("short budget: want %d, got %d", want, got)
	}
	// Even a terminal with no room at all keeps the header's blank line and
	// headline rather than going negative.
	if got := (InsightsView{width: 60, height: 3}).headerBudget(); got != insightsMinHeaderLines {
		t.Errorf("floor: want %d, got %d", insightsMinHeaderLines, got)
	}
}

// --- drills ------------------------------------------------------------------

// newTimeSeriesModel installs a hand-built history so each drill has a known
// hour, weekday and month behind it.
func newTimeSeriesModel(t *testing.T) Model {
	t.Helper()
	m := newInsightsModel(t)
	m.insights.loc = panelsTestLoc()
	// 3 Aug 2026 is a Monday, 5 Aug a Wednesday, 1 Jun a Monday.
	m.insights.plays = []insights.PlayRow{
		{Hash: "h1", StartAt: at(time.August, 3, 9).Unix()},
		{Hash: "h4", StartAt: at(time.August, 3, 9).Unix()},
		{Hash: "h1", StartAt: at(time.August, 5, 22).Unix()},
		{Hash: "h6", StartAt: at(time.August, 5, 22).Unix()},
	}
	m.insights.timeline = []insights.TimelineRow{
		{Hash: "h6", StartAt: at(time.June, 1, 9).Unix(), FirstEver: true},
		{Hash: "h1", StartAt: at(time.August, 3, 9).Unix(), FirstEver: true},
		{Hash: "h4", StartAt: at(time.August, 3, 9).Unix(), FirstEver: true},
		{Hash: "h1", StartAt: at(time.August, 5, 22).Unix()},
	}
	return m
}

func drilled(m Model, row insightsRow) string {
	var out []string
	for _, tr := range m.insightsDrillTracks(row) {
		out = append(out, tr.Hash)
	}
	return strings.Join(out, ",")
}

func TestInsightsTimeSeriesDrills(t *testing.T) {
	m := newTimeSeriesModel(t)
	m.insights.loc = panelsTestLoc()

	cases := []struct {
		name string
		row  insightsRow
		want string
	}{
		{"hour", insightsRow{drill: drillHour, key: "9"}, "h1,h4"},
		{"hour with no plays", insightsRow{drill: drillHour, key: "3"}, ""},
		{"weekday monday", insightsRow{drill: drillWeekday, key: "1"}, "h1,h4"},
		{"weekday wednesday", insightsRow{drill: drillWeekday, key: "3"}, "h1,h6"},
		// h1 and h2 are Rock, h6 is Folk; only h1 was actually played.
		{"genre", insightsRow{drill: drillRhythmGenre, key: "rock"}, "h1"},
		{"month", insightsRow{drill: drillTrendBucket, key: "2026-08"}, "h1,h4"},
		{"first heard month", insightsRow{drill: drillFirstPlaysBucket, key: "2026-06"}, "h6"},
		// August had a repeat of h1 but only h4 and h1 were first heard there.
		{"first heard excludes repeats", insightsRow{drill: drillFirstPlaysBucket, key: "2026-08"}, "h1,h4"},
		{"unparseable key", insightsRow{drill: drillHour, key: "not-a-number"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := drilled(m, tc.row); got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestInsightsMonthDrillReplaysInTheOrderItWasHeard(t *testing.T) {
	m := newTimeSeriesModel(t)
	m.insights.timeline = []insights.TimelineRow{
		{Hash: "h4", StartAt: at(time.August, 3, 9).Unix()},
		{Hash: "h1", StartAt: at(time.August, 3, 10).Unix()},
		{Hash: "h4", StartAt: at(time.August, 3, 11).Unix()},
	}

	// Deduping on first appearance rather than last is what makes "enter replays
	// a month in order" true: h4 keeps the slot of its first play, not its last.
	if got := drilled(m, insightsRow{drill: drillTrendBucket, key: "2026-08"}); got != "h4,h1" {
		t.Errorf("want h4,h1, got %q", got)
	}
}

func TestMomentumEnqueuesAMonthInTheOrderItWasHeard(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newInsightsModel(t, WithBatchEnqueuer(be))
	m.insights.loc = panelsTestLoc()
	m.insights.timeline = []insights.TimelineRow{
		{Hash: "h4", StartAt: at(time.August, 3, 9).Unix()},
		{Hash: "h1", StartAt: at(time.August, 3, 10).Unix()},
		{Hash: "h6", StartAt: at(time.September, 1, 9).Unix()},
	}
	m.insights.snap.Trends.Buckets = []insights.TrendBucket{
		{Start: at(time.August, 1, 0), Plays: 2, ListenedSecs: 600},
		{Start: at(time.September, 1, 0), Plays: 1, ListenedSecs: 300},
	}
	m = m.handleInsightsPanelCycle(1).handleInsightsPanelCycle(1)
	if m.insights.panel != panelMomentum {
		t.Fatalf("precondition: want the Momentum panel, got %v", m.insights.panel)
	}
	// Rows run newest first, so one step down lands on August.
	m = pressKey(m, "j")

	m, cmd := m.handleInsightsEnqueueAll()
	if cmd == nil {
		t.Fatal("expected an enqueue cmd for the focused month")
	}
	cmd()

	if len(be.appendTracksCalls) != 1 {
		t.Fatalf("want one AppendTracks call, got %d", len(be.appendTracksCalls))
	}
	var got []string
	for _, tr := range be.appendTracksCalls[0] {
		got = append(got, tr.Hash)
	}
	if strings.Join(got, ",") != "h4,h1" {
		t.Errorf("want August replayed as h4,h1, got %v", got)
	}
	if be.appendTracksSources[0] != "August 2026" {
		t.Errorf("source should name the month, got %q", be.appendTracksSources[0])
	}
}

// TestInsightsResolutionControlChangesTrendBucketsAndDrills is the contract for
// the w key on Momentum and Discovery. Those two panels always read the whole
// history, so the control cannot be a time window: it picks the bucket size, and
// every press has to move the boundaries, the labels, the rendered series and
// the drill keys rather than relabel identical data.
func TestInsightsResolutionControlChangesTrendBucketsAndDrills(t *testing.T) {
	m := newInsightsModel(t)
	m = m.handleInsightsPanelCycle(1).handleInsightsPanelCycle(1)
	if m.insights.panel != panelMomentum || m.insights.panelSpec().class != classResolution {
		t.Fatalf("precondition: want the Momentum panel at classResolution, got %v/%v",
			m.insights.panel, m.insights.panelSpec().class)
	}

	// cycle presses w and installs the reply, which is the only way the snapshot
	// can actually be rebucketed.
	cycle := func(m Model) Model {
		t.Helper()
		m, cmd := m.handleInsightsWindowCycle(1)
		if cmd == nil {
			t.Fatal("a resolution panel should reload when the control moves")
		}
		msg, ok := cmd().(insightsLoadedMsg)
		if !ok {
			t.Fatalf("expected insightsLoadedMsg, got %T", cmd())
		}
		return m.handleInsightsLoadedMsg(msg)
	}

	type reading struct {
		res     insights.Resolution
		hint    string
		buckets []insights.TrendBucket
		labels  []string
		keys    []string
		series  string
		plays   int
		secs    int
		hashes  map[string]struct{}
	}
	read := func(m Model) reading {
		t.Helper()
		tr := m.insights.snap.Trends
		r := reading{
			res:     tr.Resolution,
			hint:    m.insights.windowHint(),
			buckets: tr.Buckets,
			series:  strings.Join(m.insights.panelHeaderLines(styles.NewStyles(false)), "\n"),
			hashes:  map[string]struct{}{},
		}
		if len(tr.Buckets) == 0 {
			t.Fatal("the fixture history should produce trend buckets")
		}
		for _, b := range tr.Buckets {
			r.plays += b.Plays
			r.secs += b.ListenedSecs
			// Every bucket is drilled, not just the ones the row cap admits, so
			// the union below is the whole series rather than its recent tail.
			key := insightsBucketKey(tr.Resolution, b.Start)
			r.labels = append(r.labels, insightsBucketLabel(tr.Resolution, b.Start))
			r.keys = append(r.keys, key)
			for _, tr := range m.insightsDrillTracks(insightsRow{drill: drillTrendBucket, key: key}) {
				r.hashes[tr.Hash] = struct{}{}
			}
		}
		return r
	}

	monthly := read(m)
	m = cycle(m)
	weekly := read(m)
	m = cycle(m)
	daily := read(m)
	m = cycle(m)
	wrapped := read(m)

	if monthly.res != insights.ResolutionMonthly || weekly.res != insights.ResolutionWeekly ||
		daily.res != insights.ResolutionDaily || wrapped.res != insights.ResolutionMonthly {
		t.Fatalf("cycle = %v, %v, %v, %v; want monthly, weekly, daily, monthly",
			monthly.res, weekly.res, daily.res, wrapped.res)
	}
	if m.insights.windowIdx != 0 {
		t.Errorf("the resolution control moved the time window too: windowIdx = %d", m.insights.windowIdx)
	}

	// The strip names the unit, not a duration: "90 Days" on a panel that reads
	// the whole history is the label that made this a bug.
	for _, tc := range []struct {
		got  string
		want string
	}{{monthly.hint, "resolution: monthly ‹w›"}, {weekly.hint, "resolution: weekly ‹w›"}, {daily.hint, "resolution: daily ‹w›"}} {
		if tc.got != tc.want {
			t.Errorf("state strip = %q, want %q", tc.got, tc.want)
		}
	}

	// Boundaries: a month bucket opens on the first, a week bucket on a Monday,
	// a day bucket on its own date.
	for _, b := range monthly.buckets {
		if b.Start.Day() != 1 {
			t.Fatalf("monthly bucket starts %s, want the first of a month", b.Start)
		}
	}
	for _, b := range weekly.buckets {
		if b.Start.Weekday() != time.Monday {
			t.Fatalf("weekly bucket starts on %v, want Monday", b.Start.Weekday())
		}
	}
	if !(len(monthly.buckets) < len(weekly.buckets) && len(weekly.buckets) < len(daily.buckets)) {
		t.Errorf("bucket counts = %d monthly, %d weekly, %d daily; want strictly increasing",
			len(monthly.buckets), len(weekly.buckets), len(daily.buckets))
	}

	// Labels, keys and the rendered bar series all move with the control.
	if monthly.series == weekly.series || weekly.series == daily.series {
		t.Error("the rendered series is identical across resolutions")
	}
	if strings.Join(monthly.labels, ",") == strings.Join(weekly.labels, ",") {
		t.Error("bucket labels are identical across resolutions")
	}
	for _, tc := range []struct {
		name string
		key  string
		want func(string) bool
	}{
		{"monthly", monthly.keys[0], func(k string) bool { return len(k) == len("2006-01") }},
		{"weekly", weekly.keys[0], func(k string) bool { return strings.Contains(k, "-W") }},
		{"daily", daily.keys[0], func(k string) bool { return len(k) == len("2006-01-02") }},
	} {
		if !tc.want(tc.key) {
			t.Errorf("%s drill key %q is not in that resolution's key space", tc.name, tc.key)
		}
	}

	// The span is unchanged: the same history, read at a different grain.
	for name, r := range map[string]reading{"weekly": weekly, "daily": daily} {
		if r.plays != monthly.plays || r.secs != monthly.secs {
			t.Errorf("%s totals = %d plays/%ds, want %d/%ds — the source span must stay all-time",
				name, r.plays, r.secs, monthly.plays, monthly.secs)
		}
		if len(r.hashes) != len(monthly.hashes) {
			t.Errorf("%s drills reach %d tracks, monthly reaches %d — every bucket must resolve to real tracks",
				name, len(r.hashes), len(monthly.hashes))
		}
		for h := range monthly.hashes {
			if _, ok := r.hashes[h]; !ok {
				t.Errorf("%s drills lost track %s", name, h)
			}
		}
	}
	if len(monthly.hashes) == 0 {
		t.Fatal("no bucket drilled into anything; the rows would be pictures")
	}

	// A key minted at one resolution must not resolve at another: the snapshot's
	// own bucket size is what a drill is answered with.
	if m.insights.snap.Trends.Resolution != insights.ResolutionMonthly {
		t.Fatalf("precondition: the cycle should have wrapped back to monthly")
	}
	if got := drilled(m, insightsRow{drill: drillTrendBucket, key: daily.keys[0]}); got != "" {
		t.Errorf("a daily key resolved against a monthly snapshot: got %q", got)
	}

	// Discovery is the other resolution panel and shares the control.
	m = m.handleInsightsPanelCycle(4)
	if m.insights.panel != panelDiscovery || m.insights.panelSpec().class != classResolution {
		t.Fatalf("precondition: want the Discovery panel at classResolution, got %v", m.insights.panel)
	}
	if got := m.insights.windowHint(); got != "resolution: monthly ‹w›" {
		t.Errorf("Discovery strip = %q, want the same unit hint", got)
	}
	before := sectionLabels(m.insights.rows(), "First heard")
	m = cycle(m)
	after := sectionLabels(m.insights.rows(), "First heard")
	if len(before) == 0 || strings.Join(before, ",") == strings.Join(after, ",") {
		t.Errorf("Discovery rows did not move with the control: %v then %v", before, after)
	}
	for _, r := range m.insights.rows() {
		if r.drill == drillFirstPlaysBucket && !strings.Contains(r.key, "-W") {
			t.Errorf("Discovery drill key %q is still monthly at weekly resolution", r.key)
		}
	}
}

func TestInsightsBacklogMonthDrill(t *testing.T) {
	tracks := insightsTestTracks()
	tracks[1].CreatedAt = at(time.August, 3, 9).Unix() // h2, never played
	tracks[2].CreatedAt = at(time.June, 1, 9).Unix()   // h3, never played
	tracks[0].CreatedAt = at(time.August, 3, 9).Unix() // h1, played — must not appear

	m := newInsightsModel(t, WithLibraryTracks(tracks))
	m.insights.loc = panelsTestLoc()

	if got := drilled(m, insightsRow{drill: drillAddedNeverPlayed, key: "2026-08"}); got != "h2" {
		t.Errorf("want only the unheard August add, got %q", got)
	}
	if got := drilled(m, insightsRow{drill: drillAddedNeverPlayed, key: "2026-06"}); got != "h3" {
		t.Errorf("want the unheard June add, got %q", got)
	}
	if got := drilled(m, insightsRow{drill: drillAddedNeverPlayed, key: "2020-01"}); got != "" {
		t.Errorf("an empty month should drill to nothing, got %q", got)
	}
}
