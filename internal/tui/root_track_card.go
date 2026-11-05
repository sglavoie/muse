package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// The track card is a small "Now playing" card that appears over the main
// views for a few seconds when the song changes on its own or with n/p. A
// song the user just picked with enter is already on screen, so it gets no
// card; neither does the song restored at startup.
const (
	trackCardDuration = 4 * time.Second
	trackCardSlide    = 220 * time.Millisecond
	trackCardArtRows  = 3
	trackCardArtCols  = 2 * trackCardArtRows
	trackCardMaxWidth = 46
	trackCardRows     = trackCardArtRows + 2 // cover rows plus the border
)

// trackCardState is the card on screen, if any: the song it announces and
// when it appeared. gen ties the expiry tick to the card it was set for.
type trackCardState struct {
	hash    string
	shownAt time.Time
	gen     int
}

// trackCardExpiredMsg removes the card it was scheduled for.
type trackCardExpiredMsg struct{ gen int }

// announceTrackChange shows the card for a song the player moved to by
// itself. prevHash is the song before the change; an empty one means nothing
// was playing (startup restore, or the first song of a session), and an equal
// one means the change was already shown, as an explicit selection is.
func (m Model) announceTrackChange(prevHash, hash string) (Model, tea.Cmd) {
	if !m.cfg.TrackChangeCard || prevHash == "" || hash == "" || hash == prevHash || m.hasActiveOverlay() {
		return m, nil
	}
	m.trackCard.gen++
	m.trackCard.hash, m.trackCard.shownAt = hash, time.Now()
	gen := m.trackCard.gen
	return m, tea.Tick(trackCardDuration, func(time.Time) tea.Msg { return trackCardExpiredMsg{gen: gen} })
}

func (m Model) handleTrackCardExpired(msg trackCardExpiredMsg) Model {
	if msg.gen == m.trackCard.gen {
		m.trackCard.hash = ""
	}
	return m
}

// dismissTrackCard removes the card, as any key press does.
func (m Model) dismissTrackCard() Model {
	m.trackCard.hash = ""
	return m
}

// trackCardTrack returns the song the card shows while it is on screen.
func (m Model) trackCardTrack(now time.Time) (library.Track, bool) {
	c := m.trackCard
	if c.hash == "" || c.hash != m.playback.currentHash || now.Sub(c.shownAt) >= trackCardDuration ||
		!m.cfg.TrackChangeCard || m.hasActiveOverlay() {
		return library.Track{}, false
	}
	return m.library.Index.FindByHash(c.hash)
}

func (m Model) trackCardSliding(now time.Time) bool {
	return m.trackCard.hash != "" && now.Sub(m.trackCard.shownAt) < trackCardSlide
}

// trackCardWidth is the card's full width at the current terminal width.
func (m Model) trackCardWidth() int {
	return min(trackCardMaxWidth, m.termWidth*55/100)
}

// placeTrackCard draws the card over the right side of body, where it hides
// the least: in blank space above the footer when there is some, else at the
// top of the list, else at its bottom, never over the focused row. With room
// for neither it is not drawn. headerLines is how many title-bar lines body
// starts with.
func (m Model) placeTrackCard(body string, headerLines int, now time.Time) string {
	track, ok := m.trackCardTrack(now)
	if !ok {
		return body
	}
	width := m.trackCardWidth()
	lines := strings.Split(body, "\n")
	x := m.termWidth - width - 1
	if width < 24 || x < 0 || len(lines) < headerLines+trackCardRows+2 {
		return body
	}
	row, ok := m.trackCardRow(lines, headerLines+1, x)
	if !ok {
		return body
	}
	card := m.renderTrackCard(track, width)
	visible := width
	if m.motion.enabled && now.Sub(m.trackCard.shownAt) < trackCardSlide {
		visible = max(1, int(float64(width)*easeOutCubic(now.Sub(m.trackCard.shownAt).Seconds()/trackCardSlide.Seconds())))
	}
	for i, cardLine := range card {
		lines[row+i] = overlayRight(lines[row+i], ansi.Cut(cardLine, 0, visible), m.termWidth-1-visible)
	}
	return strings.Join(lines, "\n")
}

// trackCardRow picks the first body row the card covers: blank space at the
// bottom first, then the top of the list (from row top), then its bottom, as
// long as the focused row is not among them.
func (m Model) trackCardRow(lines []string, top, x int) (int, bool) {
	bottom := len(lines) - trackCardRows
	focus := m.focusRowMarker()
	switch {
	case rowsBlankFrom(lines[bottom:], x):
		return bottom, true
	case !rowsContain(lines[top:top+trackCardRows], focus):
		return top, true
	case !rowsContain(lines[bottom:], focus):
		return bottom, true
	default:
		return 0, false
	}
}

// overlayRight draws segment over line from column x, keeping what lies to
// its left and clearing the margin to its right, so no clipped column peeks
// out beside the card.
func overlayRight(line, segment string, x int) string {
	left := ansi.Truncate(line, x, "")
	if pad := x - ansi.StringWidth(left); pad > 0 {
		left += strings.Repeat(" ", pad)
	}
	return left + "\x1b[m" + segment + "\x1b[m "
}

// focusRowMarker is the escape sequence fragment that colors the focused
// row's background, which is how the card finds the row it must not cover.
func (m Model) focusRowMarker() string {
	r, g, b, _ := m.styles.Accent.GetBackground().RGBA()
	return fmt.Sprintf("48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

func rowsContain(lines []string, marker string) bool {
	for _, line := range lines {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

// rowsBlankFrom reports whether every line is empty from column x on.
func rowsBlankFrom(lines []string, x int) bool {
	for _, line := range lines {
		if strings.TrimSpace(ansi.Strip(ansi.Cut(line, x, x+trackCardMaxWidth+1))) != "" {
			return false
		}
	}
	return true
}

// renderTrackCard draws the card: a rounded frame around the small cover and
// "Now playing", the title and the artist, on a raised surface so it reads as
// separate from the list beneath it.
func (m Model) renderTrackCard(track library.Track, width int) []string {
	s := *m.styles
	surface := styles.Mix(s.Background, s.Primary, 0.10)
	paint := func(st lipgloss.Style, v string) string { return st.Background(surface).Render(v) }
	frame := s.AccentMark
	inner := width - 2
	var cover []string
	textWidth := inner - 2
	if m.art != nil {
		art := m.renderArt(track, trackCardArtCols, trackCardArtRows)
		if art == "" {
			art = artPlaceholder(s, trackCardArtCols, trackCardArtRows)
		}
		cover = strings.Split(art, "\n")
		textWidth -= trackCardArtCols + 2
	}
	textWidth = max(textWidth, 1)
	rows := []string{
		paint(s.Dim, text.Pad("♪ Now playing", textWidth)),
		paint(s.AccentMark.Bold(true), text.Pad(text.Truncate(focusedTitle(track), textWidth), textWidth)),
		paint(s.TextSecondary, text.Pad(text.Truncate(focusedContext(track), textWidth), textWidth)),
	}
	out := []string{paint(frame, "╭"+strings.Repeat("─", inner)+"╮")}
	for i, row := range rows {
		line := paint(frame, "│") + paint(s.Dim, " ")
		if cover != nil {
			line += lineOrBlank(cover, i, trackCardArtCols) + paint(s.Dim, "  ")
		}
		out = append(out, line+row+paint(s.Dim, " ")+paint(frame, "│"))
	}
	return append(out, paint(frame, "╰"+strings.Repeat("─", inner)+"╯"))
}

func lineOrBlank(lines []string, i, width int) string {
	if i < len(lines) {
		return lines[i]
	}
	return strings.Repeat(" ", width)
}
