package overlay

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// NowPlayingOverlay is the full-screen Now Playing view: the cover beside the
// song, a wide progress bar, what plays next, and why Auto-DJ chose it.
type NowPlayingOverlay struct {
	Artist    string
	Title     string
	Album     string
	Elapsed   float64
	Total     float64
	State     playback.State
	PlayCount int
	TagsLine  string // pre-rendered colored tag names; empty when none

	// Cover is the pre-rendered cover art, CoverCols×CoverRows cells; empty
	// when there is none to show at this size.
	Cover                string
	CoverCols, CoverRows int
	// Details is a one-line listening summary (length, genre, plays).
	Details string
	// Modes names the active playback modes ("Repeat all · Shuffle").
	Modes string
	// Progress renders a progress bar of the given width; nil omits the bar.
	Progress func(width int) string
	// UpNext lists the next few songs, already labeled.
	UpNext []string
	// Reason explains why Auto-DJ picked the song; empty for manual picks.
	Reason string
}

const nowPlayingHint = "space play/pause · n/p skip · ←/→ seek · esc close"

// nowPlayingMargin is the blank space kept at the screen's left and right.
const nowPlayingMargin = 3

// Render lays out the view in termWidth×termHeight cells.
func (n NowPlayingOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	if termWidth <= 0 || termHeight <= 0 {
		return ""
	}
	hint := s.Dim.Render(text.CompactHints(nowPlayingHint, termWidth-2, "space", "esc"))
	bodyHeight := max(termHeight-2, 1)
	coverCols, coverRows := n.coverSize(termWidth, bodyHeight)
	panelWidth := termWidth - 2*nowPlayingMargin
	if coverCols > 0 {
		panelWidth -= coverCols + 4
	}
	panel := n.panelLines(max(panelWidth, 10), bodyHeight, s)
	lines := composeNowPlaying(n.Cover, coverCols, coverRows, panel, bodyHeight)
	return strings.Join(lines, "\n") + "\n\n" + " " + hint
}

// coverSize returns the cover's cell size, or zero when it is missing or the
// screen leaves too little room beside it for the text.
func (n NowPlayingOverlay) coverSize(termWidth, bodyHeight int) (cols, rows int) {
	if n.Cover == "" || n.CoverRows > bodyHeight || n.CoverCols+4+24 > termWidth {
		return 0, 0
	}
	return n.CoverCols, n.CoverRows
}

// composeNowPlaying centers the cover and the text column vertically, side by
// side, in exactly height lines.
func composeNowPlaying(cover string, coverCols, coverRows int, panel []string, height int) []string {
	block := max(len(panel), coverRows)
	top := max(0, (height-block)/2)
	panelTop := max(0, (block-len(panel))/2)
	var coverLines []string
	if coverCols > 0 {
		coverLines = strings.Split(cover, "\n")
	}
	lines := make([]string, top, height)
	for i := range block {
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", nowPlayingMargin))
		if coverCols > 0 {
			b.WriteString(lineOr(coverLines, i, strings.Repeat(" ", coverCols)))
			b.WriteString("    ")
		}
		b.WriteString(lineOr(panel, i-panelTop, ""))
		lines = append(lines, b.String())
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

func lineOr(lines []string, i int, fallback string) string {
	if i >= 0 && i < len(lines) {
		return lines[i]
	}
	return fallback
}

// panelLines builds the text column, dropping its least important sections
// first when the screen is short.
func (n NowPlayingOverlay) panelLines(width, height int, s styles.Styles) []string {
	sections := [][]string{n.headSection(width, s), n.aboutSection(width, s)}
	if len(n.UpNext) > 0 {
		next := []string{s.Dim.Bold(true).Render("Up next")}
		for i, label := range n.UpNext {
			next = append(next, s.TextTertiary.Render(text.Truncate(itoa(i+1)+"  "+label, width)))
		}
		sections = append(sections, next)
	}
	if n.Reason != "" {
		why := []string{s.Dim.Bold(true).Render("Why Auto-DJ chose it")}
		for _, line := range wrapLines(n.Reason, width, 2) {
			why = append(why, s.TextSecondary.Render(line))
		}
		sections = append(sections, why)
	}
	lines := sections[0]
	for _, section := range sections[1:] {
		if len(lines)+1+len(section) > height {
			break
		}
		lines = append(lines, "")
		lines = append(lines, section...)
	}
	return lines[:min(len(lines), height)]
}

// headSection is the song itself: title, artist, album, position and state.
func (n NowPlayingOverlay) headSection(width int, s styles.Styles) []string {
	title := n.Title
	if title == "" {
		title = "(unknown title)"
	}
	artist := n.Artist
	if artist == "" {
		artist = "(unknown artist)"
	}
	head := []string{s.Dim.Render("♪ Now Playing"), ""}
	for _, line := range wrapLines(title, width, 2) {
		head = append(head, s.AccentMark.Bold(true).Render(line))
	}
	head = append(head, s.TextPrimary.Render(text.Truncate(artist, width)))
	if n.Album != "" {
		head = append(head, s.TextSecondary.Render(text.Truncate(n.Album, width)))
	}
	return append(head, "", n.progressLine(width, s), n.stateLine(width, s))
}

// aboutSection is the song's listening history and tags.
func (n NowPlayingOverlay) aboutSection(width int, s styles.Styles) []string {
	details := n.Details
	if details == "" {
		details = "Play Count: " + text.FormatCount(n.PlayCount)
	}
	about := []string{s.Dim.Render(text.Truncate(details, width))}
	if n.TagsLine != "" {
		about = append(about, ansi.Truncate(n.TagsLine, width, "…"))
	}
	return about
}

func (n NowPlayingOverlay) progressLine(width int, s styles.Styles) string {
	elapsed := text.FormatPositionSecs(n.Elapsed)
	total := text.FormatPositionSecs(n.Total)
	barWidth := width - ansi.StringWidth(elapsed) - ansi.StringWidth(total) - 2
	if n.Progress == nil || barWidth < 4 {
		return s.TextPrimary.Render(elapsed + " / " + total)
	}
	return s.TextPrimary.Render(elapsed) + " " + n.Progress(barWidth) + " " + s.TextTertiary.Render(total)
}

func (n NowPlayingOverlay) stateLine(width int, s styles.Styles) string {
	var icon, label string
	style := s.Success
	switch n.State {
	case playback.StatePaused:
		icon, label, style = "⏸", "Paused", s.Warning
	case playback.StatePlaying:
		icon, label = "▶", "Playing"
	default:
		// Idle or any unrecognised state: avoid implying playback is in
		// progress.
		icon, label, style = "■", "Stopped", s.Error
	}
	line := style.Bold(true).Render(icon + " " + label)
	if n.Modes != "" {
		line += s.Dim.Render(text.Truncate(" · "+n.Modes, max(width-ansi.StringWidth(icon+" "+label), 0)))
	}
	return line
}

// wrapLines word-wraps value to width, keeping at most limit lines and
// ending a cut-off last line with an ellipsis.
func wrapLines(value string, width, limit int) []string {
	if width <= 0 {
		return nil
	}
	lines := strings.Split(ansi.Wrap(value, width, " "), "\n")
	if len(lines) > limit {
		lines[limit-1] = text.Truncate(strings.Join(lines[limit-1:], " "), width)
		lines = lines[:limit]
	}
	return lines
}

func itoa(i int) string { return text.FormatCount(i) }
