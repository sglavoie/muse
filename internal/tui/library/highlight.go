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

// padHighlight pads or truncates s to exactly width display columns, applying
// bold highlight to the runes at positions (0-indexed into s) that fall
// within the visible portion. ANSI codes are inserted around highlighted
// runes so the visible width is preserved.
func padHighlight(s string, width int, positions []int) string {
	if width <= 0 {
		return ""
	}
	if len(positions) == 0 {
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

	boldStyle := lipgloss.NewStyle().Bold(true)
	var b strings.Builder
	for i, r := range content {
		if _, ok := posSet[i]; ok {
			b.WriteString(boldStyle.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	if ellipsis {
		b.WriteRune('…')
		contentWidth++
	}
	if contentWidth < width {
		b.WriteString(strings.Repeat(" ", width-contentWidth))
	}

	return b.String()
}
