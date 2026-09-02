package library

// StripLabel returns the view-label string shown in the state
// strip for the given library mode. The label tracks the title bar
// abbreviation so users see a consistent view name in both places.
func StripLabel(mode LibraryMode) string {
	switch mode {
	case LibraryModeRecent:
		return "Recent"
	case LibraryModeHistory:
		return "History"
	default:
		return "Library"
	}
}
