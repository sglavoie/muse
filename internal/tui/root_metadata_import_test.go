package tui

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
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
