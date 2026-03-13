package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// LibraryView renders the music library panel backed by a sorted LibraryIndex.
type LibraryView struct {
	index  library.LibraryIndex
	width  int
	height int
	cursor int // zero-based index of the focused track row
}

// render returns a string representation of the library panel with proportional
// columnar layout. Column widths resize with the terminal width.
func (v LibraryView) render() string {
	return libtui.View{
		Index:  v.index,
		Width:  v.width,
		Height: v.height,
		Cursor: v.cursor,
	}.Render()
}

// HandleAction executes a pre-resolved library action dispatched by root.Update.
func (v LibraryView) HandleAction(a action) LibraryView {
	view := libtui.View{
		Index:  v.index,
		Width:  v.width,
		Height: v.height,
		Cursor: v.cursor,
	}
	switch a {
	case actionCursorUp:
		view = view.CursorUp()
	case actionCursorDown:
		view = view.CursorDown()
	}
	v.cursor = view.Cursor
	return v
}

// formatCount formats n with comma separators (e.g. 1234 → "1,234").
func formatCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	offset := len(s) % 3
	var result []byte
	for i := range len(s) {
		if i > 0 && (i-offset)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, s[i])
	}
	return string(result)
}
