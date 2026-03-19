package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// newTestAdapter opens a migrated DB, creates a Writer backed by t.Context(),
// and returns a PlaylistAdapter ready for use. The Writer goroutine is stopped
// automatically when the test context is cancelled.
func newTestAdapter(t *testing.T) *db.PlaylistAdapter {
	t.Helper()
	d := openMigratedDB(t)
	w := db.NewWriter(t.Context(), d.DB)
	t.Cleanup(w.Wait)
	return db.NewPlaylistAdapter(d, w)
}

func TestPlaylistAdapter_CreateAndRead(t *testing.T) {
	a := newTestAdapter(t)

	id, err := a.CreatePlaylist("Chill Vibes")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if id == 0 {
		t.Fatal("CreatePlaylist returned zero ID")
	}

	rows, err := a.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d playlists, want 1", len(rows))
	}
	if rows[0].ID != id {
		t.Errorf("ID = %d, want %d", rows[0].ID, id)
	}
	if rows[0].Name != "Chill Vibes" {
		t.Errorf("Name = %q, want %q", rows[0].Name, "Chill Vibes")
	}
}

func TestPlaylistAdapter_RenameAndRead(t *testing.T) {
	a := newTestAdapter(t)

	id, err := a.CreatePlaylist("Old Name")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	if err := a.RenamePlaylist(id, "New Name"); err != nil {
		t.Fatalf("RenamePlaylist: %v", err)
	}

	rows, err := a.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d playlists, want 1", len(rows))
	}
	if rows[0].Name != "New Name" {
		t.Errorf("Name = %q, want %q", rows[0].Name, "New Name")
	}
}

func TestPlaylistAdapter_RoundTrip(t *testing.T) {
	a := newTestAdapter(t)

	// Create three playlists.
	idA, err := a.CreatePlaylist("Alpha")
	if err != nil {
		t.Fatalf("CreatePlaylist Alpha: %v", err)
	}
	idB, err := a.CreatePlaylist("Bravo")
	if err != nil {
		t.Fatalf("CreatePlaylist Bravo: %v", err)
	}
	idC, err := a.CreatePlaylist("Charlie")
	if err != nil {
		t.Fatalf("CreatePlaylist Charlie: %v", err)
	}

	// Verify all three are returned in alphabetical order.
	rows, err := a.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists (initial): %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d playlists, want 3", len(rows))
	}
	wantOrder := []struct {
		id   int64
		name string
	}{
		{idA, "Alpha"},
		{idB, "Bravo"},
		{idC, "Charlie"},
	}
	for i, w := range wantOrder {
		if rows[i].ID != w.id || rows[i].Name != w.name {
			t.Errorf("rows[%d] = {ID:%d, Name:%q}, want {ID:%d, Name:%q}",
				i, rows[i].ID, rows[i].Name, w.id, w.name)
		}
	}

	// Rename Bravo to Zulu so it moves to the end alphabetically.
	if err := a.RenamePlaylist(idB, "Zulu"); err != nil {
		t.Fatalf("RenamePlaylist: %v", err)
	}

	rows, err = a.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists (after rename): %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d playlists, want 3", len(rows))
	}
	wantAfter := []struct {
		id   int64
		name string
	}{
		{idA, "Alpha"},
		{idC, "Charlie"},
		{idB, "Zulu"},
	}
	for i, w := range wantAfter {
		if rows[i].ID != w.id || rows[i].Name != w.name {
			t.Errorf("rows[%d] = {ID:%d, Name:%q}, want {ID:%d, Name:%q}",
				i, rows[i].ID, rows[i].Name, w.id, w.name)
		}
	}
}
