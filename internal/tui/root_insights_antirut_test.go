package tui

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	instui "github.com/sglavoie/muse/internal/tui/insights"
)

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
