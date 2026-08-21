package tui

// The one Data Health finding with a fix behind it: folding every stored
// spelling of a genre into the canonical one. The write goes through the same
// metadata path as an edit, so it is undoable and lands in track_genres, which
// is what lets the reload clear the Rhythm banner.

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// insightsMergePlan is a genre merge waiting on the confirmation bar.
type insightsMergePlan struct {
	canonical string
	variants  []string
	hashes    []string
}

// insightsMergedMsg reports the merge's database writes. The reload is issued
// on this message rather than batched with the writes: a batched read would
// race them and could re-show the banner the merge just cleared.
type insightsMergedMsg struct{ err error }

// handleInsightsMergeGenre arms the confirmation bar for the focused genre
// collision. Nothing is written until the user confirms.
func (m Model) handleInsightsMergeGenre() Model {
	if m.activeView != viewInsights {
		return m
	}
	row := m.insights.focusedHealthRow()
	if row == nil || row.kind != healthGenreCollision {
		m.statusBar.notify("Only genre collisions can be merged")
		return m
	}
	n := len(row.hashes)
	m.insights.pendingMerge = &insightsMergePlan{
		canonical: row.canonical,
		variants:  row.variants,
		hashes:    row.hashes,
	}
	m.confirmationBar = fmt.Sprintf("Merge %s into %q on %d %s? (y to confirm)",
		strings.Join(row.variants, " / "), row.canonical, n, pluralize(n, "track", "tracks"))
	return m
}

// handleInsightsMergeKeyPress owns the keyboard while a merge is pending.
func (m Model) handleInsightsMergeKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.insights.pendingMerge == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	if key := msg.String(); key != "y" && key != "Y" {
		m.insights.pendingMerge = nil
		m.confirmationBar = ""
		m.statusBar.notify("Merge canceled")
		return m, nil, true
	}
	return m.confirmInsightsGenreMerge()
}

// confirmInsightsGenreMerge rewrites every affected track's genre string in
// memory and returns the command that persists it.
func (m Model) confirmInsightsGenreMerge() (Model, tea.Cmd, bool) {
	plan := *m.insights.pendingMerge
	m.insights.pendingMerge = nil
	m.confirmationBar = ""

	fold := make(map[string]struct{}, len(plan.variants))
	for _, v := range plan.variants {
		fold[insightsGenreKey(v)] = struct{}{}
	}

	type update struct{ hash, title, artist, album, genre string }
	var updates []update
	originals := make(map[string][5]string, len(plan.hashes))

	for _, hash := range plan.hashes {
		t, ok := m.library.index.FindByHash(hash)
		if !ok {
			continue
		}
		merged := mergeGenreVariants(t.Genre, fold, plan.canonical)
		if merged == t.Genre {
			continue
		}
		originals[hash] = [5]string{t.Title, t.Artist, t.Album, t.Genre, formatDurationForEditor(t.Duration)}
		m.library.index.UpdateMetadata(hash, t.Title, t.Artist, t.Album, merged)
		updates = append(updates, update{hash, t.Title, t.Artist, t.Album, merged})
	}

	if len(updates) == 0 {
		m.statusBar.notify("Genres already agree")
		return m, nil, true
	}

	m = m.syncRecentlyAddedFromLibrary()
	m.setUndoSlot(undoBulkEditMetadataAction{originals: originals}, nil)
	m = m.refreshDerivedViews()

	if m.userEditedOverlay != nil {
		for _, u := range updates {
			m.userEditedOverlay[u.hash] = db.UserEditedRow{
				Hash: u.hash, Title: u.title, Artist: u.artist, Album: u.album, Genre: u.genre,
			}
		}
	}

	m.statusBar.notify(fmt.Sprintf("Merged %d %s into %s",
		len(updates), pluralize(len(updates), "track", "tracks"), plan.canonical))

	mw := m.metadataWriter
	return m, func() tea.Msg {
		var firstErr error
		for _, u := range updates {
			if mw == nil {
				break
			}
			if err := mw.UpdateTrackMetadata(u.hash, u.title, u.artist, u.album, u.genre); err != nil {
				slog.Warn("insights: genre merge DB write failed", "hash", u.hash, "err", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
		return insightsMergedMsg{err: firstErr}
	}, true
}

// handleInsightsMergedMsg reloads the panels off the merged data, so the
// collision row and the Rhythm banner it caused both go away together.
func (m Model) handleInsightsMergedMsg(msg insightsMergedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.statusBar.notify(fmt.Sprintf("Genre merge: %s", msg.err))
	}
	return m, m.insightsLoadCmd()
}

// mergeGenreVariants rewrites one track's genre string, replacing every folded
// variant with the canonical spelling. Genres a track carries besides the
// merged one are preserved: a "rock, Ambient" track must not lose its rock.
func mergeGenreVariants(genre string, fold map[string]struct{}, canonical string) string {
	parts := library.SplitGenres(genre)
	if len(parts) == 0 {
		return genre
	}
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		if _, folded := fold[insightsGenreKey(p)]; folded {
			p = canonical
		}
		key := insightsGenreKey(p)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return strings.Join(out, ", ")
}
