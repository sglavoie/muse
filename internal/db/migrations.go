package db

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all pending goose migrations against db using the embedded SQL
// files. It must be called before any application reads or writes.
func Migrate(db *sql.DB) error {
	if err := prepareGoose(); err != nil {
		return err
	}

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}

	return nil
}

// MigrateDown rolls back the most recently applied migration. Muse never
// downgrades on its own; this exists so a migration's Down section is exercised
// rather than merely written, and so an operator can step back one version by
// hand after a bad upgrade.
func MigrateDown(db *sql.DB) error {
	if err := prepareGoose(); err != nil {
		return err
	}

	if err := goose.Down(db, "migrations"); err != nil {
		return fmt.Errorf("db: migrate down: %w", err)
	}

	return nil
}

// MigrationVersion returns the schema version currently applied to db.
func MigrationVersion(db *sql.DB) (int64, error) {
	if err := prepareGoose(); err != nil {
		return 0, err
	}

	version, err := goose.GetDBVersion(db)
	if err != nil {
		return 0, fmt.Errorf("db: migration version: %w", err)
	}

	return version, nil
}

// prepareGoose points goose at the embedded migrations and selects the SQLite
// dialect. Both settings are process-global, so every entry point re-applies
// them instead of relying on an earlier call having run.
func prepareGoose() error {
	goose.SetBaseFS(migrationsFS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("db: set goose dialect: %w", err)
	}

	return nil
}
