package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// The focused-track card's cover is a fixed size so its Kitty placement does
// not depend on how much spare space the current list leaves.
const (
	focusedCardArtRows = 4
	focusedCardArtCols = 2 * focusedCardArtRows
)

// Use otherwise empty space above the footer for the focused song, without
// taking rows from a full table or moving the cursor. Information follows focus,
// independently of the song playing in the status bar. With room for it, the
// card shows the cover beside the title and the song's listening details.
func (m Model) renderFocusedIdentity(maxLines int) string {
	if m.hasActiveOverlay() || maxLines < 2 {
		return ""
	}
	track, ok := m.focusedTrackForInspection()
	if !ok {
		return ""
	}
	if m.art != nil && maxLines >= focusedCardArtRows && m.termWidth >= 60 {
		return m.renderFocusedCard(track, maxLines)
	}
	title := focusedTitle(track)
	width := max(m.termWidth-2, 0)
	maxLines = min(maxLines, 4)
	titleLines := focusedIdentityLines("Focused · "+title, width, maxLines-1)
	contextLines := focusedIdentityLines(focusedContext(track), width, maxLines-len(titleLines))
	var lines []string
	for _, line := range titleLines {
		lines = append(lines, "  "+m.styles.TextSecondary.Render(line))
	}
	for _, line := range contextLines {
		lines = append(lines, "  "+m.styles.Dim.Render(line))
	}
	return strings.Join(lines, "\n")
}

func focusedTitle(track library.Track) string {
	if track.Title == "" {
		return filepath.Base(track.Path)
	}
	return track.Title
}

func focusedContext(track library.Track) string {
	var context []string
	for _, value := range []string{track.Artist, track.Album} {
		if value != "" {
			context = append(context, value)
		}
	}
	return strings.Join(context, " — ")
}

// focusedCardTrack is the track whose cover the focused card draws.
func (m Model) focusedCardTrack() (library.Track, bool) {
	if m.art == nil || m.hasActiveOverlay() {
		return library.Track{}, false
	}
	return m.focusedTrackForInspection()
}

func (m Model) focusedCardArtSize() (cols, rows int) {
	return focusedCardArtCols, focusedCardArtRows
}

// focusedCardMaxLines caps how tall the card grows to show a long title.
const focusedCardMaxLines = 6

// focusedCardText is the card's text column before styling.
type focusedCardText struct {
	title, context []string
	details, tags  string
}

func (c focusedCardText) lines() int {
	n := len(c.title) + len(c.context)
	if c.details != "" {
		n++
	}
	if c.tags != "" {
		n++
	}
	return n
}

// renderFocusedCard lays out the cover beside the title, artist and album,
// listening details, and tags. Long titles wrap rather than truncate while
// there is room: details and tags give way first.
func (m Model) renderFocusedCard(track library.Track, maxLines int) string {
	cover := m.renderArt(track, focusedCardArtCols, focusedCardArtRows)
	if cover == "" {
		cover = artPlaceholder(*m.styles, focusedCardArtCols, focusedCardArtRows)
	}
	textWidth := max(m.termWidth-2-focusedCardArtCols-2, 1)
	card := fitFocusedCard(focusedCardText{
		title:   focusedIdentityLines("Focused · "+focusedTitle(track), textWidth, 3),
		context: focusedIdentityLines(focusedContext(track), textWidth, 2),
		details: trackDetailsLine(track, time.Now()),
		tags:    m.buildTagsLine(track.Hash),
	}, min(maxLines, focusedCardMaxLines), textWidth)
	lines := m.styleFocusedCard(card, textWidth)

	coverLines := strings.Split(cover, "\n")
	height := max(len(lines), focusedCardArtRows)
	out := make([]string, 0, height)
	for i := range height {
		row := strings.Repeat(" ", focusedCardArtCols)
		if i < len(coverLines) {
			row = coverLines[i]
		}
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, "  "+row+"  "+line)
	}
	return strings.Join(out, "\n")
}

// fitFocusedCard trims the card to budget lines: tags and details give way
// before a long title or context loses its wrapped lines.
func fitFocusedCard(c focusedCardText, budget, width int) focusedCardText {
	for c.lines() > budget {
		switch {
		case c.tags != "" && len(c.title)+len(c.context) > 2:
			c.tags = ""
		case c.details != "" && len(c.title)+len(c.context) > 2:
			c.details = ""
		case len(c.context) > 1:
			c.context = shortenWrapped(c.context, width)
		case len(c.title) > 1:
			c.title = shortenWrapped(c.title, width)
		case c.tags != "":
			c.tags = ""
		default:
			c.details = ""
		}
	}
	return c
}

// shortenWrapped drops a wrapped value's last line, ending the new last line
// with an ellipsis.
func shortenWrapped(lines []string, width int) []string {
	return focusedIdentityLines(strings.Join(lines, " "), width, len(lines)-1)
}

func (m Model) styleFocusedCard(c focusedCardText, width int) []string {
	title := m.styles.TextPrimary.Bold(true)
	var lines []string
	for i, line := range c.title {
		if rest, ok := strings.CutPrefix(line, "Focused · "); ok && i == 0 {
			lines = append(lines, m.styles.Dim.Render("Focused · ")+title.Render(rest))
			continue
		}
		lines = append(lines, title.Render(line))
	}
	for _, line := range c.context {
		lines = append(lines, m.styles.TextSecondary.Render(line))
	}
	if c.details != "" {
		lines = append(lines, m.styles.Dim.Render(text.Truncate(c.details, width)))
	}
	if c.tags != "" {
		lines = append(lines, ansi.Truncate(c.tags, width, "…"))
	}
	return lines
}

// artPlaceholder fills a cover slot that has no art (or none yet) with a
// quiet tile, so the card's layout never shifts when a cover arrives.
func artPlaceholder(s styles.Styles, cols, rows int) string {
	tile := s.Dim.Background(styles.Mix(s.Background, s.Primary, 0.10))
	lines := make([]string, rows)
	for i := range lines {
		content := strings.Repeat(" ", cols)
		if i == (rows-1)/2 {
			content = text.Pad(strings.Repeat(" ", (cols-1)/2)+"♪", cols)
		}
		lines[i] = tile.Render(content)
	}
	return strings.Join(lines, "\n")
}

// trackDetailsLine summarizes a track's length, genre and listening history.
func trackDetailsLine(track library.Track, now time.Time) string {
	var parts []string
	if track.Duration > 0 {
		parts = append(parts, text.FormatDurationSecs(track.Duration))
	}
	if track.Genre != "" {
		parts = append(parts, track.Genre)
	}
	switch track.PlayCount {
	case 0:
		parts = append(parts, "never played")
	case 1:
		parts = append(parts, "played once")
	default:
		parts = append(parts, fmt.Sprintf("played %d times", track.PlayCount))
	}
	if track.LastPlayedAt > 0 {
		parts = append(parts, "last "+relativeAgo(time.Unix(track.LastPlayedAt, 0), now))
	}
	return strings.Join(parts, " · ")
}

// relativeAgo phrases how long ago t was, coarsely.
func relativeAgo(t, now time.Time) string {
	d := now.Sub(t)
	days := int(d.Hours() / 24)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	case days < 2:
		return "yesterday"
	case days < 14:
		return fmt.Sprintf("%d days ago", days)
	case days < 60:
		return fmt.Sprintf("%d weeks ago", days/7)
	case days < 730:
		return fmt.Sprintf("%d months ago", days/30)
	default:
		return fmt.Sprintf("%d years ago", days/365)
	}
}

func focusedIdentityLines(value string, width, budget int) []string {
	lines := strings.Split(ansi.Wrap(value, width, " "), "\n")
	if len(lines) > budget {
		lines[budget-1] = text.Truncate(strings.Join(lines[budget-1:], " "), width)
		lines = lines[:budget]
	}
	return lines
}
