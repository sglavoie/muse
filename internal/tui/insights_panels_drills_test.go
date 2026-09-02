package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/insights"
	instui "github.com/sglavoie/muse/internal/tui/insights"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// newTimeSeriesModel installs a hand-built history so each drill has a known
// hour, weekday and month behind it.
func newTimeSeriesModel(t *testing.T) Model {
	t.Helper()
	m := newInsightsModel(t)
	m.insights.Loc = panelsTestLoc()
	// 3 Aug 2026 is a Monday, 5 Aug a Wednesday, 1 Jun a Monday.
	m.insights.Plays = []insights.PlayRow{
		{Hash: "h1", StartAt: at(time.August, 3, 9).Unix()},
		{Hash: "h4", StartAt: at(time.August, 3, 9).Unix()},
		{Hash: "h1", StartAt: at(time.August, 5, 22).Unix()},
		{Hash: "h6", StartAt: at(time.August, 5, 22).Unix()},
	}
	m.insights.Timeline = []insights.TimelineRow{
		{Hash: "h6", StartAt: at(time.June, 1, 9).Unix(), FirstEver: true},
		{Hash: "h1", StartAt: at(time.August, 3, 9).Unix(), FirstEver: true},
		{Hash: "h4", StartAt: at(time.August, 3, 9).Unix(), FirstEver: true},
		{Hash: "h1", StartAt: at(time.August, 5, 22).Unix()},
	}
	return m
}

func drilled(m Model, row instui.Row) string {
	var out []string
	for _, tr := range m.insightsDrillTracks(row) {
		out = append(out, tr.Hash)
	}
	return strings.Join(out, ",")
}

func TestInsightsTimeSeriesDrills(t *testing.T) {
	m := newTimeSeriesModel(t)
	m.insights.Loc = panelsTestLoc()

	cases := []struct {
		name string
		row  instui.Row
		want string
	}{
		{"hour", instui.Row{Drill: instui.DrillHour, Key: "9"}, "h1,h4"},
		{"hour with no plays", instui.Row{Drill: instui.DrillHour, Key: "3"}, ""},
		{"weekday monday", instui.Row{Drill: instui.DrillWeekday, Key: "1"}, "h1,h4"},
		{"weekday wednesday", instui.Row{Drill: instui.DrillWeekday, Key: "3"}, "h1,h6"},
		// h1 and h2 are Rock, h6 is Folk; only h1 was actually played.
		{"genre", instui.Row{Drill: instui.DrillRhythmGenre, Key: "rock"}, "h1"},
		{"month", instui.Row{Drill: instui.DrillTrendBucket, Key: "2026-08"}, "h1,h4"},
		{"first heard month", instui.Row{Drill: instui.DrillFirstPlaysBucket, Key: "2026-06"}, "h6"},
		// August had a repeat of h1 but only h4 and h1 were first heard there.
		{"first heard excludes repeats", instui.Row{Drill: instui.DrillFirstPlaysBucket, Key: "2026-08"}, "h1,h4"},
		{"unparseable key", instui.Row{Drill: instui.DrillHour, Key: "not-a-number"}, ""},
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
	m.insights.Timeline = []insights.TimelineRow{
		{Hash: "h4", StartAt: at(time.August, 3, 9).Unix()},
		{Hash: "h1", StartAt: at(time.August, 3, 10).Unix()},
		{Hash: "h4", StartAt: at(time.August, 3, 11).Unix()},
	}

	// Deduping on first appearance rather than last is what makes "enter replays
	// a month in order" true: h4 keeps the slot of its first play, not its last.
	if got := drilled(m, instui.Row{Drill: instui.DrillTrendBucket, Key: "2026-08"}); got != "h4,h1" {
		t.Errorf("want h4,h1, got %q", got)
	}
}

func TestMomentumEnqueuesAMonthInTheOrderItWasHeard(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newInsightsModel(t, WithBatchEnqueuer(be))
	m.insights.Loc = panelsTestLoc()
	m.insights.Timeline = []insights.TimelineRow{
		{Hash: "h4", StartAt: at(time.August, 3, 9).Unix()},
		{Hash: "h1", StartAt: at(time.August, 3, 10).Unix()},
		{Hash: "h6", StartAt: at(time.September, 1, 9).Unix()},
	}
	m.insights.Snap.Trends.Buckets = []insights.TrendBucket{
		{Start: at(time.August, 1, 0), Plays: 2, ListenedSecs: 600},
		{Start: at(time.September, 1, 0), Plays: 1, ListenedSecs: 300},
	}
	m = m.handleInsightsPanelCycle(1).handleInsightsPanelCycle(1)
	if m.insights.Panel != instui.PanelMomentum {
		t.Fatalf("precondition: want the Momentum panel, got %v", m.insights.Panel)
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
	if m.insights.Panel != instui.PanelMomentum || m.insights.Spec().Class != instui.ClassResolution {
		t.Fatalf("precondition: want the Momentum panel at classResolution, got %v/%v",
			m.insights.Panel, m.insights.Spec().Class)
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
		tr := m.insights.Snap.Trends
		r := reading{
			res:     tr.Resolution,
			hint:    m.insights.WindowHint(),
			buckets: tr.Buckets,
			series:  strings.Join(m.insights.PanelHeaderLines(styles.NewStyles(false)), "\n"),
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
			key := instui.BucketKey(tr.Resolution, b.Start)
			r.labels = append(r.labels, instui.BucketLabel(tr.Resolution, b.Start))
			r.keys = append(r.keys, key)
			for _, tr := range m.insightsDrillTracks(instui.Row{Drill: instui.DrillTrendBucket, Key: key}) {
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
	if m.insights.WindowIdx != 0 {
		t.Errorf("the resolution control moved the time window too: windowIdx = %d", m.insights.WindowIdx)
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
	if m.insights.Snap.Trends.Resolution != insights.ResolutionMonthly {
		t.Fatalf("precondition: the cycle should have wrapped back to monthly")
	}
	if got := drilled(m, instui.Row{Drill: instui.DrillTrendBucket, Key: daily.keys[0]}); got != "" {
		t.Errorf("a daily key resolved against a monthly snapshot: got %q", got)
	}

	// Discovery is the other resolution panel and shares the control.
	m = m.handleInsightsPanelCycle(4)
	if m.insights.Panel != instui.PanelDiscovery || m.insights.Spec().Class != instui.ClassResolution {
		t.Fatalf("precondition: want the Discovery panel at classResolution, got %v", m.insights.Panel)
	}
	if got := m.insights.WindowHint(); got != "resolution: monthly ‹w›" {
		t.Errorf("Discovery strip = %q, want the same unit hint", got)
	}
	before := sectionLabels(m.insights.Rows(), "First heard")
	m = cycle(m)
	after := sectionLabels(m.insights.Rows(), "First heard")
	if len(before) == 0 || strings.Join(before, ",") == strings.Join(after, ",") {
		t.Errorf("Discovery rows did not move with the control: %v then %v", before, after)
	}
	for _, r := range m.insights.Rows() {
		if r.Drill == instui.DrillFirstPlaysBucket && !strings.Contains(r.Key, "-W") {
			t.Errorf("Discovery drill key %q is still monthly at weekly resolution", r.Key)
		}
	}
}

func TestInsightsBacklogMonthDrill(t *testing.T) {
	tracks := insightsTestTracks()
	tracks[1].CreatedAt = at(time.August, 3, 9).Unix() // h2, never played
	tracks[2].CreatedAt = at(time.June, 1, 9).Unix()   // h3, never played
	tracks[0].CreatedAt = at(time.August, 3, 9).Unix() // h1, played — must not appear

	m := newInsightsModel(t, WithLibraryTracks(tracks))
	m.insights.Loc = panelsTestLoc()

	if got := drilled(m, instui.Row{Drill: instui.DrillAddedNeverPlayed, Key: "2026-08"}); got != "h2" {
		t.Errorf("want only the unheard August add, got %q", got)
	}
	if got := drilled(m, instui.Row{Drill: instui.DrillAddedNeverPlayed, Key: "2026-06"}); got != "h3" {
		t.Errorf("want the unheard June add, got %q", got)
	}
	if got := drilled(m, instui.Row{Drill: instui.DrillAddedNeverPlayed, Key: "2020-01"}); got != "" {
		t.Errorf("an empty month should drill to nothing, got %q", got)
	}
}
