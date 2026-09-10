package radio

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// SeedSnapshot is the frozen metadata of the track a DJ session was seeded
// from. A new session captures a fresh snapshot; restarting Muse restores the
// active session's snapshot instead of re-reading the library, so the seed
// survives metadata edits and rescans.
type SeedSnapshot struct {
	TrackHash  string
	Title      string
	Artist     string
	Album      string
	Genres     []string
	TagIDs     []int64
	Duration   float64
	CapturedAt time.Time
}

// CaptureSeed snapshots a library track and its resolved tag ids.
func CaptureSeed(t library.Track, tagIDs map[int64]struct{}, clock Clock) SeedSnapshot {
	ids := make([]int64, 0, len(tagIDs))
	for id := range tagIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) == 0 {
		ids = nil
	}
	return SeedSnapshot{
		TrackHash:  t.Hash,
		Title:      t.Title,
		Artist:     t.Artist,
		Album:      t.Album,
		Genres:     library.SplitGenres(t.Genre),
		TagIDs:     ids,
		Duration:   t.Duration,
		CapturedAt: nowFrom(clock),
	}
}

// IsZero reports whether the snapshot identifies no track. A snapshot without
// a hash cannot be restored across restarts, so it counts as empty even when
// other fields are set.
func (s SeedSnapshot) IsZero() bool {
	return strings.TrimSpace(s.TrackHash) == ""
}

// SeedTrack projects the snapshot onto the scorer's seed type.
func (s SeedSnapshot) SeedTrack() SeedTrack {
	var tagIDs map[int64]struct{}
	if len(s.TagIDs) > 0 {
		tagIDs = make(map[int64]struct{}, len(s.TagIDs))
		for _, id := range s.TagIDs {
			tagIDs[id] = struct{}{}
		}
	}
	return SeedTrack{
		Hash:     s.TrackHash,
		Artist:   s.Artist,
		Album:    s.Album,
		Genres:   s.Genres,
		Duration: s.Duration,
		TagIDs:   tagIDs,
	}
}

// Label renders the seed for display, falling back to the hash when the track
// carries no title.
func (s SeedSnapshot) Label() string {
	title := strings.TrimSpace(s.Title)
	artist := strings.TrimSpace(s.Artist)
	switch {
	case title != "" && artist != "":
		return artist + " — " + title
	case title != "":
		return title
	case artist != "":
		return artist
	default:
		return s.TrackHash
	}
}

// Recipe is a named, saved set of DJ options.
type Recipe struct {
	ID        int64
	Name      string
	Settings  Settings
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Normalized returns a copy with a trimmed name and normalized settings.
func (r Recipe) Normalized() Recipe {
	r.Name = strings.TrimSpace(r.Name)
	r.Settings = r.Settings.Normalized()
	return r
}

// Validate reports whether the recipe can be saved. Uniqueness of names is a
// storage concern and is not checked here.
func (r Recipe) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return ErrEmptyRecipeName
	}
	if err := r.Settings.Validate(); err != nil {
		return fmt.Errorf("recipe %q: %w", r.Name, err)
	}
	return nil
}

// PlanRequest is the input contract for planning one DJ batch. Later sessions
// implement the planner against this type; the domain only validates it.
type PlanRequest struct {
	Settings  Settings
	Seed      SeedSnapshot
	BatchSize int
	// RequestedAt is the planner's view of "now", supplied from an injected
	// Clock so discovery classification is reproducible in tests.
	RequestedAt time.Time
}

// Validate reports the first problem that would make the request unplannable.
func (r PlanRequest) Validate() error {
	if err := r.Settings.Validate(); err != nil {
		return err
	}
	if r.BatchSize <= 0 {
		return fmt.Errorf("%w: %d", ErrEmptyBatch, r.BatchSize)
	}
	if r.Seed.IsZero() {
		return ErrMissingSeed
	}
	return nil
}

// DiscoveryTarget returns how many tracks of this batch should be discovery.
func (r PlanRequest) DiscoveryTarget() int {
	return DiscoveryTargetCount(r.BatchSize, r.Settings.DiscoveryPercent)
}

// PlannedTrack is one DJ-owned selection with the evidence behind it.
type PlannedTrack struct {
	Track     library.Track
	Score     float64
	Discovery DiscoveryClass
	Reasons   Reasons
}

// PlanOutcome summarizes how a planning attempt ended.
type PlanOutcome string

const (
	// PlanOutcomeOK means the batch met its discovery target.
	PlanOutcomeOK PlanOutcome = "ok"
	// PlanOutcomeRelaxed means the discovery target was relaxed to fill the
	// batch. Explicit filters are never relaxed.
	PlanOutcomeRelaxed PlanOutcome = "relaxed"
	// PlanOutcomeEmpty means no candidate survived the filters; additions
	// pause with an explanation instead of falling back to the whole library.
	PlanOutcomeEmpty PlanOutcome = "empty"
	// PlanOutcomeBlocked means the settings could not be evaluated at all
	// (invalid values or unresolved tag references).
	PlanOutcomeBlocked PlanOutcome = "blocked"
)

// PlanResult is the output contract for one planning attempt.
type PlanResult struct {
	Tracks          []PlannedTrack
	Outcome         PlanOutcome
	DiscoveryTarget int
	// Reasons explains a relaxed, empty, or blocked outcome. Empty for
	// PlanOutcomeOK.
	Reasons Reasons
}

// DiscoveryCount returns how many planned tracks are discovery selections.
func (p PlanResult) DiscoveryCount() int {
	count := 0
	for _, t := range p.Tracks {
		if t.Discovery.IsDiscovery() {
			count++
		}
	}
	return count
}

// DiscoveryShare returns the achieved discovery share as a percentage, rounded
// to the nearest point. Zero when the plan is empty.
func (p PlanResult) DiscoveryShare() int {
	if len(p.Tracks) == 0 {
		return 0
	}
	return (p.DiscoveryCount()*200 + len(p.Tracks)) / (2 * len(p.Tracks))
}

// CanContinue reports whether the DJ may enqueue this result. Empty and
// blocked outcomes pause additions.
func (p PlanResult) CanContinue() bool {
	return len(p.Tracks) > 0 && (p.Outcome == PlanOutcomeOK || p.Outcome == PlanOutcomeRelaxed)
}

// BlockedPlan builds the result for settings that could not be evaluated.
func BlockedPlan(reason Reason) PlanResult {
	return PlanResult{Outcome: PlanOutcomeBlocked, Reasons: Reasons{reason}}
}

// EmptyPlan builds the result for a filter set that matched no candidate.
func EmptyPlan(target int, reason Reason) PlanResult {
	return PlanResult{Outcome: PlanOutcomeEmpty, DiscoveryTarget: target, Reasons: Reasons{reason}}
}
