package metadata

import (
	"charm.land/bubbles/v2/textinput"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Render a copy of the input with explicit hidden-text indicators. The stored
// value and cursor stay untouched; boundaries never cut a grapheme in half.
func inputWithOverflow(input textinput.Model, width int) string {
	value := []rune(input.Value())
	pos := input.Position()
	if ansi.StringWidth(string(value))+1 <= width {
		input.SetWidth(max(width-1, 1))
		input.CursorEnd()
		input.SetCursor(pos)
		return text.Pad(input.View(), width)
	}
	budget := max(width-3, 1) // two indicators and the cursor
	start, end := inputWindow(string(value), pos, budget)
	input.SetValue(string(value[start:end]))
	input.SetWidth(0)
	input.SetCursor(pos - start)
	left, right := " ", " "
	if start > 0 {
		left = "‹"
	}
	if end < len(value) {
		right = "›"
	}
	return left + text.Pad(input.View(), max(width-2, 0)) + right
}

func inputWindow(value string, cursor, width int) (int, int) {
	boundaries := []int{0}
	graphemes := uniseg.NewGraphemes(value)
	for graphemes.Next() {
		boundaries = append(boundaries, boundaries[len(boundaries)-1]+len(graphemes.Runes()))
	}
	runes := []rune(value)
	startIndex := 0
	for i, boundary := range boundaries {
		if boundary <= cursor {
			startIndex = i
		}
	}
	for startIndex > 0 && ansi.StringWidth(string(runes[boundaries[startIndex-1]:cursor])) <= width {
		startIndex--
	}
	start, end := boundaries[startIndex], boundaries[startIndex]
	for _, boundary := range boundaries[startIndex+1:] {
		if ansi.StringWidth(string(runes[start:boundary])) > width {
			break
		}
		end = boundary
	}
	return start, end
}
