package library

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/text"
)

// searchFieldOffsetsAll returns the (offset, length) pairs for the artist,
// title, and album fields within the concatenated "artist title album filename"
// search string. The layout is:
//
//	[artist] SP [title] SP [album] SP [filename]
func searchFieldOffsetsAll(artist, title, album string) (artistOffset, artistLen, titleOffset, titleLen, albumOffset, albumLen int) {
	artistLen = len([]rune(artist))
	titleLen = len([]rune(title))
	albumLen = len([]rune(album))
	artistOffset = 0
	titleOffset = artistLen + 1
	albumOffset = titleOffset + titleLen + 1
	return
}

// fieldMatchPositions filters matchedIndexes (positions in a concatenated
// "artist title filename" search string) to those within the given field
// range [fieldOffset, fieldOffset+fieldLen) and returns them 0-indexed
// within the field.
func fieldMatchPositions(matchedIndexes []int, fieldOffset, fieldLen int) []int {
	var result []int
	for _, idx := range matchedIndexes {
		if idx >= fieldOffset && idx < fieldOffset+fieldLen {
			result = append(result, idx-fieldOffset)
		}
	}
	return result
}

// favStyleFor returns a single-element slice containing style when isFav is
// true, or nil otherwise. It is meant to be spread into padHighlight's
// variadic baseStyles parameter.
func favStyleFor(isFav bool, style lipgloss.Style) []lipgloss.Style {
	if isFav {
		return []lipgloss.Style{style}
	}
	return nil
}

// accentStyleFor returns a single-element slice containing the appropriate
// accent style for a focused row cell. When isFav is true the combined
// foreground+background favorite accent style is returned; otherwise the
// plain accent background style is used. This ensures every cell in a
// focused row carries its own background, avoiding ANSI reset leakage.
func accentStyleFor(isFav bool, favAccent, accent lipgloss.Style) []lipgloss.Style {
	if isFav {
		return []lipgloss.Style{favAccent}
	}
	return []lipgloss.Style{accent}
}

// nowPlayingStyleFor returns a single-element slice containing the appropriate
// now-playing style for a non-focused row cell. When isFav is true the
// favorite-coloured now-playing style is returned; otherwise the plain
// fg+NowPlayingBg style is used. This ensures every cell in a now-playing
// row carries the NowPlayingBg even when search-match highlighting renders
// the cell via padHighlight rather than via the default no-highlight branch.
func nowPlayingStyleFor(isFav bool, favNowPlaying, plainNowPlaying lipgloss.Style) []lipgloss.Style {
	if isFav {
		return []lipgloss.Style{favNowPlaying}
	}
	return []lipgloss.Style{plainNowPlaying}
}

// padHighlight pads or truncates s to exactly width display columns, applying
// bold highlight to the runes at positions (0-indexed into s) that fall
// within the visible portion. ANSI codes are inserted around highlighted
// runes so the visible width is preserved.
//
// An optional baseStyles parameter supplies a foreground style (e.g. a
// favorite color) that is composed with the bold highlight. When provided,
// matched characters are rendered with baseStyle+Bold and non-matched
// characters are rendered with baseStyle alone, preventing inner SGR resets
// from clearing the outer foreground color.
func padHighlight(s string, width int, positions []int, baseStyles ...lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	s = flattenCell(s)
	hasBase := len(baseStyles) > 0
	var base lipgloss.Style
	if hasBase {
		base = baseStyles[0]
	}
	if len(positions) == 0 {
		return styledOrPlain(text.Pad(s, width), base, hasBase)
	}

	content, contentWidth, ellipsis := fitToWidth(s, width)

	var b strings.Builder
	b.WriteString(renderHighlightRuns(content, positions, base, hasBase))
	if ellipsis {
		b.WriteString(styledOrPlain("…", base, hasBase))
		contentWidth++
	}
	if contentWidth < width {
		b.WriteString(styledOrPlain(strings.Repeat(" ", width-contentWidth), base, hasBase))
	}
	return b.String()
}

// flattenCell replaces embedded line breaks and tabs with spaces so a cell can
// never break the row it sits in.
func flattenCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "\t", " ")
}

// styledOrPlain renders s with base when a base style was supplied, and
// verbatim otherwise.
func styledOrPlain(s string, base lipgloss.Style, hasBase bool) string {
	if hasBase {
		return base.Render(s)
	}
	return s
}

// fitToWidth returns the leading runes of s that fit within width display
// columns, their combined width, and whether s had to be truncated. When it
// did, one column is reserved for the trailing ellipsis the caller appends.
func fitToWidth(s string, width int) (content []rune, contentWidth int, ellipsis bool) {
	runes := []rune(s)
	totalWidth := ansi.StringWidth(s)
	if totalWidth <= width {
		return runes, totalWidth, false
	}
	budget := width - 1 // reserve 1 column for "…"
	w := 0
	for i, r := range runes {
		rw := ansi.StringWidth(string(r))
		if w+rw > budget {
			return runes[:i], w, true
		}
		w += rw
	}
	// Unreachable while totalWidth > width: the runes must overrun the budget.
	return runes[:0], w, true
}

// renderHighlightRuns groups consecutive characters of the same type (matched
// vs non-matched) into runs, rendering each run with a single .Render() call to
// minimise ANSI escape overhead.
func renderHighlightRuns(content []rune, positions []int, base lipgloss.Style, hasBase bool) string {
	posSet := make(map[int]struct{}, len(positions))
	for _, p := range positions {
		posSet[p] = struct{}{}
	}
	boldStyle := lipgloss.NewStyle().Bold(true)
	if hasBase {
		boldStyle = base.Bold(true)
	}

	var b strings.Builder
	run := make([]rune, 0, len(content))
	runBold := false

	flushRun := func() {
		if len(run) == 0 {
			return
		}
		rs := string(run)
		switch {
		case runBold:
			b.WriteString(boldStyle.Render(rs))
		case hasBase:
			b.WriteString(base.Render(rs))
		default:
			b.WriteString(rs)
		}
		run = run[:0]
	}

	for i, r := range content {
		_, isMatch := posSet[i]
		if i > 0 && isMatch != runBold {
			flushRun()
		}
		runBold = isMatch
		run = append(run, r)
	}
	flushRun()
	return b.String()
}
