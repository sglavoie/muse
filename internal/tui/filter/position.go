package filter

import "slices"

// Position keeps a stable row identity as well as a fallback for deleted rows.
// It travels with State, including when a view saves its per-mode filter.
type Position struct {
	Key            string
	Cursor, Scroll int
}

func CapturePosition(keys []string, cursor, scroll int) Position {
	p := Position{Cursor: cursor, Scroll: scroll}
	if cursor >= 0 && cursor < len(keys) {
		p.Key = keys[cursor]
	}
	return p
}

// Restore prefers the current filtered row, then the row saved before search.
func (p Position) Restore(keys []string, focused string) (cursor, scroll int) {
	cursor = -1
	if focused != "" {
		cursor = slices.Index(keys, focused)
	}
	if cursor < 0 && p.Key != "" {
		cursor = slices.Index(keys, p.Key)
	}
	if cursor < 0 {
		cursor = max(0, min(p.Cursor, len(keys)-1))
	}
	return cursor, max(0, min(p.Scroll, cursor))
}
