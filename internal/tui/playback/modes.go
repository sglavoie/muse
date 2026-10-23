package playback

// ModeIndicator shares names, compact labels, state, and toggle keys across
// the playback bar, legend, Help, and notifications.
type ModeIndicator struct {
	Compact string // readable abbreviation used at narrow widths
	Name    string // "Repeat", "Shuffle", "Stop after track", "Auto-DJ"
	State   string // "all"/"one"/"off" for repeat, "on"/"off" for the rest
	Key     string // the global keybinding that toggles this mode
	Active  bool
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
	return []ModeIndicator{
		{Compact: "Rpt:" + repeat.DisplayString(), Name: "Repeat", State: repeat.DisplayString(), Key: "r", Active: repeat != RepeatOff},
		{Compact: "Shuf", Name: "Shuffle", State: ModeState(shuffle), Key: "z", Active: shuffle},
		{Compact: "Stop", Name: "Stop after track", State: ModeState(stopAfter), Key: "ctrl+n", Active: stopAfter},
		{Compact: "DJ", Name: "Auto-DJ", State: ModeState(radio), Key: "A", Active: radio},
	}
}

// Positions in the slice modeIndicators returns. The order is contractual, so a
// mode toggle can look up the same mode the cluster is about to render.
const (
	ModeIndexRepeat = iota
	ModeIndexShuffle
	ModeIndexStopAfter
	ModeIndexRadio
)

// Label is the status bar's readable active-state label.
func (m ModeIndicator) Label() string {
	if m.Name == "Repeat" {
		return m.Name + " " + m.State
	}
	return m.Name
}
