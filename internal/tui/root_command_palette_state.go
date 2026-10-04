package tui

// paletteCurrentStates describes settings without changing command names or
// stable usage keys. The values are refreshed each time the palette opens.
func (m Model) paletteCurrentStates() map[string]string {
	theme := "Light"
	if m.styles.HasDark {
		theme = "Dark"
	}
	return map[string]string{
		"radio.toggle":                onOff(m.radio.enabled),
		"playback.repeat.cycle":       m.playback.repeatMode.DisplayString(),
		"playback.shuffle.toggle":     onOff(m.playback.shuffle),
		"playback.stop_after_current": onOff(m.playback.stopAfterCurrent),
		"ui.toggle.theme":             theme,
		"ui.toggle.footer_hints":      onOff(m.showFooterHints),
		"ui.toggle.track_notify":      onOff(m.notifyOnTrackChange),
		"queue.toggle.follow":         onOff(m.followNowPlaying),
		"audio.toggle.normalize":      onOff(m.cfg.NormalizeVolume),
		"audio.toggle.eq":             onOff(m.cfg.EQEnabled),
		"audio.toggle.stereo":         onOff(m.cfg.StereoWideEnabled),
	}
}
