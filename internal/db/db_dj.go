package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/radio"
)

// ReadTagCatalog returns the ids of every active tag as a radio.TagCatalog.
//
// Soft-deleted tags are excluded on purpose: a settings reference to a deleted
// tag must read as unresolved so the DJ pauses with an explanation, and undoing
// the deletion must make the same reference valid again without rewriting the
// saved settings.
func (d *DB) ReadTagCatalog() (radio.TagIDSet, error) {
	rows, err := d.Query(`SELECT id FROM tags WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("db: read tag catalog: %w", err)
	}
	defer rows.Close()

	catalog := make(radio.TagIDSet)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("db: read tag catalog row: %w", err)
		}
		catalog[id] = struct{}{}
	}
	return catalog, rows.Err()
}

// djSessionRow is the raw dj_session singleton before validation.
type djSessionRow struct {
	active           bool
	version          int
	discoveryPercent int
	anchorMode       string
	carryUnits       int
	seedHash         string
	seedTitle        string
	seedArtist       string
	seedAlbum        string
	seedGenres       string
	seedDuration     float64
	seedCapturedAt   int64
	updatedAt        int64
}

// ReadDJSession returns the persisted active DJ session.
//
// The zero DJSession (Active false) is returned when no row exists yet. Saved
// state this build cannot use is reported as an error wrapping
// ErrDJStateInvalid or ErrDJStateUnsupported and the returned session is zero:
// there is deliberately no partially-repaired value to apply by accident,
// because falling back to defaults would swap the user's restrictive filters
// for an unrestricted DJ.
//
// Unresolved tag references are not an error here. The reference is preserved
// exactly as stored; check it with radio.Settings.ResolveTags against
// ReadTagCatalog at plan time so restoring a deleted tag restores validity.
func (d *DB) ReadDJSession() (DJSession, error) {
	row, err := d.readDJSessionRow()
	if err != nil || row == nil {
		return DJSession{}, err
	}

	if err := checkDJVersion(row.version, "session"); err != nil {
		return DJSession{}, err
	}

	byOwner, err := d.readDJFilters(djScopeSession)
	if err != nil {
		return DJSession{}, err
	}
	filters := byOwner[djSessionOwnerID]

	seed, err := row.seed(filters)
	if err != nil {
		return DJSession{}, err
	}

	session := DJSession{
		Active:     row.active,
		Settings:   filters.settings(row.discoveryPercent, row.anchorMode),
		Seed:       seed,
		Allocation: radio.Allocation{CarryUnits: row.carryUnits}.Normalized(),
	}
	if row.updatedAt > 0 {
		session.UpdatedAt = time.Unix(row.updatedAt, 0)
	}

	if err := session.Settings.Validate(); err != nil {
		return DJSession{}, fmt.Errorf("%w: session: %w", ErrDJStateInvalid, err)
	}
	// An active session without a seed cannot be resumed: PlanRequest.Validate
	// would reject every batch. Report it rather than handing back a session
	// the planner silently refuses.
	if session.Active && session.Seed.IsZero() {
		return DJSession{}, fmt.Errorf("%w: session: active session has no captured seed", ErrDJStateInvalid)
	}

	return session, nil
}

// readDJSessionRow scans the singleton row, returning nil when it is absent.
func (d *DB) readDJSessionRow() (*djSessionRow, error) {
	var row djSessionRow
	var active int
	err := d.QueryRow(`
		SELECT active, version, discovery_percent, anchor_mode, carry_units,
		       seed_hash, seed_title, seed_artist, seed_album, seed_genres,
		       seed_duration, seed_captured_at, updated_at
		FROM dj_session WHERE id = 1
	`).Scan(
		&active, &row.version, &row.discoveryPercent, &row.anchorMode, &row.carryUnits,
		&row.seedHash, &row.seedTitle, &row.seedArtist, &row.seedAlbum, &row.seedGenres,
		&row.seedDuration, &row.seedCapturedAt, &row.updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: read dj session: %w", err)
	}
	row.active = active != 0
	return &row, nil
}

// seed rebuilds the captured seed snapshot, returning the zero snapshot when no
// seed hash is stored.
func (r *djSessionRow) seed(filters *djFilters) (radio.SeedSnapshot, error) {
	seed := radio.SeedSnapshot{
		TrackHash: r.seedHash,
		Title:     r.seedTitle,
		Artist:    r.seedArtist,
		Album:     r.seedAlbum,
		Duration:  r.seedDuration,
	}
	if seed.IsZero() {
		return radio.SeedSnapshot{}, nil
	}

	genres, err := decodeDJGenres(r.seedGenres)
	if err != nil {
		return radio.SeedSnapshot{}, fmt.Errorf("%w: session seed genres: %w", ErrDJStateInvalid, err)
	}
	seed.Genres = genres
	if filters != nil {
		seed.TagIDs = filters.seedTagIDs
	}
	if r.seedCapturedAt > 0 {
		seed.CapturedAt = time.Unix(r.seedCapturedAt, 0)
	}
	return seed, nil
}

// ReadRecipes returns every active recipe ordered by name.
//
// A row this build cannot use is still listed, with DJRecipe.Invalid set and
// zero settings, so the user can rename or delete it. One broken row never
// hides the rest of the library.
func (d *DB) ReadRecipes() ([]DJRecipe, error) {
	return d.queryRecipes(`
		SELECT id, name, version, discovery_percent, anchor_mode, created_at, updated_at
		FROM dj_recipes WHERE deleted_at IS NULL
		ORDER BY name COLLATE NOCASE
	`)
}

// ReadRecipe returns the active recipe with the given id, or ErrRecipeNotFound.
func (d *DB) ReadRecipe(id int64) (DJRecipe, error) {
	recipes, err := d.queryRecipes(`
		SELECT id, name, version, discovery_percent, anchor_mode, created_at, updated_at
		FROM dj_recipes WHERE id = ? AND deleted_at IS NULL
	`, id)
	if err != nil {
		return DJRecipe{}, err
	}
	if len(recipes) == 0 {
		return DJRecipe{}, fmt.Errorf("%w: %d", ErrRecipeNotFound, id)
	}
	return recipes[0], nil
}

// queryRecipes runs a recipe SELECT and joins each row with its filters.
func (d *DB) queryRecipes(query string, args ...any) ([]DJRecipe, error) {
	byOwner, err := d.readDJFilters(djScopeRecipe)
	if err != nil {
		return nil, err
	}

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: read recipes: %w", err)
	}
	defer rows.Close()

	result := make([]DJRecipe, 0)
	for rows.Next() {
		var (
			id                   int64
			name, anchor         string
			version, discovery   int
			createdAt, updatedAt int64
		)
		if err := rows.Scan(&id, &name, &version, &discovery, &anchor, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("db: read recipe row: %w", err)
		}
		result = append(result, buildDJRecipe(id, name, version, discovery, anchor, createdAt, updatedAt, byOwner[id]))
	}
	return result, rows.Err()
}

// buildDJRecipe assembles one recipe, recording rather than repairing a row the
// current build cannot use.
func buildDJRecipe(id int64, name string, version, discovery int, anchor string, createdAt, updatedAt int64, filters *djFilters) DJRecipe {
	recipe := DJRecipe{
		Recipe:  radio.Recipe{ID: id, Name: name, CreatedAt: time.Unix(createdAt, 0), UpdatedAt: time.Unix(updatedAt, 0)},
		Version: version,
	}
	if err := checkDJVersion(version, fmt.Sprintf("recipe %q", name)); err != nil {
		recipe.Invalid = err
		return recipe
	}

	settings := filters.settings(discovery, anchor)
	if err := settings.Validate(); err != nil {
		recipe.Invalid = fmt.Errorf("%w: recipe %q: %w", ErrDJStateInvalid, name, err)
		return recipe
	}

	recipe.Recipe.Settings = settings
	if err := recipe.Recipe.Validate(); err != nil {
		recipe.Recipe.Settings = radio.Settings{}
		recipe.Invalid = fmt.Errorf("%w: %w", ErrDJStateInvalid, err)
	}
	return recipe
}

// checkDJVersion rejects state written by a newer build. A version below the
// current one is accepted: every field added since is column-defaulted, so an
// old row upgrades in place on the next write.
func checkDJVersion(version int, what string) error {
	if version > DJSettingsVersion {
		return fmt.Errorf("%w: %s: version %d, this build supports %d",
			ErrDJStateUnsupported, what, version, DJSettingsVersion)
	}
	if version <= 0 {
		return fmt.Errorf("%w: %s: version %d", ErrDJStateInvalid, what, version)
	}
	return nil
}

// readDJFilters loads every filter row in scope, grouped by owner id. Both
// tables are read in full and grouped in Go so listing N recipes costs two
// queries rather than 2N.
func (d *DB) readDJFilters(scope string) (map[int64]*djFilters, error) {
	byOwner := make(map[int64]*djFilters)
	if err := d.scanDJTagRefs(scope, byOwner); err != nil {
		return nil, err
	}
	if err := d.scanDJFilterValues(scope, byOwner); err != nil {
		return nil, err
	}
	return byOwner, nil
}

// scanDJTagRefs folds dj_tag_refs rows into byOwner. Tag ids are read exactly as
// stored, including references to deleted tags.
func (d *DB) scanDJTagRefs(scope string, byOwner map[int64]*djFilters) error {
	rows, err := d.Query(`
		SELECT owner_id, role, tag_id FROM dj_tag_refs
		WHERE scope = ? ORDER BY owner_id, role, tag_id
	`, scope)
	if err != nil {
		return fmt.Errorf("db: read dj tag refs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ownerID, tagID int64
		var role string
		if err := rows.Scan(&ownerID, &role, &tagID); err != nil {
			return fmt.Errorf("db: read dj tag ref row: %w", err)
		}
		owner := ownerFilters(byOwner, ownerID)
		switch role {
		case djRoleInclude:
			owner.include.TagIDs = append(owner.include.TagIDs, tagID)
		case djRoleExclude:
			owner.exclude.TagIDs = append(owner.exclude.TagIDs, tagID)
		case djRoleSeed:
			owner.seedTagIDs = append(owner.seedTagIDs, tagID)
		}
	}
	return rows.Err()
}

// scanDJFilterValues folds dj_filter_values rows into byOwner.
func (d *DB) scanDJFilterValues(scope string, byOwner map[int64]*djFilters) error {
	rows, err := d.Query(`
		SELECT owner_id, role, kind, value FROM dj_filter_values
		WHERE scope = ? ORDER BY owner_id, role, kind, value COLLATE NOCASE
	`, scope)
	if err != nil {
		return fmt.Errorf("db: read dj filter values: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ownerID int64
		var role, kind, value string
		if err := rows.Scan(&ownerID, &role, &kind, &value); err != nil {
			return fmt.Errorf("db: read dj filter value row: %w", err)
		}
		owner := ownerFilters(byOwner, ownerID)
		set := &owner.include
		if role == djRoleExclude {
			set = &owner.exclude
		}
		if kind == djKindGenre {
			set.Genres = append(set.Genres, value)
		} else {
			set.Artists = append(set.Artists, value)
		}
	}
	return rows.Err()
}

// ownerFilters returns (creating if needed) the filters accumulator for ownerID.
func ownerFilters(byOwner map[int64]*djFilters, ownerID int64) *djFilters {
	owner, ok := byOwner[ownerID]
	if !ok {
		owner = &djFilters{}
		byOwner[ownerID] = owner
	}
	return owner
}

// decodeDJGenres parses the seed's genre list. Genres are stored as a JSON
// array rather than a delimited string because a genre name may contain any
// separator character the library allows.
func decodeDJGenres(encoded string) ([]string, error) {
	if encoded == "" {
		return nil, nil
	}
	var genres []string
	if err := json.Unmarshal([]byte(encoded), &genres); err != nil {
		return nil, err
	}
	if len(genres) == 0 {
		return nil, nil
	}
	return genres, nil
}

// encodeDJGenres renders a seed's genre list for storage.
func encodeDJGenres(genres []string) (string, error) {
	if len(genres) == 0 {
		return "[]", nil
	}
	encoded, err := json.Marshal(genres)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
