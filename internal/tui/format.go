package tui

// trackDisplay returns a human-readable label for a track. It uses
// "Artist – Title" when both are present, falls back to whichever is
// available, and uses path as a last resort.
func trackDisplay(artist, title, path string) string {
	switch {
	case artist != "" && title != "":
		return artist + " – " + title
	case title != "":
		return title
	case artist != "":
		return artist
	default:
		return path
	}
}
