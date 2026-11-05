package tui

import (
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Recap motion: a page eases in from recapRevealFloor over recapRevealDuration
// and its numbers count up over recapCountDuration. Both only run while
// motion is enabled, so fixtures and reduce_motion draw the final frame.
const (
	recapRevealDuration = 220 * time.Millisecond
	recapCountDuration  = 650 * time.Millisecond
	recapRevealFloor    = 0.25
)

// Recap layout: the screen keeps a header and a blank row on top, and a
// blank row, the page indicator and the key hint at the bottom.
const (
	recapChromeRows = 5
	recapMargin     = 3
	recapGap        = 4
	recapPanelMax   = 56
	recapCenterMax  = 64
)

const recapHint = "space/→ next · ← back · [/] year · S save top 50 · esc close"

// recapCover is one cover a recap page draws.
type recapCover struct {
	track      library.Track
	cols, rows int
}

// recapCard is a page's content before it is placed on the screen.
type recapCard struct {
	lines  []string
	center bool        // center every line, rather than the block as a whole
	cover  *recapCover // drawn to the left of the lines
}

// recapAnimating reports whether a recap page is still easing in or counting
// up, which keeps the motion loop on its transition rate.
func (m Model) recapAnimating(now time.Time) bool {
	o := m.overlay.recap
	return o != nil && m.motion.enabled && !o.shownAt.IsZero() && now.Sub(o.shownAt) < recapCountDuration
}

// recapProgress is how far a page's entrance has run after d, from 0 to 1.
func (m Model) recapProgress(now time.Time, d time.Duration) float64 {
	o := m.overlay.recap
	if o == nil || !m.motion.enabled || o.shownAt.IsZero() {
		return 1
	}
	return easeOutCubic(now.Sub(o.shownAt).Seconds() / d.Seconds())
}

func (m Model) recapBodySize() (width, height int) {
	return m.termWidth, max(m.termHeight-recapChromeRows, 1)
}

// recapBigCoverSize is the cover a featured album or song gets, or zero on a
// screen too small to give it room beside its text.
func (m Model) recapBigCoverSize() (cols, rows int) {
	width, height := m.recapBodySize()
	rows = min(height-2, 20, (width*40/100)/2)
	if rows < 6 || width < 70 {
		return 0, 0
	}
	return 2 * rows, rows
}

// recapThumbRows is how tall each top artist's album thumbnail is; zero
// lists the artists one per line.
func (m Model) recapThumbRows() int {
	_, height := m.recapBodySize()
	switch {
	case m.termWidth < 70 || m.art == nil:
		return 0
	case height >= 21:
		return 3
	case height >= 16:
		return 2
	}
	return 0
}

// recapCovers lists the covers the current page draws, at the size it draws
// them, for both the renderer and the Kitty uploads.
func (m Model) recapCovers() []recapCover {
	o := m.overlay.recap
	if o == nil || o.loading || m.art == nil {
		return nil
	}
	if o.currentPage() == recapPageArtists {
		rows := m.recapThumbRows()
		if rows == 0 {
			return nil
		}
		var out []recapCover
		for _, a := range o.data.TopArtists {
			if t, ok := m.library.Index.FindByHash(a.Hash); ok && a.Hash != "" {
				out = append(out, recapCover{track: t, cols: 2 * rows, rows: rows})
			}
		}
		return out
	}
	if c := m.recapFeatureCover(); c != nil {
		return []recapCover{*c}
	}
	return nil
}

// recapFeatureHash is the track a cover page features, or "" for a page
// without a big cover.
func (o *recapOverlay) recapFeatureHash() string {
	switch o.currentPage() {
	case recapPageAlbum:
		return o.data.TopAlbum.Hash
	case recapPageDiscovery:
		return o.data.Discovery.Hash
	case recapPageRepeat:
		return o.data.RepeatDay.Hash
	}
	return ""
}

func (m Model) recapFeatureCover() *recapCover {
	o := m.overlay.recap
	if o == nil || m.art == nil {
		return nil
	}
	hash := o.recapFeatureHash()
	cols, rows := m.recapBigCoverSize()
	if hash == "" || cols == 0 {
		return nil
	}
	t, ok := m.library.Index.FindByHash(hash)
	if !ok {
		return nil
	}
	return &recapCover{track: t, cols: cols, rows: rows}
}

// recapCoverArt draws c, or a quiet tile while its cover is missing, so the
// page does not shift when the cover arrives.
func (m Model) recapCoverArt(c recapCover, st styles.Styles) string {
	if art := m.renderArt(c.track, c.cols, c.rows); art != "" {
		return art
	}
	return artPlaceholder(st, c.cols, c.rows)
}

// recapStyles are the styles a page draws with: on a cover page, the
// cover's accent and a background leaning toward it, the way Now Playing
// follows the playing cover.
func (m Model) recapStyles() styles.Styles {
	st := *m.styles
	o := m.overlay.recap
	if o == nil || !m.cfg.ArtworkAccent || m.art == nil {
		return st
	}
	hash := o.recapFeatureHash()
	if hash == "" {
		return st
	}
	t, ok := m.library.Index.FindByHash(hash)
	if !ok {
		return st
	}
	art, _ := m.artFor(t)
	if art == nil || !art.HasAccent {
		return st
	}
	themed := styles.NewTheme(st.HasDark, styles.Hex(art.Accent))
	themed.Background = recapTint(themed)
	return themed
}

// recapTint leans the canvas toward the accent as far as the text stays
// readable on it.
func recapTint(s styles.Styles) color.Color {
	for amount := nowPlayingTintAmount; amount > nowPlayingTintStep/2; amount -= nowPlayingTintStep {
		tint := styles.Mix(s.Background, s.Primary, amount)
		if tintReadable(s, tint) {
			return tint
		}
	}
	return s.Background
}

// renderRecapScreen draws the recap across the whole terminal, like Now
// Playing: it owns the screen, status bar included.
func (m Model) renderRecapScreen() tea.View {
	now := time.Now()
	st := m.recapStyles()
	body := m.renderRecapBody(st, now)
	reveal, from, to := m.overlayReveal(now), 0, m.termHeight
	if page := recapRevealFloor + (1-recapRevealFloor)*m.recapProgress(now, recapRevealDuration); page < reveal {
		reveal, from, to = page, 2, m.termHeight-3
	}
	v := tea.NewView(st.RenderScreenFaded(body, m.termWidth, m.termHeight, reveal, from, to))
	v.AltScreen = true
	v = m.finishView(v)
	if v.BackgroundColor != nil {
		v.BackgroundColor = st.Background
	}
	return v
}

// renderRecapBody lays out the header, the current card, the page
// indicator and the key hint in exactly termHeight lines.
func (m Model) renderRecapBody(st styles.Styles, now time.Time) string {
	o := m.overlay.recap
	if o == nil || m.termWidth <= 0 || m.termHeight <= 0 {
		return ""
	}
	width, height := m.recapBodySize()
	lines := []string{m.recapHeader(st, width), ""}
	var card recapCard
	if o.loading {
		card = recapCard{lines: []string{st.Dim.Render("Gathering your listening history…")}, center: true}
	} else {
		card = m.recapCard(st, now)
	}
	lines = append(lines, m.composeRecapCard(card, st, width, height)...)
	lines = append(lines, "", m.recapIndicator(st, width), " "+m.recapFooter(st, width-2))
	return strings.Join(lines[:min(len(lines), m.termHeight)], "\n")
}

// recapHeader names the recap on the left and the periods around the one
// shown on the right.
func (m Model) recapHeader(st styles.Styles, width int) string {
	o := m.overlay.recap
	title := " " + st.AccentMark.Bold(true).Render("Your year in muse")
	if o.loading || len(o.periods) == 0 {
		return title
	}
	var parts []string
	for i := max(0, o.periodIdx-1); i <= min(len(o.periods)-1, o.periodIdx+1); i++ {
		label := o.periods[i].Label()
		if i == o.periodIdx {
			parts = append(parts, st.TextPrimary.Bold(true).Render(label))
		} else {
			parts = append(parts, st.Dim.Render(label))
		}
	}
	strip := strings.Join(parts, st.Dim.Render("  "))
	room := width - ansi.StringWidth(title) - 2
	if ansi.StringWidth(strip) > room {
		strip = st.TextPrimary.Bold(true).Render(text.Truncate(o.periods[o.periodIdx].Label(), max(room, 0)))
	}
	gap := max(1, width-ansi.StringWidth(title)-ansi.StringWidth(strip)-1)
	return title + strings.Repeat(" ", gap) + strip
}

// recapIndicator is the "● ○ ○" row, centered.
func (m Model) recapIndicator(st styles.Styles, width int) string {
	o := m.overlay.recap
	if o.loading || len(o.pages) < 2 {
		return ""
	}
	dots := make([]string, len(o.pages))
	for i := range o.pages {
		if i == o.page {
			dots[i] = st.AccentMark.Render("●")
		} else {
			dots[i] = st.Dim.Render("○")
		}
	}
	line := strings.Join(dots, " ")
	return strings.Repeat(" ", max(0, (width-ansi.StringWidth(line))/2)) + line
}

// recapFooter is the key hint, or the notice standing in for it.
func (m Model) recapFooter(st styles.Styles, width int) string {
	if notice, isErr := m.nowPlayingNotice(); notice != "" {
		style := st.Warning
		if isErr {
			style = st.Error
		}
		return style.Render(text.Truncate(notice, width))
	}
	return st.Dim.Render(text.CompactHints(recapHint, width, "space/→", "S", "esc"))
}

// composeRecapCard centers a card in width×height: the cover and the text
// side by side, the block centered both ways.
func (m Model) composeRecapCard(card recapCard, st styles.Styles, width, height int) []string {
	coverCols, coverRows := 0, 0
	var coverLines []string
	if card.cover != nil {
		coverCols, coverRows = card.cover.cols, card.cover.rows
		coverLines = strings.Split(m.recapCoverArt(*card.cover, st), "\n")
	}
	panel := recapPanelWidth(width, coverCols)
	body := card.lines[:min(len(card.lines), height)]
	textWidth := 0
	for _, l := range body {
		textWidth = max(textWidth, ansi.StringWidth(l))
	}
	textWidth = min(textWidth, panel)
	if card.center {
		textWidth = panel
	}
	blockWidth := textWidth
	if coverCols > 0 {
		blockWidth += coverCols + recapGap
	}
	block := max(len(body), coverRows)
	top := max(0, (height-block)/2)
	textTop := max(0, (block-len(body))/2)
	left := strings.Repeat(" ", max(recapMargin, (width-blockWidth)/2))
	out := make([]string, top, height)
	for i := range min(block, height-top) {
		var b strings.Builder
		b.WriteString(left)
		if coverCols > 0 {
			b.WriteString(recapLineOr(coverLines, i, coverCols))
			b.WriteString(strings.Repeat(" ", recapGap))
		}
		b.WriteString(recapPlace(recapLineOr(body, i-textTop, 0), textWidth, card.center))
		out = append(out, b.String())
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

// recapPanelWidth is the text column's width beside a cover of coverCols.
func recapPanelWidth(width, coverCols int) int {
	room := width - 2*recapMargin
	if coverCols > 0 {
		return max(min(room-coverCols-recapGap, recapPanelMax), 10)
	}
	return max(min(room, recapCenterMax), 10)
}

func recapPlace(line string, width int, center bool) string {
	line = ansi.Truncate(line, width, "…")
	if !center {
		return line
	}
	return strings.Repeat(" ", max(0, (width-ansi.StringWidth(line))/2)) + line
}

func recapLineOr(lines []string, i, width int) string {
	if i >= 0 && i < len(lines) {
		return lines[i]
	}
	return strings.Repeat(" ", width)
}
