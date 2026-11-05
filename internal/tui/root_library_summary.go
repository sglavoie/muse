package tui

import (
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/text"
)

// renderLibrarySummary describes the whole collection in one quiet line for
// the space below a Library list that does not fill the screen: its size,
// total length, how much is still unheard, and when music last arrived. It
// only appears for the unfiltered library, where the numbers describe what
// is on screen.
func (m Model) renderLibrarySummary() string {
	if !m.showsLibrarySummary() {
		return ""
	}
	tracks := m.library.Index.ReadOnlyTracks()
	if len(tracks) == 0 {
		return ""
	}
	totalSecs, unheard, newest := librarySummaryStats(tracks)
	parts := []string{
		text.FormatCount(len(tracks)) + " " + text.Pluralize(len(tracks), "track", "tracks"),
		text.FormatListeningTime(int(totalSecs)),
	}
	switch unheard {
	case 0:
		parts = append(parts, "all heard")
	case len(tracks):
		parts = append(parts, "none played yet")
	default:
		parts = append(parts, text.FormatCount(unheard)+" never played")
	}
	if newest > 0 {
		parts = append(parts, "newest added "+addedAgo(time.Unix(newest, 0), time.Now()))
	}
	return "  " + m.styles.Dim.Render(text.Truncate(strings.Join(parts, " · "), max(m.termWidth-2, 0)))
}

// showsLibrarySummary reports whether the Library is showing the whole,
// unfiltered collection, the only case its summary describes.
func (m Model) showsLibrarySummary() bool {
	return m.ActiveView == viewLibrary && !m.hasActiveOverlay() && m.library.Mode == libtui.LibraryModeAll &&
		!m.library.Filter.Active && m.library.Filter.Text == "" && m.library.TagFilterHashes == nil
}

// librarySummaryStats totals the collection's length, its never-played
// tracks, and its newest addition.
func librarySummaryStats(tracks []library.Track) (totalSecs float64, unheard int, newest int64) {
	for _, t := range tracks {
		totalSecs += max(t.Duration, 0)
		if t.PlayCount == 0 {
			unheard++
		}
		newest = max(newest, t.CreatedAt)
	}
	return totalSecs, unheard, newest
}

// addedAgo phrases an addition date: "today" reads better than "3 h ago"
// for when music arrived.
func addedAgo(t, now time.Time) string {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return "today"
	}
	return relativeAgo(t, now)
}
