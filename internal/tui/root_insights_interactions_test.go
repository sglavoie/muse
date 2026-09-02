package tui

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
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

func TestInsightsRutBreakerSteersAwayFromTheTopArtists(t *testing.T) {
	m := newInsightsModel(t)
	m.insights.Snap.Rankings.ArtistsByHours = []insights.EntityStat{{Name: "Alice", ListenedSecs: 9000}}

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
	m.insights = m.insights.ShiftPanel(int(instui.PanelConcentration))
	m.insights.Snap.Rankings.ArtistsByHours = []insights.EntityStat{{Name: "Alice", ListenedSecs: 9000}}

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

// insightsShelfTracks piles the unheard material on one shelf: Zephyr holds 60
// never-played tracks, Yarrow 20, and twenty Aardvark artists one each. The
// Aardvarks sort first, so the alphabetical prefix the anti-rut draw used to
// return contained none of the dust the Coverage panel points at.
func insightsShelfTracks() []library.Track {
	var out []library.Track
	add := func(artist string, n int) {
		for i := range n {
			out = append(out, library.Track{
				Artist:   artist,
				Album:    "Album",
				Title:    fmt.Sprintf("%s %02d", artist, i),
				Hash:     fmt.Sprintf("%s-%02d", artist, i),
				Path:     fmt.Sprintf("/%s-%02d.flac", artist, i),
				Genre:    "Rock",
				Duration: 200,
			})
		}
	}
	add("Zephyr", 60)
	add("Yarrow", 20)
	for i := range 20 {
		add(fmt.Sprintf("Aardvark %02d", i), 1)
	}
	// One heard track on the dustiest shelf, so a draw that ignored PlayCount
	// would be caught rather than silently correct.
	out = append(out, library.Track{
		Artist: "Zephyr", Album: "Album", Title: "Heard", Hash: "zephyr-heard",
		Path: "/zephyr-heard.flac", Genre: "Rock", Duration: 200,
		PlayCount: 3, LastPlayedAt: time.Now().Unix(),
	})
	return out
}

func hashesOf(tracks []library.Track) []string {
	out := make([]string, len(tracks))
	for i, t := range tracks {
		out[i] = t.Hash
	}
	return out
}

func TestInsightsCoverageAntiRutWeightsDustyShelves(t *testing.T) {
	tracks := insightsShelfTracks()
	m := New(context.Background(), WithLibraryTracks(tracks))

	draw := func(seed uint64) []library.Track {
		m.insightsRand = rand.New(rand.NewPCG(seed, seed)).IntN //nolint:gosec // seeded source so the draw is reproducible
		return m.insightsCoverageAntiRutTracks(insightsAntiRutCount)
	}

	t.Run("dusty shelves lead every seeded draw", func(t *testing.T) {
		dustyTotal := 0
		for seed := uint64(1); seed <= 5; seed++ {
			got := draw(seed)
			if len(got) != insightsAntiRutCount {
				t.Fatalf("seed %d: want %d tracks, got %d", seed, insightsAntiRutCount, len(got))
			}
			seen := map[string]struct{}{}
			dusty := 0
			for _, tr := range got {
				if tr.PlayCount != 0 {
					t.Errorf("seed %d: drew a heard track %q", seed, tr.Hash)
				}
				if _, dup := seen[tr.Hash]; dup {
					t.Errorf("seed %d: drew %q twice", seed, tr.Hash)
				}
				seen[tr.Hash] = struct{}{}
				if tr.Artist == "Zephyr" {
					dusty++
				}
			}
			// Zephyr holds 60 of the 100 unheard tracks, so a uniform draw would
			// average 15 of the 25. The shelf weighting has to beat that clearly,
			// not just tie it.
			if dusty < 19 {
				t.Errorf("seed %d: dustiest shelf supplied only %d of %d tracks", seed, dusty, len(got))
			}
			dustyTotal += dusty
		}
		// The weight decays as a shelf is drawn from, so the biggest pile leads
		// without ever being the whole answer.
		if dustyTotal >= 5*insightsAntiRutCount {
			t.Errorf("every track across all seeds came from one shelf: %d", dustyTotal)
		}
	})

	t.Run("a seed reproduces its draw", func(t *testing.T) {
		first, second := hashesOf(draw(7)), hashesOf(draw(7))
		if !slices.Equal(first, second) {
			t.Errorf("same seed drew different sets:\n%v\n%v", first, second)
		}
		if other := hashesOf(draw(8)); slices.Equal(first, other) {
			t.Error("a different seed drew the same set, so the source is not being consulted")
		}
	})

	t.Run("the draw is not the alphabetical prefix", func(t *testing.T) {
		var unheard []library.Track
		for _, tr := range m.library.Index.Tracks() {
			if tr.PlayCount == 0 {
				unheard = append(unheard, tr)
			}
		}
		prefix := hashesOf(unheard[:insightsAntiRutCount])
		if got := hashesOf(draw(1)); slices.Equal(got, prefix) {
			t.Errorf("draw returned the artist-sorted prefix: %v", got)
		}
	})

	t.Run("the key enqueues the weighted draw", func(t *testing.T) {
		be := &stubBatchEnqueuer{}
		km := newInsightsModel(t,
			WithLibraryTracks(tracks),
			WithInsightsReader(&uitestInsights{tracks: tracks}),
			WithBatchEnqueuer(be),
		)
		km.insightsRand = rand.New(rand.NewPCG(3, 3)).IntN //nolint:gosec // seeded source so the draw is reproducible

		_, cmd := km.handleInsightsAntiRut()
		if cmd == nil {
			t.Fatal("expected an enqueue cmd")
		}
		cmd()

		if len(be.appendTracksCalls) != 1 {
			t.Fatalf("expected one AppendTracks call, got %d", len(be.appendTracksCalls))
		}
		enqueued := be.appendTracksCalls[0]
		if len(enqueued) != insightsAntiRutCount {
			t.Errorf("want %d tracks enqueued, got %d", insightsAntiRutCount, len(enqueued))
		}
		dusty := 0
		for _, tr := range enqueued {
			if tr.PlayCount != 0 {
				t.Errorf("enqueued a heard track: %q", tr.Hash)
			}
			if tr.Artist == "Zephyr" {
				dusty++
			}
		}
		if dusty < 19 {
			t.Errorf("dustiest shelf supplied only %d of the enqueued tracks", dusty)
		}
	})
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
