package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// djCountingRecReader records every behavioral query the DJ issues, so a test
// can assert both what was asked for and how many times. Each query can be made
// to fail independently, which is how the optional-signal path is exercised.
type djCountingRecReader struct {
	*stubRecReader

	mu              sync.Mutex
	completionCalls [][]string
	hourCalls       int
	coOccCalls      []string

	rates      map[string]float64
	affinities []db.HourGenreAffinity
	coOccs     []db.SessionCoOccurrence

	completionErr error
	hourErr       error
	coOccErr      error
}

func newDJRecReader() *djCountingRecReader {
	return &djCountingRecReader{stubRecReader: &stubRecReader{}}
}

func (r *djCountingRecReader) ReadCompletionRates(hashes []string) (map[string]float64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	call := make([]string, len(hashes))
	copy(call, hashes)
	r.completionCalls = append(r.completionCalls, call)
	if r.completionErr != nil {
		return nil, r.completionErr
	}
	return r.rates, nil
}

func (r *djCountingRecReader) ReadHourGenreAffinities() ([]db.HourGenreAffinity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hourCalls++
	if r.hourErr != nil {
		return nil, r.hourErr
	}
	return r.affinities, nil
}

func (r *djCountingRecReader) ReadSessionCoOccurrences(seed string, _ int) ([]db.SessionCoOccurrence, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.coOccCalls = append(r.coOccCalls, seed)
	if r.coOccErr != nil {
		return nil, r.coOccErr
	}
	return r.coOccs, nil
}

func (r *djCountingRecReader) snapshot() (completion [][]string, hours int, coOcc []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.completionCalls, r.hourCalls, r.coOccCalls
}

// djFixedNow is the instant every planning test classifies against, so the
// 90-day discovery cutoff lands on a known date rather than on the wall clock.
var djFixedNow = time.Date(2026, 4, 12, 15, 30, 0, 0, time.UTC)

// djTestTracks gives both pools three members each: f* were counted-played this
// month, d* were never played. A per-group hydration budget must reach into
// both halves.
func djTestTracks() []library.Track {
	recent := djFixedNow.AddDate(0, 0, -2).Unix()
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "F1", Hash: "f1", Path: "/f1.flac", Genre: "Rock", Duration: 200, PlayCount: 9, LastPlayedAt: recent},
		{Artist: "Bob", Album: "Beta", Title: "F2", Hash: "f2", Path: "/f2.flac", Genre: "Rock", Duration: 210, PlayCount: 7, LastPlayedAt: recent},
		{Artist: "Cara", Album: "Gamma", Title: "F3", Hash: "f3", Path: "/f3.flac", Genre: "Jazz", Duration: 220, PlayCount: 4, LastPlayedAt: recent},
		{Artist: "Dana", Album: "Delta", Title: "D1", Hash: "d1", Path: "/d1.flac", Genre: "Rock", Duration: 230},
		{Artist: "Eli", Album: "Epsilon", Title: "D2", Hash: "d2", Path: "/d2.flac", Genre: "Jazz", Duration: 240},
		{Artist: "Fay", Album: "Zeta", Title: "D3", Hash: "d3", Path: "/d3.flac", Genre: "Folk", Duration: 250},
	}
}

// djTestRequest is a valid, seeded request over the fixture above.
func djTestRequest() radio.PlanRequest {
	return radio.PlanRequest{
		Settings: radio.DefaultSettings(),
		Seed: radio.SeedSnapshot{
			TrackHash: "f1",
			Title:     "F1",
			Artist:    "Alice",
			Album:     "Alpha",
			Genres:    []string{"Rock"},
			Duration:  200,
		},
		BatchSize:   radioBatchSize,
		RequestedAt: djFixedNow,
	}
}

// newDJModel returns a Model loaded with the planning fixture.
func newDJModel(opts ...Option) Model {
	base := []Option{WithLibraryTracks(djTestTracks())}
	return New(context.Background(), append(base, opts...)...)
}

// TestDJInvalidationWalk walks every real event that changes what the Auto-DJ
// plans from and checks each one advances the data revision. A snapshot taken
// before the event has to be recognizable as stale afterwards; a missed bump
// here is a batch classified or filtered against a library that moved.
func TestDJInvalidationWalk(t *testing.T) {
	cases := []struct {
		name  string
		apply func(t *testing.T, m Model) Model
	}{
		{"a counted listen", func(_ *testing.T, m Model) Model {
			return m.handlePlayRecordedMsg(playback.PlayRecordedMsg{
				Hash: "f1", PlayedAt: djFixedNow.Unix(), Counted: true,
			})
		}},
		{"a scan batch", func(_ *testing.T, m Model) Model {
			out, _ := m.handleScanBatchMsg(library.ScanBatchMsg{Tracks: []library.Track{
				{Artist: "Gus", Album: "Eta", Title: "N1", Hash: "n1", Path: "/n1.flac", Genre: "Drone", Duration: 300},
			}})
			return out
		}},
		{"a finished scan", func(_ *testing.T, m Model) Model {
			out, _ := m.handleScanDoneMsg()
			return out
		}},
		{"a watcher batch", func(_ *testing.T, m Model) Model {
			out, _ := m.handleWatchBatchMsg(watchBatchMsg{
				tracks:  []library.Track{{Artist: "Gus", Album: "Eta", Title: "N1", Hash: "n1", Path: "/n1.flac", Genre: "Drone", Duration: 300}},
				deleted: []string{"/d3.flac"},
			})
			return out
		}},
		{"a metadata edit", func(_ *testing.T, m Model) Model {
			next, _ := m.Update(metadataEditedMsg{})
			return next.(Model)
		}},
		{"an applied metadata import", func(_ *testing.T, m Model) Model {
			return m.handleMetadataImportAppliedMsg(metadataImportAppliedMsg{applied: 1})
		}},
		{"an undone metadata edit", func(_ *testing.T, m Model) Model {
			m.setUndoSlot(undoBulkEditMetadataAction{
				originals: map[string][5]string{"f1": {"F1", "Alice", "Alpha", "Punk", "3:20"}},
			}, nil)
			next, _ := m.Update(UndoMsg{})
			return next.(Model)
		}},
		{"an undone metadata import", func(_ *testing.T, m Model) Model {
			m.setUndoSlot(undoImportMetadataAction{
				originals: map[string]importOriginal{
					"f1": {title: "F1", artist: "Alice", album: "Alpha", genre: "Punk"},
				},
			}, nil)
			next, _ := m.Update(UndoMsg{})
			return next.(Model)
		}},
		{"a track favorite", func(_ *testing.T, m Model) Model {
			return m.handleFavoriteToggledMsg(favoriteToggledMsg{
				typ: "track", value: "/f1.flac", added: true, removedIdx: -1,
			})
		}},
		{"an undone track favorite", func(_ *testing.T, m Model) Model {
			m.setUndoSlot(undoAddFavoriteAction{typ: "track", value: "/f1.flac"}, nil)
			next, _ := m.Update(UndoMsg{})
			return next.(Model)
		}},
		{"a dismissal", func(_ *testing.T, m Model) Model {
			return m.handleMixDismissedMsg(mixDismissedMsg{hash: "d1"})
		}},
		{"an undismissal", func(_ *testing.T, m Model) Model {
			m.dismissedHashes = map[string]struct{}{"d1": {}}
			return m.handleMixUndismissedMsg(mixUndismissedMsg{hash: "d1"})
		}},
		{"a tag toggle", func(_ *testing.T, m Model) Model {
			return m.applyTagToggle(7, []string{"f1"}, true)
		}},
		{"every tag removed from a track", func(_ *testing.T, m Model) Model {
			m.trackTags = map[string]map[int64]struct{}{"f1": {7: {}}}
			out, _ := m.handleRemoveAllTagsFromTrack(removeAllTagsFromTrackMsg{hash: "f1"})
			return out
		}},
		{"a tag merge", func(_ *testing.T, m Model) Model {
			m.trackTags = map[string]map[int64]struct{}{"f1": {7: {}}}
			out, _ := m.handleTagMerged(tagMergedMsg{
				src:  db.TagRow{ID: 7, Name: "src"},
				dest: db.TagRow{ID: 8, Name: "dest"},
			})
			return out
		}},
		{"a rebuilt tag catalog", func(_ *testing.T, m Model) Model {
			return m.rebuildTagCache([]db.TagRow{{ID: 7, Name: "chill"}})
		}},
		{"a tag refresh", func(_ *testing.T, m Model) Model {
			return m.handleTagsRefreshed(tagsRefreshedMsg{
				tags:      []db.TagRow{{ID: 7, Name: "chill"}},
				trackTags: map[string]map[int64]struct{}{"f1": {7: {}}},
			})
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newDJModel()
			before := m.captureDJInputs(djFixedNow)

			m = tc.apply(t, m)

			if m.djDataRevision() == before.DataRev() {
				t.Fatalf("%s left the Auto-DJ data revision at %d", tc.name, before.DataRev())
			}
			if !before.StaleAt(djFixedNow, m.djDataRevision(), m.djQueueRevision()) {
				t.Errorf("%s left a snapshot taken beforehand usable", tc.name)
			}
		})
	}
}

// TestDJInvalidationIgnoresUnrelatedEvents pins the other half of the contract:
// a revision that moves for events the planner does not read would retire every
// in-flight snapshot for nothing and turn continuous radio into a stall.
func TestDJInvalidationIgnoresUnrelatedEvents(t *testing.T) {
	t.Run("an uncounted skip", func(t *testing.T) {
		m := newDJModel()
		before := m.djDataRevision()

		// A skip lands in play_events but not in the counted history, so
		// PlayCount, LastPlayedAt, and every behavioral signal are unchanged.
		m = m.handlePlayRecordedMsg(playback.PlayRecordedMsg{
			Hash: "f1", PlayedAt: djFixedNow.Unix(), Counted: false,
		})

		if got := m.djDataRevision(); got != before {
			t.Errorf("an uncounted skip advanced the data revision to %d", got)
		}
	})

	t.Run("an artist favorite", func(t *testing.T) {
		m := newDJModel()
		before := m.djDataRevision()

		m = m.handleFavoriteToggledMsg(favoriteToggledMsg{
			typ: "artist", value: "Alice", added: true, removedIdx: -1,
		})

		if got := m.djDataRevision(); got != before {
			t.Errorf("an artist favorite advanced the data revision to %d", got)
		}
	})
}

// TestDJInputsSnapshotIsolatesLiveMutations captures a snapshot and then keeps
// the update loop's maps changing underneath it, which is exactly what happens
// while a plan runs on its own goroutine. Run with -race for the strongest
// signal: without the copies, the reads below race the writes.
func TestDJInputsSnapshotIsolatesLiveMutations(t *testing.T) {
	m := newDJModel()
	m.trackTags = map[string]map[int64]struct{}{"f1": {7: {}}}
	m.favoriteHashes = map[string]struct{}{"f1": {}}
	m.dismissedHashes = map[string]struct{}{"d1": {}}

	in := m.captureDJInputs(djFixedNow)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				for i := int64(0); i < 50; i++ {
					m.trackTags["f1"][i] = struct{}{}
					delete(m.trackTags["f1"], i)
				}
				m.trackTags["d2"] = map[int64]struct{}{9: {}}
				delete(m.trackTags, "d2")
				m.favoriteHashes["d2"] = struct{}{}
				delete(m.favoriteHashes, "d2")
				m.dismissedHashes["d2"] = struct{}{}
				delete(m.dismissedHashes, "d2")
			}
		}
	}()

	pi := in.PlanInput(djTestRequest(), radio.Allocation{})
	for i := 0; i < 200; i++ {
		radio.Partition(pi)
		if len(pi.TrackTags["f1"]) != 1 {
			close(stop)
			<-done
			t.Fatalf("the snapshot's tag set followed a live edit: %v", pi.TrackTags["f1"])
		}
		if len(pi.FavoriteHashes) != 1 || len(pi.DismissedHashes) != 1 {
			close(stop)
			<-done
			t.Fatalf("the snapshot's favorite/dismissed sets followed a live edit")
		}
	}
	close(stop)
	<-done

	if _, ok := pi.TrackTags["d2"]; ok {
		t.Error("a track added to the live tag cache appeared in the captured snapshot")
	}
}

// TestDJSignalsLoadOnceForBothPools is the query-per-track guard: whatever the
// size of the candidate set, one batch of hashes goes out in a single read.
func TestDJSignalsLoadOnceForBothPools(t *testing.T) {
	reader := newDJRecReader()
	m := newDJModel(WithRecommendationReader(reader))
	in := m.captureDJInputs(djFixedNow)

	result, signals, err := djPlanBatch(in, djTestRequest(), radio.Allocation{}, reader, djFixedNow)
	if err != nil {
		t.Fatalf("planning a fully available snapshot failed: %v", err)
	}
	if len(result.Tracks) == 0 {
		t.Fatal("the fixture produced no batch, so the hydration assertions below prove nothing")
	}
	if signals.Degraded() {
		t.Errorf("a reader that answered everything reported degradation: %v", signals.Failures())
	}

	completion, hours, coOcc := reader.snapshot()
	if len(completion) != 1 {
		t.Fatalf("completion rates were read %d times, want exactly 1 batched read", len(completion))
	}
	if hours != 1 {
		t.Errorf("hour affinities were read %d times, want exactly 1", hours)
	}
	if len(coOcc) != 1 {
		t.Errorf("co-occurrence was read %d times, want exactly 1", len(coOcc))
	}

	discovery, familiar := djCountPools(completion[0])
	if discovery == 0 || familiar == 0 {
		t.Errorf("hydration reached only one pool: %d discovery, %d familiar (%v)",
			discovery, familiar, completion[0])
	}
}

// djCountPools splits hydration targets by the fixtures' hash prefixes.
func djCountPools(hashes []string) (discovery, familiar int) {
	for _, h := range hashes {
		switch {
		case strings.HasPrefix(h, "d"):
			discovery++
		case strings.HasPrefix(h, "f"):
			familiar++
		}
	}
	return discovery, familiar
}

// TestDJHydrationReachesTheStarvedPool is the real per-group assertion. The
// familiar half here matches the seed on genre, artist, and album while the
// discovery half matches nothing, so every familiar candidate outscores every
// discovery one on metadata alone and there are more of them than the whole
// hydration budget.
//
// A single global top-N would therefore hand all of its slots to the familiar
// pool, and the discovery pool would be scored with no completion rates and no
// co-occurrence — which is how an Explore target quietly turns into a familiar
// batch. Taking perGroup from each pool separately is what prevents it.
func TestDJHydrationReachesTheStarvedPool(t *testing.T) {
	recent := djFixedNow.AddDate(0, 0, -2).Unix()
	tracks := make([]library.Track, 0, 4*djHydrationPerGroup+3)
	for i := 0; i < 4*djHydrationPerGroup; i++ {
		tracks = append(tracks, library.Track{
			Artist: "Alice", Album: "Alpha", Title: "Familiar", Genre: "Rock",
			Hash:      "f" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Path:      "/f" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".flac",
			Duration:  200,
			PlayCount: 5, LastPlayedAt: recent,
		})
	}
	for i := 0; i < 3; i++ {
		tracks = append(tracks, library.Track{
			Artist: "Zed", Album: "Omega", Title: "Discovery", Genre: "Ambient",
			Hash:     "d" + string(rune('a'+i)),
			Path:     "/d" + string(rune('a'+i)) + ".flac",
			Duration: 900,
		})
	}

	reader := newDJRecReader()
	m := New(context.Background(), WithLibraryTracks(tracks), WithRecommendationReader(reader))
	in := m.captureDJInputs(djFixedNow)

	if _, _, err := djPlanBatch(in, djTestRequest(), radio.Allocation{}, reader, djFixedNow); err != nil {
		t.Fatalf("planning failed: %v", err)
	}

	completion, _, _ := reader.snapshot()
	if len(completion) != 1 {
		t.Fatalf("completion rates were read %d times, want exactly 1 batched read", len(completion))
	}
	discovery, familiar := djCountPools(completion[0])
	if familiar != djHydrationPerGroup {
		t.Errorf("the dominant pool hydrated %d candidates, want its full budget of %d",
			familiar, djHydrationPerGroup)
	}
	if discovery != 3 {
		t.Errorf("the starved pool hydrated %d of its 3 candidates (%v)", discovery, completion[0])
	}
}
