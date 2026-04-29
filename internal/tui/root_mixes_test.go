package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// stubRecReader is a minimal recommendationReader for mixes tests.
type stubRecReader struct {
	sufficient bool
	tracks     []library.Track
}

func (s *stubRecReader) HasSufficientHistory(_ int) (bool, error) {
	return s.sufficient, nil
}

func (s *stubRecReader) ReadRediscoverMix(_, _, _ int) ([]library.Track, error) {
	return s.tracks, nil
}
func (s *stubRecReader) ReadHiddenGemsMix(_, _, _, _ int) ([]library.Track, error) {
	return s.tracks, nil
}
func (s *stubRecReader) ReadOnRepeatMix(_, _, _ int) ([]library.Track, error) {
	return s.tracks, nil
}
func (s *stubRecReader) ReadDeepCutsMix(_, _ int, _ float64, _ int) ([]library.Track, error) {
	return s.tracks, nil
}
func (s *stubRecReader) ReadNeverPlayedMix(_ int) ([]library.Track, error) {
	return s.tracks, nil
}
func (s *stubRecReader) ReadCompletionRates(_ []string) (map[string]float64, error) {
	return nil, nil
}
func (s *stubRecReader) ReadSessionCoOccurrences(_ string, _ int) ([]db.SessionCoOccurrence, error) {
	return nil, nil
}
func (s *stubRecReader) ReadHourGenreAffinities() ([]db.HourGenreAffinity, error) {
	return nil, nil
}
func (s *stubRecReader) ReadAllMixProgress() (map[db.MixID]db.MixProgress, error) {
	return nil, nil
}

// newMixesModel returns a Model wired with a stubRecReader.
func newMixesModel(sufficient bool, tracks []library.Track) Model {
	m := New(context.Background(), WithRecommendationReader(&stubRecReader{
		sufficient: sufficient,
		tracks:     tracks,
	}))
	return m
}

// sampleMixTracks returns three distinct tracks for mix tests.
func sampleMixTracks() []library.Track {
	return []library.Track{
		{Hash: "mx1", Title: "Mix Track 1", Artist: "ArtistX", Path: "/mx1.flac"},
		{Hash: "mx2", Title: "Mix Track 2", Artist: "ArtistX", Path: "/mx2.flac"},
		{Hash: "mx3", Title: "Mix Track 3", Artist: "ArtistY", Path: "/mx3.flac"},
	}
}

// TestMixesViewSwitchLazyLoad verifies that actionSwitchMixes fires a
// non-nil loadMixCountsCmd on the first switch, before mixes are populated.
func TestMixesViewSwitchLazyLoad(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	// mixes.mixes is nil/empty before any switch.
	if len(m.mixes.mixes) != 0 {
		t.Fatal("precondition: expected empty mixes before first switch")
	}

	// mixesMaybeLoadCmd fires because len(mixes) == 0.
	cmd := m.mixesMaybeLoadCmd()
	if cmd == nil {
		t.Fatal("expected loadMixCountsCmd when mixes not yet initialized")
	}

	// After the switch, mixes are populated from mixDefs.
	m = m.switchToView(viewMixes)
	if len(m.mixes.mixes) == 0 {
		t.Fatal("expected mixes to be populated after switchToView")
	}
}

// TestMixesViewCountsLoaded verifies that mixCountsLoadedMsg populates
// TrackCount for each MixInfo and resets playsSinceRefresh.
func TestMixesViewCountsLoaded(t *testing.T) {
	m := newMixesModel(true, nil)
	m = m.switchToView(viewMixes)
	m.mixes.playsSinceRefresh = 3

	counts := map[db.MixID]int{
		db.MixRediscover:  10,
		db.MixHiddenGems:  5,
		db.MixOnRepeat:    20,
		db.MixDeepCuts:    7,
		db.MixNeverPlayed: 15,
	}
	m = m.handleMixCountsLoadedMsg(mixCountsLoadedMsg{counts: counts})

	for _, mix := range m.mixes.mixes {
		want := counts[mix.ID]
		if mix.TrackCount != want {
			t.Errorf("mix %q: want TrackCount=%d, got %d", mix.Name, want, mix.TrackCount)
		}
	}
	if m.mixes.playsSinceRefresh != 0 {
		t.Errorf("expected playsSinceRefresh reset to 0, got %d", m.mixes.playsSinceRefresh)
	}
}

// TestMixesViewDrillDown verifies that opening a mix transitions to level 1
// and returns a non-nil loadMixCmd.
func TestMixesViewDrillDown(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	m = m.switchToView(viewMixes)
	m.mixes.selectedMix = 0 // Rediscover

	m2, cmd := m.handleMixOpen()

	if m2.mixes.level != 1 {
		t.Fatalf("expected level 1 after drill-down, got %d", m2.mixes.level)
	}
	if cmd == nil {
		t.Fatal("expected loadMixCmd to be returned on drill-down")
	}
}

// TestMixesViewBack verifies that esc at level 1 returns to level 0.
func TestMixesViewBack(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	m = m.switchToView(viewMixes)
	m.mixes.selectedMix = 1

	m, _ = m.handleMixOpen()
	if m.mixes.level != 1 {
		t.Fatal("precondition: expected level 1")
	}

	m = m.handleMixBack()

	if m.mixes.level != 0 {
		t.Errorf("expected level 0 after back, got %d", m.mixes.level)
	}
}

// TestMixesViewTrackEnqueue verifies that handleLibraryEnqueueAction at
// track level (level 1) fires an enqueue cmd for the focused track.
func TestMixesViewTrackEnqueue(t *testing.T) {
	tracks := sampleMixTracks()
	qa := &stubQueueAppender{}
	m := New(context.Background(),
		WithRecommendationReader(&stubRecReader{sufficient: true, tracks: tracks}),
		WithQueueWriter(qa),
	)
	m = m.switchToView(viewMixes)
	m.mixes.mixes = append([]MixInfo(nil), mixDefs...)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})
	// Cursor at 0 → mx1.

	_, cmd, handled := m.handleLibraryEnqueueAction()
	if !handled {
		t.Fatal("expected enqueue action to be handled for viewMixes at level 1")
	}
	if cmd == nil {
		t.Fatal("expected non-nil enqueue cmd")
	}
	// Execute the cmd — this calls qa.AppendTrack synchronously.
	cmd()

	if len(qa.tracks) == 0 {
		t.Fatal("expected track to be enqueued after cmd execution, got 0")
	}
	if qa.tracks[0].Hash != "mx1" {
		t.Errorf("expected hash mx1, got %q", qa.tracks[0].Hash)
	}
}

// TestMixesViewBulkSelect verifies that bulkSelectedHashes returns the
// selected track hashes at level 1.
func TestMixesViewBulkSelect(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	// Select mx1 and mx3.
	m.mixes.trackView.selectedHashes = map[string]struct{}{"mx1": {}, "mx3": {}}

	hashes := m.bulkSelectedHashes()

	if len(hashes) != 2 {
		t.Fatalf("expected 2 selected hashes, got %d: %v", len(hashes), hashes)
	}
	got := map[string]struct{}{}
	for _, h := range hashes {
		got[h] = struct{}{}
	}
	if _, ok := got["mx1"]; !ok {
		t.Error("expected mx1 in selected hashes")
	}
	if _, ok := got["mx3"]; !ok {
		t.Error("expected mx3 in selected hashes")
	}
}

// TestMixesViewBulkSelectAtListLevel verifies that bulkSelectedHashes returns
// nil when mixes is at list level (level 0).
func TestMixesViewBulkSelectAtListLevel(t *testing.T) {
	m := newMixesModel(true, nil)
	m = m.switchToView(viewMixes)
	// level == 0 by default after switchToView.

	hashes := m.bulkSelectedHashes()
	if hashes != nil {
		t.Errorf("expected nil at list level, got %v", hashes)
	}
}

// TestMixesViewRefreshDerived verifies that refreshDerivedViews updates
// cached mix tracks with new metadata from the library index.
func TestMixesViewRefreshDerived(t *testing.T) {
	tracks := []library.Track{
		{Hash: "mx1", Title: "Old Title", Artist: "ArtistX"},
	}
	m := New(context.Background(),
		WithRecommendationReader(&stubRecReader{sufficient: true, tracks: tracks}),
		WithLibraryTracks([]library.Track{{Hash: "mx1", Title: "New Title", Artist: "ArtistX"}}),
	)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	m = m.refreshDerivedViews()

	cached := m.mixes.cachedMixes[db.MixRediscover]
	if len(cached) == 0 {
		t.Fatal("expected cached tracks to exist after refresh")
	}
	if cached[0].Title != "New Title" {
		t.Errorf("expected title updated to 'New Title', got %q", cached[0].Title)
	}
}

// TestMixesViewCacheInvalidation verifies that mixesMaybeLoadCmd fires a
// reload when playsSinceRefresh >= 5.
func TestMixesViewCacheInvalidation(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	m = m.switchToView(viewMixes)
	// Simulate mixes already loaded and 5 plays accumulated.
	m.mixes.mixes = append([]MixInfo(nil), mixDefs...)
	m.mixes.playsSinceRefresh = 5

	cmd := m.mixesMaybeLoadCmd()

	if cmd == nil {
		t.Fatal("expected reload cmd when playsSinceRefresh >= 5")
	}
}

// TestMixesViewNoReloadBelowThreshold verifies that mixesMaybeLoadCmd returns
// nil when mixes are loaded and playsSinceRefresh < 5.
func TestMixesViewNoReloadBelowThreshold(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	m = m.switchToView(viewMixes)
	m.mixes.mixes = append([]MixInfo(nil), mixDefs...)
	m.mixes.playsSinceRefresh = 4

	cmd := m.mixesMaybeLoadCmd()

	if cmd != nil {
		t.Fatal("expected no reload cmd when playsSinceRefresh < 5 and mixes loaded")
	}
}

// TestMixesViewEmptyState verifies that insufficientData renders the expected
// empty state message at list level.
func TestMixesViewEmptyState(t *testing.T) {
	m := newMixesModel(false, nil)
	m = m.switchToView(viewMixes)
	m = m.handleMixCountsLoadedMsg(mixCountsLoadedMsg{insufficient: true})

	rendered := m.mixes.render(styles.NewStyles(false), nil, nil, nil)

	if !strings.Contains(rendered, "minimum 20 plays") {
		t.Errorf("expected empty state with 20 plays hint, got: %q", rendered)
	}
	if !strings.Contains(rendered, "Personalized Mixes") {
		t.Errorf("expected 'Personalized Mixes' header in empty state, got: %q", rendered)
	}
}

// TestMixesViewEmptyTrackLevel verifies that an empty track list at level 1
// renders the "No tracks match yet" message.
func TestMixesViewEmptyTrackLevel(t *testing.T) {
	m := newMixesModel(true, nil)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: {}}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: nil})

	rendered := m.mixes.render(styles.NewStyles(false), nil, nil, nil)

	if !strings.Contains(rendered, "keep listening") {
		t.Errorf("expected 'keep listening' message for empty track level, got: %q", rendered)
	}
}

// TestMixesViewAsyncGuard verifies that a mixLoadedMsg for a different mixID
// does not overwrite the active track view.
func TestMixesViewAsyncGuard(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})
	// Now active mix is MixRediscover with 3 tracks.
	if len(m.mixes.trackView.sourceTracks) != 3 {
		t.Fatalf("precondition: expected 3 tracks, got %d", len(m.mixes.trackView.sourceTracks))
	}

	// Stale load arrives for a different mix.
	staleTracks := []library.Track{{Hash: "stale", Title: "Stale Track"}}
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixHiddenGems, tracks: staleTracks})

	// trackView must not have been replaced.
	if len(m.mixes.trackView.sourceTracks) != 3 {
		t.Errorf("stale mixLoadedMsg overwrote active track view: got %d tracks", len(m.mixes.trackView.sourceTracks))
	}
}

// TestMixesViewVisualModeExit verifies that switching to mixes exits visual mode.
func TestMixesViewVisualModeExit(t *testing.T) {
	m := newLibraryModel(5)
	m.visual.active = true

	m = m.switchToView(viewMixes)

	if m.visual.active {
		t.Error("expected visual mode to be exited when switching to mixes view")
	}
}

// TestMixesViewFilterAtTrackLevel verifies that the filter bar activates and
// filters tracks within the drill-down level.
func TestMixesViewFilterAtTrackLevel(t *testing.T) {
	tracks := []library.Track{
		{Hash: "mx1", Title: "Alpha Song", Artist: "ArtistX", Path: "/mx1.flac"},
		{Hash: "mx2", Title: "Beta Song", Artist: "ArtistX", Path: "/mx2.flac"},
		{Hash: "mx3", Title: "Gamma Track", Artist: "ArtistY", Path: "/mx3.flac"},
	}
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	// Press '/' to activate filter, then type "Alpha".
	m = pressSlash(m)
	m = typeText(m, "Alpha")

	// Verify the filter text was applied to the track-level view.
	if !strings.Contains(m.mixes.trackView.filter.Text, "Alpha") {
		t.Errorf("expected filter text to contain 'Alpha', got: %q", m.mixes.trackView.filter.Text)
	}
}

// TestMixesViewSortAtTrackLevel verifies that sort cycling changes the sort
// field within the drill-down track view.
func TestMixesViewSortAtTrackLevel(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	before := m.mixes.trackView.sortState.Field

	// Cycle sort.
	m = pressKey(m, "s")

	after := m.mixes.trackView.sortState.Field
	if after == before {
		t.Errorf("expected sort field to change after 's', still %v", after)
	}
}

// TestBuildMixSeed verifies that buildMixSeed computes the correct aggregate
// metadata from a set of tracks.
func TestBuildMixSeed(t *testing.T) {
	tracks := []library.Track{
		{Hash: "a", Artist: "ArtistX", Genre: "Rock", Duration: 180},
		{Hash: "b", Artist: "ArtistX", Genre: "Rock;Pop", Duration: 240},
		{Hash: "c", Artist: "ArtistY", Genre: "Pop", Duration: 120},
	}
	trackTags := map[string]map[int64]struct{}{
		"a": {1: {}, 2: {}},
		"b": {2: {}, 3: {}},
	}

	seed := buildMixSeed(tracks, trackTags)

	// ArtistX appears twice, ArtistY once.
	if seed.Artist != "ArtistX" {
		t.Errorf("expected top artist ArtistX, got %q", seed.Artist)
	}

	// Genres: Rock, Pop (union).
	genreSet := make(map[string]struct{}, len(seed.Genres))
	for _, g := range seed.Genres {
		genreSet[g] = struct{}{}
	}
	if _, ok := genreSet["Rock"]; !ok {
		t.Error("expected genre Rock in seed")
	}
	if _, ok := genreSet["Pop"]; !ok {
		t.Error("expected genre Pop in seed")
	}

	// Median of [120, 180, 240] is 180.
	if seed.Duration != 180 {
		t.Errorf("expected median duration 180, got %v", seed.Duration)
	}

	// Tags: union of {1,2} and {2,3} = {1,2,3}.
	for _, id := range []int64{1, 2, 3} {
		if _, ok := seed.TagIDs[id]; !ok {
			t.Errorf("expected tag ID %d in seed", id)
		}
	}

	// Hash must be empty (synthetic seed).
	if seed.Hash != "" {
		t.Errorf("expected empty Hash for synthetic seed, got %q", seed.Hash)
	}
}

// TestMixesRadioSeedEnable verifies that pressing A on a populated mix enables
// radio mode with a synthetic seed and a mix-specific notification.
func TestMixesRadioSeedEnable(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.mixes = append([]MixInfo(nil), mixDefs...)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	m2, _ := m.handleMixesRadioSeed()

	if !m2.radio.enabled {
		t.Fatal("expected radio to be enabled after handleMixesRadioSeed")
	}
	if m2.radio.syntheticSeed == nil {
		t.Fatal("expected syntheticSeed to be set")
	}
	if m2.radio.seedHash != "" {
		t.Errorf("expected seedHash to be empty for synthetic seed, got %q", m2.radio.seedHash)
	}
	if m2.statusBar.notification.msg == "" {
		t.Error("expected a status notification")
	}
}

// TestMixesRadioSeedDisable verifies that pressing A when radio is already
// enabled disables it and clears syntheticSeed.
func TestMixesRadioSeedDisable(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.mixes = append([]MixInfo(nil), mixDefs...)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	// Enable via mix seed.
	m, _ = m.handleMixesRadioSeed()
	if !m.radio.enabled {
		t.Fatal("precondition: radio should be enabled")
	}

	// Press A again — should disable.
	m2, _ := m.handleMixesRadioSeed()

	if m2.radio.enabled {
		t.Error("expected radio to be disabled after second A press")
	}
	if m2.radio.syntheticSeed != nil {
		t.Error("expected syntheticSeed to be cleared after disable")
	}
}

// TestMixesRadioSeedEmptyFallback verifies that pressing A on an empty mix
// falls back to a normal radio toggle.
func TestMixesRadioSeedEmptyFallback(t *testing.T) {
	m := newMixesModel(true, nil)
	m = m.switchToView(viewMixes)
	m.mixes.mixes = append([]MixInfo(nil), mixDefs...)
	// No cached tracks for the active mix.

	m2, _ := m.handleMixesRadioSeed()

	if !m2.radio.enabled {
		t.Error("expected fallback radioToggle to enable radio")
	}
	if m2.radio.syntheticSeed != nil {
		t.Error("expected syntheticSeed to remain nil in fallback path")
	}
}

// TestMixesViewHelpSection verifies that the help overlay includes a Mixes
// section with the expected context.
func TestMixesViewHelpSection(t *testing.T) {
	sections := helpSections()
	var found bool
	for _, s := range sections {
		if s.title == "Mixes" && s.context == contextMixes {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a 'Mixes' help section with contextMixes")
	}
}

// TestMixesViewFilteredHelp verifies that filteredHelpSections for contextMixes
// returns the Mixes section first.
func TestMixesViewFilteredHelp(t *testing.T) {
	sections := filteredHelpSections(contextMixes)
	if len(sections) == 0 {
		t.Fatal("expected at least one help section for contextMixes")
	}
	if sections[0].title != "Mixes" {
		t.Errorf("expected Mixes section first, got %q", sections[0].title)
	}
}
