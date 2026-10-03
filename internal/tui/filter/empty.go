package filter

// NoMatchesMessage names the independent filters and their recovery keys.
func NoMatchesMessage(hasText, hasTag bool) string {
	switch {
	case hasText && hasTag:
		return "No matches — Esc clears text filter\nTag filter active — ctrl+t clears tag filter"
	case hasText:
		return "No matches — Esc clears text filter"
	case hasTag:
		return "No matches — ctrl+t clears tag filter"
	default:
		return "No matches"
	}
}
