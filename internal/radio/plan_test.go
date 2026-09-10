package radio

import (
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestCaptureSeed_SnapshotsMetadata(t *testing.T) {
	seed := library.Track{
		Hash:     "h1",
		Title:    "Roygbiv",
		Artist:   "Boards of Canada",
		Album:    "Music Has the Right to Children",
		Genre:    "Electronic/Ambient",
		Duration: 150,
	}
	snapshot := CaptureSeed(seed, map[int64]struct{}{5: {}, 2: {}}, FixedClock(testNow))

	if snapshot.TrackHash != "h1" || snapshot.Title != "Roygbiv" {
		t.Errorf("snapshot = %+v, want the track identity carried over", snapshot)
	}
	if len(snapshot.Genres) != 2 {
		t.Errorf("genres = %v, want the split genres", snapshot.Genres)
	}
	if len(snapshot.TagIDs) != 2 || snapshot.TagIDs[0] != 2 || snapshot.TagIDs[1] != 5 {
		t.Errorf("tag ids = %v, want [2 5] sorted for a stable snapshot", snapshot.TagIDs)
	}
	if !snapshot.CapturedAt.Equal(testNow) {
		t.Errorf("CapturedAt = %v, want the injected clock time", snapshot.CapturedAt)
	}
	if snapshot.IsZero() {
		t.Errorf("a snapshot with a hash is not empty")
	}
}

func TestSeedSnapshot_IsZero(t *testing.T) {
	if !(SeedSnapshot{}).IsZero() {
		t.Errorf("the zero snapshot must report empty")
	}
	if !(SeedSnapshot{TrackHash: "  ", Title: "Ghost"}).IsZero() {
		t.Errorf("a snapshot without a usable hash cannot be restored, so it is empty")
	}
}

func TestSeedSnapshot_SeedTrack_BridgesToScorer(t *testing.T) {
	snapshot := CaptureSeed(
		library.Track{Hash: "h1", Artist: "A", Album: "B", Genre: "Rock", Duration: 200},
		map[int64]struct{}{7: {}},
		FixedClock(testNow),
	)
	seed := snapshot.SeedTrack()
	if seed.Hash != "h1" || seed.Artist != "A" || seed.Album != "B" || seed.Duration != 200 {
		t.Errorf("SeedTrack = %+v, want the snapshot fields", seed)
	}
	if _, ok := seed.TagIDs[7]; !ok {
		t.Errorf("SeedTrack.TagIDs = %v, want tag 7", seed.TagIDs)
	}
	if len(seed.Genres) != 1 || seed.Genres[0] != "Rock" {
		t.Errorf("SeedTrack.Genres = %v, want [Rock]", seed.Genres)
	}

	// The bridge must work with the existing scorer.
	scored := ScoreAll(ScoreInput{
		Seed:      seed,
		AllTracks: []library.Track{{Hash: "h2", Artist: "A", Genre: "Rock"}},
	})
	if len(scored) != 1 || scored[0].Score != 15 {
		t.Errorf("scored = %+v, want genre+artist match = 15", scored)
	}
}

func TestSeedSnapshot_SeedTrackWithoutTags(t *testing.T) {
	seed := CaptureSeed(library.Track{Hash: "h1"}, nil, FixedClock(testNow)).SeedTrack()
	if seed.TagIDs != nil {
		t.Errorf("no tags must project to a nil map, got %v", seed.TagIDs)
	}
}

func TestSeedSnapshot_Label(t *testing.T) {
	cases := []struct {
		name     string
		snapshot SeedSnapshot
		want     string
	}{
		{"artist and title", SeedSnapshot{Artist: "A", Title: "T"}, "A — T"},
		{"title only", SeedSnapshot{Title: "T"}, "T"},
		{"artist only", SeedSnapshot{Artist: "A"}, "A"},
		{"neither", SeedSnapshot{TrackHash: "h1"}, "h1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.snapshot.Label(); got != tc.want {
				t.Errorf("Label() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRecipe_Validate(t *testing.T) {
	valid := Recipe{Name: "Late night", Settings: DefaultSettings()}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid recipe rejected: %v", err)
	}
	if err := (Recipe{Name: "   ", Settings: DefaultSettings()}).Validate(); !errors.Is(err, ErrEmptyRecipeName) {
		t.Errorf("blank name must fail with ErrEmptyRecipeName, got %v", err)
	}

	bad := valid
	bad.Settings.DiscoveryPercent = 45
	if err := bad.Validate(); !errors.Is(err, ErrDiscoveryStep) {
		t.Errorf("invalid settings must surface through the recipe, got %v", err)
	}
}

func TestRecipe_Normalized(t *testing.T) {
	recipe := Recipe{
		Name:     "  Late night  ",
		Settings: Settings{DiscoveryPercent: 47, Include: FilterSet{Genres: []string{"jazz", "Jazz"}}},
	}
	got := recipe.Normalized()
	if got.Name != "Late night" {
		t.Errorf("name = %q, want trimmed", got.Name)
	}
	if got.Settings.DiscoveryPercent != 50 {
		t.Errorf("discovery = %d, want snapped to 50", got.Settings.DiscoveryPercent)
	}
	if len(got.Settings.Include.Genres) != 1 {
		t.Errorf("duplicate genres must collapse: %v", got.Settings.Include.Genres)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("normalized recipe must validate: %v", err)
	}
}

func TestPlanRequest_Validate(t *testing.T) {
	seed := CaptureSeed(library.Track{Hash: "h1"}, nil, FixedClock(testNow))
	base := PlanRequest{Settings: DefaultSettings(), Seed: seed, BatchSize: 5, RequestedAt: testNow}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	noBatch := base
	noBatch.BatchSize = 0
	if err := noBatch.Validate(); !errors.Is(err, ErrEmptyBatch) {
		t.Errorf("zero batch size = %v, want ErrEmptyBatch", err)
	}

	noSeed := base
	noSeed.Seed = SeedSnapshot{}
	if err := noSeed.Validate(); !errors.Is(err, ErrMissingSeed) {
		t.Errorf("missing seed = %v, want ErrMissingSeed", err)
	}

	badSettings := base
	badSettings.Settings.Anchor = "sideways"
	if err := badSettings.Validate(); !errors.Is(err, ErrUnknownAnchorMode) {
		t.Errorf("invalid settings = %v, want ErrUnknownAnchorMode", err)
	}
}

func TestPlanRequest_DiscoveryTarget(t *testing.T) {
	seed := CaptureSeed(library.Track{Hash: "h1"}, nil, FixedClock(testNow))
	request := PlanRequest{Settings: DefaultSettings(), Seed: seed, BatchSize: 5}
	if got := request.DiscoveryTarget(); got != 2 {
		t.Errorf("balanced target over 5 tracks = %d, want 2", got)
	}
	explore, err := SettingsForPreset(PresetExplore)
	if err != nil {
		t.Fatalf("SettingsForPreset: %v", err)
	}
	request.Settings = explore
	if got := request.DiscoveryTarget(); got != 4 {
		t.Errorf("explore target over 5 tracks = %d, want 4", got)
	}
}

func TestPlanResult_DiscoveryAccounting(t *testing.T) {
	result := PlanResult{
		Outcome:         PlanOutcomeOK,
		DiscoveryTarget: 2,
		Tracks: []PlannedTrack{
			{Track: library.Track{Hash: "a"}, Discovery: DiscoveryNever},
			{Track: library.Track{Hash: "b"}, Discovery: DiscoveryStale},
			{Track: library.Track{Hash: "c"}, Discovery: DiscoveryFamiliar},
			{Track: library.Track{Hash: "d"}, Discovery: DiscoveryFamiliar},
		},
	}
	if got := result.DiscoveryCount(); got != 2 {
		t.Errorf("DiscoveryCount = %d, want 2", got)
	}
	if got := result.DiscoveryShare(); got != 50 {
		t.Errorf("DiscoveryShare = %d, want 50", got)
	}
	if !result.CanContinue() {
		t.Errorf("an ok plan with tracks must be enqueueable")
	}
	if got := (PlanResult{}).DiscoveryShare(); got != 0 {
		t.Errorf("empty plan share = %d, want 0", got)
	}
}

func TestPlanResult_DiscoveryShareRounds(t *testing.T) {
	// 1 of 3 discovery = 33.3% -> 33; 2 of 3 = 66.7% -> 67.
	third := PlanResult{Tracks: []PlannedTrack{
		{Discovery: DiscoveryNever}, {Discovery: DiscoveryFamiliar}, {Discovery: DiscoveryFamiliar},
	}}
	if got := third.DiscoveryShare(); got != 33 {
		t.Errorf("share = %d, want 33", got)
	}
	twoThirds := PlanResult{Tracks: []PlannedTrack{
		{Discovery: DiscoveryNever}, {Discovery: DiscoveryStale}, {Discovery: DiscoveryFamiliar},
	}}
	if got := twoThirds.DiscoveryShare(); got != 67 {
		t.Errorf("share = %d, want 67", got)
	}
}

func TestPlanResult_PausedOutcomes(t *testing.T) {
	blocked := BlockedPlan(Reason{Code: ReasonUnresolvedTag, Value: "7"})
	if blocked.Outcome != PlanOutcomeBlocked || blocked.CanContinue() {
		t.Errorf("blocked plans must pause additions: %+v", blocked)
	}
	if !blocked.Reasons.Has(ReasonUnresolvedTag) {
		t.Errorf("blocked plan must explain itself: %v", blocked.Reasons.Codes())
	}

	empty := EmptyPlan(2, Reason{Code: ReasonNoCandidates, Detail: "filters matched nothing"})
	if empty.Outcome != PlanOutcomeEmpty || empty.CanContinue() {
		t.Errorf("empty plans must pause additions: %+v", empty)
	}
	if empty.DiscoveryTarget != 2 {
		t.Errorf("empty plan must keep the requested target, got %d", empty.DiscoveryTarget)
	}

	relaxed := PlanResult{
		Outcome: PlanOutcomeRelaxed,
		Tracks:  []PlannedTrack{{Discovery: DiscoveryFamiliar}},
		Reasons: Reasons{{Code: ReasonDiscoveryRelaxed}},
	}
	if !relaxed.CanContinue() {
		t.Errorf("a relaxed plan with tracks may still be enqueued")
	}
	if (PlanResult{Outcome: PlanOutcomeOK}).CanContinue() {
		t.Errorf("an ok plan with no tracks has nothing to enqueue")
	}
}
