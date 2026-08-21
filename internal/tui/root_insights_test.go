package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// insightsTestTracks mirrors the rich fixture's shape: Alice has two never
// played tracks behind one heard one, Beta is untouched, and Charlie's single
// track was played long enough ago to be dormant.
func insightsTestTracks() []library.Track {
	now := time.Now().Unix()
	day := int64(86400)
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Hash: "h1", Path: "/a1.flac", Genre: "Rock", Duration: 180, PlayCount: 12, LastPlayedAt: now - 2*day},
		{Artist: "Alice", Album: "Alpha", Title: "Track A2", Hash: "h2", Path: "/a2.flac", Genre: "Rock", Duration: 240},
		{Artist: "Alice", Album: "Beta", Title: "Track B1", Hash: "h3", Path: "/b1.flac", Genre: "Jazz", Duration: 300},
		{Artist: "Bob", Album: "Gamma", Title: "Track G1", Hash: "h4", Path: "/g1.flac", Genre: "Jazz", Duration: 120, PlayCount: 5, LastPlayedAt: now - 20*day},
		{Artist: "Bob", Album: "Gamma", Title: "Track G2", Hash: "h5", Path: "/g2.flac", Genre: "Jazz", Duration: 60},
		{Artist: "Charlie", Album: "Delta", Title: "Track D1", Hash: "h6", Path: "/d1.flac", Genre: "Folk", Duration: 200, PlayCount: 2, LastPlayedAt: now - 400*day},
	}
}

// newInsightsModel returns a Model sitting on a loaded Insights view.
func newInsightsModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	m, _ := newInsightsModelReader(t, opts...)
	return m
}

// newInsightsModelReader is newInsightsModel plus the fake reader behind it, so
// a test can change the history the next load will see.
func newInsightsModelReader(t *testing.T, opts ...Option) (Model, *uitestInsights) {
	t.Helper()
	tracks := insightsTestTracks()
	reader := &uitestInsights{tracks: tracks}
	base := []Option{
		WithLibraryTracks(tracks),
		WithInsightsReader(reader),
	}
	m := New(context.Background(), append(base, opts...)...)
	m.termWidth, m.termHeight = 100, 30
	m.insights.setSize(100, 30)
	m = m.switchToView(viewInsights)

	cmd := m.insightsMaybeLoadCmd()
	if cmd == nil {
		t.Fatal("expected a load cmd on first switch to Insights")
	}
	msg, ok := cmd().(insightsLoadedMsg)
	if !ok {
		t.Fatalf("expected insightsLoadedMsg, got %T", cmd())
	}
	return m.handleInsightsLoadedMsg(msg), reader
}

func TestInsightsWindowSince(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, loc)

	if got := insightsWindowSince(0, now, loc); got != 0 {
		t.Errorf("All Time: want 0, got %d", got)
	}
	if got, want := insightsWindowSince(1, now, loc), now.Add(-7*24*time.Hour).Unix(); got != want {
		t.Errorf("7 Days: want %d, got %d", want, got)
	}
	if got, want := insightsWindowSince(4, now, loc), time.Date(2026, 3, 1, 0, 0, 0, 0, loc).Unix(); got != want {
		t.Errorf("This Month: want %d, got %d", want, got)
	}
	// Out-of-range indices wrap rather than panic, so a stale index from a
	// resized window list cannot crash the load path.
	if got := insightsWindowSince(len(insightsWindows), now, loc); got != 0 {
		t.Errorf("wrapped index should equal All Time, got %d", got)
	}
	if got, want := insightsWindowSince(-1, now, loc), time.Date(2026, 3, 1, 0, 0, 0, 0, loc).Unix(); got != want {
		t.Errorf("negative index should wrap to This Month: want %d, got %d", want, got)
	}
}

func TestInsightsShiftPanelWraps(t *testing.T) {
	v := InsightsView{panel: panelDataHealth, cursor: 4, scrollOffset: 2}

	forward := v.shiftPanel(1)
	if forward.panel != panelCoverage {
		t.Errorf("forward from last panel: want panelCoverage, got %v", forward.panel)
	}
	if forward.cursor != 0 || forward.scrollOffset != 0 {
		t.Errorf("panel change should reset cursor/scroll, got %d/%d", forward.cursor, forward.scrollOffset)
	}

	if back := (InsightsView{panel: panelCoverage}).shiftPanel(-1); back.panel != panelDataHealth {
		t.Errorf("back from first panel: want panelDataHealth, got %v", back.panel)
	}
}

func TestInsightsShiftWindowWraps(t *testing.T) {
	last := len(insightsWindows) - 1

	if got := (InsightsView{windowIdx: last}).shiftWindow(1); got.windowIdx != 0 {
		t.Errorf("forward from last window: want 0, got %d", got.windowIdx)
	}
	if got := (InsightsView{windowIdx: 0}).shiftWindow(-1); got.windowIdx != last {
		t.Errorf("back from first window: want %d, got %d", last, got.windowIdx)
	}
}

func TestInsightsLoadGenerationGuard(t *testing.T) {
	m := newInsightsModel(t)
	fresh := m.insights.snap

	stale := insightsLoadedMsg{
		gen:  m.insights.gen - 1,
		snap: insights.Snapshot{TotalPlays: 999},
	}
	m = m.handleInsightsLoadedMsg(stale)

	if m.insights.snap.TotalPlays != fresh.TotalPlays {
		t.Errorf("stale reply overwrote current snapshot: got %d plays", m.insights.snap.TotalPlays)
	}
}

// updateInsightsModel routes msg through the real Update dispatch rather than
// calling a handler directly, so the test breaks if the case is unwired.
func updateInsightsModel(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	return updated
}

// reloadInsights runs the refresh the periodic tick issues for an invalidated
// active view and installs its reply.
func reloadInsights(t *testing.T, m Model) Model {
	t.Helper()
	cmd := m.insightsActiveRefreshCmd()
	if cmd == nil {
		t.Fatal("a stale active Insights view produced no refresh command")
	}
	msg, ok := cmd().(insightsLoadedMsg)
	if !ok {
		t.Fatalf("expected insightsLoadedMsg, got %T", cmd())
	}
	return m.handleInsightsLoadedMsg(msg)
}

func hasInsightsShelf(shelves []insights.ShelfStat, name string) bool {
	for _, s := range shelves {
		if s.Name == name {
			return true
		}
	}
	return false
}

// TestInsightsInvalidatesAfterPlayRescanAndMetadataEdit walks every real event
// that changes what Insights measures and checks each one both invalidates the
// memo and yields a reload that carries the change.
func TestInsightsInvalidatesAfterPlayRescanAndMetadataEdit(t *testing.T) {
	t.Run("a recorded play", func(t *testing.T) {
		m, reader := newInsightsModelReader(t)
		before := m.insights.snap.TotalPlays

		// What the play path writes before dispatching the message: a row in
		// play_history and one more play on the in-memory track.
		now := time.Now().Unix()
		reader.tracks[0].PlayCount++
		reader.tracks[0].LastPlayedAt = now
		m.library.index.IncrementPlayCount("h1", now)

		m = updateInsightsModel(t, m, playRecordedMsg{Hash: "h1", PlayedAt: now})
		if !m.insights.stale {
			t.Fatal("a recorded play left the Insights memo current")
		}

		m = reloadInsights(t, m)
		if got := m.insights.snap.TotalPlays; got != before+1 {
			t.Errorf("reload missed the new play: want %d plays, got %d", before+1, got)
		}
	})

	t.Run("a completed rescan", func(t *testing.T) {
		m, reader := newInsightsModelReader(t)
		before := m.insights.snap.Coverage.TotalTracks

		found := library.Track{
			Artist: "Delta", Album: "Epsilon", Title: "Track E1",
			Hash: "h7", Path: "/e1.flac", Genre: "Drone", Duration: 500,
		}
		reader.tracks = append(reader.tracks, found)
		m.library.index.Insert(found)

		m = updateInsightsModel(t, m, library.ScanDoneMsg{})
		if !m.insights.stale {
			t.Fatal("a completed rescan left the Insights memo current")
		}

		m = reloadInsights(t, m)
		if got := m.insights.snap.Coverage.TotalTracks; got != before+1 {
			t.Errorf("coverage missed the scanned track: want %d tracks, got %d", before+1, got)
		}
		if !hasInsightsShelf(m.insights.genreCoverage, "Drone") {
			t.Error("the library-derived shelves missed the scanned genre")
		}
	})

	t.Run("a metadata edit", func(t *testing.T) {
		m, reader := newInsightsModelReader(t)
		if !hasInsightsShelf(m.insights.genreCoverage, "Rock") {
			t.Fatal("precondition: Rock should start out as a dusty shelf")
		}

		// h2 is the only unheard Rock track, so moving it empties the shelf.
		reader.tracks[1].Genre = "Jazz"
		m.library.index.UpdateMetadata("h2", "Track A2", "Alice", "Alpha", "Jazz")

		m = updateInsightsModel(t, m, metadataEditedMsg{})
		if !m.insights.stale {
			t.Fatal("a metadata edit left the Insights memo current")
		}

		m = reloadInsights(t, m)
		if hasInsightsShelf(m.insights.genreCoverage, "Rock") {
			t.Error("reload kept the shelf the edit emptied")
		}
	})

	t.Run("an undo", func(t *testing.T) {
		m, reader := newInsightsModelReader(t)

		// No metadata writer is wired, so perform() rewrites only the index;
		// the reader stands in for the database the undo also restores.
		reader.tracks[1].Genre = "Punk"
		m.setUndoSlot(undoBulkEditMetadataAction{
			originals: map[string][5]string{"h2": {"Track A2", "Alice", "Alpha", "Punk", "4:00"}},
		}, nil)

		m = updateInsightsModel(t, m, UndoMsg{})
		if !m.insights.stale {
			t.Fatal("an undone metadata edit left the Insights memo current")
		}

		m = reloadInsights(t, m)
		if !hasInsightsShelf(m.insights.genreCoverage, "Punk") {
			t.Error("reload missed the genre the undo restored")
		}
	})

	t.Run("the tick refreshes the view on screen", func(t *testing.T) {
		m, _ := newInsightsModelReader(t)
		m.insightsInvalidate()

		m, _ = m.handleTickMsg()

		if !m.insights.loading {
			t.Error("the tick did not start a reload for a stale active Insights view")
		}
	})

	t.Run("the tick leaves a stale view alone from elsewhere", func(t *testing.T) {
		m, _ := newInsightsModelReader(t)
		m = m.switchToView(viewLibrary)
		m.insightsInvalidate()

		m, _ = m.handleTickMsg()

		if m.insights.loading {
			t.Error("the tick reloaded Insights while the user was in another view")
		}
	})

	t.Run("a reply computed before the invalidation is discarded", func(t *testing.T) {
		m, _ := newInsightsModelReader(t)
		inFlight := m.insightsLoadCmd()
		reply, ok := inFlight().(insightsLoadedMsg)
		if !ok {
			t.Fatalf("expected insightsLoadedMsg, got %T", inFlight())
		}

		m.insightsInvalidate()
		m = m.handleInsightsLoadedMsg(reply)

		if !m.insights.stale {
			t.Error("a load that predates the change was allowed to clear the memo")
		}
		if m.insightsActiveRefreshCmd() == nil {
			t.Error("the discarded reply left no reload pending")
		}
	})
}

// brokenInsightsReader answers exactly like the fixture reader until fail is
// set, at which point two of its seven reads return an error. Two rather than
// one because the load message is supposed to carry every failure rather than
// the first one it hits.
type brokenInsightsReader struct {
	*uitestInsights
	fail error
}

func (r *brokenInsightsReader) ReadInsightPlays(sinceUnix int64) ([]insights.PlayRow, error) {
	if r.fail != nil {
		return nil, r.fail
	}
	return r.uitestInsights.ReadInsightPlays(sinceUnix)
}

func (r *brokenInsightsReader) ReadInsightCoverage() ([]insights.CoverageRow, error) {
	if r.fail != nil {
		return nil, r.fail
	}
	return r.uitestInsights.ReadInsightCoverage()
}

// newBrokenInsightsModel returns an Insights view that has not loaded yet,
// alongside the reader whose reads the test can break and mend.
func newBrokenInsightsModel(t *testing.T) (Model, *brokenInsightsReader) {
	t.Helper()
	tracks := insightsTestTracks()
	reader := &brokenInsightsReader{uitestInsights: &uitestInsights{tracks: tracks}}
	m := New(context.Background(), WithLibraryTracks(tracks), WithInsightsReader(reader))
	m.termWidth, m.termHeight = 100, 30
	m.insights.setSize(100, 30)
	return m, reader
}

// enterInsights runs the real view-entry action, which is also the retry after
// a failed read, and returns the load it issued.
func enterInsights(t *testing.T, m Model) (Model, insightsLoadedMsg) {
	t.Helper()
	next, cmd := m.handleKeyAction(actionSwitchInsights)
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKeyAction returned %T", next)
	}
	if cmd == nil {
		t.Fatal("entering Insights issued no load")
	}
	msg, ok := cmd().(insightsLoadedMsg)
	if !ok {
		t.Fatalf("expected insightsLoadedMsg, got %T", cmd())
	}
	return updated, msg
}

// TestInsightsLoadFailureDoesNotInstallPartialSnapshotAndCanRetry covers both
// halves of a failed read: the first one, which has no numbers to fall back on,
// and a refresh, which must not replace good numbers with a partial fold.
func TestInsightsLoadFailureDoesNotInstallPartialSnapshotAndCanRetry(t *testing.T) {
	dbDown := errors.New("database is locked")

	t.Run("an initial failure installs nothing and stays retryable", func(t *testing.T) {
		m, reader := newBrokenInsightsModel(t)
		reader.fail = dbDown

		m, msg := enterInsights(t, m)

		if msg.err == nil {
			t.Fatal("a failed read produced a load message with no error")
		}
		for _, want := range []string{"plays", "coverage"} {
			if !strings.Contains(msg.err.Error(), want) {
				t.Errorf("the error should name every failed reader: %q missing from %v", want, msg.err)
			}
		}
		if msg.snap.TotalPlays != 0 || msg.snap.Coverage.TotalTracks != 0 {
			t.Errorf("a failed read computed a snapshot anyway: %+v", msg.snap)
		}

		m = m.handleInsightsLoadedMsg(msg)

		if m.insights.loaded {
			t.Error("a failed read marked the view loaded")
		}
		if m.insights.loading {
			t.Error("a failed read left the view loading forever")
		}
		if m.insights.loadErr == nil {
			t.Fatal("the failure was swallowed")
		}
		rendered := m.insights.render(styles.NewStyles(false), nil, nil, nil, "", "")
		if !strings.Contains(rendered, "Could not read your listening history") {
			t.Errorf("expected a visible error state, got: %q", rendered)
		}
		// The active refresh rides the 1 Hz tick, so retrying there would turn a
		// broken database into a failed read every second.
		if m.insightsActiveRefreshCmd() != nil {
			t.Error("a failed read is being retried by the automatic refresh")
		}

		reader.fail = nil
		m, msg = enterInsights(t, m)
		m = m.handleInsightsLoadedMsg(msg)

		if !m.insights.loaded || m.insights.loadErr != nil {
			t.Fatalf("re-entering the view did not recover: loaded=%v err=%v", m.insights.loaded, m.insights.loadErr)
		}
		if m.insights.snap.TotalPlays == 0 {
			t.Error("the retry installed no plays")
		}
	})

	t.Run("a failed refresh keeps the last complete snapshot", func(t *testing.T) {
		m, reader := newBrokenInsightsModel(t)
		m, msg := enterInsights(t, m)
		m = m.handleInsightsLoadedMsg(msg)
		before := m.insights.snap.TotalPlays
		if before == 0 {
			t.Fatal("precondition: the first load should carry plays")
		}

		// The real play path: a row in play_history and one more play on the
		// in-memory track, then the message that invalidates the memo.
		now := time.Now().Unix()
		reader.tracks[0].PlayCount++
		reader.tracks[0].LastPlayedAt = now
		m.library.index.IncrementPlayCount("h1", now)
		m = updateInsightsModel(t, m, playRecordedMsg{Hash: "h1", PlayedAt: now})

		reader.fail = dbDown
		m = reloadInsights(t, m)

		if got := m.insights.snap.TotalPlays; got != before {
			t.Errorf("a failed refresh replaced the good snapshot: want %d plays, got %d", before, got)
		}
		if !m.insights.loaded {
			t.Error("a failed refresh threw away a snapshot the view already had")
		}
		rendered := m.insights.render(styles.NewStyles(false), nil, nil, nil, "", "")
		if !strings.Contains(rendered, "retry · read failed") {
			t.Errorf("the strip should say the figures are not current, got: %q", rendered)
		}
		if m.insightsActiveRefreshCmd() != nil {
			t.Error("a failed refresh is being retried by the automatic refresh")
		}

		reader.fail = nil
		m, msg = enterInsights(t, m)
		m = m.handleInsightsLoadedMsg(msg)

		if got := m.insights.snap.TotalPlays; got != before+1 {
			t.Errorf("the retry missed the play recorded during the outage: want %d, got %d", before+1, got)
		}
		if m.insights.loadErr != nil || m.insights.stale {
			t.Errorf("a recovered load left the view marked bad: err=%v stale=%v", m.insights.loadErr, m.insights.stale)
		}
	})
}

func TestInsightsWindowCycleFixedPanelDoesNotReload(t *testing.T) {
	m := newInsightsModel(t)
	if m.insights.panelSpec().class != classFixed {
		t.Fatal("precondition: Coverage should be a fixed panel")
	}
	before := m.insights.windowIdx

	m, cmd := m.handleInsightsWindowCycle(1)

	if cmd != nil {
		t.Error("fixed panel should not trigger a reload")
	}
	if m.insights.windowIdx != before {
		t.Errorf("fixed panel should not move the window: got %d, want %d", m.insights.windowIdx, before)
	}
}

func TestInsightsWindowCycleWindowedPanelReloads(t *testing.T) {
	m := newInsightsModel(t)
	m = m.handleInsightsPanelCycle(1)
	if m.insights.panelSpec().class != classWindowed {
		t.Fatalf("precondition: Rhythm should be windowed, got %v", m.insights.panelSpec().class)
	}

	m, cmd := m.handleInsightsWindowCycle(1)

	if cmd == nil {
		t.Fatal("windowed panel should trigger a reload")
	}
	if m.insights.windowIdx != 1 {
		t.Errorf("window did not advance: got %d, want 1", m.insights.windowIdx)
	}
}

func TestGenreCoverageShelves(t *testing.T) {
	shelves := genreCoverageShelves(insightsTestTracks())

	got := map[string]insights.ShelfStat{}
	for _, s := range shelves {
		got[s.Name] = s
	}
	if _, ok := got["Folk"]; ok {
		t.Error("fully heard genre Folk should be omitted")
	}
	jazz, ok := got["Jazz"]
	if !ok {
		t.Fatalf("expected a Jazz shelf, got %v", shelves)
	}
	if jazz.Tracks != 3 || jazz.Unheard != 2 {
		t.Errorf("Jazz: want 2/3 unheard, got %d/%d", jazz.Unheard, jazz.Tracks)
	}
	if jazz.UnheardSecs != 360 {
		t.Errorf("Jazz unheard secs: want 360, got %v", jazz.UnheardSecs)
	}
	if shelves[0].Name != "Jazz" {
		t.Errorf("shelves should be sorted by unheard count desc, got %q first", shelves[0].Name)
	}
}

func TestDormantTracks(t *testing.T) {
	tracks := insightsTestTracks()
	ref := time.Now().Unix()

	dormant := dormantTracks(tracks, ref)

	if len(dormant) != 1 {
		t.Fatalf("want 1 dormant track, got %d: %+v", len(dormant), dormant)
	}
	if dormant[0].Hash != "h6" {
		t.Errorf("want h6 dormant, got %q", dormant[0].Hash)
	}
	if got := dormantTracks(tracks, 0); got != nil {
		t.Errorf("no reference play means no dormancy cutoff, got %v", got)
	}
}

func TestInsightsDrillTracks(t *testing.T) {
	m := newInsightsModel(t)

	cases := []struct {
		name  string
		row   insightsRow
		want  []string
		exact bool
	}{
		{"artist", insightsRow{drill: drillUnheardArtist, key: "Alice"}, []string{"h2", "h3"}, true},
		{"album", insightsRow{drill: drillUnheardAlbum, key: "Beta"}, []string{"h3"}, true},
		{"genre", insightsRow{drill: drillUnheardGenre, key: "jazz"}, []string{"h3", "h5"}, true},
		{"all unheard", insightsRow{drill: drillUnheardAll}, []string{"h2", "h3", "h5"}, true},
		{"none", insightsRow{drill: drillNone}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, tr := range m.insightsDrillTracks(tc.row) {
				got = append(got, tr.Hash)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}

	dormant := m.insightsDrillTracks(insightsRow{drill: drillDormant, key: "h6"})
	if len(dormant) != 1 || dormant[0].Hash != "h6" {
		t.Errorf("dormant drill should open the whole dormant set, got %+v", dormant)
	}
}

func TestInsightsOpenAndBack(t *testing.T) {
	m := newInsightsModel(t)
	row := m.insights.focusedRow()
	if row == nil {
		t.Fatal("expected the Coverage body to have rows")
	}

	m = m.handleInsightsOpen()

	if m.insights.level != 1 {
		t.Fatalf("expected level 1 after open, got %d", m.insights.level)
	}
	if len(m.insights.trackView.sourceTracks) == 0 {
		t.Fatal("expected the drill to carry tracks")
	}
	if m.insights.drillLabel != row.label {
		t.Errorf("drill label: want %q, got %q", row.label, m.insights.drillLabel)
	}

	m = m.handleInsightsBack()

	if m.insights.level != 0 {
		t.Errorf("expected level 0 after back, got %d", m.insights.level)
	}
	if m.insights.drillLabel != "" {
		t.Errorf("expected drill label cleared, got %q", m.insights.drillLabel)
	}
}

// newSessionsModel installs two hand-built sessions so a drill has a known
// shape, opener and play order behind it. s-mixed repeats h1 deliberately.
func newSessionsModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	m := newInsightsModel(t, opts...)
	m.insights.loc = panelsTestLoc()
	m.insights.plays = []insights.PlayRow{
		{Hash: "h4", StartAt: at(time.August, 3, 9).Unix(), SessionID: "s-solo", Artist: "Bob"},
		{Hash: "h1", StartAt: at(time.August, 5, 20).Unix(), SessionID: "s-mixed", Artist: "Alice"},
		{Hash: "h6", StartAt: at(time.August, 5, 21).Unix(), SessionID: "s-mixed", Artist: "Charlie"},
		{Hash: "h1", StartAt: at(time.August, 5, 22).Unix(), SessionID: "s-mixed", Artist: "Alice"},
	}
	m.insights.sessions = []insights.SessionRow{
		{ID: "s-solo", StartedAt: at(time.August, 3, 9).Unix(), EndedAt: at(time.August, 3, 9).Unix() + 600,
			Plays: 1, ListenedSecs: 600, DistinctArtists: 1, OpenerArtist: "Bob", CloserArtist: "Bob"},
		{ID: "s-mixed", StartedAt: at(time.August, 5, 20).Unix(), EndedAt: at(time.August, 5, 22).Unix() + 600,
			Plays: 3, ListenedSecs: 1800, DistinctArtists: 2, OpenerArtist: "Alice", CloserArtist: "Alice"},
	}
	m.insights.snap.SessionStats = insights.SessionStats{
		Count: 2, SinglePlay: 1, TotalSecs: 2400, LongestSecs: 8400, MedianSecs: 600, AvgPlays: 2,
		Durations:  []int{600, 8400},
		TopOpeners: []insights.EntityStat{{Name: "Alice", Plays: 1}, {Name: "Bob", Plays: 1}},
	}
	m.insights = m.insights.shiftPanel(int(panelSessions))
	if m.insights.panel != panelSessions {
		t.Fatalf("precondition: want the Sessions panel, got %v", m.insights.panel)
	}
	m.insights.clampCursor()
	return m
}

func TestInsightsSessionRowOpensSessionsThenTracks(t *testing.T) {
	m := newSessionsModel(t)
	row := m.insights.focusedRow()
	if row == nil || row.drill != drillSessionsAll {
		t.Fatalf("precondition: want the All sessions row focused, got %+v", row)
	}

	// A Sessions row opens the sessions themselves, because a session is a thing
	// in its own right rather than a bag of tracks.
	m = m.handleInsightsOpen()
	if m.insights.level != insightsLevelSessions {
		t.Fatalf("want the session level after open, got %d", m.insights.level)
	}
	if len(m.insights.sessionList) != 2 {
		t.Fatalf("want both sessions listed, got %d", len(m.insights.sessionList))
	}
	// Newest first, so the mixed session leads.
	if got := m.insights.focusedSession(); got == nil || got.ID != "s-mixed" {
		t.Fatalf("want s-mixed focused, got %+v", got)
	}

	m = m.handleInsightsOpen()
	if m.insights.level != insightsLevelSessionTracks {
		t.Fatalf("want the session's tracks after a second open, got %d", m.insights.level)
	}
	if !m.insights.atTracks() {
		t.Error("the session track level should count as a track level")
	}
	if !strings.HasPrefix(m.insights.drillLabel, "Session ") {
		t.Errorf("drill label should name the session, got %q", m.insights.drillLabel)
	}

	// esc steps out one level at a time rather than falling straight back to the
	// briefing, so the session list is not lost on the way.
	m = m.handleInsightsBack()
	if m.insights.level != insightsLevelSessions {
		t.Fatalf("want the session list after one back, got %d", m.insights.level)
	}
	if len(m.insights.sessionList) != 2 {
		t.Errorf("back should keep the session list, got %d", len(m.insights.sessionList))
	}
	m = m.handleInsightsBack()
	if m.insights.level != insightsLevelBriefing {
		t.Fatalf("want the briefing after two backs, got %d", m.insights.level)
	}
	if m.insights.sessionList != nil || m.insights.sessionLabel != "" {
		t.Error("returning to the briefing should clear the session list")
	}
}

func TestInsightsSessionTracksKeepRepeatsInPlayOrder(t *testing.T) {
	m := newSessionsModel(t)

	// This is the one drill that must not dedupe: a replay that drops the second
	// listen is not the session that happened.
	var got []string
	for _, tr := range m.insightsSessionTracks("s-mixed") {
		got = append(got, tr.Hash)
	}
	if strings.Join(got, ",") != "h1,h6,h1" {
		t.Errorf("want h1,h6,h1 in play order, got %v", got)
	}
	if tracks := m.insightsSessionTracks(""); tracks != nil {
		t.Errorf("an empty id should resolve to nothing, got %+v", tracks)
	}

	// The briefing row above it stands for a set rather than one replay, so there
	// the repeat folds away.
	var set []string
	for _, tr := range m.insightsDrillTracks(insightsRow{drill: drillSessionsAll}) {
		set = append(set, tr.Hash)
	}
	if strings.Join(set, ",") != "h4,h1,h6" {
		t.Errorf("the whole-set drill should dedupe in play order, got %v", set)
	}
}

func TestInsightsEnqueueReplaysASessionInOrder(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newSessionsModel(t, WithBatchEnqueuer(be))
	m = m.handleInsightsOpen()

	_, cmd := m.handleInsightsEnqueueAll()
	if cmd == nil {
		t.Fatal("expected an enqueue cmd for the focused session")
	}
	cmd()

	if len(be.appendTracksCalls) != 1 {
		t.Fatalf("want one AppendTracks call, got %d", len(be.appendTracksCalls))
	}
	var got []string
	for _, tr := range be.appendTracksCalls[0] {
		got = append(got, tr.Hash)
	}
	if strings.Join(got, ",") != "h1,h6,h1" {
		t.Errorf("want the session replayed as h1,h6,h1, got %v", got)
	}
	if want := "Session 5 Aug 20:30"; be.appendTracksSources[0] != want {
		t.Errorf("source should name the session: want %q, got %q", want, be.appendTracksSources[0])
	}
}

func TestInsightsJumpBrowseFromASession(t *testing.T) {
	m := newSessionsModel(t)
	m = m.handleInsightsOpen()

	m = m.handleInsightsJumpBrowse()

	if m.activeView != viewBrowse {
		t.Fatalf("b should land in Browse, got view %d", m.activeView)
	}
	// A session is named by where it starts, so b follows its opener.
	if got := m.browse.artists.artists[m.browse.artists.cursor]; got != "Alice" {
		t.Errorf("want the session's opener focused, got %q", got)
	}
}

func TestInsightsJumpBrowseFromARankingRow(t *testing.T) {
	m := newInsightsModel(t)
	m.insights = m.insights.shiftPanel(int(panelTimeVsPlays))
	m.insights.snap.Rankings = rankingsFixture()
	m.insights.clampCursor()
	if row := m.insights.focusedRow(); row == nil || row.key != "Dana" {
		t.Fatalf("precondition: want the biggest mover focused, got %+v", row)
	}

	// Dana is not in the fixture library, so the jump has nothing to land on and
	// says so rather than switching views to an arbitrary artist.
	if got := m.handleInsightsJumpBrowse(); got.activeView != viewInsights {
		t.Errorf("an unknown artist should not change view, got %d", got.activeView)
	}

	m.insights.cursor = 1 // Bob, who does exist
	m = m.handleInsightsJumpBrowse()
	if m.activeView != viewBrowse {
		t.Fatalf("b should land in Browse, got view %d", m.activeView)
	}
	if got := m.browse.artists.artists[m.browse.artists.cursor]; got != "Bob" {
		t.Errorf("want Bob focused, got %q", got)
	}
}

func TestInsightsRutBreakerSteersAwayFromTheTopArtists(t *testing.T) {
	m := newInsightsModel(t)
	m.insights.snap.Rankings.ArtistsByHours = []insights.EntityStat{{Name: "Alice", ListenedSecs: 9000}}

	got := m.insightsRutBreakerTracks(10)

	if len(got) == 0 {
		t.Fatal("expected a rut-breaking set")
	}
	for _, tr := range got {
		if tr.Artist == "Alice" {
			t.Errorf("the top artist should be excluded, got %q", tr.Hash)
		}
	}
	// Artists are visited round-robin, least-heard first, so one prolific
	// outsider cannot simply become the new rut. Charlie has 2 plays to Bob's 5.
	if got[0].Artist != "Charlie" || got[1].Artist != "Bob" {
		t.Errorf("want the least-heard artist first then a round-robin, got %q then %q", got[0].Artist, got[1].Artist)
	}
	if n := len(m.insightsRutBreakerTracks(2)); n != 2 {
		t.Errorf("the limit should be honoured, got %d tracks", n)
	}
}

func TestInsightsAntiRutOnConcentrationLeavesTheTopArtists(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newInsightsModel(t, WithBatchEnqueuer(be))
	m.insights = m.insights.shiftPanel(int(panelConcentration))
	m.insights.snap.Rankings.ArtistsByHours = []insights.EntityStat{{Name: "Alice", ListenedSecs: 9000}}

	_, cmd := m.handleInsightsAntiRut()
	if cmd == nil {
		t.Fatal("expected an enqueue cmd")
	}
	cmd()

	if len(be.appendTracksCalls) != 1 {
		t.Fatalf("want one AppendTracks call, got %d", len(be.appendTracksCalls))
	}
	// On Concentration the rut is not the unheard backlog but the handful of
	// artists holding the time, so ! pulls from outside that group instead.
	for _, tr := range be.appendTracksCalls[0] {
		if tr.Artist == "Alice" {
			t.Errorf("anti-rut enqueued a dominant artist: %q", tr.Hash)
		}
	}
	if want := "Outside your top 20 artists"; be.appendTracksSources[0] != want {
		t.Errorf("source: want %q, got %q", want, be.appendTracksSources[0])
	}
}

func TestInsightsCursorDoesNotFallThroughToQueue(t *testing.T) {
	m := newInsightsModel(t)
	m.queue.entries = []QueueEntry{{ID: 1}, {ID: 2}, {ID: 3}}

	m = pressKey(m, "j")
	m = pressKey(m, "j")

	if m.insights.cursor != 2 {
		t.Errorf("j should move the Insights cursor: want 2, got %d", m.insights.cursor)
	}
	if m.queue.cursor != 0 {
		t.Errorf("j leaked into the Queue cursor: got %d", m.queue.cursor)
	}

	m = pressKey(m, "k")

	if m.insights.cursor != 1 {
		t.Errorf("k should move the Insights cursor: want 1, got %d", m.insights.cursor)
	}
	if m.queue.cursor != 0 {
		t.Errorf("k leaked into the Queue cursor: got %d", m.queue.cursor)
	}
}

func TestInsightsEnqueueAll(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newInsightsModel(t, WithBatchEnqueuer(be))

	m, cmd := m.handleInsightsEnqueueAll()
	if cmd == nil {
		t.Fatal("expected an enqueue cmd")
	}
	msg, ok := cmd().(BatchEnqueuedMsg)
	if !ok {
		t.Fatalf("expected BatchEnqueuedMsg, got %T", cmd())
	}
	if msg.Err != nil {
		t.Fatalf("unexpected enqueue error: %v", msg.Err)
	}
	if len(be.appendTracksCalls) != 1 {
		t.Fatalf("expected one AppendTracks call, got %d", len(be.appendTracksCalls))
	}
	if len(be.appendTracksCalls[0]) != 2 {
		t.Errorf("expected the 2 unheard Alice tracks, got %d", len(be.appendTracksCalls[0]))
	}
	if be.appendTracksSources[0] != m.insights.focusedRow().label {
		t.Errorf("source should be the row label, got %q", be.appendTracksSources[0])
	}
}

func TestInsightsAntiRutEnqueuesUnheard(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newInsightsModel(t, WithBatchEnqueuer(be))

	_, cmd := m.handleInsightsAntiRut()
	if cmd == nil {
		t.Fatal("expected an enqueue cmd")
	}
	cmd()

	if len(be.appendTracksCalls) != 1 {
		t.Fatalf("expected one AppendTracks call, got %d", len(be.appendTracksCalls))
	}
	for _, tr := range be.appendTracksCalls[0] {
		if tr.PlayCount != 0 {
			t.Errorf("anti-rut enqueued a heard track: %q", tr.Hash)
		}
	}
	if be.appendTracksSources[0] != "Never played" {
		t.Errorf("source: want %q, got %q", "Never played", be.appendTracksSources[0])
	}
}

func TestInsightsSavePlaylist(t *testing.T) {
	pw := &stubPlaylistCreator{nextID: 7}
	pa := &stubPlaylistAdder{}
	m := newInsightsModel(t, WithPlaylistWriter(pw), WithPlaylistAdder(pa))
	m = m.handleInsightsOpen()
	if m.insights.level != 1 {
		t.Fatal("precondition: expected a drill before saving")
	}

	m, cmd := m.handleInsightsSavePlaylist()
	if cmd == nil {
		t.Fatal("expected a save cmd at level 1")
	}
	msg, ok := cmd().(mixSavedAsPlaylistMsg)
	if !ok {
		t.Fatalf("expected mixSavedAsPlaylistMsg, got %T", cmd())
	}

	want := "Insights: " + m.insights.drillLabel
	if msg.playlistName != want {
		t.Errorf("playlist name: want %q, got %q", want, msg.playlistName)
	}
	if msg.count != len(m.insights.trackView.sourceTracks) {
		t.Errorf("count: want %d, got %d", len(m.insights.trackView.sourceTracks), msg.count)
	}
	if len(pa.calls) != 1 || len(pa.calls[0].hashes) != msg.count {
		t.Errorf("expected %d hashes added to the new playlist, got %+v", msg.count, pa.calls)
	}
}

func TestInsightsSavePlaylistIgnoredAtBriefingLevel(t *testing.T) {
	m := newInsightsModel(t, WithPlaylistWriter(&stubPlaylistCreator{}))

	if _, cmd := m.handleInsightsSavePlaylist(); cmd != nil {
		t.Error("save should be a no-op at level 0, where there is no track list")
	}
}

func TestInsightsEmptyState(t *testing.T) {
	m := New(context.Background(), WithInsightsReader(&uitestInsights{}))
	m.insights.setSize(80, 24)
	m = m.switchToView(viewInsights)
	cmd := m.insightsMaybeLoadCmd()
	if cmd == nil {
		t.Fatal("expected a load cmd even with no tracks")
	}
	m = m.handleInsightsLoadedMsg(cmd().(insightsLoadedMsg))

	if !m.insights.empty() {
		t.Fatal("expected the empty state with no plays and no library")
	}
	rendered := m.insights.render(styles.NewStyles(false), nil, nil, nil, "", "")
	if !strings.Contains(rendered, "Nothing to measure yet") {
		t.Errorf("expected the empty card, got: %q", rendered)
	}
}

func TestInsightsRenderFitsHeight(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 15}, {40, 10}} {
		m := newInsightsModel(t)
		m.insights.setSize(size[0], size[1])
		m.insights.clampCursor()

		lines := strings.Split(m.insights.render(styles.NewStyles(false), nil, nil, nil, "", ""), "\n")

		if len(lines) > size[1] {
			t.Errorf("%dx%d: rendered %d lines, exceeds height", size[0], size[1], len(lines))
		}
	}
}

// insightsNotified reports whether any live notification carries substr.
func insightsNotified(m Model, substr string) bool {
	for _, n := range m.statusBar.notifications {
		if strings.Contains(n.msg, substr) {
			return true
		}
	}
	return false
}

func TestInsightsVisualSelectionWorksAtTrackLevel(t *testing.T) {
	m := newInsightsModel(t)

	// The briefing is not a track list, and v is advertised, so it has to say
	// what is missing rather than swallow the key.
	m = doVisual(m)
	if m.visual.active {
		t.Fatal("visual mode must not enter on the Insights briefing")
	}
	if !insightsNotified(m, "track list") {
		t.Errorf("expected a refusal naming the missing track list, got %+v", m.statusBar.notifications)
	}

	// A session list is a list of sessions, not of tracks, and refuses too.
	m.insights.openSessionList("Sessions", []insights.SessionRow{{ID: "s1"}, {ID: "s2"}})
	m = doVisual(m)
	if m.visual.active {
		t.Fatal("visual mode must not enter on the Insights session list")
	}
	m.insights.back()

	tracks := m.insightsDrillTracks(insightsRow{drill: drillUnheardAll})
	if len(tracks) != 3 {
		t.Fatalf("precondition: want 3 never-played tracks, got %d", len(tracks))
	}
	m.insights.openDrill("Never played", tracks)
	// A selection made before v must survive every restore of the range.
	m.insights.trackView.selectedHashes = map[string]struct{}{"h5": {}}

	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode to enter at the Insights track level")
	}
	if m.visual.view != viewInsights {
		t.Errorf("visual view: want viewInsights, got %v", m.visual.view)
	}
	if m.visual.anchor != 0 {
		t.Errorf("anchor: want the cursor position 0, got %d", m.visual.anchor)
	}
	assertSelectedHashesExact(t, m.insights.trackView.selectedHashes, "h2", "h5")

	m = doNav(m, actionCursorDown)
	assertSelectedHashesExact(t, m.insights.trackView.selectedHashes, "h2", "h3", "h5")

	// Shrinking the range restores the snapshot rather than leaving h3 behind.
	m = doNav(m, actionCursorUp)
	assertSelectedHashesExact(t, m.insights.trackView.selectedHashes, "h2", "h5")

	m.showFooterHints = true
	m.termWidth = 200
	hint := ansi.Strip(m.renderFooterHint())
	for _, want := range []string{"extend", "exit", "clear"} {
		if !strings.Contains(hint, want) {
			t.Errorf("visual footer hints missing %q: %q", want, hint)
		}
	}

	// v again exits, keeping what the range selected.
	m = doVisual(m)
	if m.visual.active {
		t.Fatal("expected a second v to exit visual mode")
	}
	assertSelectedHashesExact(t, m.insights.trackView.selectedHashes, "h2", "h5")

	// esc leaves the drill, which is the list the anchor pointed into.
	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode to re-enter")
	}
	m = m.handleInsightsBack()
	if m.visual.active {
		t.Error("stepping out of the drill must exit visual mode")
	}
	if m.insights.level != insightsLevelBriefing {
		t.Errorf("expected the briefing after back, got level %d", m.insights.level)
	}
}

func TestInsightsHelpSection(t *testing.T) {
	sections := filteredHelpSections(contextInsights)
	if len(sections) == 0 {
		t.Fatal("expected at least one help section for contextInsights")
	}
	if sections[0].title != "Insights" {
		t.Errorf("expected the Insights section first, got %q", sections[0].title)
	}
}
