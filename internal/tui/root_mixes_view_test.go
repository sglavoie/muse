package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestMixesViewSwitchLazyLoad(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	if len(m.mixes.mixes) != 0 {
		t.Fatal("precondition: expected empty mixes before first switch")
	}

	cmd := m.mixesMaybeLoadCmd()
	if cmd == nil {
		t.Fatal("expected loadMixCountsCmd when mixes not yet initialized")
	}

	m = m.switchToView(viewMixes)
	if len(m.mixes.mixes) == 0 {
		t.Fatal("expected mixes to be populated after switchToView")
	}
}

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

func TestMixesViewDrillDown(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	m = m.switchToView(viewMixes)
	m.mixes.selectedMix = 0

	m2, cmd := m.handleMixOpen()

	if m2.mixes.level != 1 {
		t.Fatalf("expected level 1 after drill-down, got %d", m2.mixes.level)
	}
	if cmd == nil {
		t.Fatal("expected loadMixCmd to be returned on drill-down")
	}
}

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

func TestMixesViewTrackEnqueue(t *testing.T) {
	tracks := sampleMixTracks()
	qa := &stubQueueAppender{}
	m := New(context.Background(),
		WithRecommendationReader(&stubRecReader{sufficient: true, tracks: tracks}),
		WithQueueWriter(qa),
	)
	m = m.switchToView(viewMixes)
	m.mixes.mixes = append([]MixInfo(nil), recommendation.Registry()...)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	_, cmd, handled := m.handleLibraryEnqueueAction()
	if !handled {
		t.Fatal("expected enqueue action to be handled for viewMixes at level 1")
	}
	if cmd == nil {
		t.Fatal("expected non-nil enqueue cmd")
	}
	cmd()

	if len(qa.tracks) == 0 {
		t.Fatal("expected track to be enqueued after cmd execution, got 0")
	}
	if qa.tracks[0].Hash != "mx1" {
		t.Errorf("expected hash mx1, got %q", qa.tracks[0].Hash)
	}
}

func TestMixesViewBulkSelect(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})
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

func TestMixesViewBulkSelectAtListLevel(t *testing.T) {
	m := newMixesModel(true, nil)
	m = m.switchToView(viewMixes)

	hashes := m.bulkSelectedHashes()
	if hashes != nil {
		t.Errorf("expected nil at list level, got %v", hashes)
	}
}

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

func TestMixesViewCacheInvalidation(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	m = m.switchToView(viewMixes)
	m = seedMixRegistry(m)
	m.mixes.playsSinceRefresh = 5

	cmd := m.mixesMaybeLoadCmd()

	if cmd == nil {
		t.Fatal("expected reload cmd when playsSinceRefresh >= 5")
	}
}

func TestMixesViewNoReloadBelowThreshold(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	m = m.switchToView(viewMixes)
	m = seedMixRegistry(m)
	m.mixes.playsSinceRefresh = 4

	cmd := m.mixesMaybeLoadCmd()

	if cmd != nil {
		t.Fatal("expected no reload cmd when playsSinceRefresh < 5 and mixes loaded")
	}
}

func TestMixesViewEmptyState(t *testing.T) {
	m := newMixesModel(false, nil)
	m = m.switchToView(viewMixes)
	m = m.handleMixCountsLoadedMsg(mixCountsLoadedMsg{insufficient: true})

	rendered := m.mixes.render(styles.NewStyles(false), nil, nil, nil, "", "")

	if !strings.Contains(rendered, "minimum 20 plays") {
		t.Errorf("expected empty state with 20 plays hint, got: %q", rendered)
	}
	if !strings.Contains(rendered, "Personalized Mixes") {
		t.Errorf("expected 'Personalized Mixes' header in empty state, got: %q", rendered)
	}
}

func TestMixesViewEmptyTrackLevel(t *testing.T) {
	m := newMixesModel(true, nil)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: {}}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: nil})

	rendered := m.mixes.render(styles.NewStyles(false), nil, nil, nil, "", "")

	if !strings.Contains(rendered, "keep listening") {
		t.Errorf("expected 'keep listening' message for empty track level, got: %q", rendered)
	}
}

func TestMixLoadedFiltersDismissedHashesLikeRecommendationPackage(t *testing.T) {
	tracks := []library.Track{
		{Hash: "abc", Title: "Dismissed"},
		{Hash: "", Title: "Unhashed"},
		{Hash: "def", Title: "Kept"},
	}
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.activeMix = db.MixRediscover
	m.mixes.level = 1
	paddedDismissedHash := " ABC "
	m.dismissedHashes = map[string]struct{}{
		paddedDismissedHash: {},
		"":                  {},
	}

	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})
	cached := m.mixes.cachedMixes[db.MixRediscover]

	if len(cached) != 2 {
		t.Fatalf("expected 2 tracks after dismissed filtering, got %d: %+v", len(cached), cached)
	}
	if cached[0].Hash != "" || cached[0].Title != "Unhashed" {
		t.Fatalf("empty-hash track should be preserved, got %+v", cached[0])
	}
	if cached[1].Hash != "def" {
		t.Fatalf("expected non-dismissed hash def to remain, got %+v", cached[1])
	}
}

func TestMixesViewAsyncGuard(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})
	if len(m.mixes.trackView.sourceTracks) != 3 {
		t.Fatalf("precondition: expected 3 tracks, got %d", len(m.mixes.trackView.sourceTracks))
	}

	staleTracks := []library.Track{{Hash: "stale", Title: "Stale Track"}}
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixHiddenGems, tracks: staleTracks})

	if len(m.mixes.trackView.sourceTracks) != 3 {
		t.Errorf("stale mixLoadedMsg overwrote active track view: got %d tracks", len(m.mixes.trackView.sourceTracks))
	}
}

func TestMixesViewVisualModeExit(t *testing.T) {
	m := newLibraryModel(5)
	m.visual.active = true

	m = m.switchToView(viewMixes)

	if m.visual.active {
		t.Error("expected visual mode to be exited when switching to mixes view")
	}
}

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

	m = pressSlash(m)
	m = typeText(m, "Alpha")

	if !strings.Contains(m.mixes.trackView.filter.Text, "Alpha") {
		t.Errorf("expected filter text to contain 'Alpha', got: %q", m.mixes.trackView.filter.Text)
	}
}

func TestMixesViewSortAtTrackLevel(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	before := m.mixes.trackView.sortState.Field
	m = pressKey(m, "s")
	after := m.mixes.trackView.sortState.Field
	if after == before {
		t.Errorf("expected sort field to change after 's', still %v", after)
	}
}

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

func TestMixesViewFilteredHelp(t *testing.T) {
	sections := filteredHelpSections(contextMixes)
	// The status-bar legend is pinned ahead of everything; the view-specific
	// section still leads the rest, before any global section.
	if len(sections) < 2 {
		t.Fatalf("expected at least two help sections for contextMixes, got %d", len(sections))
	}
	if sections[0].title != statusBarHelpSectionTitle {
		t.Errorf("expected the pinned %q section first, got %q", statusBarHelpSectionTitle, sections[0].title)
	}
	if sections[1].title != "Mixes" {
		t.Errorf("expected Mixes section after the pinned legend, got %q", sections[1].title)
	}
}

func TestMixesReloadPreservesCursorAndSort(t *testing.T) {
	tracks := []library.Track{
		{Hash: "mx1", Title: "Alpha", Artist: "ArtistX", Path: "/mx1.flac", Duration: 180},
		{Hash: "mx2", Title: "Beta", Artist: "ArtistX", Path: "/mx2.flac", Duration: 120},
		{Hash: "mx3", Title: "Gamma", Artist: "ArtistY", Path: "/mx3.flac", Duration: 240},
	}
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	m.mixes.trackView.cursor = 1
	m.mixes.trackView.sortState = sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortDesc}
	m.mixes.trackView = m.mixes.trackView.recomputeSortedTracks()

	reloaded := []library.Track{
		{Hash: "mx1", Title: "Alpha Reloaded", Artist: "ArtistX", Path: "/mx1.flac", Duration: 180},
		{Hash: "mx2", Title: "Beta Reloaded", Artist: "ArtistX", Path: "/mx2.flac", Duration: 120},
		{Hash: "mx3", Title: "Gamma Reloaded", Artist: "ArtistY", Path: "/mx3.flac", Duration: 240},
	}
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: reloaded})

	if m.mixes.trackView.sortState.Field != sorting.SortDuration {
		t.Errorf("sort field reset: got %v, want SortDuration", m.mixes.trackView.sortState.Field)
	}
	if m.mixes.trackView.sortState.Dir != sorting.SortDesc {
		t.Errorf("sort dir reset: got %v, want SortDesc", m.mixes.trackView.sortState.Dir)
	}
	if m.mixes.trackView.cursor != 1 {
		t.Errorf("cursor changed: got %d, want 1", m.mixes.trackView.cursor)
	}
	if len(m.mixes.trackView.sourceTracks) != 3 {
		t.Fatalf("sourceTracks length: got %d, want 3", len(m.mixes.trackView.sourceTracks))
	}
	if m.mixes.trackView.sourceTracks[0].Title != "Alpha Reloaded" {
		t.Errorf("expected reloaded title, got %q", m.mixes.trackView.sourceTracks[0].Title)
	}
}
