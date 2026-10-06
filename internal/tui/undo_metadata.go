package tui

import (
	"fmt"
	"log/slog"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/text"
)

// undoBulkEditMetadataAction reverts a bulk metadata edit (undoKindBulkEditMetadata).
// originals maps track hash → {title, artist, album, genre, duration (M:SS or "")}.
type undoBulkEditMetadataAction struct {
	originals    map[string][5]string
	albumArtists map[string]string
	durations    map[string]float64 // precise seconds, when captured by the editor
}

func (a undoBulkEditMetadataAction) originalDuration(hash string) float64 {
	if duration, ok := a.durations[hash]; ok {
		return duration
	}
	duration, _ := metadata.ParseDuration(a.originals[hash][4])
	return duration
}

func (a undoBulkEditMetadataAction) kind() undoKind { return undoKindBulkEditMetadata }

func (a undoBulkEditMetadataAction) description() string {
	n := len(a.originals)
	return fmt.Sprintf("Updated %d %s · u to undo", n, text.Pluralize(n, "track", "tracks"))
}

func (a undoBulkEditMetadataAction) perform(m Model) (Model, error) {
	mw := m.metadataWriter
	dw := m.durationWriter
	for hash, orig := range a.originals {
		m.library.Index.UpdateMetadata(hash, orig[0], orig[1], orig[2], orig[3], a.albumArtistValues(hash)...)
		origDur := a.originalDuration(hash)
		m.library.Index.UpdateDuration(hash, origDur)
		if t, ok := m.library.Index.FindByHash(hash); ok && m.userEditedOverlay != nil {
			m.userEditedOverlay[hash] = db.UserEditedRow{Hash: hash, Title: t.Title, Artist: t.Artist, Album: t.Album, Genre: t.Genre, AlbumArtistEdited: len(a.albumArtistValues(hash)) > 0 || m.userEditedOverlay[hash].AlbumArtistEdited, AlbumArtist: t.AlbumArtist, TrackNumber: t.TrackNumber, DiscNumber: t.DiscNumber}
		}
		display := text.TrackDisplay(orig[1], orig[0], "")
		origDurMs := int64(origDur * 1000)
		for i := range m.queue.Entries {
			if m.queue.Entries[i].Hash == hash {
				m.queue.Entries[i].Title = orig[0]
				m.queue.Entries[i].Artist = orig[1]
				m.queue.Entries[i].Album = orig[2]
				m.queue.Entries[i].Display = display
				m.queue.Entries[i].DurationMs = origDurMs
			}
		}
		m = m.syncStatusBarMetadata(hash, orig[1], orig[0])
	}
	m = m.syncRecentlyAddedFromLibrary()
	m = m.refreshDerivedViews()
	m.insightsInvalidate()
	// The index was rewritten back to the original artist/genre values, which
	// is the same kind of change to Auto-DJ filtering that the edit was.
	m.djInvalidate()
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
						return mw.UpdateTrackMetadata(h, orig[0], orig[1], orig[2], orig[3], a.albumArtistValues(h)...)
					})
					if err != nil {
						slog.Warn("undo bulk metadata edit: DB write failed", "hash", h, "err", err)
					}
				}
				if dw != nil {
					origDur := a.originalDuration(h)
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

func (a undoBulkEditMetadataAction) albumArtistValues(hash string) []string {
	if value, ok := a.albumArtists[hash]; ok {
		return []string{value}
	}
	return nil
}
