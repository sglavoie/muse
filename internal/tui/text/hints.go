package text

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// CompactHints keeps essential key/label pairs and fits secondary pairs in the
// remaining width. Pairs retain their original order and are never cut in half.
func CompactHints(hint string, width int, essentialKeys ...string) string {
	if width <= 0 {
		return ""
	}
	const sep = " · "
	pairs := strings.Split(hint, sep)
	keep := make([]bool, len(pairs))
	used, count := 0, 0
	for _, i := range hintPriorityOrder(pairs, essentialKeys) {
		cost := ansi.StringWidth(pairs[i])
		if count > 0 {
			cost += ansi.StringWidth(sep)
		}
		if !keep[i] && used+cost <= width {
			keep[i] = true
			used += cost
			count++
		}
	}
	var out []string
	for i, pair := range pairs {
		if keep[i] {
			out = append(out, pair)
		}
	}
	return strings.Join(out, sep)
}

// FocusMarker reserves the same two-column gutter in lists and pickers as in
// track tables. Selection checkboxes and playback icons remain separate states.
func FocusMarker(focused bool) string {
	if focused {
		return "▎ "
	}
	return "  "
}

func hintPriorityOrder(pairs, essentialKeys []string) []int {
	var order []int
	for _, key := range essentialKeys {
		for i, pair := range pairs {
			if strings.HasPrefix(pair, key+" ") {
				order = append(order, i)
			}
		}
	}
	for i := range pairs {
		order = append(order, i)
	}
	return order
}
