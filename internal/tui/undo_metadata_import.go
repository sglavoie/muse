package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/text"
)

// importOriginal is one track's pre-import state. It is a struct rather than
// the bulk editor's [5]string tuple because an import also rewrites track and
// disc, which no other metadata undo touches.
//
// overlay/hadOverlay carry the user-edited overlay row the import replaced.
// Without them an undo would leave the imported row in m.userEditedOverlay and
// the next watcher or scan observation would reapply it through
// normalizeScannedTrack.
type importOriginal struct {
	title, artist, album, genre string
	track, disc                 int
	overlay                     db.UserEditedRow
	hadOverlay                  bool
}

// undoImportMetadataAction reverts a TSV metadata import (undoKindImportMetadata).
type undoImportMetadataAction struct {
	originals map[string]importOriginal
}

func (a undoImportMetadataAction) kind() undoKind { return undoKindImportMetadata }

func (a undoImportMetadataAction) description() string {
	n := len(a.originals)
	return fmt.Sprintf("Imported metadata for %d %s · u to undo", n, text.Pluralize(n, "track", "tracks"))
}

func (a undoImportMetadataAction) perform(m Model) (Model, tea.Cmd) {
	mw := m.metadataWriter
	pw := m.trackPositionWriter
	for hash, orig := range a.originals {
		m.library.index.UpdateMetadata(hash, orig.title, orig.artist, orig.album, orig.genre)
		m.library.index.UpdateTrackPosition(hash, orig.track, orig.disc)
		if m.userEditedOverlay != nil {
			if orig.hadOverlay {
				m.userEditedOverlay[hash] = orig.overlay
			} else {
				delete(m.userEditedOverlay, hash)
			}
		}
		display := text.TrackDisplay(orig.artist, orig.title, "")
		for i := range m.queue.entries {
			if m.queue.entries[i].Hash == hash {
				m.queue.entries[i].Title = orig.title
				m.queue.entries[i].Artist = orig.artist
				m.queue.entries[i].Album = orig.album
				m.queue.entries[i].Display = display
				m.queue.entries[i].TrackNumber = orig.track
				m.queue.entries[i].DiscNumber = orig.disc
			}
		}
		m = m.syncStatusBarMetadata(hash, orig.artist, orig.title)
	}
	m = m.syncRecentlyAddedFromLibrary()
	m = m.refreshDerivedViews()
	m.insightsInvalidate()
	if mw != nil || pw != nil {
		originals := a.originals
		q := m.metadataWrites
		tickets := make(map[string]uint64, len(originals))
		if mw != nil {
			for h := range originals {
				tickets[h] = q.publish(h)
			}
		}
		// Hold the Insights reload until these rows land: the index above already
		// reads undone, so a snapshot taken now would mix restored library figures
		// with pre-undo history and never be corrected.
		pending := m.insightsPending
		pending.begin()
		go func() {
			defer pending.done()
			for h, orig := range originals {
				write := func() error {
					if mw != nil {
						if err := mw.UpdateTrackMetadata(h, orig.title, orig.artist, orig.album, orig.genre); err != nil {
							return err
						}
					}
					if pw != nil {
						return pw.UpdateTrackPosition(h, orig.track, orig.disc)
					}
					return nil
				}
				var err error
				if mw != nil {
					_, err = q.claim(h, tickets[h], write)
				} else {
					err = write()
				}
				if err != nil {
					slog.Warn("undo metadata import: DB write failed", "hash", h, "err", err)
				}
			}
		}()
	}
	n := len(a.originals)
	m.statusBar.notify(fmt.Sprintf("Undone metadata import for %d %s", n, text.Pluralize(n, "track", "tracks")))
	return m, nil
}
