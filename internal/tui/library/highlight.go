package library

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// applyHighlight wraps the runes at positions in s with bold lipgloss styling.
// Positions are 0-indexed into the rune slice of s; out-of-range positions are ignored.
func applyHighlight(s string, positions []int) string {
	if len(positions) == 0 {
		return s
	}
	posSet := make(map[int]struct{}, len(positions))
	for _, p := range positions {
		posSet[p] = struct{}{}
	}
	boldStyle := lipgloss.NewStyle().Bold(true)
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if _, ok := posSet[i]; ok {
			b.WriteString(boldStyle.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// searchFieldOffsets returns the (offset, length) pair for the artist and
// title fields within the concatenated "artist title filename" search string
// that sahilm/fuzzy operates on. The layout is:
//
//	[artist] SP [title] SP [filename]
//
// Each separator is exactly one rune wide, so the title field starts at
// len(artistRunes)+1.
func searchFieldOffsets(artist, title string) (artistOffset, artistLen, titleOffset, titleLen int) {
	artistLen = len([]rune(artist))
	titleLen = len([]rune(title))
	artistOffset = 0
	titleOffset = artistLen + 1 // +1 for the single space separator
	return
}

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

// padHighlight pads or truncates s to exactly width visible runes, applying
// bold highlight to the runes at positions (0-indexed into s) that fall
// within the visible portion. ANSI codes are inserted around highlighted
// runes so the visible width is preserved.
func padHighlight(s string, width int, positions []int) string {
	if width <= 0 {
		return ""
	}
	if len(positions) == 0 {
		return pad(s, width)
	}
	runes := []rune(s)
	n := len(runes)

	var content []rune
	ellipsis := false
	if n > width {
		if width == 1 {
			content = runes[:1]
		} else {
			content = runes[:width-1]
			ellipsis = true
		}
	} else {
		content = runes
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
	}

	visibleLen := len(content)
	if ellipsis {
		visibleLen++
	}
	if visibleLen < width {
		b.WriteString(strings.Repeat(" ", width-visibleLen))
	}

	return b.String()
}
