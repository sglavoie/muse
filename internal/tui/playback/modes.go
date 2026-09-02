package playback

// ModeIndicator describes one glyph of the status-bar mode cluster. It is the
// single source of truth for what a letter means, so the cluster, the legend
// overlay and the toggle notifications can never disagree.
type ModeIndicator struct {
	Glyph  string // "R"/"1", "S", "N", "D"
	Name   string // "Repeat", "Shuffle", "Stop after current", "Radio (Auto-DJ)"
	State  string // "all"/"one"/"off" for repeat, "on"/"off" for the rest
	Key    string // the global keybinding that toggles this mode
	Active bool
}

// ModeState renders a boolean mode state to match RepeatMode.DisplayString's
// lowercase style.
func ModeState(active bool) string {
	if active {
		return "on"
	}
	return "off"
}

// ModeIndicators returns the mode cluster in render order. The order is
// contractual: renderModeCluster pairs it positionally with the per-mode
// active styles of statusBarRenderContext.
func ModeIndicators(repeat RepeatMode, shuffle, stopAfter, radio bool) []ModeIndicator {
	repeatGlyph := "R"
	if repeat == RepeatOne {
		repeatGlyph = "1"
	}
	return []ModeIndicator{
		{Glyph: repeatGlyph, Name: "Repeat", State: repeat.DisplayString(), Key: "r", Active: repeat != RepeatOff},
		{Glyph: "S", Name: "Shuffle", State: ModeState(shuffle), Key: "z", Active: shuffle},
		{Glyph: "N", Name: "Stop after current", State: ModeState(stopAfter), Key: "ctrl+n", Active: stopAfter},
		{Glyph: "D", Name: "Radio (Auto-DJ)", State: ModeState(radio), Key: "A", Active: radio},
	}
}

// Positions in the slice modeIndicators returns. The order is contractual, so a
// mode toggle can look up the very glyph the cluster is about to render.
const (
	ModeIndexRepeat = iota
	ModeIndexShuffle
	ModeIndexStopAfter
	ModeIndexRadio
)
