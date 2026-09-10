package db

import (
	"errors"
	"time"

	"github.com/sglavoie/muse/internal/radio"
)

// DJSettingsVersion is the settings-schema version this build writes into
// dj_session.version and dj_recipes.version. Bump it when the meaning of a
// stored column changes so an older build refuses the row instead of
// misreading it.
const DJSettingsVersion = 1

// Storage errors. Callers match them with errors.Is.
var (
	// ErrDJStateInvalid reports saved DJ state this build understands but
	// cannot use: an out-of-range discovery target, an unknown anchor mode, an
	// active session with no seed. Callers must surface it. Substituting
	// defaults would quietly replace the user's restrictive filters with an
	// unrestricted DJ.
	ErrDJStateInvalid = errors.New("db: saved DJ state is invalid")

	// ErrDJStateUnsupported reports saved DJ state written by a newer build.
	// The row is left untouched so downgrading and upgrading again does not
	// lose it.
	ErrDJStateUnsupported = errors.New("db: saved DJ state was written by a newer version of muse")

	// ErrRecipeNameTaken reports a name collision with another active recipe.
	// Names are compared case-insensitively.
	ErrRecipeNameTaken = errors.New("db: a recipe with that name already exists")

	// ErrRecipeNotFound reports an id that matches no active recipe.
	ErrRecipeNotFound = errors.New("db: recipe not found")
)

// DJSession is the persisted active DJ session: the settings snapshot in force,
// the seed captured when the session started, and the discovery rounding carry.
//
// Settings are a snapshot, never a pointer at a recipe. A recipe supplies the
// starting values when the user applies it and is not consulted again, so
// editing or deleting that recipe cannot change live playback.
type DJSession struct {
	// Active reports whether a DJ session is running. The remaining fields are
	// still readable when it is false: the settings survive so the Tune tab
	// reopens on the user's last filters rather than on stock defaults.
	Active bool
	// Settings is the applied snapshot. Tag references may be unresolved; call
	// radio.Settings.ResolveTags against ReadTagCatalog before planning.
	Settings radio.Settings
	// Seed is the captured seed. Zero when no seed has been captured;
	// StopDJSession clears it, and an active session may never have one.
	Seed radio.SeedSnapshot
	// Allocation is the discovery rounding carry, restored so a long session's
	// discovery share stays honest across a restart.
	Allocation radio.Allocation
	// UpdatedAt is when the row was last written. Zero before the first write.
	UpdatedAt time.Time
}

// DJRecipe is one stored recipe together with the schema version it was written
// under.
//
// Invalid is non-nil when the stored row cannot be used (it wraps
// ErrDJStateInvalid or ErrDJStateUnsupported). Such a recipe is still listed —
// so the user can see it, rename it, or delete it — but Recipe.Settings is left
// zero, which fails radio.Settings.Validate. A broken recipe therefore cannot be
// mistaken for a permissive one.
type DJRecipe struct {
	Recipe  radio.Recipe
	Version int
	Invalid error
}

// Usable reports whether the recipe's settings may be applied.
func (r DJRecipe) Usable() bool { return r.Invalid == nil }

// DJReader reads the active DJ session, the saved recipes, and the tag catalog
// used to resolve their tag references.
type DJReader interface {
	ReadDJSession() (DJSession, error)
	ReadRecipes() ([]DJRecipe, error)
	ReadRecipe(id int64) (DJRecipe, error)
	ReadTagCatalog() (radio.TagIDSet, error)
}

var _ DJReader = (*DB)(nil)

// Row discriminators for dj_tag_refs and dj_filter_values. They mirror the
// CHECK constraints in migration 023 and must stay in sync with them.
const (
	djScopeSession = "session"
	djScopeRecipe  = "recipe"

	djRoleInclude = "include"
	djRoleExclude = "exclude"
	djRoleSeed    = "seed"

	djKindGenre  = "genre"
	djKindArtist = "artist"

	// djSessionOwnerID is the owner id every session-scoped filter row carries,
	// matching the singleton dj_session row.
	djSessionOwnerID = 1
)

// djFilters is the include/exclude sets plus the seed's tag references for a
// single owner, assembled from the two filter tables.
type djFilters struct {
	include    radio.FilterSet
	exclude    radio.FilterSet
	seedTagIDs []int64
}

// settings folds the two sets onto a settings value carrying discoveryPercent
// and anchor.
func (f *djFilters) settings(discoveryPercent int, anchor string) radio.Settings {
	settings := radio.Settings{
		DiscoveryPercent: discoveryPercent,
		Anchor:           radio.AnchorMode(anchor),
	}
	if f != nil {
		settings.Include = f.include
		settings.Exclude = f.exclude
	}
	return settings
}
