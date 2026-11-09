package overlay

import (
	"slices"
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
	// Loudness renders the loudness strip drawn above the progress bar, the
	// bar's width; nil omits it. It is the first thing to go when the screen
	// is short.
	Loudness func(width int) string
	// SeekPreview, when set, is the direction arrow of a seek still being
	// dialed in; Elapsed then holds its target, drawn highlighted.
	SeekPreview string
	// UpNext lists the next few songs.
	UpNext []UpNextSong
	// Reason explains why Auto-DJ picked the song; empty for manual picks.
	Reason string
	// Lyrics are the song's lyrics; nil when it has none. See
	// nowplaying_lyrics.go.
	Lyrics *NowPlayingLyrics

	// Marquee scrolls a title too wide for the panel instead of truncating
	// it; TitleScroll is how many marquee steps have passed since the title
	// appeared.
	Marquee     bool
	TitleScroll int

	// Notice replaces the key hint at the bottom while it is set, so
	// confirmations and errors stay visible without the status bar.
	Notice      string
	NoticeError bool
}

// UpNextSong is one song in the Now Playing view's "Up next" list.
type UpNextSong struct {
	Title, Artist string
}

const nowPlayingHint = "space play/pause · n/p skip · ←/→ seek · esc close"

// nowPlayingMargin is the least blank space kept at the screen's left and
// right; nowPlayingGap separates the cover from the text.
const (
	nowPlayingMargin = 3
	nowPlayingGap    = 4
)

// nowPlayingPanelMax caps the text column's width, so on a wide screen the
// composition stays together in the middle instead of stretching the
// progress bar across the whole terminal.
const nowPlayingPanelMax = 56

// The marquee holds the start of a long title for marqueePauseSteps steps,
// then moves one cell per step, and loops with marqueeGap blank cells
// between the end of the title and its start.
const (
	marqueePauseSteps = 8
	marqueeGap        = 6
)

// Render lays out the view in termWidth×termHeight cells.
func (n NowPlayingOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	if termWidth <= 0 || termHeight <= 0 {
		return ""
	}
	bodyHeight, coverCols, coverRows, panelWidth := n.layout(termWidth, termHeight)
	panel, _ := n.panelLines(panelWidth, bodyHeight, s)
	lines := composeNowPlaying(n.Cover, coverCols, coverRows, panel, panelWidth, termWidth, bodyHeight)
	return strings.Join(lines, "\n") + "\n\n" + " " + n.footerLine(termWidth-2, s)
}

// ShowsLoudness reports whether Render at this size would draw the loudness
// strip, so the caller can stop measuring levels nobody sees.
func (n NowPlayingOverlay) ShowsLoudness(termWidth, termHeight int, s styles.Styles) bool {
	if n.Loudness == nil || termWidth <= 0 || termHeight <= 0 {
		return false
	}
	bodyHeight, _, _, panelWidth := n.layout(termWidth, termHeight)
	_, shown := n.panelLines(panelWidth, bodyHeight, s)
	return shown
}

// layout sizes the view: the rows for the body, the cover, and the text
// column's width.
func (n NowPlayingOverlay) layout(termWidth, termHeight int) (bodyHeight, coverCols, coverRows, panelWidth int) {
	bodyHeight = max(termHeight-2, 1)
	coverCols, coverRows = n.coverSize(termWidth, bodyHeight)
	panelWidth = termWidth - 2*nowPlayingMargin
	if coverCols > 0 {
		panelWidth -= coverCols + nowPlayingGap
	}
	return bodyHeight, coverCols, coverRows, max(min(panelWidth, nowPlayingPanelMax), 10)
}

// footerLine is the key hint, or the notice standing in for it.
func (n NowPlayingOverlay) footerLine(width int, s styles.Styles) string {
	if n.Notice == "" {
		return s.Dim.Render(text.CompactHints(nowPlayingHint+n.Lyrics.hint(), width, "space", "esc"))
	}
	style := s.Warning
	if n.NoticeError {
		style = s.Error
	}
	return style.Render(text.Truncate(n.Notice, width))
}

// coverSize returns the cover's cell size, or zero when it is missing or the
// screen leaves too little room beside it for the text.
func (n NowPlayingOverlay) coverSize(termWidth, bodyHeight int) (cols, rows int) {
	if n.Cover == "" || n.CoverRows > bodyHeight || n.CoverCols+nowPlayingGap+24 > termWidth {
		return 0, 0
	}
	return n.CoverCols, n.CoverRows
}

// composeNowPlaying centers the cover and the text column side by side, both
// horizontally and vertically, in exactly height lines.
func composeNowPlaying(cover string, coverCols, coverRows int, panel []string, panelWidth, width, height int) []string {
	block := max(len(panel), coverRows)
	top := max(0, (height-block)/2)
	panelTop := max(0, (block-len(panel))/2)
	blockWidth := panelWidth
	var coverLines []string
	if coverCols > 0 {
		coverLines = strings.Split(cover, "\n")
		blockWidth += coverCols + nowPlayingGap
	}
	left := strings.Repeat(" ", max(nowPlayingMargin, (width-blockWidth)/2))
	lines := make([]string, top, height)
	for i := range block {
		var b strings.Builder
		b.WriteString(left)
		if coverCols > 0 {
			b.WriteString(lineOr(coverLines, i, strings.Repeat(" ", coverCols)))
			b.WriteString(strings.Repeat(" ", nowPlayingGap))
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
// first when the screen is short. Shown lyrics come right after the song
// itself: they keep their smallest window before the details, up next and
// Auto-DJ's reason get room, and then grow into whatever is left. They sit
// between the details and up next on screen. The loudness strip is the least
// important of all: it goes in above the progress bar only when everything
// else fits with a row to spare, and lyrics leave it that row. loudness
// reports whether it went in.
func (n NowPlayingOverlay) panelLines(width, height int, s styles.Styles) (lines []string, loudness bool) {
	head := n.headSection(width, s)
	used := len(head)
	lyricsRows := 0
	if n.Lyrics.visible() && used+2+lyricsMinRows <= height {
		lyricsRows = lyricsMinRows
		used += 2 + lyricsMinRows
	}
	all := [][]string{n.aboutSection(width, s), n.upNextSection(width, s), n.reasonSection(width, s)}
	sections, used := sectionsThatFit(all, used, height)
	strip := n.loudnessLine(width)
	loudness = strip != "" && len(sections) == len(all) && used < height
	spare := height - used
	if loudness {
		spare--
	}
	if lyricsRows > 0 {
		lyricsRows += max(0, min(spare, n.Lyrics.rowsWanted()-lyricsRows))
		sections = slices.Insert(sections, min(1, len(sections)), n.Lyrics.section(width, lyricsRows, s))
	}
	lines = head
	for _, section := range sections {
		lines = appendSection(lines, section)
	}
	if loudness {
		// The head ends with the progress line and the state line.
		lines = slices.Insert(lines, len(head)-2, strip)
	}
	return lines[:min(len(lines), height)], loudness
}

// loudnessLine is the loudness strip indented to sit exactly over the
// progress bar, or "" when there is none or the bar has no room.
func (n NowPlayingOverlay) loudnessLine(width int) string {
	if n.Loudness == nil || n.Progress == nil {
		return ""
	}
	lead, barWidth := n.progressBarSpan(width)
	if barWidth < 4 {
		return ""
	}
	return strings.Repeat(" ", lead) + n.Loudness(barWidth)
}

// sectionsThatFit keeps sections, in order, while each still fits under a
// blank line in height with used lines taken; it returns them and the lines
// then in use.
func sectionsThatFit(sections [][]string, used, height int) ([][]string, int) {
	for i, section := range sections {
		if len(section) > 0 && used+1+len(section) > height {
			return sections[:i], used
		}
		if len(section) > 0 {
			used += 1 + len(section)
		}
	}
	return sections, used
}

// appendSection adds a section under a blank line; an empty one adds nothing.
func appendSection(lines, section []string) []string {
	if len(section) == 0 {
		return lines
	}
	return append(append(lines, ""), section...)
}

func (n NowPlayingOverlay) upNextSection(width int, s styles.Styles) []string {
	if len(n.UpNext) == 0 {
		return nil
	}
	next := []string{s.Dim.Bold(true).Render("Up next")}
	for i, song := range n.UpNext {
		next = append(next, upNextLine(i, song, width, s))
	}
	return next
}

func (n NowPlayingOverlay) reasonSection(width int, s styles.Styles) []string {
	if n.Reason == "" {
		return nil
	}
	why := []string{s.Dim.Bold(true).Render("Why Auto-DJ chose it")}
	for _, line := range wrapLines(n.Reason, width, 2) {
		why = append(why, s.TextSecondary.Render(line))
	}
	return why
}

// upNextMinArtist is the narrowest artist worth keeping beside a title; with
// less room the row shows the title alone.
const upNextMinArtist = 6

// upNextLine renders one upcoming song title first, with its artist dimmed
// after it. The artist gives way before the title does.
func upNextLine(i int, song UpNextSong, width int, s styles.Styles) string {
	prefix := itoa(i+1) + "  "
	room := max(width-ansi.StringWidth(prefix), 0)
	title := song.Title
	sep := " · "
	artistRoom := room - ansi.StringWidth(title) - ansi.StringWidth(sep)
	line := s.TextTertiary.Render(prefix)
	if song.Artist == "" || artistRoom < min(upNextMinArtist, ansi.StringWidth(song.Artist)) {
		return line + s.TextSecondary.Render(text.Truncate(title, room))
	}
	return line + s.TextSecondary.Render(title) + s.Dim.Render(sep+text.Truncate(song.Artist, artistRoom))
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
	head = append(head, s.AccentMark.Bold(true).Render(n.titleLine(title, width)))
	head = append(head, s.TextPrimary.Render(text.Truncate(artist, width)))
	if n.Album != "" {
		head = append(head, s.TextSecondary.Render(text.Truncate(n.Album, width)))
	}
	return append(head, "", n.progressLine(width, s), n.stateLine(width, s))
}

// titleLine fits the title on one line: whole when it fits, scrolling as a
// marquee when that is enabled, and truncated otherwise.
func (n NowPlayingOverlay) titleLine(title string, width int) string {
	if ansi.StringWidth(title) <= width {
		return title
	}
	if !n.Marquee {
		return text.Truncate(title, width)
	}
	return marqueeWindow(title, width, n.TitleScroll)
}

// marqueeWindow returns the width cells of a scrolling title visible step
// steps after it appeared: the start for marqueePauseSteps steps, then one
// cell further each step, wrapping around with a gap back to the start,
// where it pauses again.
func marqueeWindow(title string, width, step int) string {
	titleWidth := ansi.StringWidth(title)
	if width <= 0 || titleWidth <= width {
		return title
	}
	cycle := titleWidth + marqueeGap
	offset := max(0, step%(marqueePauseSteps+cycle)-marqueePauseSteps)
	loop := title + strings.Repeat(" ", marqueeGap) + title
	// A wide character straddling an edge can stretch or shrink the cut by a
	// cell; trim and pad so the line keeps its width and does not jitter.
	window := ansi.Truncate(ansi.Cut(loop, offset, offset+width), width, "")
	return window + strings.Repeat(" ", max(0, width-ansi.StringWidth(window)))
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

// progressBarSpan is where the progress bar sits in the progress line: the
// cells before it (the elapsed time and a space) and its width.
func (n NowPlayingOverlay) progressBarSpan(width int) (lead, barWidth int) {
	elapsed := ansi.StringWidth(n.elapsedLabel())
	total := ansi.StringWidth(text.FormatPositionSecs(n.Total))
	return elapsed + 1, width - elapsed - total - 2
}

// elapsedLabel is the elapsed time, led by the direction arrow while a seek
// is still being dialed in.
func (n NowPlayingOverlay) elapsedLabel() string {
	elapsed := text.FormatPositionSecs(n.Elapsed)
	if n.SeekPreview != "" {
		return n.SeekPreview + " " + elapsed
	}
	return elapsed
}

func (n NowPlayingOverlay) progressLine(width int, s styles.Styles) string {
	elapsed := n.elapsedLabel()
	total := text.FormatPositionSecs(n.Total)
	elapsedStyle := s.TextPrimary
	if n.SeekPreview != "" {
		elapsedStyle = s.AccentMark.Bold(true)
	}
	_, barWidth := n.progressBarSpan(width)
	if n.Progress == nil || barWidth < 4 {
		return elapsedStyle.Render(elapsed) + s.TextPrimary.Render(" / "+total)
	}
	return elapsedStyle.Render(elapsed) + " " + n.Progress(barWidth) + " " + s.TextTertiary.Render(total)
}

func (n NowPlayingOverlay) stateLine(width int, s styles.Styles) string {
	var icon, label string
	style := s.NowPlaying
	switch n.State {
	case playback.StatePaused:
		icon, label, style = "⏸", "Paused", s.TextSecondary
	case playback.StatePlaying:
		icon, label = "▶", "Playing"
	default:
		// Idle or any unrecognised state: avoid implying playback is in
		// progress.
		icon, label, style = "■", "Stopped", s.TextTertiary
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
