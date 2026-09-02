package insights

import (
	"strconv"
	"strings"
	"testing"
	"time"

	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// sessionsFixture is four sessions with deliberately different lengths, shapes
// and artists, so every Sessions section has something to say.
func sessionsFixture() []insightsdata.SessionRow {
	base := at(time.August, 3, 9).Unix()
	return []insightsdata.SessionRow{
		// 10m, one play, one artist: the false start.
		{ID: "s1", StartedAt: base, EndedAt: base + 600, Plays: 1, ListenedSecs: 600,
			DistinctArtists: 1, OpenerArtist: "Alice", CloserArtist: "Alice"},
		// 45m, three artists.
		{ID: "s2", StartedAt: base + 86400, EndedAt: base + 86400 + 45*60, Plays: 8, ListenedSecs: 2400,
			DistinctArtists: 3, OpenerArtist: "Bob", CloserArtist: "Alice"},
		// 90m, five artists.
		{ID: "s3", StartedAt: base + 2*86400, EndedAt: base + 2*86400 + 90*60, Plays: 20, ListenedSecs: 5000,
			DistinctArtists: 5, OpenerArtist: "Alice", CloserArtist: "Carol"},
		// 20m, one artist.
		{ID: "s4", StartedAt: base + 3*86400, EndedAt: base + 3*86400 + 20*60, Plays: 4, ListenedSecs: 1100,
			DistinctArtists: 1, OpenerArtist: "Bob", CloserArtist: "Bob"},
	}
}

func sessionsFixtureView() View {
	rows := sessionsFixture()
	durations := make([]int, len(rows))
	for i, r := range rows {
		durations[i] = insightsSessionSecs(r)
	}
	return View{
		Panel:    PanelSessions,
		Sessions: rows,
		width:    100,
		Height:   30,
		Loc:      panelsTestLoc(),
		Snap: insightsdata.Snapshot{SessionStats: insightsdata.SessionStats{
			Count: len(rows), SinglePlay: 1, TotalSecs: 9100, LongestSecs: 90 * 60,
			MedianSecs: 20 * 60, AvgPlays: 8.25, Durations: durations,
			TopOpeners: []insightsdata.EntityStat{{Name: "Alice", Plays: 2, ListenedSecs: 5600}, {Name: "Bob", Plays: 2, ListenedSecs: 3500}},
			TopClosers: []insightsdata.EntityStat{{Name: "Alice", Plays: 2, ListenedSecs: 3000}},
		}},
	}
}

func TestInsightsDurationBucketBoundaries(t *testing.T) {
	cases := []struct {
		secs int
		want string
	}{
		{0, "under 15m"},
		{15*60 - 1, "under 15m"},
		// The bounds are exclusive upper edges, so a session of exactly 15
		// minutes belongs to the bucket above rather than to both.
		{15 * 60, "15–30m"},
		{30 * 60, "30m–1h"},
		{3600, "1–2h"},
		{2 * 3600, "2–4h"},
		{4 * 3600, "4h+"},
		{40 * 3600, "4h+"},
	}
	for _, tc := range cases {
		if got := insightsDurationBuckets[insightsDurationBucketOf(tc.secs)].label; got != tc.want {
			t.Errorf("%ds: want %q, got %q", tc.secs, tc.want, got)
		}
	}
}

func TestInsightsSessionsMatching(t *testing.T) {
	rows := sessionsFixture()

	ids := func(got []insightsdata.SessionRow) string {
		var out []string
		for _, r := range got {
			out = append(out, r.ID)
		}
		return strings.Join(out, ",")
	}

	cases := []struct {
		name string
		kind DrillKind
		key  string
		want string
	}{
		// Newest first: a session list is read like history, most recent at top.
		{"all", DrillSessionsAll, "", "s4,s3,s2,s1"},
		{"under 15m bucket", drillSessionsBucket, "0", "s1"},
		{"15-30m bucket", drillSessionsBucket, "1", "s4"},
		{"empty bucket", drillSessionsBucket, "5", ""},
		{"unparseable bucket", drillSessionsBucket, "x", ""},
		{"single play", drillSessionsShape, insightsShapeSinglePlay, "s1"},
		{"one artist", drillSessionsShape, insightsDietOne, "s4,s1"},
		{"few artists", drillSessionsShape, insightsDietFew, "s2"},
		{"many artists", drillSessionsShape, insightsDietMany, "s3"},
		{"opener", DrillSessionsOpener, "Alice", "s3,s1"},
		// s1 both opens and closes with Alice, so it belongs to either list.
		{"closer", DrillSessionsCloser, "Alice", "s2,s1"},
		{"non-session kind", DrillUnheardAll, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ids(SessionsMatching(rows, tc.kind, tc.key)); got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSessionRowsCountsMatchWhatTheyOpen(t *testing.T) {
	v := sessionsFixtureView()
	rows := v.sessionRows()

	if got := sectionLabels(rows, "Session length"); strings.Join(got, ",") != "under 15m,15–30m,30m–1h,1–2h" {
		t.Errorf("want only the populated length buckets, got %v", got)
	}
	if got := sectionLabels(rows, "False starts"); len(got) != 1 {
		t.Errorf("want the single-play row, got %v", got)
	}
	// SessionStats does not carry the split above one artist, so the diet comes
	// off the raw rows; all three bands are present in this fixture.
	if got := sectionLabels(rows, "Session diet"); strings.Join(got, ",") != "One artist only,Two or three artists,Four or more artists" {
		t.Errorf("want the full diet split, got %v", got)
	}
	if got := sectionLabels(rows, "Opens with"); strings.Join(got, ",") != "Alice,Bob" {
		t.Errorf("want the openers, got %v", got)
	}
	if got := sectionLabels(rows, "Ends with"); strings.Join(got, ",") != "Alice" {
		t.Errorf("want the closers, got %v", got)
	}

	// A row's number and the set it opens come from the same bucketing, so they
	// cannot drift the way a separately-computed count would.
	for _, r := range rows {
		if !r.Drill.OpensSessions() {
			t.Errorf("every Sessions row should open sessions, %q does not", r.Label)
			continue
		}
		n := len(SessionsMatching(v.Sessions, r.Drill, r.Key))
		if n == 0 || !strings.HasPrefix(r.note, strconv.Itoa(n)+" ") {
			t.Errorf("row %q notes %q but opens %d sessions", r.Label, r.note, n)
		}
	}
}

func TestSessionRowsEmptyWithoutSessions(t *testing.T) {
	if rows := (View{Panel: PanelSessions}).sessionRows(); rows != nil {
		t.Errorf("no sessions should produce no rows, got %v", rows)
	}
}

func TestSessionListHeaderStaysInLockstepWithSizing(t *testing.T) {
	v := sessionsFixtureView()
	v.OpenSessionList("All sessions", SessionsMatching(v.Sessions, DrillSessionsAll, ""))

	styled := v.sessionListHeaderLines(styles.NewStyles(false))
	zero := v.sessionListHeaderLines(styles.Styles{})
	if len(styled) != len(zero) {
		t.Fatalf("styled header is %d lines, zero-styles header is %d — sizing would drift", len(styled), len(zero))
	}
	if got, want := v.sessionBodyRows(), v.Height-len(zero); got != want {
		t.Errorf("body rows: want %d, got %d", want, got)
	}

	lines := strings.Split(v.renderSessionList(styles.NewStyles(false)), "\n")
	if len(lines) != len(zero)+len(v.SessionList) {
		t.Errorf("want %d header lines plus %d sessions, got %d lines", len(zero), len(v.SessionList), len(lines))
	}
	if want := insightsSessionArc(v.SessionList[0]); !strings.Contains(lines[len(zero)], want) {
		t.Errorf("first session row should name its arc %q, got %q", want, lines[len(zero)])
	}
}

func TestRenderSessionListClampsToHeight(t *testing.T) {
	v := sessionsFixtureView()
	v.SetSize(60, 8)
	v.OpenSessionList("All sessions", SessionsMatching(v.Sessions, DrillSessionsAll, ""))

	if got := len(strings.Split(v.renderSessionList(styles.NewStyles(false)), "\n")); got > 8 {
		t.Errorf("rendered %d lines into a height of 8", got)
	}
}

func TestInsightsSessionArc(t *testing.T) {
	cases := []struct {
		row  insightsdata.SessionRow
		want string
	}{
		{insightsdata.SessionRow{OpenerArtist: "Alice", CloserArtist: "Bob"}, "Alice → Bob"},
		// A session that never left one artist says so once instead of twice.
		{insightsdata.SessionRow{OpenerArtist: "Alice", CloserArtist: "Alice"}, "Alice"},
		{insightsdata.SessionRow{}, "unknown artist"},
	}
	for _, tc := range cases {
		if got := insightsSessionArc(tc.row); got != tc.want {
			t.Errorf("%+v: want %q, got %q", tc.row, tc.want, got)
		}
	}
}

func TestSessionsHeaderShowsTheHistogramAndFalseStarts(t *testing.T) {
	v := sessionsFixtureView()
	s := styles.NewStyles(false)

	header := strings.Join(v.sessionsHeaderLines(s), "\n")
	for _, want := range []string{"<15m", "4h+", "1 false start", "enter opens the sessions themselves"} {
		if !strings.Contains(header, want) {
			t.Errorf("header should contain %q, got:\n%s", want, header)
		}
	}

	// Below the compact width the histogram goes but the prose either side of it
	// stays, so the panel still answers its question.
	v.SetSize(60, 30)
	compact := strings.Join(v.sessionsHeaderLines(s), "\n")
	if strings.Contains(compact, "<15m") {
		t.Errorf("compact header should drop the histogram, got:\n%s", compact)
	}
	if !strings.Contains(compact, "1 false start") {
		t.Errorf("compact header should keep the summary, got:\n%s", compact)
	}
}
