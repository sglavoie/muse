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
	if len(positions) == 0 {
		if len(baseStyles) > 0 {
			return baseStyles[0].Render(text.Pad(s, width))
		}
		return text.Pad(s, width)
	}
	runes := []rune(s)

	// Determine how many runes fit within the display width budget.
	totalWidth := ansi.StringWidth(s)
	var content []rune
	var contentWidth int
	ellipsis := false
	if totalWidth > width {
		budget := width - 1 // reserve 1 column for "…"
		w := 0
		for i, r := range runes {
			rw := ansi.StringWidth(string(r))
			if w+rw > budget {
				content = runes[:i]
				break
			}
			w += rw
		}
		if content == nil {
			content = runes[:0]
		}
		contentWidth = w
		ellipsis = true
	} else {
		content = runes
		contentWidth = totalWidth
	}

	posSet := make(map[int]struct{}, len(positions))
	for _, p := range positions {
		posSet[p] = struct{}{}
	}

	hasBase := len(baseStyles) > 0
	var base lipgloss.Style
	if hasBase {
		base = baseStyles[0]
	}
	boldStyle := lipgloss.NewStyle().Bold(true)
	if hasBase {
		boldStyle = base.Bold(true)
	}

	// Group consecutive characters of the same type (matched vs non-matched)
	// into runs, rendering each run with a single .Render() call to minimise
	// ANSI escape overhead.
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

	if ellipsis {
		if hasBase {
			b.WriteString(base.Render("…"))
		} else {
			b.WriteRune('…')
		}
		contentWidth++
	}
	if contentWidth < width {
		pad := strings.Repeat(" ", width-contentWidth)
		if hasBase {
			b.WriteString(base.Render(pad))
		} else {
			b.WriteString(pad)
		}
	}

	return b.String()
}
