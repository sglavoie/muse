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
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/text"
)

// insightsMergePlan is a genre merge waiting on the confirmation bar.
type insightsMergePlan struct {
	canonical string
	variants  []string
	hashes    []string
}

// insightsMergeWrite is one track's genre rewrite, carried from the plan
// through the database write to the memory update only a persisted row earns.
type insightsMergeWrite struct {
	hash, title, artist, album, genre string
	original                          [5]string // title, artist, album, genre, duration — for undo
	ticket                            uint64    // this row's slot in the metadata write queue
}

// planStillHolds reports whether the track is still the row the merge planned
// against. The plan is a genre rewrite of one specific row; once any of the
// five edited fields has moved, reapplying it would revert the change that
// moved them.
func (w insightsMergeWrite) planStillHolds(t library.Track) bool {
	return t.Title == w.original[0] && t.Artist == w.original[1] &&
		t.Album == w.original[2] && t.Genre == w.original[3] &&
		metadata.FormatDuration(t.Duration) == w.original[4]
}

// insightsMergedMsg reports which of the merge's writes actually landed. The
// reload is issued on this message rather than batched with the writes: a
// batched read would race them and could re-show the banner the merge just
// cleared.
type insightsMergedMsg struct {
	canonical  string
	applied    []insightsMergeWrite
	failed     int
	superseded int // rows edited after confirmation, which the merge yields to
	err        error
}

// handleInsightsMergeGenre arms the confirmation bar for the focused genre
// collision. Nothing is written until the user confirms.
func (m Model) handleInsightsMergeGenre() Model {
	if m.activeView != viewInsights {
		return m
	}
	row := m.insights.focusedHealthRow()
	if row == nil || row.kind != healthGenreCollision {
		m.statusBar.Notify("Only genre collisions can be merged")
		return m
	}
	n := len(row.hashes)
	m.insights.pendingMerge = &insightsMergePlan{
		canonical: row.canonical,
		variants:  row.variants,
		hashes:    row.hashes,
	}
	m.confirmationBar = fmt.Sprintf("Merge %s into %q on %d %s? (y to confirm)",
		strings.Join(row.variants, " / "), row.canonical, n, text.Pluralize(n, "track", "tracks"))
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
		m.statusBar.Notify("Merge canceled")
		return m, nil, true
	}
	return m.confirmInsightsGenreMerge()
}

// confirmInsightsGenreMerge plans every affected track's genre rewrite and
// returns the command that persists it. Nothing changes in memory here: the
// database goes first, and only rows it accepts are applied on the reply.
func (m Model) confirmInsightsGenreMerge() (Model, tea.Cmd, bool) {
	plan := *m.insights.pendingMerge
	m.insights.pendingMerge = nil
	m.confirmationBar = ""

	fold := make(map[string]struct{}, len(plan.variants))
	for _, v := range plan.variants {
		fold[insightsGenreKey(v)] = struct{}{}
	}

	var writes []insightsMergeWrite
	for _, hash := range plan.hashes {
		t, ok := m.library.Index.FindByHash(hash)
		if !ok {
			continue
		}
		merged := mergeGenreVariants(t.Genre, fold, plan.canonical)
		if merged == t.Genre {
			continue
		}
		writes = append(writes, insightsMergeWrite{
			hash: hash, title: t.Title, artist: t.Artist, album: t.Album, genre: merged,
			original: [5]string{t.Title, t.Artist, t.Album, t.Genre, metadata.FormatDuration(t.Duration)},
		})
	}

	if len(writes) == 0 {
		m.statusBar.Notify("Genres already agree")
		return m, nil, true
	}

	mw := m.metadataWriter
	if mw == nil {
		m.statusBar.Notify("Genre merge needs a database connection")
		return m, nil, true
	}

	// Each row takes its slot now, on the UI goroutine, so an edit handled while
	// the write below is in flight publishes a newer one and this stale full row
	// stands down rather than reverting it.
	q := m.metadataWrites
	for i := range writes {
		writes[i].ticket = q.Publish(writes[i].hash)
	}

	canonical := plan.canonical
	return m, func() tea.Msg {
		msg := insightsMergedMsg{canonical: canonical}
		for _, w := range writes {
			wrote, err := q.Claim(w.hash, w.ticket, func() error {
				return mw.UpdateTrackMetadata(w.hash, w.title, w.artist, w.album, w.genre)
			})
			switch {
			case !wrote:
				msg.superseded++
			case err != nil:
				slog.Warn("insights: genre merge DB write failed", "hash", w.hash, "err", err)
				msg.failed++
				if msg.err == nil {
					msg.err = err
				}
			default:
				msg.applied = append(msg.applied, w)
			}
		}
		return msg
	}, true
}

// handleInsightsMergedMsg installs the rows the database accepted and reloads
// the panels off them, so the collision row and the Rhythm banner it caused go
// away together for the set that persisted. A row whose write failed, or whose
// track was edited after the merge was planned, keeps its old spelling in
// memory, which is what keeps the collision reported and makes `m` a real retry
// rather than a no-op that claims the genres already agree.
func (m Model) handleInsightsMergedMsg(msg insightsMergedMsg) (Model, tea.Cmd) {
	originals := make(map[string][5]string, len(msg.applied))
	superseded := msg.superseded
	for _, w := range msg.applied {
		// The write is durable but this reply is handled later than it ran: an
		// edit processed in between owns the row now, and the newer write it
		// queued is what the database converges to.
		t, ok := m.library.Index.FindByHash(w.hash)
		if !ok || !w.planStillHolds(t) {
			superseded++
			continue
		}
		m.library.Index.UpdateMetadata(w.hash, w.title, w.artist, w.album, w.genre)
		originals[w.hash] = w.original
		if m.userEditedOverlay != nil {
			m.userEditedOverlay[w.hash] = db.UserEditedRow{
				Hash: w.hash, Title: w.title, Artist: w.artist, Album: w.album, Genre: w.genre,
				TrackNumber: t.TrackNumber, DiscNumber: t.DiscNumber,
			}
		}
	}

	done := len(originals)
	if done > 0 {
		m = m.syncRecentlyAddedFromLibrary()
		m.setUndoSlot(undoBulkEditMetadataAction{originals: originals}, nil)
		m = m.refreshDerivedViews()
	}

	lost := msg.failed + superseded
	switch {
	case done == 0 && msg.err != nil:
		m.statusBar.Notify(fmt.Sprintf("Genre merge failed, nothing changed: %s", msg.err))
	case done == 0 && lost > 0:
		m.statusBar.Notify(fmt.Sprintf("Genre merge stood down: %d %s changed while it ran · m retries",
			lost, text.Pluralize(lost, "track", "tracks")))
	case lost > 0:
		total := done + lost
		m.statusBar.Notify(fmt.Sprintf("Merged %d of %d %s into %s · m retries the rest",
			done, total, text.Pluralize(total, "track", "tracks"), msg.canonical))
	default:
		m.statusBar.Notify(fmt.Sprintf("Merged %d %s into %s",
			done, text.Pluralize(done, "track", "tracks"), msg.canonical))
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
