package overlay

import (
	"charm.land/lipgloss/v2"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// NowPlayingLyrics is the lyrics section of the Now Playing view: a window
// of lines that follows the song for synced lyrics, or plain text the
// listener scrolls for unsynced ones.
type NowPlayingLyrics struct {
	Lines  []string
	Synced bool
	// Current is the line being sung, or -1 before the first one and for
	// unsynced lyrics.
	Current int
	// Anchor is the line the window follows. For synced lyrics it trails
	// Current by a line or two while the window glides after it; for
	// unsynced lyrics it is the first line shown.
	Anchor int
	// Fade is how far the highlight has moved onto Current, from 0 to 1.
	Fade float64
	// Hidden keeps the section off screen while the song still has lyrics,
	// so the key hint can offer to show them.
	Hidden bool
}

// Lyrics window sizes: the section needs room for lyricsMinRows lines to be
// worth showing and grows with spare room up to lyricsMaxRows.
const (
	lyricsMinRows = 3
	lyricsMaxRows = 9
	// lyricsWrapRows is how many rows the current line may wrap to.
	lyricsWrapRows = 2
)

// lyricsGap stands in for an instrumental break while it plays.
const lyricsGap = "♪"

func (l *NowPlayingLyrics) visible() bool {
	return l != nil && !l.Hidden && len(l.Lines) > 0
}

// rowsWanted is the most rows the lyrics could use: every line, plus room
// for the current one to wrap, but no more than lyricsMaxRows.
func (l *NowPlayingLyrics) rowsWanted() int {
	return min(lyricsMaxRows, max(lyricsMinRows, len(l.Lines)+lyricsWrapRows-1))
}

// hint is what the key hint offers for the lyrics.
func (l *NowPlayingLyrics) hint() string {
	switch {
	case l == nil || len(l.Lines) == 0:
		return ""
	case l.Hidden:
		return " · L lyrics"
	case !l.Synced:
		return " · ↑/↓ lyrics · L hide"
	default:
		return " · L hide lyrics"
	}
}

// section renders the heading and rows lines of lyrics.
func (l *NowPlayingLyrics) section(width, rows int, s styles.Styles) []string {
	lines := []string{s.Dim.Bold(true).Render("Lyrics")}
	if !l.Synced {
		top := max(0, min(l.Anchor, len(l.Lines)-rows))
		for i := top; i < len(l.Lines) && len(lines) <= rows; i++ {
			lines = append(lines, s.TextSecondary.Render(text.Truncate(l.Lines[i], width)))
		}
		return lines
	}
	// Keep about a third of the window for lines already sung.
	top := max(0, min(max(l.Anchor, 0)-rows/3, len(l.Lines)-rows))
	for i := top; i < len(l.Lines) && len(lines) <= rows; i++ {
		lines = append(lines, l.syncedRows(i, width, s)...)
	}
	return lines[:min(len(lines), rows+1)]
}

// syncedRows renders line i: the current line bold in the accent color and
// wrapped if it is long, lines already sung dimmed, the next one plain and
// the ones after it a little dimmer.
func (l *NowPlayingLyrics) syncedRows(i, width int, s styles.Styles) []string {
	line := l.Lines[i]
	if i == l.Current {
		if line == "" {
			line = lyricsGap
		}
		style := s.AccentMark.Bold(true)
		if l.Fade < 1 {
			style = style.Foreground(styles.Mix(s.TextSecondary.GetForeground(), s.AccentMark.GetForeground(), l.Fade))
		}
		var out []string
		for _, row := range wrapLines(line, width, lyricsWrapRows) {
			out = append(out, style.Render(row))
		}
		return out
	}
	return []string{l.lineStyle(i, s).Render(text.Truncate(line, width))}
}

func (l *NowPlayingLyrics) lineStyle(i int, s styles.Styles) lipgloss.Style {
	switch {
	case i == l.Current-1 && l.Fade < 1:
		// The line just left eases from the accent back to dim.
		return s.Dim.Foreground(styles.Mix(s.AccentMark.GetForeground(), s.Dim.GetForeground(), l.Fade))
	case i < l.Current:
		return s.Dim
	case i == l.Current+1:
		return s.TextSecondary
	default:
		return s.TextTertiary
	}
}
