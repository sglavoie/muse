package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/metaexport"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestMetadataImportApplyWritesSixFieldsThroughOneClaim(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	spy := newImportWriterSpy()
	m.metadataWriter = spy
	m.trackPositionWriter = spy
	writeImportFile(t, dir, editedExport)

	m = m.handleMetadataImportParsedMsg(parseImport(t, m))
	if m.overlay.metadataImport == nil {
		t.Fatal("expected the confirmation overlay")
	}

	next, cmd, handled := m.handleMetadataImportKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !handled || cmd == nil {
		t.Fatalf("apply must be handled and return a write command (handled=%v)", handled)
	}
	m = next
	if m.overlay.metadataImport != nil {
		t.Fatal("apply must close the overlay")
	}

	got, ok := m.library.index.FindByHash("h1")
	if !ok {
		t.Fatal("h1 vanished from the index")
	}
	if got.Artist != "Pink Floyd" || got.Title != "Sheep" || got.TrackNumber != 1 {
		t.Fatalf("index not updated: %+v", got)
	}

	row := m.userEditedOverlay["h1"]
	if row.Album != "Relax" || row.Genre != "Classical" || row.TrackNumber != 1 {
		t.Fatalf("the overlay row must carry all six fields, got %+v", row)
	}
	if m.undo.slot.action == nil || m.undo.slot.action.kind() != undoKindImportMetadata {
		t.Fatal("apply must arm an import undo slot")
	}

	applied, ok := cmd().(metadataImportAppliedMsg)
	if !ok {
		t.Fatalf("expected metadataImportAppliedMsg, got %T", cmd())
	}
	if applied.applied != 1 || applied.failed != 0 || applied.superseded != 0 {
		t.Fatalf("unexpected outcome: %+v", applied)
	}
	meta, pos := spy.snapshot()
	if meta["h1"] != [4]string{"Sheep", "Pink Floyd", "Relax", "Classical"} {
		t.Fatalf("metadata write = %v", meta["h1"])
	}
	if pos["h1"] != [2]int{1, 0} {
		t.Fatalf("position write = %v", pos["h1"])
	}
	if _, touched := meta["h2"]; touched {
		t.Fatal("an unchanged row must not be written")
	}
}

// TestMetadataImportRefreshesTheOpenInsightsDrill is the reported repro: drill
// into a data-health bucket, export, edit, import. The drill list must show the
// new values, and a re-export must offer them rather than the pre-import ones.
func TestMetadataImportRefreshesTheOpenInsightsDrill(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	m.activeView = viewInsights
	m.insights.setSize(100, 30)
	m.insights.dormant = m.library.index.Tracks()
	m.insights.openDrill("No artist", m.insightsDrillTracks(insightsRow{drill: drillDormant}))
	writeImportFile(t, dir, editedExport)

	m = m.handleMetadataImportParsedMsg(parseImport(t, m))
	next, cmd, _ := m.handleMetadataImportKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("expected a write command")
	}
	m = next

	var drilled library.Track
	for _, tr := range m.insights.trackView.sourceTracks {
		if tr.Hash == "h1" {
			drilled = tr
		}
	}
	if drilled.Artist != "Pink Floyd" || drilled.Title != "Sheep" || drilled.TrackNumber != 1 {
		t.Fatalf("the open drill still renders pre-import values: %+v", drilled)
	}

	rows, label, ok := m.insightsVisibleTracks()
	if !ok {
		t.Fatalf("expected an exportable drill, got %q", label)
	}
	for _, r := range rows {
		if r.Hash == "h1" && (r.Artist != "Pink Floyd" || r.Title != "Sheep") {
			t.Fatalf("a re-export would round-trip stale values back in: %+v", r)
		}
	}
}

func TestMetadataImportApplyStandsDownForASupersedingEdit(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	spy := newImportWriterSpy()
	m.metadataWriter = spy
	m.trackPositionWriter = spy
	writeImportFile(t, dir, editedExport)

	m = m.handleMetadataImportParsedMsg(parseImport(t, m))
	next, cmd, _ := m.handleMetadataImportKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next

	// A later edit of the same row publishes a newer ticket before the import's
	// command runs; the import must not overwrite it.
	m.metadataWrites.Publish("h1")

	applied := cmd().(metadataImportAppliedMsg)
	if applied.superseded != 1 || applied.applied != 0 {
		t.Fatalf("expected the write to stand down, got %+v", applied)
	}
	meta, pos := spy.snapshot()
	if len(meta) != 0 || len(pos) != 0 {
		t.Fatalf("a superseded row must write neither half: meta=%v pos=%v", meta, pos)
	}

	m = m.handleMetadataImportAppliedMsg(applied)
	if got := m.statusBar.TopNotification().Msg; !strings.Contains(got, "Applied 0 of 1") ||
		!strings.Contains(got, "superseded") {
		t.Fatalf("expected a partial-outcome notification, got %q", got)
	}
}

func TestMetadataImportAppliedNotifiesOnFailureOnly(t *testing.T) {
	var m Model
	m = m.handleMetadataImportAppliedMsg(metadataImportAppliedMsg{applied: 3})
	if got := m.statusBar.TopNotification().Msg; got != "" {
		t.Fatalf("a clean run must leave the undo hint standing, got %q", got)
	}
	m = m.handleMetadataImportAppliedMsg(metadataImportAppliedMsg{applied: 2, failed: 1})
	if got := m.statusBar.TopNotification().Msg; !strings.Contains(got, "Applied 2 of 3") ||
		!strings.Contains(got, "1 failed") {
		t.Fatalf("expected the failure count, got %q", got)
	}
}

func TestMetadataImportApplyRederivesAgainstTheLiveIndex(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	writeImportFile(t, dir, editedExport)
	m = m.handleMetadataImportParsedMsg(parseImport(t, m))

	// The user made the same edit by hand between parse and confirm.
	m.library.index.UpdateMetadata("h1", "Sheep", "Pink Floyd", "Relax", "Classical")
	m.library.index.UpdateTrackPosition("h1", 1, 0)

	next, cmd, _ := m.handleMetadataImportKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd != nil {
		t.Fatal("a row that already matches must produce no write")
	}
	if next.undo.slot.action != nil {
		t.Fatal("nothing changed, so no undo slot may be armed")
	}
	if got := next.statusBar.TopNotification().Msg; !strings.Contains(got, "already matches") {
		t.Fatalf("expected a no-op notification, got %q", got)
	}
}

func TestMetadataImportEscapeAppliesNothing(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	writeImportFile(t, dir, editedExport)
	m = m.handleMetadataImportParsedMsg(parseImport(t, m))

	next, cmd, handled := m.handleMetadataImportKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !handled || cmd != nil {
		t.Fatalf("esc must be handled and issue no command (handled=%v)", handled)
	}
	if next.overlay.metadataImport != nil {
		t.Fatal("esc must close the overlay")
	}
	if got, _ := next.library.index.FindByHash("h1"); got.Artist != "" {
		t.Fatalf("esc must not touch the index, got %+v", got)
	}
	if msg := next.statusBar.TopNotification().Msg; !strings.Contains(msg, "canceled") {
		t.Fatalf("expected a cancel notification, got %q", msg)
	}
}

func TestMetadataImportUndoRestoresAllSixFields(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	spy := newImportWriterSpy()
	m.metadataWriter = spy
	m.trackPositionWriter = spy
	writeImportFile(t, dir, editedExport)

	m = m.handleMetadataImportParsedMsg(parseImport(t, m))
	next, cmd, _ := m.handleMetadataImportKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next
	cmd()

	action := m.undo.slot.action
	m, _ = action.perform(m)
	got, _ := m.library.index.FindByHash("h1")
	if got.Artist != "" || got.Title != "01. Sheep" || got.Album != "Relax" ||
		got.Genre != "Classical" || got.TrackNumber != 0 || got.DiscNumber != 0 {
		t.Fatalf("undo must revert all six fields, got %+v", got)
	}
}

// TestMetadataImportUndoRestoresTheUserEditedOverlayForTheNextScan covers the
// window between an undo and the next full overlay reload: undo used to leave
// the imported row in m.userEditedOverlay, so the first watcher or scan
// observation reapplied it through normalizeScannedTrack.
func TestMetadataImportUndoRestoresTheUserEditedOverlayForTheNextScan(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	// h1 has never been user-edited; h2 has, so undo must delete one key and
	// restore the other.
	m.userEditedOverlay["h2"] = db.UserEditedRow{
		Hash: "h2", Title: "Two", Artist: "Bach", Album: "Relax", Genre: "Classical", TrackNumber: 2,
	}
	writeImportFile(t, dir, importHeader+
		"h1\t1\ta/1.mp3\tPink Floyd\tSheep\tRelax\tClassical\t1\t0\n"+
		"h2\t1\ta/2.mp3\tJ.S. Bach\tTwo\tRelax\tClassical\t2\t0\n")

	m = m.handleMetadataImportParsedMsg(parseImport(t, m))
	next, cmd, _ := m.handleMetadataImportKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next
	if cmd == nil {
		t.Fatal("expected a write command")
	}
	cmd()

	m, _ = m.undo.slot.action.perform(m)

	if row, ok := m.userEditedOverlay["h1"]; ok {
		t.Fatalf("undo must delete the overlay entry the import created, got %+v", row)
	}
	if row := m.userEditedOverlay["h2"]; row.Artist != "Bach" || row.TrackNumber != 2 {
		t.Fatalf("undo must restore the pre-import overlay row, got %+v", row)
	}

	// The scanner reports each file's own tags: h1 must land as scanned, and h2
	// must land as its restored overlay says — never as the import wrote it.
	m, _ = m.handleScanBatchMsg(library.ScanBatchMsg{Tracks: []library.Track{
		{Hash: "h1", Path: "/music/a/1.mp3", Title: "01. Sheep", Album: "Relax", Genre: "Classical"},
		{Hash: "h2", Path: "/music/a/2.mp3", Artist: "Johann Sebastian Bach", Title: "Two", Album: "Relax", Genre: "Classical", TrackNumber: 2},
	}})

	got, ok := m.library.index.FindByHash("h1")
	if !ok {
		t.Fatal("h1 vanished from the index")
	}
	if got.Artist != "" || got.Title != "01. Sheep" || got.TrackNumber != 0 {
		t.Fatalf("a scan after undo resurrected the imported values: %+v", got)
	}
	got, ok = m.library.index.FindByHash("h2")
	if !ok {
		t.Fatal("h2 vanished from the index")
	}
	if got.Artist != "Bach" {
		t.Fatalf("the pre-import overlay must still govern the scan, got artist %q", got.Artist)
	}
}

func TestMetadataImportOverlayRendersCountsBadgesAndCanonicalGenre(t *testing.T) {
	plan := metaexport.Plan{
		Changes: []metaexport.Change{{
			Hash:    "h1",
			Current: metaexport.Current{Artist: "", Title: "01. Sheep", Genre: "Rock/Pop"},
			Artist:  ptr("Pink Floyd"),
			Title:   ptr("Sheep"),
			Genre:   ptr("Prog Rock"),
			Track:   ptrInt(1),
		}},
		Unchanged: 12,
		Unknown:   []string{"gone1", "gone2"},
		Counts:    metaexport.FieldCounts{Artist: 1, Title: 1, Genre: 1, Track: 1},
		Warnings:  []metaexport.Warning{{Line: 9, Msg: "bad track number"}},
	}
	o := metadata.NewImportOverlay("/tmp/metadata-export.tsv", plan, 3)
	out := o.Render(120, 40, styles.NewStyles(true))

	for _, want := range []string{
		"/tmp/metadata-export.tsv",
		"1 changed · 12 unchanged · 2 unknown · 3 skipped",
		"artist 1 · title 1 · album 0 · genre 1 · trk 1 · disc 0",
		"2 hashes no longer in the library",
		"[a t g #]",
		"Pink Floyd — 01. Sheep",
		"Pop; Rock → Prog Rock",
		"artist (empty) → Pink Floyd",
		"w 1 warning",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("overlay is missing %q in:\n%s", want, out)
		}
	}

	o.ShowWarnings = true
	if got := o.Render(120, 40, styles.NewStyles(true)); !strings.Contains(got, "line 9: bad track number") {
		t.Errorf("warnings page must list the parse warnings, got:\n%s", got)
	}
}

func TestMetadataImportOverlayPreSlicesItsOwnWindow(t *testing.T) {
	var plan metaexport.Plan
	for i := range 80 {
		plan.Changes = append(plan.Changes, metaexport.Change{
			Hash:    string(rune('a' + i%26)),
			Current: metaexport.Current{Artist: "A", Title: "T"},
			Title:   ptr("new"),
		})
	}
	o := metadata.NewImportOverlay("/tmp/x.tsv", plan, 0)

	const height = 20
	if got := len(o.ChangeRows(height)); got > metadata.ImportRowBudget(height) {
		t.Fatalf("changeRows returned %d rows for a budget of %d", got, metadata.ImportRowBudget(height))
	}
	// G lands on the last change and the window must follow it there.
	o.Cursor = len(plan.Changes) - 1
	o = o.Reclamp(height)
	if o.Offset == 0 {
		t.Fatal("the window must scroll to keep the cursor visible")
	}
	if o.Cursor < o.Offset || o.Cursor >= o.Offset+o.PerPage(height) {
		t.Fatalf("cursor %d outside window [%d,%d)", o.Cursor, o.Offset, o.Offset+o.PerPage(height))
	}
}

func TestMetadataImportPaletteEntryIsGatedOnTheExportFile(t *testing.T) {
	dir := t.TempDir()
	m := New(context.Background())
	m.dataDir = dir

	find := func() (palette.PaletteCommand, bool) {
		for _, c := range m.buildPaletteCommands() {
			if c.ID == "metadata.import" {
				return c, true
			}
		}
		return palette.PaletteCommand{}, false
	}

	cmd, ok := find()
	if !ok {
		t.Fatal("palette is missing metadata.import")
	}
	if cmd.Name != "Import metadata edits…" || cmd.Category != "lib" {
		t.Fatalf("unexpected entry: name=%q category=%q", cmd.Name, cmd.Category)
	}
	if cmd.IsEnabled() {
		t.Fatal("import must stay disabled until an export exists")
	}
	if _, safe := safeDirectCommandIDs()["metadata.import"]; safe {
		t.Error("import writes to the library and must not be fixture-safe")
	}

	writeImportFile(t, dir, editedExport)
	cmd, _ = find()
	if !cmd.IsEnabled() {
		t.Fatal("import must become enabled once an export exists")
	}
	if _, ok := cmd.Exec()().(metadataImportRequestedMsg); !ok {
		t.Fatal("the entry must dispatch metadataImportRequestedMsg rather than act directly")
	}
}

func ptr(s string) *string { return &s }
func ptrInt(i int) *int    { return &i }
