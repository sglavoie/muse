package radio

import (
	"errors"
	"testing"
)

func TestPresets_AgreedDiscoveryTargets(t *testing.T) {
	want := map[Preset]int{
		PresetFamiliar: 20,
		PresetBalanced: 40,
		PresetExplore:  80,
	}
	for preset, expected := range want {
		got, ok := preset.DiscoveryPercent()
		if !ok {
			t.Fatalf("preset %q has no discovery target", preset)
		}
		if got != expected {
			t.Errorf("preset %q: got %d%%, want %d%%", preset, got, expected)
		}
	}
	if _, ok := PresetCustom.DiscoveryPercent(); ok {
		t.Errorf("PresetCustom must not report a stock discovery target")
	}
	if len(Presets()) != 3 {
		t.Errorf("expected 3 selectable presets, got %d", len(Presets()))
	}
}

func TestDefaultSettings_IsBalancedAndAnchored(t *testing.T) {
	settings := DefaultSettings()
	if settings.Preset() != PresetBalanced {
		t.Errorf("default preset = %q, want balanced", settings.Preset())
	}
	if settings.DiscoveryPercent != 40 {
		t.Errorf("default discovery = %d, want 40", settings.DiscoveryPercent)
	}
	if settings.Anchor != AnchorSeed {
		t.Errorf("default anchor = %q, want %q", settings.Anchor, AnchorSeed)
	}
	if err := settings.Validate(); err != nil {
		t.Errorf("default settings must validate: %v", err)
	}
}

func TestPresetFor_TunedTargetIsCustom(t *testing.T) {
	if got := PresetFor(40); got != PresetBalanced {
		t.Errorf("PresetFor(40) = %q, want balanced", got)
	}
	if got := PresetFor(50); got != PresetCustom {
		t.Errorf("PresetFor(50) = %q, want custom", got)
	}
}

func TestSettingsForPreset(t *testing.T) {
	settings, err := SettingsForPreset(PresetExplore)
	if err != nil {
		t.Fatalf("SettingsForPreset(explore): %v", err)
	}
	if settings.DiscoveryPercent != 80 {
		t.Errorf("explore discovery = %d, want 80", settings.DiscoveryPercent)
	}
	if _, err := SettingsForPreset(PresetCustom); err == nil {
		t.Errorf("SettingsForPreset(custom) must fail")
	}
}

func TestWithPreset_PreservesFilters(t *testing.T) {
	base := DefaultSettings()
	base.Include = FilterSet{Genres: []string{"Jazz"}}
	base.Anchor = AnchorFollow

	tuned, err := base.WithPreset(PresetFamiliar)
	if err != nil {
		t.Fatalf("WithPreset: %v", err)
	}
	if tuned.DiscoveryPercent != 20 {
		t.Errorf("discovery = %d, want 20", tuned.DiscoveryPercent)
	}
	if tuned.Anchor != AnchorFollow {
		t.Errorf("anchor mode must survive a preset change")
	}
	if len(tuned.Include.Genres) != 1 || tuned.Include.Genres[0] != "Jazz" {
		t.Errorf("include filters must survive a preset change: %+v", tuned.Include)
	}
	if base.DiscoveryPercent != 40 {
		t.Errorf("WithPreset must not mutate the receiver: %d", base.DiscoveryPercent)
	}
}

func TestSnapDiscoveryPercent(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{-30, 0},
		{0, 0},
		{4, 0},
		{5, 10},
		{44, 40},
		{45, 50},
		{95, 100},
		{100, 100},
		{130, 100},
	}
	for _, tc := range cases {
		if got := SnapDiscoveryPercent(tc.in); got != tc.want {
			t.Errorf("SnapDiscoveryPercent(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestTuneDiscovery_TenPointIncrementsAndClamping(t *testing.T) {
	settings := DefaultSettings()

	up := settings.TuneDiscovery(1)
	if up.DiscoveryPercent != 50 {
		t.Errorf("one step up = %d, want 50", up.DiscoveryPercent)
	}
	down := settings.TuneDiscovery(-2)
	if down.DiscoveryPercent != 20 {
		t.Errorf("two steps down = %d, want 20", down.DiscoveryPercent)
	}
	if got := settings.TuneDiscovery(99).DiscoveryPercent; got != 100 {
		t.Errorf("upper clamp = %d, want 100", got)
	}
	if got := settings.TuneDiscovery(-99).DiscoveryPercent; got != 0 {
		t.Errorf("lower clamp = %d, want 0", got)
	}
	if settings.DiscoveryPercent != 40 {
		t.Errorf("TuneDiscovery must not mutate the receiver: %d", settings.DiscoveryPercent)
	}
}

func TestSettings_Validate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Settings)
		wantErr error
	}{
		{"negative discovery", func(s *Settings) { s.DiscoveryPercent = -10 }, ErrDiscoveryOutOfRange},
		{"above range", func(s *Settings) { s.DiscoveryPercent = 110 }, ErrDiscoveryOutOfRange},
		{"off-step discovery", func(s *Settings) { s.DiscoveryPercent = 45 }, ErrDiscoveryStep},
		{"unknown anchor", func(s *Settings) { s.Anchor = "sideways" }, ErrUnknownAnchorMode},
		{"empty anchor", func(s *Settings) { s.Anchor = "" }, ErrUnknownAnchorMode},
		{"bad include tag", func(s *Settings) { s.Include.TagIDs = []int64{0} }, ErrInvalidTagID},
		{"bad exclude tag", func(s *Settings) { s.Exclude.TagIDs = []int64{-3} }, ErrInvalidTagID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultSettings()
			tc.mutate(&settings)
			err := settings.Validate()
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestSettings_ValidateAcceptsEveryStep(t *testing.T) {
	settings := DefaultSettings()
	for pct := MinDiscoveryPercent; pct <= MaxDiscoveryPercent; pct += DiscoveryStep {
		settings.DiscoveryPercent = pct
		if err := settings.Validate(); err != nil {
			t.Errorf("discovery %d%% must validate: %v", pct, err)
		}
	}
}

func TestSettings_Normalized(t *testing.T) {
	settings := Settings{
		DiscoveryPercent: 47,
		Anchor:           "bogus",
		Include:          FilterSet{Genres: []string{" rock ", "Rock"}, TagIDs: []int64{3, 3, -1}},
	}
	normalized := settings.Normalized()
	if normalized.DiscoveryPercent != 50 {
		t.Errorf("discovery = %d, want 50", normalized.DiscoveryPercent)
	}
	if normalized.Anchor != DefaultAnchorMode {
		t.Errorf("unknown anchor must normalize to the default, got %q", normalized.Anchor)
	}
	if len(normalized.Include.Genres) != 1 || normalized.Include.Genres[0] != "rock" {
		t.Errorf("duplicate genres must collapse to the first spelling: %+v", normalized.Include.Genres)
	}
	if len(normalized.Include.TagIDs) != 1 || normalized.Include.TagIDs[0] != 3 {
		t.Errorf("tag ids = %v, want [3]", normalized.Include.TagIDs)
	}
	if err := normalized.Validate(); err != nil {
		t.Errorf("normalized settings must validate: %v", err)
	}
}

func TestSettings_Equal_IgnoresOrderAndCase(t *testing.T) {
	left := Settings{
		DiscoveryPercent: 40,
		Anchor:           AnchorSeed,
		Include:          FilterSet{Genres: []string{"Rock", "jazz"}, TagIDs: []int64{2, 1}},
	}
	right := Settings{
		DiscoveryPercent: 40,
		Anchor:           AnchorSeed,
		Include:          FilterSet{Genres: []string{"JAZZ", "rock"}, TagIDs: []int64{1, 2, 2}},
	}
	if !left.Equal(right) {
		t.Errorf("re-ordered, re-cased filters must compare equal")
	}

	right.Anchor = AnchorFollow
	if left.Equal(right) {
		t.Errorf("different anchor modes must not compare equal")
	}

	right = left
	right.Exclude = FilterSet{Artists: []string{"Nobody"}}
	if left.Equal(right) {
		t.Errorf("an added exclusion must not compare equal")
	}
}

func TestSettings_ResolveTags(t *testing.T) {
	catalog := TagIDSet{1: {}, 2: {}}
	settings := DefaultSettings()
	settings.Include.TagIDs = []int64{1, 7}
	settings.Exclude.TagIDs = []int64{2, 5, 7}

	err := settings.ResolveTags(catalog)
	if !errors.Is(err, ErrUnresolvedTag) {
		t.Fatalf("ResolveTags() = %v, want ErrUnresolvedTag", err)
	}
	var unresolved *UnresolvedTagError
	if !errors.As(err, &unresolved) {
		t.Fatalf("error must expose the unresolved ids: %v", err)
	}
	if len(unresolved.TagIDs) != 2 || unresolved.TagIDs[0] != 5 || unresolved.TagIDs[1] != 7 {
		t.Errorf("unresolved ids = %v, want [5 7] deduplicated and sorted", unresolved.TagIDs)
	}

	settings.Include.TagIDs = []int64{1}
	settings.Exclude.TagIDs = []int64{2}
	if err := settings.ResolveTags(catalog); err != nil {
		t.Errorf("known tags must resolve: %v", err)
	}
}

func TestSettings_ResolveTags_NilCatalogResolvesNothing(t *testing.T) {
	settings := DefaultSettings()
	settings.Include.TagIDs = []int64{1}
	if err := settings.ResolveTags(nil); !errors.Is(err, ErrUnresolvedTag) {
		t.Errorf("a nil catalog must report references as unresolved, got %v", err)
	}
	if err := DefaultSettings().ResolveTags(nil); err != nil {
		t.Errorf("settings without tag references must resolve even without a catalog: %v", err)
	}
}

func TestAnchorMode_Labels(t *testing.T) {
	if !AnchorSeed.Valid() || !AnchorFollow.Valid() {
		t.Errorf("both anchor modes must be valid")
	}
	if AnchorMode("nope").Valid() {
		t.Errorf("unknown anchor mode must be invalid")
	}
	if AnchorSeed.Label() == "" || AnchorFollow.Label() == "" {
		t.Errorf("anchor modes need display labels")
	}
	if PresetBalanced.Label() != "Balanced" {
		t.Errorf("preset label = %q, want Balanced", PresetBalanced.Label())
	}
}
