package tui

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/metaexport"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// importWriterSpy records both halves of an import write so a test can assert
// they went through the same claim.
type importWriterSpy struct {
	mu        sync.Mutex
	meta      map[string][4]string
	positions map[string][2]int
	failMeta  map[string]bool
}

func newImportWriterSpy() *importWriterSpy {
	return &importWriterSpy{
		meta:      map[string][4]string{},
		positions: map[string][2]int{},
		failMeta:  map[string]bool{},
	}
}

func (w *importWriterSpy) UpdateTrackMetadata(hash, title, artist, album, genre string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failMeta[hash] {
		return errors.New("database is locked")
	}
	w.meta[hash] = [4]string{title, artist, album, genre}
	return nil
}

func (w *importWriterSpy) UpdateTrackPosition(hash string, track, disc int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.positions[hash] = [2]int{track, disc}
	return nil
}

func (w *importWriterSpy) snapshot() (map[string][4]string, map[string][2]int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	meta := make(map[string][4]string, len(w.meta))
	for k, v := range w.meta {
		meta[k] = v
	}
	pos := make(map[string][2]int, len(w.positions))
	for k, v := range w.positions {
		pos[k] = v
	}
	return meta, pos
}

func importModel(t *testing.T, dataDir string) Model {
	t.Helper()
	tracks := []library.Track{
		{Hash: "h1", Path: "/music/a/1.mp3", Artist: "", Title: "01. Sheep", Album: "Relax", Genre: "Classical"},
		{Hash: "h2", Path: "/music/a/2.mp3", Artist: "Bach", Title: "Two", Album: "Relax", Genre: "Classical", TrackNumber: 2},
	}
	var idx library.LibraryIndex
	idx.BulkLoad(tracks)
	return Model{
		activeView:        viewLibrary,
		dataDir:           dataDir,
		termHeight:        40,
		termWidth:         100,
		cfg:               config.Config{MusicDirectories: []string{"/music"}},
		library:           LibraryView{index: idx},
		metadataWrites:    newMetadataWriteQueue(),
		userEditedOverlay: map[string]db.UserEditedRow{},
	}
}

const importHeader = "# muse-export v1\n" +
	"# generated\t2026-08-21T00:00:00Z\n" +
	"# view\tLibrary\n" +
	"# count\t2\n" +
	"# root\t1\t/music\n" +
	"hash\troot\tpath\tartist\ttitle\talbum\tgenre\ttrack\tdisc\n"

// editedExport is the file as an LLM would hand it back: h1 gains an artist, a
// stripped title and a track number; h2 is untouched.
const editedExport = importHeader +
	"h1\t1\ta/1.mp3\tPink Floyd\tSheep\tRelax\tClassical\t1\t0\n" +
	"h2\t1\ta/2.mp3\tBach\tTwo\tRelax\tClassical\t2\t0\n"

func writeImportFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(metadataExportPath(dir), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func parseImport(t *testing.T, m Model) metadataImportParsedMsg {
	t.Helper()
	_, cmd := m.handleMetadataImportRequest()
	if cmd == nil {
		t.Fatal("expected a read command")
	}
	msg, ok := cmd().(metadataImportParsedMsg)
	if !ok {
		t.Fatalf("expected metadataImportParsedMsg, got %T", cmd())
	}
	return msg
}

func TestMetadataImportRefusesWithoutDataDirOrFile(t *testing.T) {
	m := importModel(t, "")
	next, cmd := m.handleMetadataImportRequest()
	if cmd != nil {
		t.Fatal("an empty data directory must not produce a read command")
	}
	if msg := next.(Model).statusBar.topNotification().msg; !strings.Contains(msg, "data directory") {
		t.Fatalf("expected a refusal notification, got %q", msg)
	}

	m = importModel(t, t.TempDir())
	next, cmd = m.handleMetadataImportRequest()
	if cmd != nil {
		t.Fatal("a missing export must not produce a read command")
	}
	if msg := next.(Model).statusBar.topNotification().msg; !strings.Contains(msg, "No export") {
		t.Fatalf("expected a missing-file refusal, got %q", msg)
	}
}

func TestMetadataImportParsesOffLoopAndDiffsAgainstTheIndex(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	writeImportFile(t, dir, editedExport)

	msg := parseImport(t, m)
	if msg.err != nil {
		t.Fatalf("parse failed: %v", msg.err)
	}
	if len(msg.plan.Changes) != 1 {
		t.Fatalf("expected exactly the h1 change, got %d", len(msg.plan.Changes))
	}
	if msg.plan.Unchanged != 1 {
		t.Fatalf("expected h2 unchanged, got %d", msg.plan.Unchanged)
	}
	ch := msg.plan.Changes[0]
	if ch.Hash != "h1" || ch.Artist == nil || *ch.Artist != "Pink Floyd" || ch.Track == nil || *ch.Track != 1 {
		t.Fatalf("unexpected change: %+v", ch)
	}
	if ch.Genre != nil || ch.Album != nil {
		t.Fatalf("unedited fields must stay nil: %+v", ch)
	}
}

func TestMetadataImportSurfacesWholeFileRejection(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	writeImportFile(t, dir, "| hash | artist |\n| --- | --- |\n")

	msg := parseImport(t, m)
	if msg.err == nil {
		t.Fatal("a markdown table must be rejected")
	}
	m = m.handleMetadataImportParsedMsg(msg)
	if m.overlay.metadataImport != nil {
		t.Fatal("a rejected file must not open the overlay")
	}
	if got := m.statusBar.topNotification().msg; !strings.Contains(got, "Import failed") {
		t.Fatalf("expected the parser's own wording, got %q", got)
	}
}

func TestMetadataImportOfAnUnchangedFileReportsNoChanges(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	writeImportFile(t, dir, importHeader+
		"h1\t1\ta/1.mp3\t\t01. Sheep\tRelax\tClassical\t0\t0\n"+
		"h9\t1\ta/gone.mp3\tGhost\tGone\t\t\t0\t0\n")

	m = m.handleMetadataImportParsedMsg(parseImport(t, m))
	if m.overlay.metadataImport != nil {
		t.Fatal("an import with no changes must not open the overlay")
	}
	got := m.statusBar.topNotification().msg
	if !strings.Contains(got, "No changes") || !strings.Contains(got, "1 unknown hash") {
		t.Fatalf("expected a no-op notification naming the stale hash, got %q", got)
	}
}

// TestMetadataImportWithNoChangesStillSurfacesSkippedRowsAndWarnings covers a
// file whose only intended edit was dropped by the parser: the remaining valid
// row changes nothing, and the zero-change path used to report a clean no-op
// that hid both the skipped count and the warning explaining it.
func TestMetadataImportWithNoChangesStillSurfacesSkippedRowsAndWarnings(t *testing.T) {
	dir := t.TempDir()
	m := importModel(t, dir)
	writeImportFile(t, dir, importHeader+
		"h1\t1\ta/1.mp3\t\t01. Sheep\tRelax\tClassical\t0\t0\n"+
		"h2\t1\ta/2.mp3\tJohann Sebastian Bach\tTwo\tRelax\tClassical\ttwo\t0\n")

	msg := parseImport(t, m)
	if len(msg.plan.Changes) != 0 || msg.skipped != 1 {
		t.Fatalf("expected zero changes and one skipped row, got %d changes and %d skipped",
			len(msg.plan.Changes), msg.skipped)
	}

	m = m.handleMetadataImportParsedMsg(msg)
	if m.overlay.metadataImport == nil {
		t.Fatal("a parse that dropped an intended edit must not be reported as a clean no-op")
	}
	out := m.overlay.metadataImport.Render(m.termWidth, m.termHeight, styles.NewStyles(true))
	if !strings.Contains(out, "1 row skipped") {
		t.Errorf("the overlay must name the skipped row:\n%s", out)
	}
	if !strings.Contains(out, "w 1 warning") {
		t.Errorf("the overlay must offer the warnings page:\n%s", out)
	}
	if strings.Contains(out, "enter/y apply") {
		t.Errorf("an empty change list must not advertise apply:\n%s", out)
	}

	next, cmd, handled := m.handleMetadataImportKeyPress(tea.KeyPressMsg{Code: 'w', Text: "w"})
	if !handled || cmd != nil {
		t.Fatalf("w must be handled and issue no command (handled=%v)", handled)
	}
	warned := next.overlay.metadataImport.Render(next.termWidth, next.termHeight, styles.NewStyles(true))
	if !strings.Contains(warned, `track "two" is not a number`) {
		t.Errorf("the warnings page must explain the dropped row:\n%s", warned)
	}

	after, cmd, handled := next.handleMetadataImportKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !handled || cmd != nil {
		t.Fatalf("y must not start a write for an empty change list (handled=%v)", handled)
	}
	if after.overlay.metadataImport == nil {
		t.Fatal("the warnings must stay inspectable after y")
	}
	if after.undo.slot.action != nil {
		t.Fatal("nothing was applied, so no undo slot may be armed")
	}
}

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
	m.metadataWrites.publish("h1")

	applied := cmd().(metadataImportAppliedMsg)
	if applied.superseded != 1 || applied.applied != 0 {
		t.Fatalf("expected the write to stand down, got %+v", applied)
	}
	meta, pos := spy.snapshot()
	if len(meta) != 0 || len(pos) != 0 {
		t.Fatalf("a superseded row must write neither half: meta=%v pos=%v", meta, pos)
	}

	m = m.handleMetadataImportAppliedMsg(applied)
	if got := m.statusBar.topNotification().msg; !strings.Contains(got, "Applied 0 of 1") ||
		!strings.Contains(got, "superseded") {
		t.Fatalf("expected a partial-outcome notification, got %q", got)
	}
}

func TestMetadataImportAppliedNotifiesOnFailureOnly(t *testing.T) {
	var m Model
	m = m.handleMetadataImportAppliedMsg(metadataImportAppliedMsg{applied: 3})
	if got := m.statusBar.topNotification().msg; got != "" {
		t.Fatalf("a clean run must leave the undo hint standing, got %q", got)
	}
	m = m.handleMetadataImportAppliedMsg(metadataImportAppliedMsg{applied: 2, failed: 1})
	if got := m.statusBar.topNotification().msg; !strings.Contains(got, "Applied 2 of 3") ||
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
	if got := next.statusBar.topNotification().msg; !strings.Contains(got, "already matches") {
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
	if msg := next.statusBar.topNotification().msg; !strings.Contains(msg, "canceled") {
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
	o := newMetadataImportOverlay("/tmp/metadata-export.tsv", plan, 3)
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

	o.showWarnings = true
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
	o := newMetadataImportOverlay("/tmp/x.tsv", plan, 0)

	const height = 20
	if got := len(o.changeRows(height)); got > metadataImportRowBudget(height) {
		t.Fatalf("changeRows returned %d rows for a budget of %d", got, metadataImportRowBudget(height))
	}
	// G lands on the last change and the window must follow it there.
	o.cursor = len(plan.Changes) - 1
	o = o.reclamp(height)
	if o.offset == 0 {
		t.Fatal("the window must scroll to keep the cursor visible")
	}
	if o.cursor < o.offset || o.cursor >= o.offset+o.perPage(height) {
		t.Fatalf("cursor %d outside window [%d,%d)", o.cursor, o.offset, o.offset+o.perPage(height))
	}
}

func TestMetadataImportPaletteEntryIsGatedOnTheExportFile(t *testing.T) {
	dir := t.TempDir()
	m := New(context.Background())
	m.dataDir = dir

	find := func() (PaletteCommand, bool) {
		for _, c := range m.buildPaletteCommands() {
			if c.ID == "metadata.import" {
				return c, true
			}
		}
		return PaletteCommand{}, false
	}

	cmd, ok := find()
	if !ok {
		t.Fatal("palette is missing metadata.import")
	}
	if cmd.Name != "Import metadata edits…" || cmd.Category != "lib" {
		t.Fatalf("unexpected entry: name=%q category=%q", cmd.Name, cmd.Category)
	}
	if cmd.isEnabled() {
		t.Fatal("import must stay disabled until an export exists")
	}
	if _, safe := safeDirectCommandIDs()["metadata.import"]; safe {
		t.Error("import writes to the library and must not be fixture-safe")
	}

	writeImportFile(t, dir, editedExport)
	cmd, _ = find()
	if !cmd.isEnabled() {
		t.Fatal("import must become enabled once an export exists")
	}
	if _, ok := cmd.Exec()().(metadataImportRequestedMsg); !ok {
		t.Fatal("the entry must dispatch metadataImportRequestedMsg rather than act directly")
	}
}

func ptr(s string) *string { return &s }
func ptrInt(i int) *int    { return &i }
