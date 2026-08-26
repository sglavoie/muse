package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// DB wraps sql.DB with muse-specific setup.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at path and configures it for
// WAL mode and safe concurrent use. The caller is responsible for calling
// Close when done. Open creates the parent directory if it does not exist.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("db: create directory %s: %w", dir, err)
		}
	}

	sqlDB, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", path, err)
	}

	if err := configure(sqlDB); err != nil {
		sqlDB.Close()
		return nil, err
	}

	return &DB{sqlDB}, nil
}

func sqliteDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")

	if strings.HasPrefix(path, "file:") {
		if strings.Contains(path, "?") {
			return path + "&" + q.Encode()
		}
		return path + "?" + q.Encode()
	}

	u := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: q.Encode(),
	}
	return u.String()
}

// OpenReadOnly opens an existing SQLite database at path for reading only. It
// never creates the file or its parent directory and applies no write pragmas,
// making it safe for a short-lived process running alongside the instance that
// holds the WAL writer. The caller is responsible for calling Close when done.
func OpenReadOnly(path string) (*DB, error) {
	// sql.Open is lazy, so stat first: a missing file would otherwise surface
	// much later as an opaque driver error.
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("db: open read-only %s: %w", path, err)
	}

	sqlDB, err := sql.Open("sqlite", readOnlySqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("db: open read-only %s: %w", path, err)
	}

	// Ping forces a real connection so DSN pragma errors surface here rather
	// than on the first query.
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("db: open read-only %s: %w", path, err)
	}

	return &DB{sqlDB}, nil
}

func readOnlySqliteDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("mode", "ro")

	if strings.HasPrefix(path, "file:") {
		if strings.Contains(path, "?") {
			return path + "&" + q.Encode()
		}
		return path + "?" + q.Encode()
	}

	u := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: q.Encode(),
	}
	return u.String()
}

// rowScanner is the minimal subset of *sql.Rows needed by the package-internal
// scan helpers. Using a named interface instead of repeating the literal in
// every scan function signature keeps the signatures concise and ensures all
// callers accept the same contract.
type rowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// configure applies the required PRAGMA settings.
func configure(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("db: %s: %w", p, err)
		}
	}

	return nil
}
