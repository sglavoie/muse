package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/library"
)

func exportModel(t *testing.T, dataDir string) Model {
	t.Helper()
	tracks := []library.Track{
		{Hash: "h1", Path: "/music/a/1.mp3", Artist: "Bach", Title: "One", Album: "Relax", Genre: "Classical", TrackNumber: 1},
		{Hash: "h2", Path: "/elsewhere/2.mp3", Artist: "Bach", Title: "Two", Album: "Relax", Genre: "Classical", TrackNumber: 2},
	}
	var idx library.LibraryIndex
	idx.BulkLoad(tracks)
	return Model{
		activeView: viewLibrary,
		dataDir:    dataDir,
		cfg:        config.Config{MusicDirectories: []string{"/music"}},
		library:    LibraryView{index: idx},
	}
}

func TestMetadataExportRefusesWithoutDataDirAndReturnsNoCmd(t *testing.T) {
	m := exportModel(t, "")

	next, cmd := m.handleMetadataExportRequest()
	if cmd != nil {
		t.Fatal("an empty data directory must not produce a write command")
	}
	if msg := next.(Model).statusBar.topNotification().msg; !strings.Contains(msg, "data directory") {
		t.Fatalf("expected a refusal notification, got %q", msg)
	}
}

func TestMetadataExportRefusalNotifiesWithTheViewsReason(t *testing.T) {
	m := exportModel(t, t.TempDir())
	m.activeView = viewInsights
	m.insights.level = insightsLevelBriefing

	next, cmd := m.handleMetadataExportRequest()
	if cmd != nil {
		t.Fatal("a view with no track list must not produce a write command")
	}
	_, reason, _ := m.visibleTracks()
	if msg := next.(Model).statusBar.topNotification().msg; msg != reason {
		t.Fatalf("expected the collector's reason %q, got %q", reason, msg)
	}
}

func TestMetadataExportDocumentHydratesDedupesAndSkipsEmptyHashes(t *testing.T) {
	m := exportModel(t, t.TempDir())

	// Queue-shaped rows: no genre of their own, one duplicate, one hashless.
	rows := []library.Track{
		{Hash: "h1", Path: "/music/a/1.mp3", Artist: "stale", Title: "stale"},
		{Hash: "", Path: "/music/a/ghost.mp3"},
		{Hash: "h1", Path: "/music/a/1.mp3", Artist: "second occurrence"},
		{Hash: "h2", Path: "/elsewhere/2.mp3"},
		{Hash: "h9", Path: "/music/a/gone.mp3", Artist: "Only Here", Genre: "Fallback"},
	}

	doc, skipped := m.metadataExportDocument(rows, "Library")
	if skipped != 1 {
		t.Fatalf("expected 1 hashless row skipped, got %d", skipped)
	}
	if len(doc.Rows) != 3 {
		t.Fatalf("expected 3 deduped rows, got %d", len(doc.Rows))
	}
	if doc.Rows[0].Artist != "Bach" || doc.Rows[0].Genre != "Classical" {
		t.Fatalf("row must be hydrated from the index, got %+v", doc.Rows[0])
	}
	if doc.Rows[0].RootID != 1 || doc.Rows[0].RelPath != "a/1.mp3" {
		t.Fatalf("expected root 1 with a relative path, got %+v", doc.Rows[0])
	}
	if doc.Rows[1].RootID != 0 || doc.Rows[1].RelPath != "/elsewhere/2.mp3" {
		t.Fatalf("a path under no root must be absolute at root 0, got %+v", doc.Rows[1])
	}
	if doc.Rows[2].Artist != "Only Here" || doc.Rows[2].Genre != "Fallback" {
		t.Fatalf("a hash the index no longer holds must fall back to the row, got %+v", doc.Rows[2])
	}
	if doc.ViewLabel != "Library" || len(doc.Roots) != 1 {
		t.Fatalf("unexpected preamble: label=%q roots=%v", doc.ViewLabel, doc.Roots)
	}
}

func TestMetadataExportWritesTheFileAndNotifies(t *testing.T) {
	dir := t.TempDir()
	m := exportModel(t, dir)

	next, cmd := m.handleMetadataExportRequest()
	if cmd == nil {
		t.Fatal("expected a write command")
	}
	msg, ok := cmd().(metadataExportedMsg)
	if !ok {
		t.Fatalf("expected metadataExportedMsg, got %T", msg)
	}
	if msg.err != nil {
		t.Fatalf("write failed: %v", msg.err)
	}

	path := filepath.Join(dir, "metadata-export.tsv")
	if msg.path != path {
		t.Fatalf("expected %q, got %q", path, msg.path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("export file missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected mode 0600, got %v", perm)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.HasPrefix(body, "# muse-export v1\n") {
		t.Fatalf("unexpected magic line in:\n%s", body)
	}
	if !strings.Contains(body, "# root\t1\t/music\n") {
		t.Fatalf("missing root preamble in:\n%s", body)
	}
	if !strings.Contains(body, "h1\t1\ta/1.mp3\tBach\tOne\tRelax\tClassical\t1\t0\n") {
		t.Fatalf("missing hydrated data row in:\n%s", body)
	}

	notified := next.(Model).handleMetadataExportedMsg(msg)
	got := notified.statusBar.topNotification().msg
	if !strings.Contains(got, "Exported 2 tracks") || !strings.Contains(got, "y copies path") {
		t.Fatalf("notification must name the count and offer the copy key, got %q", got)
	}
	if strings.Contains(got, path) {
		t.Fatalf("the absolute path belongs on the clipboard, not in the notification: %q", got)
	}
	if notified.exportCopyHint.path != path {
		t.Fatalf("y must be armed with %q, got %q", path, notified.exportCopyHint.path)
	}
}

func TestMetadataExportedNotificationWarnsOnHugeExports(t *testing.T) {
	var m Model
	m = m.handleMetadataExportedMsg(metadataExportedMsg{
		path:    "/tmp/metadata-export.tsv",
		label:   "Library",
		count:   metadataExportWarnRows + 1,
		skipped: 3,
	})
	got := m.statusBar.topNotification().msg
	if !strings.Contains(got, "3 rows skipped") {
		t.Fatalf("expected the skipped count, got %q", got)
	}
	if !strings.Contains(got, "too large") {
		t.Fatalf("expected a size warning above %d rows, got %q", metadataExportWarnRows, got)
	}
}

func TestMetadataExportPaletteEntriesAreHandWrittenAndUnsafe(t *testing.T) {
	dir := t.TempDir()
	m := New(context.Background())
	m.dataDir = dir

	built := make(map[string]PaletteCommand)
	for _, c := range m.buildPaletteCommands() {
		built[c.ID] = c
	}

	for _, id := range []string{"metadata.export.tsv", "metadata.export.reveal"} {
		cmd, ok := built[id]
		if !ok {
			t.Fatalf("palette is missing %q", id)
		}
		if cmd.Category != "lib" {
			t.Errorf("%s category = %q, want lib", id, cmd.Category)
		}
		if _, safe := safeDirectCommandIDs()[id]; safe {
			t.Errorf("%s writes to disk and must not be fixture-safe", id)
		}
		for _, e := range paletteCatalog {
			if e.id == id {
				t.Errorf("%s must stay hand-written, not a catalog entry", id)
			}
		}
	}

	if !built["metadata.export.tsv"].isEnabled() {
		t.Error("export must be enabled once a data directory is configured")
	}
	if built["metadata.export.reveal"].isEnabled() {
		t.Error("reveal must stay disabled until an export exists")
	}
	if err := os.WriteFile(metadataExportPath(dir), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range m.buildPaletteCommands() {
		if c.ID == "metadata.export.reveal" && !c.isEnabled() {
			t.Error("reveal must become enabled once an export exists")
		}
	}
}

// TestExportPathCopyKeyArmsAndExpires covers the whole life of the y hint: it
// copies while the notification that advertises it is readable, and goes back
// to being an ordinary key once that notification is gone.
func TestExportPathCopyKeyArmsAndExpires(t *testing.T) {
	const path = "/tmp/metadata-export.tsv"
	var m Model
	// A cancelled context makes the copy fail at exec.Start, which proves the
	// path was handed to the clipboard command without this test writing to the
	// developer's real clipboard.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.appCtx = ctx
	m = m.handleMetadataExportedMsg(metadataExportedMsg{path: path, label: "Library", count: 2})

	armed, cmd, ok := m.handleExportPathCopyKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !ok || cmd == nil {
		t.Fatalf("y must copy while the export notification is up (consumed=%v)", ok)
	}
	if res, isResult := cmd().(clipboardResultMsg); !isResult || res.text != path {
		t.Fatalf("expected the export path on the clipboard, got %#v", cmd())
	}
	if armed.exportCopyHint.armed() {
		t.Fatal("one copy must disarm the key so a later y reaches the view")
	}

	expired := m
	expired.exportCopyHint.expiresAt = time.Now().Add(-time.Second)
	if _, _, ok := expired.handleExportPathCopyKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"}); ok {
		t.Fatal("y must reach the view once the notification has expired")
	}
}

// TestExportPathCopyKeyLeavesEveryOtherKeyAlone is the non-modal guarantee: the
// hint rides along with normal browsing, so arming it must not make the app
// swallow the keys the user is actually navigating with.
func TestExportPathCopyKeyLeavesEveryOtherKeyAlone(t *testing.T) {
	var m Model
	m.appCtx = context.Background()
	m = m.handleMetadataExportedMsg(metadataExportedMsg{path: "/tmp/e.tsv", label: "Library", count: 1})
	if !m.exportCopyHint.armed() {
		t.Fatal("precondition: the hint should be armed")
	}
	for _, key := range []tea.KeyPressMsg{
		{Code: 'j', Text: "j"},
		{Code: 'Y', Text: "Y"},
		{Code: tea.KeyEscape},
	} {
		if _, _, ok := m.handleExportPathCopyKeyPress(key); ok {
			t.Fatalf("%q must fall through to the view", key.String())
		}
	}
}

// TestExportPathCopyKeyYieldsToOverlaysOwningY guards the routing precedence:
// the track info overlay binds y to copy its own track path, and an export
// notification that happens to still be up must not shadow it.
func TestExportPathCopyKeyYieldsToOverlaysOwningY(t *testing.T) {
	var m Model
	m.appCtx = context.Background()
	m = m.handleMetadataExportedMsg(metadataExportedMsg{path: "/tmp/e.tsv", label: "Library", count: 1})
	m.overlay.trackInfo = &TrackInfoOverlay{path: "/music/a.mp3"}

	next, _, ok := m.routeKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !ok {
		t.Fatal("the overlay must consume y")
	}
	if !next.exportCopyHint.armed() {
		t.Fatal("the overlay's y must not spend the export hint")
	}
}

func TestMetadataExportExistsTracksTheFile(t *testing.T) {
	dir := t.TempDir()
	if metadataExportExists(dir) {
		t.Fatal("no export written yet")
	}
	if metadataExportExists("") {
		t.Fatal("an empty data directory can hold no export")
	}
	if err := os.WriteFile(metadataExportPath(dir), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !metadataExportExists(dir) {
		t.Fatal("expected the written export to be found")
	}
}
