package tui

// paletteAliases supplies a small search vocabulary without changing command
// names, IDs, shortcuts, usage tracking, or availability checks.
func paletteAliases(id string) []string {
	return paletteSearchAliases[id]
}

var paletteSearchAliases = map[string][]string{
	"mix.undismiss":           {"undismiss track"},
	"queue.play_next":         {"play focused track next", "play tracks next"},
	"queue.enqueue.replace":   {"replace queue with track", "replace queue with tracks"},
	"messages.show":           {"errors"},
	"playback.seek.start":     {"restart song"},
	"track.reveal":            {"locate file"},
	"library.rescan":          {"refresh library", "scan music"},
	"audio.toggle.eq":         {"eq"},
	"radio.toggle":            {"toggle radio mode", "autodj", "auto dj"},
	"mix.radio.seed":          {"start radio from mix"},
	"insights.radio.seed":     {"start radio from insight row"},
	"insights.enqueue_all":    {"enqueue tracks behind row"},
	"nav.insights":            {"stats", "statistics", "listening stats"},
	"insights.panel.next":     {"next stats panel"},
	"insights.panel.previous": {"previous stats panel"},
	"autodj.tune":             {"tune radio", "autodj", "auto dj"},
}
