package radio

import (
	"errors"
	"fmt"
)

// Discovery target bounds and the increment used when tuning it. The UI only
// ever offers ten-point steps, so the domain refuses values in between rather
// than silently accepting settings the pickers cannot reproduce.
const (
	MinDiscoveryPercent = 0
	MaxDiscoveryPercent = 100
	DiscoveryStep       = 10
)

// Preset names a stock discovery target.
type Preset string

const (
	PresetFamiliar Preset = "familiar"
	PresetBalanced Preset = "balanced"
	PresetExplore  Preset = "explore"
	// PresetCustom is reported for any target that is not a stock value. It is
	// never returned by Presets and has no discovery target of its own.
	PresetCustom Preset = "custom"
)

// DefaultPreset is the preset new DJ sessions start from.
const DefaultPreset = PresetBalanced

// AnchorMode decides which track the DJ scores candidates against.
type AnchorMode string

const (
	// AnchorSeed keeps scoring against the seed captured when the session
	// started.
	AnchorSeed AnchorMode = "anchor"
	// AnchorFollow re-anchors on every confirmed track change.
	AnchorFollow AnchorMode = "follow"
)

// DefaultAnchorMode is the anchor mode new DJ sessions start from.
const DefaultAnchorMode = AnchorSeed

// Domain errors. Callers match them with errors.Is; the wrapping errors carry
// the offending value in their message.
var (
	ErrDiscoveryOutOfRange = errors.New("radio: discovery target out of range")
	ErrDiscoveryStep       = errors.New("radio: discovery target must be a multiple of 10")
	ErrUnknownAnchorMode   = errors.New("radio: unknown anchor mode")
	ErrInvalidTagID        = errors.New("radio: tag id must be positive")
	ErrUnresolvedTag       = errors.New("radio: unresolved tag reference")
	ErrEmptyRecipeName     = errors.New("radio: recipe name is empty")
	ErrEmptyBatch          = errors.New("radio: batch size must be positive")
	ErrMissingSeed         = errors.New("radio: seed snapshot is empty")
)

// presetTargets holds the agreed stock discovery targets.
var presetTargets = map[Preset]int{
	PresetFamiliar: 20,
	PresetBalanced: 40,
	PresetExplore:  80,
}

// Presets returns the selectable presets in display order.
func Presets() []Preset {
	return []Preset{PresetFamiliar, PresetBalanced, PresetExplore}
}

// DiscoveryPercent returns the stock discovery target for p. The second result
// is false for PresetCustom and for unknown values.
func (p Preset) DiscoveryPercent() (int, bool) {
	pct, ok := presetTargets[p]
	return pct, ok
}

// Label returns the human-facing name of the preset.
func (p Preset) Label() string {
	switch p {
	case PresetFamiliar:
		return "Familiar"
	case PresetBalanced:
		return "Balanced"
	case PresetExplore:
		return "Explore"
	case PresetCustom:
		return "Custom"
	default:
		return string(p)
	}
}

// PresetFor returns the preset whose stock target equals pct, or PresetCustom
// when the target has been tuned away from a stock value.
func PresetFor(pct int) Preset {
	for _, preset := range Presets() {
		if target, ok := preset.DiscoveryPercent(); ok && target == pct {
			return preset
		}
	}
	return PresetCustom
}

// ClampDiscoveryPercent constrains pct to the valid range without snapping it
// to a step boundary.
func ClampDiscoveryPercent(pct int) int {
	if pct < MinDiscoveryPercent {
		return MinDiscoveryPercent
	}
	if pct > MaxDiscoveryPercent {
		return MaxDiscoveryPercent
	}
	return pct
}

// SnapDiscoveryPercent clamps pct into range and rounds it to the nearest
// ten-point increment; exact halves round up.
func SnapDiscoveryPercent(pct int) int {
	clamped := ClampDiscoveryPercent(pct)
	snapped := ((clamped + DiscoveryStep/2) / DiscoveryStep) * DiscoveryStep
	return ClampDiscoveryPercent(snapped)
}

// Settings is the full set of tunable DJ options. It is a value type: every
// mutator returns a copy so draft edits never alias the applied settings.
type Settings struct {
	DiscoveryPercent int
	Anchor           AnchorMode
	Include          FilterSet
	Exclude          FilterSet
}

// DefaultSettings returns the settings a fresh DJ session starts with.
func DefaultSettings() Settings {
	pct, _ := DefaultPreset.DiscoveryPercent()
	return Settings{DiscoveryPercent: pct, Anchor: DefaultAnchorMode}
}

// SettingsForPreset returns default settings with p's stock discovery target.
// Filters are intentionally left empty: presets tune discovery only.
func SettingsForPreset(p Preset) (Settings, error) {
	pct, ok := p.DiscoveryPercent()
	if !ok {
		return Settings{}, fmt.Errorf("radio: preset %q has no stock discovery target", string(p))
	}
	settings := DefaultSettings()
	settings.DiscoveryPercent = pct
	return settings, nil
}

// Preset reports the preset matching the current discovery target.
func (s Settings) Preset() Preset {
	return PresetFor(s.DiscoveryPercent)
}

// Validate reports the first problem that would make these settings
// unusable. It deliberately runs against the raw values so callers can surface
// a non-positive tag id instead of having Normalized quietly drop it.
func (s Settings) Validate() error {
	if s.DiscoveryPercent < MinDiscoveryPercent || s.DiscoveryPercent > MaxDiscoveryPercent {
		return fmt.Errorf("%w: %d", ErrDiscoveryOutOfRange, s.DiscoveryPercent)
	}
	if s.DiscoveryPercent%DiscoveryStep != 0 {
		return fmt.Errorf("%w: %d", ErrDiscoveryStep, s.DiscoveryPercent)
	}
	if !s.Anchor.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownAnchorMode, string(s.Anchor))
	}
	if err := s.Include.Validate(); err != nil {
		return fmt.Errorf("include filters: %w", err)
	}
	if err := s.Exclude.Validate(); err != nil {
		return fmt.Errorf("exclude filters: %w", err)
	}
	return nil
}

// Valid reports whether m is a known anchor mode.
func (m AnchorMode) Valid() bool {
	return m == AnchorSeed || m == AnchorFollow
}

// Label returns the human-facing name of the anchor mode.
func (m AnchorMode) Label() string {
	switch m {
	case AnchorSeed:
		return "Anchor"
	case AnchorFollow:
		return "Follow playback"
	default:
		return string(m)
	}
}

// Normalized returns an equivalent Settings with a canonical anchor mode, a
// snapped discovery target, and normalized filter sets.
func (s Settings) Normalized() Settings {
	normalized := Settings{
		DiscoveryPercent: SnapDiscoveryPercent(s.DiscoveryPercent),
		Anchor:           s.Anchor,
		Include:          s.Include.Normalized(),
		Exclude:          s.Exclude.Normalized(),
	}
	if !normalized.Anchor.Valid() {
		normalized.Anchor = DefaultAnchorMode
	}
	return normalized
}

// WithDiscoveryPercent returns a copy whose discovery target is pct, snapped
// to the nearest ten-point increment.
func (s Settings) WithDiscoveryPercent(pct int) Settings {
	s.DiscoveryPercent = SnapDiscoveryPercent(pct)
	return s
}

// TuneDiscovery returns a copy moved by steps ten-point increments, clamped to
// the valid range. Negative steps lower the discovery target.
func (s Settings) TuneDiscovery(steps int) Settings {
	base := SnapDiscoveryPercent(s.DiscoveryPercent)
	return s.WithDiscoveryPercent(base + steps*DiscoveryStep)
}

// WithPreset returns a copy carrying p's stock discovery target, leaving the
// filters and anchor mode untouched.
func (s Settings) WithPreset(p Preset) (Settings, error) {
	pct, ok := p.DiscoveryPercent()
	if !ok {
		return s, fmt.Errorf("radio: preset %q has no stock discovery target", string(p))
	}
	s.DiscoveryPercent = pct
	return s, nil
}

// Equal reports whether s and other are semantically equal. Both sides are
// normalized first so re-ordered or differently-cased filter values do not
// register as an unapplied draft edit.
func (s Settings) Equal(other Settings) bool {
	left, right := s.Normalized(), other.Normalized()
	return left.DiscoveryPercent == right.DiscoveryPercent &&
		left.Anchor == right.Anchor &&
		left.Include.Equal(right.Include) &&
		left.Exclude.Equal(right.Exclude)
}

// ResolveTags reports unresolved tag references across both filter sets.
// Unresolved references are blocking: a stale tag id silently widens an
// exclusion or narrows an inclusion, so the caller must repair the settings
// rather than plan around them.
func (s Settings) ResolveTags(catalog TagCatalog) error {
	return resolveTagIDs(catalog, s.Include.TagIDs, s.Exclude.TagIDs)
}

// Match evaluates facts against the include/exclude filters. Use Compile
// instead when evaluating more than a handful of candidates.
func (s Settings) Match(facts TrackFacts) FilterVerdict {
	return MatchFilters(s.Include, s.Exclude, facts)
}

// Compile prepares the filters for repeated matching.
func (s Settings) Compile() CompiledFilters {
	return CompileFilters(s.Include, s.Exclude)
}

// Conflicts returns the values listed in both the inclusion and the exclusion
// filters. Exclusions win, so every conflicting value is unsatisfiable and the
// UI should warn about it.
func (s Settings) Conflicts() FilterSet {
	return Conflicts(s.Include, s.Exclude)
}
