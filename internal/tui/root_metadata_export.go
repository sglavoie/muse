package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/desktop"
	"github.com/sglavoie/muse/internal/fsutil"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/metaexport"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/text"
)

// metadataExportFile is the fixed name of the round-trip export. It is not
// timestamped because import must find it again with zero arguments, and it is
// overwritten on every export for the same reason.
const metadataExportFile = "metadata-export.tsv"

// metadataExportWarnRows is the row count above which the notification warns
// that the file is unwieldy to paste into a chat. Nothing is ever truncated.
const metadataExportWarnRows = 2000

// metadataExportPath returns the export destination inside the XDG data
// directory. Session 4's import reads the same path.
func metadataExportPath(dataDir string) string {
	return filepath.Join(dataDir, metadataExportFile)
}

// metadataExportExists reports whether an export is on disk and ready to be
// revealed or re-read.
func metadataExportExists(dataDir string) bool {
	return metadataExportArtifactExists(dataDir, "")
}

func metadataExportArtifactExists(dataDir, suffix string) bool {
	if dataDir == "" {
		return false
	}
	info, err := os.Stat(metadataExportPath(dataDir) + suffix)
	return err == nil && info.Mode().IsRegular()
}

func (m Model) metadataExportCommand() palette.PaletteCommand {
	tracks, label, ok := m.visibleTracks()
	count, skipped := metadataExportCounts(tracks)
	scope := "visible "
	if m.hasBulkSelection() {
		scope = "selected "
	} else if m.ActiveView == viewBrowse && !hasFocusedTrack(m) {
		// At collection level these are the focused collection's tracks.
		scope = ""
	}
	reason := ""
	switch {
	case m.dataDir == "":
		reason = "Data directory is unavailable"
	case m.metadataExportBusy:
		reason = "Metadata export already in progress"
	case !ok:
		reason = label
	case count == 0:
		reason = "No exportable tracks in this scope"
	}
	detail := label
	if skipped > 0 || len(tracks)-skipped > count {
		detail += fmt.Sprintf(" · %d missing hashes skipped · %d duplicates collapsed", skipped, len(tracks)-skipped-count)
	}
	return palette.PaletteCommand{
		ID:             "metadata.export.tsv",
		Name:           fmt.Sprintf("Export %d %s%s to TSV", count, scope, text.Pluralize(count, "track", "tracks")),
		Category:       "lib",
		Detail:         detail,
		Exec:           func() tea.Cmd { return func() tea.Msg { return metadataExportRequestedMsg{} } },
		Enabled:        func() bool { return reason == "" },
		DisabledReason: reason,
	}
}

func metadataExportCounts(tracks []library.Track) (int, int) {
	seen := make(map[string]struct{}, len(tracks))
	skipped := 0
	for _, track := range tracks {
		if track.Hash == "" {
			skipped++
			continue
		}
		seen[track.Hash] = struct{}{}
	}
	return len(seen), skipped
}

// metadataExportRequestedMsg asks the root model to export whatever the active
// view is showing.
type metadataExportRequestedMsg struct{}

// metadataExportedMsg reports the outcome of the off-loop write.
type metadataExportedMsg struct {
	err     error
	path    string
	backup  string
	label   string
	count   int
	skipped int
}

// handleMetadataExportRequest builds the document on the main loop — it is pure
// slice work over rows the view already holds — and hands the write to a
// tea.Cmd. Refusals notify and return no command, so a model with no data
// directory (every test model) cannot touch the filesystem.
func (m Model) handleMetadataExportRequest() (tea.Model, tea.Cmd) {
	if m.metadataExportBusy {
		m.statusBar.Notify("Metadata export already in progress")
		return m, nil
	}
	if m.dataDir == "" {
		m.statusBar.NotifyError("Cannot export: no data directory configured")
		return m, nil
	}
	tracks, label, ok := m.visibleTracks()
	if !ok {
		m.statusBar.Notify(label)
		return m, nil
	}
	doc, skipped := m.metadataExportDocument(tracks, label)
	if len(doc.Rows) == 0 {
		m.statusBar.Notify("Nothing to export from " + label)
		return m, nil
	}
	m.metadataExportBusy = true
	return m, writeMetadataExportCmd(metadataExportPath(m.dataDir), doc, label, skipped)
}

// exportPathCopy arms y to copy the export destination. The path is long enough
// to crowd the counts out of the status bar, so the key is offered in its place,
// armed for exactly as long as the notification advertising it stays readable.
type exportPathCopy struct {
	path      string
	expiresAt time.Time
}

func (e exportPathCopy) armed() bool {
	return e.path != "" && time.Now().Before(e.expiresAt)
}

func (m Model) handleMetadataExportedMsg(msg metadataExportedMsg) Model {
	m.metadataExportBusy = false
	if msg.err != nil {
		m.statusBar.NotifyError("Export failed: " + msg.err.Error())
		return m
	}
	notice := fmt.Sprintf("Exported %d %s from %s",
		msg.count, text.Pluralize(msg.count, "track", "tracks"), msg.label)
	if msg.skipped > 0 {
		notice += fmt.Sprintf(" (%d %s skipped: no content hash)",
			msg.skipped, text.Pluralize(msg.skipped, "row", "rows"))
	}
	if msg.count > metadataExportWarnRows {
		notice += " — too large for one chat paste; split it"
	}
	if msg.backup != "" {
		m.statusBar.Notify("Previous export saved to " + msg.backup)
		notice += " · backup: " + filepath.Base(msg.backup)
	}
	notice += " · y copies path"
	m.exportCopyHint = exportPathCopy{
		path:      msg.path,
		expiresAt: time.Now().Add(m.statusBar.NotifyDuration()),
	}
	m.statusBar.Notify(notice)
	return m
}

// handleExportPathCopyKeyPress claims y, and only y, while the export
// notification is up. This hint rides along with normal browsing rather than
// taking the keyboard the way a confirmation bar does, so every other key must
// fall through to the view underneath.
func (m Model) handleExportPathCopyKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.exportCopyHint.armed() || msg.String() != "y" {
		return m, nil, false
	}
	path := m.exportCopyHint.path
	m.exportCopyHint = exportPathCopy{}
	return m, desktop.CopyToClipboard(m.appCtx, path), true
}

// metadataExportDocument turns visible rows into an export document, returning
// the number of rows dropped for having no content hash.
//
// Every row is re-read from the library index: the view's own row type may be a
// db.PlaylistTrackRow or a queue.Entry, neither of which carries a genre at all,
// so the row itself is only a fallback for hashes the index no longer holds.
// Duplicates collapse to the first visible occurrence, because one content hash
// is shared by duplicate files and a queue can hold the same track twice.
func (m Model) metadataExportDocument(tracks []library.Track, label string) (metaexport.Document, int) {
	roots := metaexport.NewRoots(m.cfg.MusicDirectories)
	rows := make([]metaexport.Row, 0, len(tracks))
	seen := make(map[string]struct{}, len(tracks))
	skipped := 0
	for _, t := range tracks {
		if t.Hash == "" {
			skipped++
			continue
		}
		if _, dup := seen[t.Hash]; dup {
			continue
		}
		seen[t.Hash] = struct{}{}
		if live, ok := m.library.Index.FindByHash(t.Hash); ok {
			t = live
		}
		rootID, rel := roots.Locate(t.Path)
		rows = append(rows, metaexport.Row{
			Hash:        t.Hash,
			RootID:      rootID,
			RelPath:     rel,
			Artist:      t.Artist,
			Title:       t.Title,
			Album:       t.Album,
			AlbumArtist: t.AlbumArtist, AlbumArtistPresent: true,
			Genre: t.Genre,
			Track: t.TrackNumber,
			Disc:  t.DiscNumber,
		})
	}
	return metaexport.Document{
		Roots:       roots.Dirs(),
		GeneratedAt: time.Now(),
		ViewLabel:   label,
		Rows:        rows,
	}, skipped
}

func writeMetadataExportCmd(path string, doc metaexport.Document, label string, skipped int) tea.Cmd {
	return func() tea.Msg {
		backup, err := backupMetadataExport(path)
		if err != nil {
			return metadataExportedMsg{err: err}
		}
		err = fsutil.WriteAtomically(path, 0o600, func(f *os.File) error {
			w := bufio.NewWriter(f)
			if err := metaexport.Encode(w, doc); err != nil {
				return err
			}
			return w.Flush()
		})
		return metadataExportedMsg{
			err:     err,
			path:    path,
			backup:  backup,
			label:   label,
			count:   len(doc.Rows),
			skipped: skipped,
		}
	}
}

// Keep the previous bytes (including unimported external edits) before replacing
// the export. A failed backup must leave the working export untouched.
func backupMetadataExport(path string) (string, error) {
	source, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read previous export: %w", err)
	}
	defer source.Close()
	backup := path + ".bak"
	if info, statErr := os.Lstat(backup); statErr == nil && !info.Mode().IsRegular() {
		return "", fmt.Errorf("backup destination is not a regular file: %s", backup)
	}
	err = fsutil.WriteAtomically(backup, 0o600, func(f *os.File) error {
		_, copyErr := io.Copy(f, source)
		return copyErr
	})
	if err != nil {
		return "", fmt.Errorf("back up previous export: %w", err)
	}
	return backup, nil
}
