package tui

import (
	"errors"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

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
