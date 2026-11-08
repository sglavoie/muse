package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/insights"
	instui "github.com/sglavoie/muse/internal/tui/insights"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// newSessionsModel installs two hand-built sessions so a drill has a known
// shape, opener and play order behind it. s-mixed repeats h1 deliberately.
func newSessionsModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	m := newInsightsModel(t, opts...)
	m.insights.Loc = panelsTestLoc()
	m.insights.Plays = []insights.PlayRow{
		{Hash: "h4", StartAt: at(time.August, 3, 9).Unix(), SessionID: "s-solo", Artist: "Bob"},
		{Hash: "h1", StartAt: at(time.August, 5, 20).Unix(), SessionID: "s-mixed", Artist: "Alice"},
		{Hash: "h6", StartAt: at(time.August, 5, 21).Unix(), SessionID: "s-mixed", Artist: "Charlie"},
		{Hash: "h1", StartAt: at(time.August, 5, 22).Unix(), SessionID: "s-mixed", Artist: "Alice"},
	}
	m.insights.Sessions = []insights.SessionRow{
		{ID: "s-solo", StartedAt: at(time.August, 3, 9).Unix(), EndedAt: at(time.August, 3, 9).Unix() + 600,
			Plays: 1, ListenedSecs: 600, DistinctArtists: 1, OpenerArtist: "Bob", CloserArtist: "Bob"},
		{ID: "s-mixed", StartedAt: at(time.August, 5, 20).Unix(), EndedAt: at(time.August, 5, 22).Unix() + 600,
			Plays: 3, ListenedSecs: 1800, DistinctArtists: 2, OpenerArtist: "Alice", CloserArtist: "Alice"},
	}
	m.insights.Snap.SessionStats = insights.SessionStats{
		Count: 2, SinglePlay: 1, TotalSecs: 2400, LongestSecs: 8400, MedianSecs: 600, AvgPlays: 2,
		Durations:  []int{600, 8400},
		TopOpeners: []insights.EntityStat{{Name: "Alice", Plays: 1}, {Name: "Bob", Plays: 1}},
	}
	m.insights = m.insights.ShiftPanel(int(instui.PanelSessions))
	if m.insights.Panel != instui.PanelSessions {
		t.Fatalf("precondition: want the Sessions panel, got %v", m.insights.Panel)
	}
	m.insights.ClampCursor()
	return m
}

func TestInsightsSessionRowOpensSessionsThenTracks(t *testing.T) {
	m := newSessionsModel(t)
	row := m.insights.FocusedRow()
	if row == nil || row.Drill != instui.DrillSessionsAll {
		t.Fatalf("precondition: want the All sessions row focused, got %+v", row)
	}

	// A Sessions row opens the sessions themselves, because a session is a thing
	// in its own right rather than a bag of tracks.
	m, _ = m.handleInsightsOpen()
	if m.insights.Level != instui.LevelSessions {
		t.Fatalf("want the session level after open, got %d", m.insights.Level)
	}
	if len(m.insights.SessionList) != 2 {
		t.Fatalf("want both sessions listed, got %d", len(m.insights.SessionList))
	}
	// Newest first, so the mixed session leads.
	if got := m.insights.FocusedSession(); got == nil || got.ID != "s-mixed" {
		t.Fatalf("want s-mixed focused, got %+v", got)
	}

	m, _ = m.handleInsightsOpen()
	if m.insights.Level != instui.LevelSessionTracks {
		t.Fatalf("want the session's tracks after a second open, got %d", m.insights.Level)
	}
	if !m.insights.AtTracks() {
		t.Error("the session track level should count as a track level")
	}
	if !strings.HasPrefix(m.insights.DrillLabel, "Session ") {
		t.Errorf("drill label should name the session, got %q", m.insights.DrillLabel)
	}

	// esc steps out one level at a time rather than falling straight back to the
	// briefing, so the session list is not lost on the way.
	m = m.handleInsightsBack()
	if m.insights.Level != instui.LevelSessions {
		t.Fatalf("want the session list after one back, got %d", m.insights.Level)
	}
	if len(m.insights.SessionList) != 2 {
		t.Errorf("back should keep the session list, got %d", len(m.insights.SessionList))
	}
	m = m.handleInsightsBack()
	if m.insights.Level != instui.LevelBriefing {
		t.Fatalf("want the briefing after two backs, got %d", m.insights.Level)
	}
	if m.insights.SessionList != nil || m.insights.SessionLabel != "" {
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
	for _, tr := range m.insightsDrillTracks(instui.Row{Drill: instui.DrillSessionsAll}) {
		set = append(set, tr.Hash)
	}
	if strings.Join(set, ",") != "h4,h1,h6" {
		t.Errorf("the whole-set drill should dedupe in play order, got %v", set)
	}
}

func TestInsightsEnqueueReplaysASessionInOrder(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newSessionsModel(t, WithBatchEnqueuer(be))
	m, _ = m.handleInsightsOpen()

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
	m, _ = m.handleInsightsOpen()

	m = m.handleInsightsJumpBrowse()

	if m.ActiveView != viewBrowse {
		t.Fatalf("b should land in Browse, got view %d", m.ActiveView)
	}
	// A session is named by where it starts, so b follows its opener.
	if got := m.browse.Artists.Artists[m.browse.Artists.Cursor]; got != "Alice" {
		t.Errorf("want the session's opener focused, got %q", got)
	}
}

func TestInsightsJumpBrowseFromARankingRow(t *testing.T) {
	m := newInsightsModel(t)
	m.insights = m.insights.ShiftPanel(int(instui.PanelTimeVsPlays))
	m.insights.Snap.Rankings = rankingsFixture()
	m.insights.ClampCursor()
	if row := m.insights.FocusedRow(); row == nil || row.Key != "Dana" {
		t.Fatalf("precondition: want the biggest mover focused, got %+v", row)
	}

	// Dana is not in the fixture library, so the jump has nothing to land on and
	// says so rather than switching views to an arbitrary artist.
	if got := m.handleInsightsJumpBrowse(); got.ActiveView != viewInsights {
		t.Errorf("an unknown artist should not change view, got %d", got.ActiveView)
	}

	m.insights.Cursor = 1 // Bob, who does exist
	m = m.handleInsightsJumpBrowse()
	if m.ActiveView != viewBrowse {
		t.Fatalf("b should land in Browse, got view %d", m.ActiveView)
	}
	if got := m.browse.Artists.Artists[m.browse.Artists.Cursor]; got != "Bob" {
		t.Errorf("want Bob focused, got %q", got)
	}
}

func TestInsightsCursorDoesNotFallThroughToQueue(t *testing.T) {
	m := newInsightsModel(t)
	m.queue.Entries = []queue.Entry{{ID: 1}, {ID: 2}, {ID: 3}}

	m = pressKey(m, "j")
	m = pressKey(m, "j")

	if m.insights.Cursor != 2 {
		t.Errorf("j should move the Insights cursor: want 2, got %d", m.insights.Cursor)
	}
	if m.queue.Cursor != 0 {
		t.Errorf("j leaked into the Queue cursor: got %d", m.queue.Cursor)
	}

	m = pressKey(m, "k")

	if m.insights.Cursor != 1 {
		t.Errorf("k should move the Insights cursor: want 1, got %d", m.insights.Cursor)
	}
	if m.queue.Cursor != 0 {
		t.Errorf("k leaked into the Queue cursor: got %d", m.queue.Cursor)
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
	if be.appendTracksSources[0] != m.insights.FocusedRow().Label {
		t.Errorf("source should be the row label, got %q", be.appendTracksSources[0])
	}
}

func TestInsightsSavePlaylist(t *testing.T) {
	pw := &stubPlaylistCreator{nextID: 7}
	pa := &stubPlaylistAdder{}
	m := newInsightsModel(t, WithPlaylistWriter(pw), WithPlaylistAdder(pa))
	m, _ = m.handleInsightsOpen()
	if m.insights.Level != 1 {
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

	want := "Insights: " + m.insights.DrillLabel
	if msg.playlistName != want {
		t.Errorf("playlist name: want %q, got %q", want, msg.playlistName)
	}
	if msg.count != len(m.insights.TrackView.SourceTracks) {
		t.Errorf("count: want %d, got %d", len(m.insights.TrackView.SourceTracks), msg.count)
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
