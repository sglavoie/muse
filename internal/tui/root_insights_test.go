package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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

	m, _ = m.handleInsightsOpen()

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
