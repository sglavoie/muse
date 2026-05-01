package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// trackHashes returns the Hash field of each track for use in error messages.
func trackHashes(tracks []library.Track) []string {
	out := make([]string, len(tracks))
	for i, t := range tracks {
		out[i] = t.Hash
	}
	return out
}

func markMissing(t *testing.T, d *db.DB, hash string) {
	t.Helper()
	if _, err := d.Exec(`UPDATE tracks SET missing = 1 WHERE hash = ?`, hash); err != nil {
		t.Fatalf("markMissing(%q): %v", hash, err)
	}
}
