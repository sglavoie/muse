package tui

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/metaexport"
	"github.com/sglavoie/muse/internal/tui/text"
)

// metadataImportRequestedMsg asks the root model to re-read the export file.
type metadataImportRequestedMsg struct{}

// metadataImportParsedMsg carries the diff back from the off-loop read.
//
// skipped is metaexport.File.Skipped rather than len(plan.Warnings): the
// overlay's summary counts dropped data rows, and the warning list is longer.
type metadataImportParsedMsg struct {
	path    string
	plan    metaexport.Plan
	skipped int
	err     error
}

// metadataImportAppliedMsg reports how the batch of writes actually landed.
type metadataImportAppliedMsg struct {
	applied    int
	superseded int
	failed     int
}

// metadataImportWrite is one row's post-import state, carried from the UI
// goroutine into the write command.
type metadataImportWrite struct {
	hash, title, artist, album, genre string
	track, disc                       int
	ticket                            uint64
}

// handleMetadataImportRequest snapshots the library on the UI goroutine and
// hands the file read to a tea.Cmd. The Current map is built from the whole
// index because the hashes an import touches are unknown until it is parsed.
func (m Model) handleMetadataImportRequest() (tea.Model, tea.Cmd) {
	if m.dataDir == "" {
		m.statusBar.Notify("Cannot import: no data directory configured")
		return m, nil
	}
	if !metadataExportExists(m.dataDir) {
		m.statusBar.Notify("No export to import — export visible tracks first")
		return m, nil
	}
	return m, readMetadataImportCmd(metadataExportPath(m.dataDir), m.metadataImportCurrent())
}

// metadataImportCurrent is the library state the diff compares against.
func (m Model) metadataImportCurrent() map[string]metaexport.Current {
	tracks := m.library.index.Tracks()
	cur := make(map[string]metaexport.Current, len(tracks))
	for _, t := range tracks {
		if t.Hash == "" {
			continue
		}
		cur[t.Hash] = metaexport.Current{
			Path:   t.Path,
			Artist: t.Artist,
			Title:  t.Title,
			Album:  t.Album,
			Genre:  t.Genre,
			Track:  t.TrackNumber,
			Disc:   t.DiscNumber,
		}
	}
	return cur
}

func readMetadataImportCmd(path string, cur map[string]metaexport.Current) tea.Cmd {
	return func() tea.Msg {
		f, err := os.Open(path)
		if err != nil {
			return metadataImportParsedMsg{path: path, err: err}
		}
		defer f.Close()
		file, err := metaexport.Parse(bufio.NewReader(f))
		if err != nil {
			return metadataImportParsedMsg{path: path, err: err}
		}
		plan := metaexport.Diff(file.Rows, cur)
		plan.Warnings = file.Warnings
		return metadataImportParsedMsg{path: path, plan: plan, skipped: file.Skipped}
	}
}

// handleMetadataImportParsedMsg opens the confirmation overlay. Nothing is
// written until the user confirms there.
func (m Model) handleMetadataImportParsedMsg(msg metadataImportParsedMsg) Model {
	if msg.err != nil {
		m.statusBar.Notify("Import failed: " + msg.err.Error())
		return m
	}
	// A parse that dropped rows or warned is not a clean no-op: an intended edit
	// may be missing from the change list precisely because it was skipped, so
	// the overlay opens read-only rather than reporting nothing happened.
	if len(msg.plan.Changes) == 0 && msg.skipped == 0 && len(msg.plan.Warnings) == 0 {
		notice := "No changes to import"
		if n := len(msg.plan.Unknown); n > 0 {
			notice += fmt.Sprintf(" · %d unknown %s", n, text.Pluralize(n, "hash", "hashes"))
		}
		m.statusBar.Notify(notice)
		return m
	}
	overlay := newMetadataImportOverlay(msg.path, msg.plan, msg.skipped)
	m.overlay.metadataImport = &overlay
	return m
}

// handleMetadataImportKeyPress owns the keyboard while the confirmation overlay
// is up.
func (m Model) handleMetadataImportKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.metadataImport == nil {
		return m, nil, false
	}
	o := *m.overlay.metadataImport
	// Same height renderOverlayBody passes, so a page step matches what is drawn.
	height := max(m.termHeight-3, 1)
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc", "q", "n":
		m.overlay.metadataImport = nil
		m.statusBar.Notify("Import canceled")
		return m, nil, true
	case "enter", "y":
		if !o.canApply() {
			return m, nil, true
		}
		return m.applyMetadataImport(o)
	case "w":
		o.showWarnings = !o.showWarnings
		o.cursor, o.offset = 0, 0
	case "j", "down":
		o.cursor++
	case "k", "up":
		o.cursor--
	case "g", "home":
		o.cursor = 0
	case "G", "end":
		o.cursor = o.rowCount() - 1
	case "pgdown":
		o.cursor += o.perPage(height)
	case "pgup":
		o.cursor -= o.perPage(height)
	default:
		return m, nil, true
	}
	o = o.reclamp(height)
	m.overlay.metadataImport = &o
	return m, nil, true
}

// applyMetadataImport mirrors handleBulkMetadataSave: memory first on the UI
// goroutine, then one ordered database write per row.
func (m Model) applyMetadataImport(o MetadataImportOverlay) (Model, tea.Cmd, bool) {
	m.overlay.metadataImport = nil

	originals := make(map[string]importOriginal, len(o.plan.Changes))
	var writes []metadataImportWrite

	for _, ch := range o.plan.Changes {
		// Re-derived against the live index, not against ch.Current: an edit
		// between parse and confirm owns the row now.
		t, ok := m.library.index.FindByHash(ch.Hash)
		if !ok {
			continue
		}
		title, artist, album, genre := t.Title, t.Artist, t.Album, t.Genre
		track, disc := t.TrackNumber, t.DiscNumber
		if ch.Title != nil {
			title = *ch.Title
		}
		if ch.Artist != nil {
			artist = *ch.Artist
		}
		if ch.Album != nil {
			album = *ch.Album
		}
		if ch.Genre != nil {
			genre = *ch.Genre
		}
		if ch.Track != nil {
			track = *ch.Track
		}
		if ch.Disc != nil {
			disc = *ch.Disc
		}
		if title == t.Title && artist == t.Artist && album == t.Album &&
			metaexport.CanonicalGenre(genre) == metaexport.CanonicalGenre(t.Genre) &&
			track == t.TrackNumber && disc == t.DiscNumber {
			continue
		}

		// Read before the overlay is rewritten below, so undo can put back the
		// exact prior row — or remove a key this import introduced.
		prevOverlay, hadOverlay := m.userEditedOverlay[ch.Hash]
		originals[ch.Hash] = importOriginal{
			title: t.Title, artist: t.Artist, album: t.Album, genre: t.Genre,
			track: t.TrackNumber, disc: t.DiscNumber,
			overlay: prevOverlay, hadOverlay: hadOverlay,
		}
		m.library.index.UpdateMetadata(ch.Hash, title, artist, album, genre)
		m.library.index.UpdateTrackPosition(ch.Hash, track, disc)

		display := text.TrackDisplay(artist, title, "")
		for i := range m.queue.entries {
			if m.queue.entries[i].Hash == ch.Hash {
				m.queue.entries[i].Title = title
				m.queue.entries[i].Artist = artist
				m.queue.entries[i].Album = album
				m.queue.entries[i].Display = display
				m.queue.entries[i].TrackNumber = track
				m.queue.entries[i].DiscNumber = disc
			}
		}
		m = m.syncStatusBarMetadata(ch.Hash, artist, title)

		writes = append(writes, metadataImportWrite{
			hash: ch.Hash, title: title, artist: artist, album: album, genre: genre,
			track: track, disc: disc,
		})
	}

	if len(writes) == 0 {
		m.statusBar.Notify("Nothing left to import — every row already matches")
		return m, nil, true
	}

	m = m.syncRecentlyAddedFromLibrary()
	m = m.preserveQueueSort()
	m.setUndoSlot(undoImportMetadataAction{originals: originals}, nil)
	m = m.refreshDerivedViews()

	if m.userEditedOverlay != nil {
		for _, w := range writes {
			// All six fields, because normalizeScannedTrack overlays them as a
			// block: a partial row would zero what the import did not change.
			m.userEditedOverlay[w.hash] = db.UserEditedRow{
				Hash: w.hash, Title: w.title, Artist: w.artist, Album: w.album, Genre: w.genre,
				TrackNumber: w.track, DiscNumber: w.disc,
			}
		}
	}

	mw := m.metadataWriter
	pw := m.trackPositionWriter
	// Published after memory reads imported, so this row outranks a genre merge
	// planned before it that has not reached the database yet.
	q := m.metadataWrites
	for i := range writes {
		writes[i].ticket = q.publish(writes[i].hash)
	}
	// Hold the Insights reload until the rows land: the index already reads
	// imported, so a snapshot taken now would mix new library figures with
	// pre-import history and never be corrected.
	pending := m.insightsPending
	pending.begin()
	return m, func() tea.Msg {
		defer pending.done()
		var msg metadataImportAppliedMsg
		for _, w := range writes {
			wrote, err := q.claim(w.hash, w.ticket, func() error {
				if mw != nil {
					if err := mw.UpdateTrackMetadata(w.hash, w.title, w.artist, w.album, w.genre); err != nil {
						return err
					}
				}
				if pw != nil {
					return pw.UpdateTrackPosition(w.hash, w.track, w.disc)
				}
				return nil
			})
			switch {
			case !wrote:
				msg.superseded++
			case err != nil:
				slog.Warn("metadata import: DB write failed", "hash", w.hash, "err", err)
				msg.failed++
			default:
				msg.applied++
			}
		}
		return msg
	}, true
}

// handleMetadataImportAppliedMsg reports the outcome. A clean run says nothing:
// the undo slot's own message already names the count and offers `u`.
func (m Model) handleMetadataImportAppliedMsg(msg metadataImportAppliedMsg) Model {
	m.insightsInvalidate()
	lost := msg.failed + msg.superseded
	if lost == 0 {
		return m
	}
	total := msg.applied + lost
	text := fmt.Sprintf("Applied %d of %d", msg.applied, total)
	if msg.failed > 0 {
		text += fmt.Sprintf(" · %d failed", msg.failed)
	}
	if msg.superseded > 0 {
		text += fmt.Sprintf(" · %d superseded by a later edit", msg.superseded)
	}
	m.statusBar.Notify(text)
	return m
}
