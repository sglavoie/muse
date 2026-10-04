package tui

import (
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/browse"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func polishCommand(t *testing.T, m Model, id string) palette.PaletteCommand {
	t.Helper()
	for _, c := range m.BuildPaletteCommands() {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("missing command %s", id)
	return palette.PaletteCommand{}
}

func TestExportScopeAndBackupPalette(t *testing.T) {
	m := newRichFixture()
	m.dataDir = t.TempDir()
	c := polishCommand(t, m, "metadata.export.tsv")
	if c.Name != "Export 6 visible tracks to TSV" || !c.IsEnabled() {
		t.Fatal(c.Name, c.DisabledReason)
	}
	tracks := m.library.Index.Tracks()
	m.library.SelectedHashes = map[string]struct{}{tracks[0].Hash: {}}
	c = polishCommand(t, m, "metadata.export.tsv")
	if c.Name != "Export 1 selected track to TSV" {
		t.Fatal(c.Name)
	}
	m.library.Filter.Text = "no-matching-track"
	m.library = m.library.RebuildFilterCache()
	if polishCommand(t, m, "metadata.export.tsv").IsEnabled() {
		t.Fatal("empty filtered selection is exportable")
	}
	m.ActiveView = viewQueue
	m.queue.Entries = append(m.queue.Entries, m.queue.Entries[0])
	m.queue.Entries[0].Selected = true
	m.queue.Entries[3].Selected = true
	c = polishCommand(t, m, "metadata.export.tsv")
	if c.Name != "Export 1 selected track to TSV" || !strings.Contains(c.Detail, "1 duplicates collapsed") {
		t.Fatal(c.Name, c.Detail)
	}
	m.queue.Entries[0].Hash = ""
	c = polishCommand(t, m, "metadata.export.tsv")
	if !strings.Contains(c.Detail, "1 missing hashes skipped") {
		t.Fatal(c.Detail)
	}
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.browse.Artists.Artists = []string{"Alice"}
	c = polishCommand(t, m, "metadata.export.tsv")
	if strings.Contains(c.Name, "visible") || !strings.Contains(c.Detail, "Artists") {
		t.Fatal(c.Name, c.Detail)
	}
	const backupID = "metadata.export.backup.reveal"
	if polishCommand(t, m, backupID).IsEnabled() || IsSafeDirectCommand(backupID) {
		t.Fatal("backup incorrectly available/safe")
	}
	path := metadataExportPath(m.dataDir) + ".bak"
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if polishCommand(t, m, backupID).IsEnabled() {
		t.Fatal("directory accepted as backup")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("previous export"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !polishCommand(t, m, backupID).IsEnabled() {
		t.Fatal("backup unavailable")
	}
}

func TestDirectSortPreservesQueueAndUsesSupportedFields(t *testing.T) {
	m := newRichFixture()
	m.ActiveView = viewQueue
	entries := slices.Clone(m.queue.Entries)
	originalDisplay := m.queue.FilteredEntries()
	c := polishCommand(t, m, "sort.field.4") // Duration
	next, _ := m.Update(c.Exec()())
	m = next.(Model)
	if m.queue.SortState.Field != sorting.SortDuration {
		t.Fatal(m.queue.SortState)
	}
	if !reflect.DeepEqual(entries, m.queue.Entries) {
		t.Fatal("display sort changed playback order")
	}
	rows := m.queue.FilteredEntries()
	if rows[0].DurationMs > rows[1].DurationMs {
		t.Fatal("duration sort not applied")
	}
	if !strings.Contains(polishCommand(t, m, c.ID).CurrentState, "Selected") {
		t.Fatal("active field unmarked")
	}
	m.queue.Cursor = 1
	next, _ = m.Update(c.Exec()())
	m = next.(Model)
	if m.queue.Cursor != 1 {
		t.Fatal("selecting current sort moved cursor")
	}
	next, _ = m.Update(selectSortFieldMsg{sorting.SortDateAdded})
	if next.(Model).queue.SortState != m.queue.SortState {
		t.Fatal("unsupported queue field accepted")
	}
	next, _ = m.Update(polishCommand(t, m, "sort.field.0").Exec()())
	m = next.(Model)
	if !reflect.DeepEqual(m.queue.FilteredEntries(), originalDisplay) {
		t.Fatal("default order not restored")
	}
	m.ActiveView = viewLibrary
	m.library.Mode = libtui.LibraryModeHistory
	commands := m.sortPaletteCommands()
	if len(commands) != 1 || commands[0].Name != "Sort by Last Played" {
		t.Fatal(commands)
	}
	m.ActiveView = viewFavorites
	if len(m.sortPaletteCommands()) != 0 {
		t.Fatal("category offers unsupported sorting")
	}
}

func TestFilterHistoryAcrossViews(t *testing.T) {
	m := newRichFixture()
	m = pressEnter(typeText(pressSlash(m), "Alice"))
	m = pressEsc(m)
	m.ActiveView = viewQueue
	m = typeText(pressSlash(m), "draft")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = next.(Model)
	if m.queue.Filter.Text != "Alice" || len(m.queue.FilteredEntries()) != 1 {
		t.Fatal("recall did not filter queue")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	if m.queue.Filter.Text != "draft" {
		t.Fatal("draft not restored")
	}
	m = pressEsc(m)
	if len(m.filterHistory) != 1 || m.filterHistory[0] != "Alice" {
		t.Fatal("cancelled draft entered history")
	}
}

func TestMessagesCopyGuardAndResult(t *testing.T) {
	m := newRichFixture()
	m.statusBar.Notify("a long error")
	m = m.showRecentMessages()
	snapshot := m.overlay.messages.CopyText()
	_, cmd, handled := m.handleMessagesKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !handled || cmd == nil {
		t.Fatal("copy command not returned")
	}
	// Never execute the clipboard command in tests.
	next, _ := m.Update(messagesCopiedMsg{})
	m = next.(Model)
	if m.overlay.messages.CopyText() != snapshot {
		t.Fatal("copy result changed snapshot")
	}
	if m.statusBar.RecentMessages()[1].Text != "Copied recent messages" {
		t.Fatal("copied contents repeated into history")
	}
	next, _ = m.Update(messagesCopiedMsg{err: errors.New("clipboard unavailable")})
	m = next.(Model)
	if !strings.Contains(m.statusBar.RecentMessages()[2].Text, "clipboard unavailable") {
		t.Fatal("copy error lost")
	}
	m.fixtureSafeCommands = true
	_, cmd, _ = m.handleMessagesKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd != nil {
		t.Fatal("fixture can access clipboard")
	}
}
