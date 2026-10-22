// Package testdb provides isolated, file-backed database fixtures for tests.
package testdb

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// Each test process migrates one empty database, then keeps only its bytes.
// Closing the sole connection checkpoints the WAL before we read the file.
// Tests receive independent copies, including their own WAL and connection pool.
var migratedImage = sync.OnceValues(func() ([]byte, error) {
	dir, err := os.MkdirTemp("", "muse-testdb-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "template.db")
	d, err := db.Open(path)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(d.DB); err != nil {
		d.Close()
		return nil, err
	}
	if err := d.Close(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
})

// Open returns a fresh migrated database and closes it during test cleanup.
// Tests of migrations themselves should open an empty database with db.Open.
func Open(t testing.TB) *db.DB {
	t.Helper()
	image, err := migratedImage()
	if err != nil {
		t.Fatalf("create migrated database fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, image, 0o600); err != nil {
		t.Fatalf("copy migrated database fixture: %v", err)
	}
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("open database fixture: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
