package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/text"
)

// undoBulkEditMetadataAction reverts a bulk metadata edit (undoKindBulkEditMetadata).
// originals maps track hash → {title, artist, album, genre, duration (M:SS or "")}.
type undoBulkEditMetadataAction struct {
	originals map[string][5]string
}

func (a undoBulkEditMetadataAction) kind() undoKind { return undoKindBulkEditMetadata }

func (a undoBulkEditMetadataAction) description() string {
	n := len(a.originals)
	return fmt.Sprintf("Updated %d %s · u to undo", n, text.Pluralize(n, "track", "tracks"))
}

func (a undoBulkEditMetadataAction) perform(m Model) (Model, tea.Cmd) {
	mw := m.metadataWriter
	dw := m.durationWriter
	for hash, orig := range a.originals {
		m.library.Index.UpdateMetadata(hash, orig[0], orig[1], orig[2], orig[3])
		origDur, _ := metadata.ParseDuration(orig[4])
		m.library.Index.UpdateDuration(hash, origDur)
		display := text.TrackDisplay(orig[1], orig[0], "")
		origDurMs := int64(origDur * 1000)
		for i := range m.queue.entries {
			if m.queue.entries[i].Hash == hash {
				m.queue.entries[i].Title = orig[0]
				m.queue.entries[i].Artist = orig[1]
				m.queue.entries[i].Album = orig[2]
				m.queue.entries[i].Display = display
				m.queue.entries[i].DurationMs = origDurMs
			}
		}
		m = m.syncStatusBarMetadata(hash, orig[1], orig[0])
	}
	m = m.syncRecentlyAddedFromLibrary()
	m = m.refreshDerivedViews()
	m.insightsInvalidate()
	if mw != nil || dw != nil {
		originals := a.originals
		q := m.metadataWrites
		tickets := make(map[string]uint64, len(originals))
		if mw != nil {
			for h := range originals {
				tickets[h] = q.Publish(h)
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
				if mw != nil {
					_, err := q.Claim(h, tickets[h], func() error {
						return mw.UpdateTrackMetadata(h, orig[0], orig[1], orig[2], orig[3])
					})
					if err != nil {
						slog.Warn("undo bulk metadata edit: DB write failed", "hash", h, "err", err)
					}
				}
				if dw != nil {
					origDur, _ := metadata.ParseDuration(orig[4])
					origDurMs := int64(origDur * 1000)
					if err := dw.UpdateTrackDuration(h, origDurMs); err != nil {
						slog.Warn("undo bulk metadata edit: duration DB write failed", "hash", h, "err", err)
					}
				}
			}
		}()
	}
	n := len(a.originals)
	m.statusBar.Notify(fmt.Sprintf("Undone metadata edit for %d %s", n, text.Pluralize(n, "track", "tracks")))
	return m, nil
}
