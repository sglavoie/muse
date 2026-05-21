package db_test

import (
	"testing"
)

func TestWriter_RemoveFavoritesByTypeAndValues(t *testing.T) {
	d := openTestDB(t)
	if _, err := d.Exec(`CREATE TABLE favorites (type TEXT, value TEXT, created_at INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	w := newWriter(t, d)

	// Insert some favorites
	favorites := []struct {
		typ, value string
	}{
		{"artist", "A"},
		{"artist", "B"},
		{"album", "X"},
	}
	for _, f := range favorites {
		if err := w.AddFavorite(f.typ, f.value); err != nil {
			t.Fatalf("AddFavorite: %v", err)
		}
	}

	// Remove one artist
	if err := w.RemoveFavoritesByTypeAndValues("artist", []string{"A"}); err != nil {
		t.Fatalf("RemoveFavoritesByTypeAndValues: %v", err)
	}
	// Check remaining
	rows, err := d.Query(`SELECT type, value FROM favorites`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	got := map[string]struct{}{}
	for rows.Next() {
		var typ, value string
		if err := rows.Scan(&typ, &value); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[typ+":"+value] = struct{}{}
	}
	want := map[string]struct{}{"artist:B": {}, "album:X": {}}
	if len(got) != len(want) {
		t.Errorf("remaining = %v, want %v", got, want)
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}

	// Remove with empty values (should be no-op)
	if err := w.RemoveFavoritesByTypeAndValues("artist", nil); err != nil {
		t.Fatalf("RemoveFavoritesByTypeAndValues empty: %v", err)
	}
}
