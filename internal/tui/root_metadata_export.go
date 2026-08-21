package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/fsutil"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/metaexport"
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
	if dataDir == "" {
		return false
	}
	info, err := os.Stat(metadataExportPath(dataDir))
	return err == nil && info.Mode().IsRegular()
}

// metadataExportRequestedMsg asks the root model to export whatever the active
// view is showing.
type metadataExportRequestedMsg struct{}

// metadataExportedMsg reports the outcome of the off-loop write.
type metadataExportedMsg struct {
	err     error
	path    string
	label   string
	count   int
	skipped int
}

// handleMetadataExportRequest builds the document on the main loop — it is pure
// slice work over rows the view already holds — and hands the write to a
// tea.Cmd. Refusals notify and return no command, so a model with no data
// directory (every test model) cannot touch the filesystem.
func (m Model) handleMetadataExportRequest() (tea.Model, tea.Cmd) {
	if m.dataDir == "" {
		m.statusBar.notify("Cannot export: no data directory configured")
		return m, nil
	}
	tracks, label, ok := m.visibleTracks()
	if !ok {
		m.statusBar.notify(label)
		return m, nil
	}
	doc, skipped := m.metadataExportDocument(tracks, label)
	if len(doc.Rows) == 0 {
		m.statusBar.notify("Nothing to export from " + label)
		return m, nil
	}
	return m, writeMetadataExportCmd(metadataExportPath(m.dataDir), doc, label, skipped)
}

func (m Model) handleMetadataExportedMsg(msg metadataExportedMsg) Model {
	if msg.err != nil {
		m.statusBar.notify("Export failed: " + msg.err.Error())
		return m
	}
	text := fmt.Sprintf("Exported %d %s from %s → %s",
		msg.count, pluralize(msg.count, "track", "tracks"), msg.label, msg.path)
	if msg.skipped > 0 {
		text += fmt.Sprintf(" (%d %s skipped: no content hash)",
			msg.skipped, pluralize(msg.skipped, "row", "rows"))
	}
	if msg.count > metadataExportWarnRows {
		text += " — too large for one chat paste; split it"
	}
	m.statusBar.notify(text)
	return m
}

// metadataExportDocument turns visible rows into an export document, returning
// the number of rows dropped for having no content hash.
//
// Every row is re-read from the library index: the view's own row type may be a
// db.PlaylistTrackRow or a QueueEntry, neither of which carries a genre at all,
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
		if live, ok := m.library.index.FindByHash(t.Hash); ok {
			t = live
		}
		rootID, rel := roots.Locate(t.Path)
		rows = append(rows, metaexport.Row{
			Hash:    t.Hash,
			RootID:  rootID,
			RelPath: rel,
			Artist:  t.Artist,
			Title:   t.Title,
			Album:   t.Album,
			Genre:   t.Genre,
			Track:   t.TrackNumber,
			Disc:    t.DiscNumber,
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
		err := fsutil.WriteAtomically(path, 0o600, func(f *os.File) error {
			w := bufio.NewWriter(f)
			if err := metaexport.Encode(w, doc); err != nil {
				return err
			}
			return w.Flush()
		})
		return metadataExportedMsg{
			err:     err,
			path:    path,
			label:   label,
			count:   len(doc.Rows),
			skipped: skipped,
		}
	}
}
