package tui

// paletteAliases supplies a small search vocabulary without changing command
// names, IDs, shortcuts, usage tracking, or availability checks.
func paletteAliases(id string) []string {
	switch id {
	case "library.rescan":
		return []string{"refresh library", "scan music"}
	case "audio.toggle.eq":
		return []string{"eq"}
	case "radio.toggle":
		return []string{"auto-dj", "autodj", "auto dj"}
	case "autodj.tune":
		return []string{"tune radio", "autodj", "auto dj"}
	default:
		return nil
	}
}
