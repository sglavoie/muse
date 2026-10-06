package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	instui "github.com/sglavoie/muse/internal/tui/insights"
	"github.com/sglavoie/muse/internal/tui/metadata"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
)

func TestPlaylistFooterTracksAndSelection(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 120
	m.showFooterHints = true
	m.ActiveView = viewPlaylists
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = []db.PlaylistTrackRow{{ID: 1, Hash: "h", Title: "Song"}}
	for _, selected := range []bool{false, true} {
		if selected {
			m.playlists.SelectedTrackIDs = map[int64]struct{}{1: {}}
		}
		hint := ansi.Strip(m.renderFooterHint())
		if !strings.Contains(hint, "enter enqueue") || strings.Contains(hint, "enter open") || strings.Contains(hint, "c create") || strings.Contains(hint, "r rename") {
			t.Fatalf("misleading track hints: %s", hint)
		}
	}
}

func TestHelpFooterStaysVisibleAndFits(t *testing.T) {
	for _, size := range [][2]int{{60, 15}, {100, 30}} {
		m := New(context.Background())
		m.termWidth = size[0]
		m.termHeight = size[1]
		next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
		m = next.(Model)
		for _, key := range []rune{'g', 'G', '/'} {
			next, _ = m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
			m = next.(Model)
			screen := ansi.Strip(m.View().Content)
			if !strings.Contains(screen, "Esc ") || !strings.Contains(screen, "%") {
				t.Fatalf("missing help footer: %s", screen)
			}
			if key == 'G' && !strings.Contains(screen, "100%") {
				t.Fatal("bottom position missing")
			}
			if lines := strings.Split(screen, "\n"); len(lines) > size[1] {
				t.Fatalf("help overflows height: %d", len(lines))
			}
			for _, line := range strings.Split(screen, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatal("help overflows width")
				}
			}
		}
	}
}

func TestClearSelectionPaletteIncludesHiddenDuplicateRows(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "a", Title: "A", Selected: true}, {ID: 2, Hash: "a", Title: "A", Selected: true}, {ID: 3, Hash: "b", Title: "B", Selected: true}}
	m.queue.Filter.Text = "A"
	m.library.SelectedHashes = map[string]struct{}{"other": {}}
	cmd := polishCommand(t, m, "selection.clear")
	if !cmd.IsEnabled() || cmd.Detail != "Clear 3 selected rows, including 1 hidden" {
		t.Fatalf("bad selection command: %+v", cmd)
	}
	m = runPaletteCommandForTest(t, m, cmd.Name)
	if m.activeViewSelectionCount() != 0 || len(m.library.SelectedHashes) != 1 {
		t.Fatal("clear selection changed wrong scope")
	}
	if polishCommand(t, m, "selection.clear").IsEnabled() {
		t.Fatal("empty selection remains enabled")
	}
}

func TestAlbumArtistEditorSaveUndoAndScan(t *testing.T) {
	m := New(context.Background())
	m.userEditedOverlay = map[string]db.UserEditedRow{}
	for _, track := range []library.Track{{Hash: "a", Path: "/disc1/a.flac", Title: "A", Album: "Compilation", AlbumArtist: "First"}, {Hash: "b", Path: "/disc2/b.flac", Title: "B", Album: "Compilation", AlbumArtist: "Second"}} {
		m.library.Index.Insert(track)
	}
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "a", AlbumArtist: "First"}}
	ed := m.bulkEditorForHashes([]string{"a", "b"})
	for range 2 {
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	}
	for _, r := range "Various Artists" {
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.overlay.metadataEditor = &ed
	next, cmd := m.handleMetadataSave()
	m = next.(Model)
	if cmd != nil {
		if result := cmd().(metadataEditedMsg); result.err != nil {
			t.Fatal(result.err)
		}
	}
	a, _ := m.library.Index.FindByHash("a")
	b, _ := m.library.Index.FindByHash("b")
	if a.AlbumArtist != "Various Artists" || library.AlbumKey(a) != library.AlbumKey(b) || m.queue.Entries[0].AlbumArtist != a.AlbumArtist {
		t.Fatal("album artist did not regroup live tracks")
	}
	if m.userEditedOverlay["a"].AlbumArtist != "Various Artists" || !m.userEditedOverlay["a"].AlbumArtistEdited {
		t.Fatal("scan overlay lost edit")
	}
	m = m.handleUndo()
	a, _ = m.library.Index.FindByHash("a")
	b, _ = m.library.Index.FindByHash("b")
	if a.AlbumArtist != "First" || b.AlbumArtist != "Second" || m.userEditedOverlay["a"].AlbumArtist != "First" {
		t.Fatal("undo lost original album artists")
	}
	// A single-track edit also seeds and changes this field.
	m.ActiveView = viewLibrary
	ed, _ = m.metadataEditorForContext()
	if ed.AlbumArtist() == "" {
		t.Fatal("single editor was not seeded")
	}
	ed.Fields[metadata.FieldAlbumArtist].SetValue("Solo")
	m.overlay.metadataEditor = &ed
	next, _ = m.handleMetadataSave()
	m = next.(Model)
	changed, _ := m.library.Index.FindByHash(ed.Hash())
	if changed.AlbumArtist != "Solo" {
		t.Fatal("single edit ignored album artist")
	}
}

func TestAlbumArtistImportPersistsAndUndoRestores(t *testing.T) {
	m := importModel(t, t.TempDir())
	m.styles = New(context.Background()).styles
	m.library.Index.UpdateMetadata("h1", "01. Sheep", "", "Relax", "Classical", "Original")
	writer := newImportWriterSpy()
	m.metadataWriter = writer
	writeImportFile(t, m.dataDir, "# muse-export v1\nhash\troot\tpath\tartist\ttitle\talbum\tgenre\ttrack\tdisc\talbum_artist\nh1\t1\ta/1.mp3\t\t01. Sheep\tRelax\tClassical\t0\t0\tVarious Artists\n")
	parsed := parseImport(t, m)
	if parsed.err != nil || parsed.plan.Counts.AlbumArtist != 1 {
		t.Fatalf("bad import: %+v", parsed)
	}
	overlay := metadata.NewImportOverlay("export", parsed.plan, 0)
	updated, cmd, _ := m.applyMetadataImport(overlay)
	m = updated
	if cmd == nil {
		t.Fatal("missing import write")
	}
	cmd()
	writer.mu.Lock()
	got := writer.albumArtists["h1"]
	writer.mu.Unlock()
	if got != "Various Artists" {
		t.Fatalf("database writer got %q", got)
	}
	m = m.handleUndo()
	tr, _ := m.library.Index.FindByHash("h1")
	if tr.AlbumArtist != "Original" {
		t.Fatalf("undo restored %q", tr.AlbumArtist)
	}
	// Undo persists asynchronously; observe the synchronized fake until it lands.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		writer.mu.Lock()
		got = writer.albumArtists["h1"]
		writer.mu.Unlock()
		if got == "Original" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("undo database value = %q", got)
}

func TestGenreMergeCarriesPendingAlbumArtistEdit(t *testing.T) {
	m := newHealthModel(t)
	m.insights.Cursor = healthRowIndex(t, m, instui.HealthGenreCollision)
	row := m.insights.Health[m.insights.Cursor]
	var target library.Track
	for _, hash := range row.Hashes {
		track, _ := m.library.Index.FindByHash(hash)
		if track.Genre != row.Canonical {
			target = track
			break
		}
	}
	if target.Hash == "" {
		t.Fatal("no merge target")
	}
	writer := newImportWriterSpy()
	m.metadataWriter = writer
	ed := metadata.NewEditor(target.Hash, target.Title, target.Artist, target.Album, target.Genre, target.Duration).WithAlbumArtists("Compilation Artist")
	m.overlay.metadataEditor = &ed
	next, editCmd := m.handleMetadataSave()
	m = next.(Model)
	m = m.handleInsightsMergeGenre()
	_, reply := runMergeWrite(t, m)
	if len(reply.applied) == 0 {
		t.Fatal("merge did not write")
	}
	editCmd() // The newer merge owns the write ticket; this earlier save stands down.
	writer.mu.Lock()
	got := writer.albumArtists[target.Hash]
	writer.mu.Unlock()
	if got != "Compilation Artist" {
		t.Fatalf("merge discarded pending album artist: %q", got)
	}
}
