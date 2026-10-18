package tui

// paletteAliases supplies a small search vocabulary without changing command
// names, IDs, shortcuts, usage tracking, or availability checks.
func paletteAliases(id string) []string {
	return paletteSearchAliases[id]
}

var paletteSearchAliases = map[string][]string{
	"queue.play_next":       {"play focused track next", "play tracks next"},
	"queue.enqueue.replace": {"replace queue with track", "replace queue with tracks"},
	"messages.show":         {"errors"},
	"playback.seek.start":   {"restart song"},
	"track.reveal":          {"locate file"},
	"library.rescan":        {"refresh library", "scan music"},
	"audio.toggle.eq":       {"eq"},
	"radio.toggle":          {"auto-dj", "autodj", "auto dj"},
	"autodj.tune":           {"tune radio", "autodj", "auto dj"},
}
