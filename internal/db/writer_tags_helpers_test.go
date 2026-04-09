package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// openTagDB returns a migrated DB and a Writer wired to its context.
func openTagDB(t *testing.T) (*db.DB, *db.Writer) {
	t.Helper()
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)
	t.Cleanup(func() { w.Wait() })
	return d, w
}
