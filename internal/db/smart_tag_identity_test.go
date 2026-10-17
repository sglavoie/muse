package db_test

import (
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func storedSmartQuery(t *testing.T, d *db.DB, id int64) db.SmartPlaylistQuery {
	t.Helper()
	var raw string
	if err := d.QueryRow(`SELECT query FROM playlists WHERE id = ?`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	q, err := db.ParseSmartPlaylistQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestSmartTagIdentityLifecycle(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	insertFullTrack(t, d, "h1", "/a.flac", "One", "A", "B", "rock", 1000, 100, time.Now().Unix())
	insertFullTrack(t, d, "h2", "/b.flac", "Two", "A", "B", "rock", 1000, 100, time.Now().Unix())
	tag := insertTag(t, d, w, "Chill")
	tagTrack(t, w, "h1", tag)
	q := db.SmartPlaylistQuery{Root: db.SmartRule{Field: db.FieldTag, Operator: db.OpEquals, Value: "chill"}}
	raw, err := db.MarshalSmartPlaylistQuery(q)
	if err != nil {
		t.Fatal(err)
	}
	id, err := w.CreateSmartPlaylist("Quiet", raw)
	if err != nil {
		t.Fatal(err)
	}
	q = storedSmartQuery(t, d, id)
	if q.Root.TagID != tag {
		t.Fatalf("rule was not bound: %+v", q.Root)
	}
	stale, err := db.MarshalSmartPlaylistQuery(q)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.RenameTag(tag, "Rest"); err != nil {
		t.Fatal(err)
	}
	if rows := evalQuery(t, d, q); len(rows) != 1 || rows[0].Hash != "h1" {
		t.Fatal(rows)
	}
	playlists, err := d.ReadPlaylists()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(playlists[0].Query, "Rest") {
		t.Fatal("stale label", playlists)
	}
	if err := w.UpdateSmartPlaylistQuery(id, "Quiet", stale); err != nil {
		t.Fatal(err)
	}
	if got := storedSmartQuery(t, d, id); got.Root.Value != "Rest" {
		t.Fatal(got)
	}

	dest := insertTag(t, d, w, "Night")
	final := insertTag(t, d, w, "Sleep")
	if err := w.MergeTags(tag, dest); err != nil {
		t.Fatal(err)
	}
	if err := w.MergeTags(dest, final); err != nil {
		t.Fatal(err)
	}
	if err := w.PurgeDeletedTags(time.Now().Unix() + 1); err != nil {
		t.Fatal(err)
	}
	// A query captured before both merges still follows them after source purge.
	if rows := evalQuery(t, d, q); len(rows) != 1 || rows[0].Hash != "h1" {
		t.Fatal(rows)
	}
	if err := w.UpdateSmartPlaylistQuery(id, "Quiet", stale); err != nil {
		t.Fatal(err)
	}
	q = storedSmartQuery(t, d, id)
	if q.Root.TagID != final || q.Root.Value != "Sleep" {
		t.Fatal(q)
	}
	if err := w.DeleteTagSoft(final); err != nil {
		t.Fatal(err)
	}
	for _, op := range []db.RuleOperator{db.OpEquals, db.OpNotEquals} {
		missing := q
		missing.Root.Operator = op
		missing.Root = db.SmartRule{Conjunction: db.ConjOr, Rules: []db.SmartRule{missing.Root, {Field: db.FieldArtist, Operator: db.OpEquals, Value: "A"}}}
		if _, err := d.EvalSmartPlaylist(missing); err == nil || !strings.Contains(err.Error(), "choose a replacement") {
			t.Fatalf("missing dependency must fail even in OR/NOT: %v", err)
		}
	}
	if err := w.UndoDeleteTag(final); err != nil {
		t.Fatal(err)
	}
	if rows := evalQuery(t, d, q); len(rows) != 1 {
		t.Fatal(rows)
	}
}

func TestSmartTagMigrationNestedAndMissing(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	migrateDownTo(t, d, 28)
	tag := insertTag(t, d, w, "Quiet")
	const raw = `{"root":{"conjunction":"or","rules":[{"field":"has_tag","operator":"equals","value":"quiet"},{"conjunction":"and","rules":[{"field":"has_tag","operator":"not_equals","value":"Gone"}]}]},"max_tracks":12}`
	res, err := d.Exec(`INSERT INTO playlists(name, query, created_at, updated_at, deleted_at) VALUES ('Legacy', ?, 1, 1, 1)`, raw)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(d.DB); err != nil {
		t.Fatal(err)
	}
	q := storedSmartQuery(t, d, id)
	if q.MaxTracks != 12 || q.Root.Rules[0].TagID != tag || q.Root.Rules[1].Rules[0].TagID != -1 {
		t.Fatalf("bad migration: %+v", q)
	}
	_ = insertTag(t, d, w, "Gone")
	if _, err := d.EvalSmartPlaylist(q); err == nil {
		t.Fatal("unresolved legacy reference rebound to a new namesake")
	}
	if err := w.RenameTag(tag, "Rest"); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateDown(d.DB); err != nil {
		t.Fatal(err)
	}
	down := storedSmartQuery(t, d, id)
	if down.Root.Rules[0].TagID != 0 || down.Root.Rules[0].Value != "Rest" || down.Root.Rules[1].Rules[0].Value != "Gone" {
		t.Fatal(down)
	}
	if err := db.Migrate(d.DB); err != nil {
		t.Fatal(err)
	}
}

func TestSmartTagMergeFailureRollsBackRedirect(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	src := insertTag(t, d, w, "Source")
	dst := insertTag(t, d, w, "Destination")
	_, err := d.Exec(`CREATE TRIGGER fail_tag_delete BEFORE UPDATE OF deleted_at ON tags BEGIN SELECT RAISE(ABORT, 'fail merge'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.MergeTags(src, dst); err == nil {
		t.Fatal("merge should fail")
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM tag_redirects`); n != 0 {
		t.Fatal("partial redirect survived rollback")
	}
}
