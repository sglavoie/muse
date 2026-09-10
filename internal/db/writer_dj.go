package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/radio"
)

// DJWriter persists the active DJ session and the saved recipe library. Every
// method validates before it writes, so invalid settings never reach the
// database and a later read never has to guess what the user meant.
type DJWriter interface {
	WriteDJSession(session DJSession) error
	WriteDJAllocation(allocation radio.Allocation) error
	StopDJSession() error
	CreateRecipe(recipe radio.Recipe) (int64, error)
	UpdateRecipe(recipe radio.Recipe) error
	DeleteRecipeSoft(id int64) error
	UndoDeleteRecipe(id int64) error
	PurgeDeletedRecipes(before int64) error
}

var _ DJWriter = (*Writer)(nil)

// WriteDJSession replaces the active DJ session snapshot: settings, seed,
// allocation, and both filter tables, in one transaction.
//
// The settings written here are a copy. Nothing links the row back to the
// recipe they may have come from, so editing or deleting that recipe afterwards
// cannot change what is playing.
//
// Filter values are normalized before storage, but tag references are stored
// verbatim: an id that no longer resolves stays put so the DJ can report it
// instead of silently dropping the filter it belongs to.
func (w *Writer) WriteDJSession(session DJSession) error {
	if err := session.Settings.Validate(); err != nil {
		return fmt.Errorf("db: write dj session: %w", err)
	}
	if session.Active && session.Seed.IsZero() {
		return fmt.Errorf("db: write dj session: %w", radio.ErrMissingSeed)
	}
	// Seed references obey the same positivity rule Settings.Validate enforces
	// on filter references, so every stored id is one a tag could actually
	// carry. A zero or negative id would round-trip into the scorer's seed and
	// silently match nothing.
	for _, id := range session.Seed.TagIDs {
		if id <= 0 {
			return fmt.Errorf("db: write dj session: seed: %w: %d", radio.ErrInvalidTagID, id)
		}
	}

	genres, err := encodeDJGenres(session.Seed.Genres)
	if err != nil {
		return fmt.Errorf("db: write dj session: encode seed genres: %w", err)
	}

	// Validate ran first, so Normalized cannot drop a tag reference here: the
	// only ids it discards are non-positive ones, which Validate rejects. A
	// reference to a deleted tag has a positive id and survives.
	settings := session.Settings.Normalized()

	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		if err := updateDJSessionRow(tx, session, settings, genres); err != nil {
			return err
		}
		if err := replaceDJFilters(tx, djScopeSession, djSessionOwnerID, settings, session.Seed.TagIDs); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// updateDJSessionRow writes the singleton dj_session columns.
func updateDJSessionRow(tx *sql.Tx, session DJSession, settings radio.Settings, genres string) error {
	active := 0
	if session.Active {
		active = 1
	}
	var capturedAt int64
	if !session.Seed.CapturedAt.IsZero() {
		capturedAt = session.Seed.CapturedAt.Unix()
	}

	if _, err := tx.Exec(`
		UPDATE dj_session SET
			active = ?, version = ?, discovery_percent = ?, anchor_mode = ?, carry_units = ?,
			seed_hash = ?, seed_title = ?, seed_artist = ?, seed_album = ?, seed_genres = ?,
			seed_duration = ?, seed_captured_at = ?, updated_at = ?
		WHERE id = 1
	`,
		active, DJSettingsVersion, settings.DiscoveryPercent, string(settings.Anchor),
		session.Allocation.Normalized().CarryUnits,
		session.Seed.TrackHash, session.Seed.Title, session.Seed.Artist, session.Seed.Album, genres,
		session.Seed.Duration, capturedAt, time.Now().Unix(),
	); err != nil {
		return fmt.Errorf("db: write dj session: %w", err)
	}
	return nil
}

// WriteDJAllocation persists only the discovery rounding carry. Accepting a
// batch adopts radio.PlanResult.NextAllocation on the hot path; rewriting the
// whole snapshot and both filter tables for one integer would be wasteful and
// would race a concurrent settings edit.
func (w *Writer) WriteDJAllocation(allocation radio.Allocation) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE dj_session SET carry_units = ?, updated_at = ? WHERE id = 1`,
			allocation.Normalized().CarryUnits, time.Now().Unix(),
		)
		return err
	})
}

// StopDJSession ends the active session: the seed and the carried allocation
// are cleared because a new session captures a fresh seed and starts from a
// zero carry. The settings snapshot and its filters are kept so the Tune tab
// reopens on what the user last applied.
func (w *Writer) StopDJSession() error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		if _, err := tx.Exec(`
			UPDATE dj_session SET
				active = 0, carry_units = 0,
				seed_hash = '', seed_title = '', seed_artist = '', seed_album = '',
				seed_genres = '[]', seed_duration = 0, seed_captured_at = 0,
				updated_at = ?
			WHERE id = 1
		`, time.Now().Unix()); err != nil {
			return fmt.Errorf("db: stop dj session: %w", err)
		}
		if _, err := tx.Exec(
			`DELETE FROM dj_tag_refs WHERE scope = ? AND owner_id = ? AND role = ?`,
			djScopeSession, djSessionOwnerID, djRoleSeed,
		); err != nil {
			return fmt.Errorf("db: stop dj session: clear seed tags: %w", err)
		}
		return tx.Commit()
	})
}

// CreateRecipe saves a new named recipe and returns its id. Recipes hold
// settings only — never a seed — so applying one starts a session that captures
// its own fresh seed.
//
// Names are unique case-insensitively among active recipes; a collision returns
// ErrRecipeNameTaken rather than overwriting the existing recipe, because
// silently replacing a saved recipe is not recoverable from the UI.
func (w *Writer) CreateRecipe(recipe radio.Recipe) (int64, error) {
	normalized, err := validatedRecipe(recipe)
	if err != nil {
		return 0, err
	}

	var id int64
	err = w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		if err := checkRecipeNameFree(tx, normalized.Name, 0); err != nil {
			return err
		}

		now := time.Now().Unix()
		res, err := tx.Exec(`
			INSERT INTO dj_recipes (name, version, discovery_percent, anchor_mode, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, normalized.Name, DJSettingsVersion, normalized.Settings.DiscoveryPercent,
			string(normalized.Settings.Anchor), now, now)
		if err != nil {
			return fmt.Errorf("db: create recipe: %w", err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return fmt.Errorf("db: create recipe id: %w", err)
		}

		if err := replaceDJFilters(tx, djScopeRecipe, id, normalized.Settings, nil); err != nil {
			return err
		}
		return tx.Commit()
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateRecipe rewrites an existing recipe's name and settings.
//
// It never touches dj_session: the active session holds its own copy of the
// settings, so editing a recipe mid-playback changes nothing the user is
// currently hearing.
func (w *Writer) UpdateRecipe(recipe radio.Recipe) error {
	normalized, err := validatedRecipe(recipe)
	if err != nil {
		return err
	}
	if normalized.ID <= 0 {
		return fmt.Errorf("db: update recipe: %w: %d", ErrRecipeNotFound, normalized.ID)
	}

	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		if err := requireActiveRecipe(tx, normalized.ID); err != nil {
			return err
		}
		if err := checkRecipeNameFree(tx, normalized.Name, normalized.ID); err != nil {
			return err
		}

		if _, err := tx.Exec(`
			UPDATE dj_recipes SET name = ?, version = ?, discovery_percent = ?, anchor_mode = ?, updated_at = ?
			WHERE id = ?
		`, normalized.Name, DJSettingsVersion, normalized.Settings.DiscoveryPercent,
			string(normalized.Settings.Anchor), time.Now().Unix(), normalized.ID); err != nil {
			return fmt.Errorf("db: update recipe: %w", err)
		}

		if err := replaceDJFilters(tx, djScopeRecipe, normalized.ID, normalized.Settings, nil); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// DeleteRecipeSoft hides a recipe, keeping its rows for undo. An active session
// started from this recipe keeps playing: it holds a snapshot, not a reference.
func (w *Writer) DeleteRecipeSoft(id int64) error { return w.softDeleteByID("dj_recipes", id) }

// UndoDeleteRecipe restores a soft-deleted recipe. It fails with
// ErrRecipeNameTaken when another recipe claimed the name in the meantime,
// rather than restoring a duplicate the unique index would reject anyway.
func (w *Writer) UndoDeleteRecipe(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		var name string
		if err := tx.QueryRow(
			`SELECT name FROM dj_recipes WHERE id = ? AND deleted_at IS NOT NULL`, id,
		).Scan(&name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("db: undo delete recipe: %w: %d", ErrRecipeNotFound, id)
			}
			return fmt.Errorf("db: undo delete recipe: %w", err)
		}
		if err := checkRecipeNameFree(tx, name, id); err != nil {
			return err
		}

		if _, err := tx.Exec(`UPDATE dj_recipes SET deleted_at = NULL WHERE id = ?`, id); err != nil {
			return fmt.Errorf("db: undo delete recipe: %w", err)
		}
		return tx.Commit()
	})
}

// PurgeDeletedRecipes hard-deletes recipes soft-deleted before the given Unix
// timestamp, along with their filter rows. dj_tag_refs and dj_filter_values
// carry no foreign key to dj_recipes (owner_id is scope-qualified), so the
// cleanup is explicit and shares the delete's transaction.
func (w *Writer) PurgeDeletedRecipes(before int64) error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		const owners = `SELECT id FROM dj_recipes WHERE deleted_at IS NOT NULL AND deleted_at < ?`
		for _, table := range []string{"dj_tag_refs", "dj_filter_values"} {
			if _, err := tx.Exec(
				`DELETE FROM `+table+` WHERE scope = ? AND owner_id IN (`+owners+`)`, //nolint:gosec // table is a hardcoded internal name, not user input
				djScopeRecipe, before,
			); err != nil {
				return fmt.Errorf("db: purge deleted recipes: %s: %w", table, err)
			}
		}
		if _, err := tx.Exec(
			`DELETE FROM dj_recipes WHERE deleted_at IS NOT NULL AND deleted_at < ?`, before,
		); err != nil {
			return fmt.Errorf("db: purge deleted recipes: %w", err)
		}
		return tx.Commit()
	})
}

// validatedRecipe trims and validates a recipe for storage. Validation runs
// before normalization so a non-positive tag id is reported rather than
// silently dropped; references to deleted tags are positive and survive, so an
// unresolved reference outlives a save-and-reload cycle.
func validatedRecipe(recipe radio.Recipe) (radio.Recipe, error) {
	if err := recipe.Validate(); err != nil {
		return radio.Recipe{}, fmt.Errorf("db: recipe: %w", err)
	}
	return recipe.Normalized(), nil
}

// requireActiveRecipe fails when id names no active recipe.
func requireActiveRecipe(tx *sql.Tx, id int64) error {
	var found int64
	if err := tx.QueryRow(
		`SELECT id FROM dj_recipes WHERE id = ? AND deleted_at IS NULL`, id,
	).Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %d", ErrRecipeNotFound, id)
		}
		return fmt.Errorf("db: look up recipe %d: %w", id, err)
	}
	return nil
}

// checkRecipeNameFree reports ErrRecipeNameTaken when another active recipe
// already uses name. exceptID is the recipe being written, or 0 on insert.
func checkRecipeNameFree(tx *sql.Tx, name string, exceptID int64) error {
	var existing int64
	err := tx.QueryRow(
		`SELECT id FROM dj_recipes WHERE name = ? COLLATE NOCASE AND deleted_at IS NULL AND id != ?`,
		name, exceptID,
	).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("db: check recipe name: %w", err)
	}
	return fmt.Errorf("%w: %q", ErrRecipeNameTaken, name)
}

// replaceDJFilters rewrites every filter row owned by (scope, ownerID). The
// delete-then-insert keeps the stored set exactly equal to the supplied
// settings; a partial update would leave removed filter values behind and keep
// narrowing selection after the user cleared them.
func replaceDJFilters(tx *sql.Tx, scope string, ownerID int64, settings radio.Settings, seedTagIDs []int64) error {
	for _, table := range []string{"dj_tag_refs", "dj_filter_values"} {
		if _, err := tx.Exec(
			`DELETE FROM `+table+` WHERE scope = ? AND owner_id = ?`, //nolint:gosec // table is a hardcoded internal name, not user input
			scope, ownerID,
		); err != nil {
			return fmt.Errorf("db: replace dj filters: %s: %w", table, err)
		}
	}

	sides := []struct {
		role string
		set  radio.FilterSet
	}{
		{djRoleInclude, settings.Include},
		{djRoleExclude, settings.Exclude},
	}
	for _, side := range sides {
		if err := insertDJTagRefs(tx, scope, ownerID, side.role, side.set.TagIDs); err != nil {
			return err
		}
		if err := insertDJFilterValues(tx, scope, ownerID, side.role, djKindGenre, side.set.Genres); err != nil {
			return err
		}
		if err := insertDJFilterValues(tx, scope, ownerID, side.role, djKindArtist, side.set.Artists); err != nil {
			return err
		}
	}
	return insertDJTagRefs(tx, scope, ownerID, djRoleSeed, seedTagIDs)
}

// insertDJTagRefs stores tag references verbatim. Duplicates within one role are
// ignored; ids that no longer resolve are kept on purpose.
func insertDJTagRefs(tx *sql.Tx, scope string, ownerID int64, role string, tagIDs []int64) error {
	for _, tagID := range tagIDs {
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO dj_tag_refs (scope, owner_id, role, tag_id) VALUES (?, ?, ?, ?)
		`, scope, ownerID, role, tagID); err != nil {
			return fmt.Errorf("db: insert dj tag ref: %w", err)
		}
	}
	return nil
}

// insertDJFilterValues stores genre or artist filter values.
func insertDJFilterValues(tx *sql.Tx, scope string, ownerID int64, role, kind string, values []string) error {
	for _, value := range values {
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO dj_filter_values (scope, owner_id, role, kind, value) VALUES (?, ?, ?, ?, ?)
		`, scope, ownerID, role, kind, value); err != nil {
			return fmt.Errorf("db: insert dj filter value: %w", err)
		}
	}
	return nil
}
