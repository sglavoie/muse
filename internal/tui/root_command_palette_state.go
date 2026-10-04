package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/text"
)

// palettePlaybackDetails uses the same configured steps as the playback actions.
func (m Model) palettePlaybackDetails() map[string]string {
	seconds := func(n float64) string {
		unit := "seconds"
		if n == 1 {
			unit = "second"
		}
		return fmt.Sprintf("%g %s", n, unit)
	}
	volume := fmt.Sprintf("%d percentage %s", m.cfg.VolumeIncrement, text.Pluralize(m.cfg.VolumeIncrement, "point", "points"))
	return map[string]string{
		"playback.seek.back":         "Backward " + seconds(m.cfg.SeekOffsetShort),
		"playback.seek.forward":      "Forward " + seconds(m.cfg.SeekOffsetShort),
		"playback.seek.back.long":    "Backward " + seconds(m.cfg.SeekOffsetLong),
		"playback.seek.forward.long": "Forward " + seconds(m.cfg.SeekOffsetLong),
		"playback.volume.up":         "Raise volume by " + volume,
		"playback.volume.down":       "Lower volume by " + volume,
	}
}

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
