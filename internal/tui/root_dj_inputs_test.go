package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
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

// TestDJSignalsLoadWithoutRealSeed covers the synthetic and unseeded starts:
// what the listener plays at this hour is true of them regardless of the seed,
// and only co-occurrence is genuinely seed-relative.
func TestDJSignalsLoadWithoutRealSeed(t *testing.T) {
	reader := newDJRecReader()
	reader.affinities = []db.HourGenreAffinity{
		{Hour: djFixedNow.Hour(), Genre: "Jazz", Count: 40},
		{Hour: (djFixedNow.Hour() + 1) % 24, Genre: "Rock", Count: 90},
	}

	signals := djLoadSignals(reader, []string{"d1", "f1"}, "", djFixedNow)

	completion, hours, coOcc := reader.snapshot()
	if hours != 1 {
		t.Errorf("hour affinities were read %d times without a seed, want 1", hours)
	}
	if len(completion) != 1 {
		t.Errorf("completion rates were read %d times without a seed, want 1", len(completion))
	}
	if len(coOcc) != 0 {
		t.Errorf("co-occurrence was asked for %d times with no seed to relate to", len(coOcc))
	}
	if signals.Seeded() {
		t.Error("an unseeded load reported itself as seeded")
	}
	if signals.Degraded() {
		t.Errorf("a missing seed was reported as a degradation: %v", signals.Failures())
	}
	if got := signals.timeGenreBonus["Jazz"]; got == 0 {
		t.Error("the hour bonus for the current hour was dropped")
	}
	if _, ok := signals.timeGenreBonus["Rock"]; ok {
		t.Error("the hour bonus picked up another hour's genre")
	}
	if got := signals.Hour(); got != djFixedNow.Hour() {
		t.Errorf("signals reported hour %d, want %d", got, djFixedNow.Hour())
	}
}

// TestDJBehavioralFailureDegradesWithoutBlocking is the optional half of the
// eligibility/behaviour split: a reader that cannot answer costs the batch some
// precision, and nothing else.
func TestDJBehavioralFailureDegradesWithoutBlocking(t *testing.T) {
	boom := errors.New("db closed")
	reader := newDJRecReader()
	reader.completionErr = boom
	reader.hourErr = boom
	reader.coOccErr = boom

	m := newDJModel(WithRecommendationReader(reader))
	in := m.captureDJInputs(djFixedNow)

	result, signals, err := djPlanBatch(in, djTestRequest(), radio.Allocation{}, reader, djFixedNow)
	if err != nil {
		t.Fatalf("a failed behavioral read blocked the plan: %v", err)
	}
	if len(result.Tracks) == 0 {
		t.Fatal("a failed behavioral read emptied the batch")
	}
	if !signals.Degraded() {
		t.Fatal("three failed reads were not reported as degradation")
	}
	if got := len(signals.Failures()); got != 3 {
		t.Errorf("reported %d failures, want 3: %v", got, signals.Failures())
	}
	for _, f := range signals.Failures() {
		if !errors.Is(f.Err, boom) {
			t.Errorf("failure %q lost the underlying error: %v", f.Signal, f.Err)
		}
	}
	// Failures() hands out a copy, so a caller logging them cannot corrupt the
	// record the acceptance path reads.
	got := signals.Failures()
	got[0] = djSignalFailure{}
	if signals.Failures()[0].Signal == "" {
		t.Error("Failures() exposed the internal slice")
	}
}

// TestDJEligibilityFailureBlocksThePlan is the other half: with no trustworthy
// tag catalog the planner would skip its unresolved-tag check and quietly
// widen an exclusion or narrow an inclusion, so a tag-filtered request must
// fail instead of returning a batch that ignores the user's filters.
func TestDJEligibilityFailureBlocksThePlan(t *testing.T) {
	boom := errors.New("read tags: disk error")
	reader := newDJRecReader()

	base := newDJModel(WithRecommendationReader(reader))
	base.tagCacheErr = boom
	in := base.captureDJInputs(djFixedNow)

	t.Run("tag-filtered settings", func(t *testing.T) {
		req := djTestRequest()
		req.Settings.Include = radio.FilterSet{TagIDs: []int64{7}}

		result, signals, err := djPlanBatch(in, req, radio.Allocation{}, reader, djFixedNow)
		if !errors.Is(err, errDJTagCatalogUnavailable) {
			t.Fatalf("an unreadable catalog produced err=%v", err)
		}
		if !errors.Is(err, boom) {
			t.Errorf("the error lost the underlying cause: %v", err)
		}
		if len(result.Tracks) != 0 || signals.Degraded() {
			t.Error("a blocked plan still produced work")
		}
		if calls, _, _ := reader.snapshot(); len(calls) != 0 {
			t.Errorf("a blocked plan still issued %d behavioral reads", len(calls))
		}
	})

	t.Run("settings that reference no tag", func(t *testing.T) {
		result, _, err := djPlanBatch(in, djTestRequest(), radio.Allocation{}, reader, djFixedNow)
		if err != nil {
			t.Fatalf("an unreadable catalog blocked a request with no tag filter: %v", err)
		}
		if len(result.Tracks) == 0 {
			t.Error("a request with no tag filter produced no batch")
		}
	})

	t.Run("an empty but trusted catalog still resolves", func(t *testing.T) {
		m := newDJModel(WithRecommendationReader(reader))
		fresh := m.captureDJInputs(djFixedNow)
		req := djTestRequest()
		req.Settings.Include = radio.FilterSet{TagIDs: []int64{7}}

		result, _, err := djPlanBatch(fresh, req, radio.Allocation{}, reader, djFixedNow)
		if err != nil {
			t.Fatalf("a trusted empty catalog was treated as unavailable: %v", err)
		}
		// The library genuinely has no tags, so tag 7 is unresolved and the
		// planner blocks rather than silently planning without the filter.
		if result.Outcome != radio.PlanOutcomeBlocked {
			t.Errorf("a filter on a nonexistent tag produced outcome %q, want %q",
				result.Outcome, radio.PlanOutcomeBlocked)
		}
	})
}

// TestDJInputsStaleAt covers both ways a snapshot retires. The clock case is
// the one no writer announces: a Muse left running overnight would otherwise
// keep scoring against the hour it was started in and classifying against a
// discovery cutoff that has since slid.
func TestDJInputsStaleAt(t *testing.T) {
	m := newDJModel()
	in := m.captureDJInputs(djFixedNow)

	if in.StaleAt(djFixedNow, m.djDataRevision(), m.djQueueRevision()) {
		t.Fatal("a snapshot was stale at the instant it was taken")
	}
	if in.StaleAt(djFixedNow.Add(20*time.Minute), m.djDataRevision(), m.djQueueRevision()) {
		t.Error("a snapshot retired within the same hour")
	}
	if !in.StaleAt(djFixedNow.Add(time.Hour), m.djDataRevision(), m.djQueueRevision()) {
		t.Error("a snapshot survived the hour rolling over")
	}
	if !in.StaleAt(djFixedNow.AddDate(0, 0, 1), m.djDataRevision(), m.djQueueRevision()) {
		t.Error("a snapshot survived a full day, so the discovery cutoff slid unnoticed")
	}
	if !in.StaleAt(djFixedNow, m.djDataRevision()+1, m.djQueueRevision()) {
		t.Error("a snapshot survived a data revision it no longer matches")
	}
	if !in.StaleAt(djFixedNow, m.djDataRevision(), m.djQueueRevision()+1) {
		t.Error("a snapshot survived a queue revision it no longer matches")
	}
}

// TestDJSignalsStaleAt reports the narrower fact the acceptance path needs: the
// hour bonus a batch was scored with now describes a different hour.
func TestDJSignalsStaleAt(t *testing.T) {
	signals := djLoadSignals(nil, nil, "", djFixedNow)
	if signals.StaleAt(djFixedNow.Add(20 * time.Minute)) {
		t.Error("the hour bonus retired within its own hour")
	}
	if !signals.StaleAt(djFixedNow.Add(time.Hour)) {
		t.Error("the hour bonus survived the hour rolling over")
	}
}

// TestDJProtectedHashesCoverNowPlayingAndUpcoming checks the only hard
// duplicate guard the planner has. Already-played rows are deliberately absent:
// protecting the whole persisted queue would shrink the reachable library with
// every batch continuous radio enqueues.
func TestDJProtectedHashesCoverNowPlayingAndUpcoming(t *testing.T) {
	m := newDJModel()
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "f1", Path: "/f1.flac"},
		{ID: 2, Hash: "f2", Path: "/f2.flac"},
		{ID: 3, Hash: "d1", Path: "/d1.flac"},
	}
	m.playback.currentIndex = 1
	m.playback.currentHash = "f2"

	protected := m.djProtectedHashes()

	for _, want := range []string{"f2", "d1"} {
		if _, ok := protected[want]; !ok {
			t.Errorf("the playing/upcoming hash %q was left selectable", want)
		}
	}
	if _, ok := protected["f1"]; ok {
		t.Error("an already-played queue row was protected, shrinking the reachable library")
	}

	// The protection has to survive into the planner's input, where it is the
	// only guard that removes candidates outright.
	in := m.captureDJInputs(djFixedNow)
	pools := radio.Partition(in.PlanInput(djTestRequest(), radio.Allocation{}))
	for _, pool := range [][]radio.Candidate{pools.Discovery, pools.Familiar} {
		for _, c := range pool {
			if c.Track.Hash == "f2" || c.Track.Hash == "d1" {
				t.Errorf("protected hash %q survived into a candidate pool", c.Track.Hash)
			}
		}
	}
}

// TestHydrateTrackRecoversLibraryFields covers the adapters that lose columns.
// A queue entry and a db.PlaylistTrackRow carry no Genre, PlayCount, or
// LastPlayedAt at all, so a selection adapted straight from either would read
// as a genre-less never-played track: every row would fail a genre inclusion
// and classify as pure discovery.
func TestHydrateTrackRecoversLibraryFields(t *testing.T) {
	rows := []db.PlaylistTrackRow{
		{ID: 10, Hash: "f1", Path: "/f1.flac", Artist: "Alice", Title: "F1", Album: "Alpha", DurationMs: 200000},
		{ID: 20, Hash: "gone", Path: "/gone.flac", Artist: "Ghost", Title: "Gone", Album: "Nowhere", DurationMs: 90000},
	}

	assertHydrated := func(t *testing.T, tracks []library.Track, label string) {
		t.Helper()
		byHash := make(map[string]library.Track, len(tracks))
		for _, tr := range tracks {
			byHash[tr.Hash] = tr
		}
		live, ok := byHash["f1"]
		if !ok {
			t.Fatalf("%s: the indexed track was dropped", label)
		}
		if live.Genre != "Rock" {
			t.Errorf("%s: genre = %q, want Rock", label, live.Genre)
		}
		if live.PlayCount != 9 {
			t.Errorf("%s: play count = %d, want 9", label, live.PlayCount)
		}
		if live.LastPlayedAt == 0 {
			t.Errorf("%s: last played was not recovered", label)
		}
		// A hash the index no longer holds keeps the view's own row, which is
		// what keeps a missing file exportable.
		missing, ok := byHash["gone"]
		if !ok {
			t.Fatalf("%s: the unindexed row was dropped instead of falling back", label)
		}
		if missing.Title != "Gone" || missing.Path != "/gone.flac" {
			t.Errorf("%s: the fallback row lost its own values: %+v", label, missing)
		}
	}

	t.Run("playlists", func(t *testing.T) {
		m := newDJModel()
		m.ActiveView = viewPlaylists
		m.playlists.Level = pltui.TrackLevel
		m.playlists.Tracks = rows

		tracks, _, ok := m.playlistsVisibleTracks()
		if !ok {
			t.Fatal("playlists offered no track list")
		}
		assertHydrated(t, tracks, "playlists")
	})

	t.Run("tags", func(t *testing.T) {
		m := newDJModel()
		m.ActiveView = viewTags
		m.tagsView.Level = tagtui.TrackLevel
		m.tagsView.SelectedTag = db.TagRow{ID: 7, Name: "chill"}
		m.tagsView.Tracks = rows

		tracks, _, ok := m.tagsVisibleTracks()
		if !ok {
			t.Fatal("tags offered no track list")
		}
		assertHydrated(t, tracks, "tags")
	})

	t.Run("queue", func(t *testing.T) {
		m := newDJModel()
		m.ActiveView = viewQueue
		m.queue.Entries = []queue.Entry{
			{ID: 1, Hash: "f1", Path: "/f1.flac", Artist: "Alice", Title: "F1", Album: "Alpha", DurationMs: 200000},
			{ID: 2, Hash: "gone", Path: "/gone.flac", Artist: "Ghost", Title: "Gone", Album: "Nowhere", DurationMs: 90000},
		}

		tracks, _, ok := m.queueVisibleTracks()
		if !ok {
			t.Fatal("the queue offered no track list")
		}
		assertHydrated(t, tracks, "queue")
	})

	t.Run("an unhashed row is left alone", func(t *testing.T) {
		m := newDJModel()
		row := library.Track{Title: "No hash", Path: "/nohash.flac"}
		if got := m.hydrateTrack(row); got != row {
			t.Errorf("a row with no hash was rewritten: %+v", got)
		}
	})
}

// TestRadioEnqueueLoadsHourAffinityWithoutSeed pins the live path, not just the
// helper: the pre-DJ radio command gated all three behavioral queries on a real
// seed hash, which threw the time-of-day signal away for precisely the starts
// with the least other evidence to go on.
func TestRadioEnqueueLoadsHourAffinityWithoutSeed(t *testing.T) {
	reader := newDJRecReader()
	be := &stubBatchEnqueuer{}
	m := newDJModel(WithRecommendationReader(reader))
	m.batchEnqueuer = be
	m.radio.enabled = true
	m.radio.seedHash = ""

	cmd := m.radioEnqueueCmd()
	if cmd == nil {
		t.Fatal("radio offered no enqueue cmd")
	}
	drainCmd(cmd)

	completion, hours, coOcc := reader.snapshot()
	if hours != 1 {
		t.Errorf("hour affinities were read %d times for an unseeded start, want 1", hours)
	}
	if len(completion) != 1 {
		t.Errorf("completion rates were read %d times, want 1 batched read", len(completion))
	}
	if len(coOcc) != 0 {
		t.Errorf("co-occurrence was read %d times with no seed", len(coOcc))
	}
}
