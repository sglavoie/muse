package player

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/sglavoie/muse/internal/config"
)

// SetRepeat configures mpv's repeat behaviour. mode is the string form of
// RepeatMode: "none" disables looping, "all" loops the playlist, "one" loops
// the current file. Repeat-all is driven entirely by the TUI calling
// PlayFromIndex(0) on queue exhaustion; we keep loop-playlist=no so that
// the prefetch mechanism (2-entry mpv playlist) is not disrupted by mpv's
// internal playlist looping. The command is queued for replay if mpv is not
// yet connected.
func (m *Manager) SetRepeat(mode string) error {
	c, err := m.ipcOrEnqueue(func() error { return m.SetRepeat(mode) })
	if err != nil || c == nil {
		return err
	}
	switch mode {
	case "all":
		if _, err = c.SendCommand(cmdSetProperty, propLoopPlaylist, "no"); err != nil {
			return err
		}
		_, err = c.SendCommand(cmdSetProperty, propLoopFile, "no")
	case "one":
		if _, err = c.SendCommand(cmdSetProperty, propLoopFile, "inf"); err != nil {
			return err
		}
		_, err = c.SendCommand(cmdSetProperty, propLoopPlaylist, "no")
	default:
		if _, err = c.SendCommand(cmdSetProperty, propLoopPlaylist, "no"); err != nil {
			return err
		}
		_, err = c.SendCommand(cmdSetProperty, propLoopFile, "no")
	}
	return err
}

// SetShuffle enables or disables mpv's shuffle mode. The command is queued
// for replay if mpv is not yet connected.
func (m *Manager) SetShuffle(enabled bool) error {
	c, err := m.ipcOrEnqueue(func() error { return m.SetShuffle(enabled) })
	if err != nil || c == nil {
		return err
	}
	_, err = c.SendCommand(cmdSetProperty, propShuffle, enabled)
	return err
}

// Play resumes playback.
func (m *Manager) Play() error {
	c, err := m.ipcOrEnqueue(m.Play)
	if err != nil || c == nil {
		return err
	}
	if _, err = c.SendCommand(cmdSetProperty, propPause, false); err != nil {
		return err
	}
	// Mirror the requested pause state so an auto-restart firing before
	// the property-change event arrives resumes in the correct state.
	m.mu.Lock()
	m.lastPaused = false
	m.mu.Unlock()
	return nil
}

// Pause pauses playback.
func (m *Manager) Pause() error {
	c, err := m.ipcOrEnqueue(m.Pause)
	if err != nil || c == nil {
		return err
	}
	if _, err = c.SendCommand(cmdSetProperty, propPause, true); err != nil {
		return err
	}
	// Mirror the requested pause state so an auto-restart firing before
	// the property-change event arrives resumes in the correct state.
	m.mu.Lock()
	m.lastPaused = true
	m.mu.Unlock()
	return nil
}

// Stop pauses playback and seeks to the beginning of the current track.
func (m *Manager) Stop() error {
	c, err := m.ipcOrEnqueue(m.Stop)
	if err != nil || c == nil {
		return err
	}
	if _, err = c.SendCommand(cmdSetProperty, propPause, true); err != nil {
		return err
	}
	if _, err = c.SendCommand(cmdSeek, 0, "absolute"); err != nil {
		return err
	}
	// Mirror the requested pause state so an auto-restart firing before
	// the property-change event arrives resumes in the correct state.
	m.mu.Lock()
	m.lastPaused = true
	m.mu.Unlock()
	return nil
}

// SeekAbsolute seeks to an absolute position in seconds within the current track.
func (m *Manager) SeekAbsolute(seconds float64) error {
	c, err := m.ipcOrEnqueue(func() error { return m.SeekAbsolute(seconds) })
	if err != nil || c == nil {
		return err
	}
	_, err = c.SendCommand(cmdSeek, seconds, "absolute")
	return err
}

// SeekRelative seeks forward (positive) or backward (negative) by seconds.
func (m *Manager) SeekRelative(seconds float64) error {
	c, err := m.ipcOrEnqueue(func() error { return m.SeekRelative(seconds) })
	if err != nil || c == nil {
		return err
	}
	_, err = c.SendCommand(cmdSeek, seconds, "relative")
	return err
}

// SetVolumeRelative adjusts volume by the given percentage delta.
func (m *Manager) SetVolumeRelative(delta float64) error {
	c, err := m.ipcOrEnqueue(func() error { return m.SetVolumeRelative(delta) })
	if err != nil || c == nil {
		return err
	}
	_, err = c.SendCommand(cmdAdd, propVolume, delta)
	return err
}

// SetVolumeAbsolute sets the volume to an absolute level (0–100). The command
// is queued for replay if mpv is not yet connected, so it is safe to call
// immediately after Start before the IPC socket is ready.
func (m *Manager) SetVolumeAbsolute(volume float64) error {
	c, err := m.ipcOrEnqueue(func() error { return m.SetVolumeAbsolute(volume) })
	if err != nil || c == nil {
		return err
	}
	_, err = c.SendCommand(cmdSetProperty, propVolume, volume)
	return err
}

// rebuildAudioFilters sends the current audioFilters map to mpv as a single
// lavfi af property. Must be called without m.mu held (ipc() takes RLock).
func (m *Manager) rebuildAudioFilters() error {
	c, err := m.ipc()
	if err != nil {
		return err
	}

	m.mu.RLock()
	snapshot := make(map[string]string, len(m.audioFilters))
	maps.Copy(snapshot, m.audioFilters)
	m.mu.RUnlock()

	if len(snapshot) == 0 {
		_, err = c.SendCommand(cmdSetProperty, propAF, "")
		return err
	}
	names := make([]string, 0, len(snapshot))
	for name := range snapshot {
		names = append(names, name)
	}
	sort.Strings(names)
	exprs := make([]string, 0, len(names))
	for _, name := range names {
		exprs = append(exprs, snapshot[name])
	}
	_, err = c.SendCommand(cmdSetProperty, propAF, "lavfi=["+strings.Join(exprs, ",")+"]")
	return err
}

func (m *Manager) updateFilterLocked(name, expr string, enabled bool) {
	if enabled {
		m.audioFilters[name] = expr
	} else {
		delete(m.audioFilters, name)
	}
}

// setFilter adds or removes a named lavfi filter expression and rebuilds the
// af property. It only mutates audioFilters after confirming mpv is either
// connected or still in the pre-connect queueing phase.
func (m *Manager) setFilter(name, expr string, enabled bool) error {
	m.mu.Lock()
	switch {
	case m.client != nil:
		m.updateFilterLocked(name, expr, enabled)
		m.mu.Unlock()
		return m.rebuildAudioFilters()
	case m.cmdQueue != nil:
		m.updateFilterLocked(name, expr, enabled)
		m.cmdQueue = append(m.cmdQueue, func() error { return m.rebuildAudioFilters() })
		fn := m.callbacks.OnStatus
		m.mu.Unlock()

		if fn != nil {
			fn("Connecting to mpv…")
		}
		return nil
	default:
		m.mu.Unlock()
		return ErrNotConnected
	}
}

// SetNormalize enables or disables real-time EBU R128 volume normalization via
// mpv's lavfi loudnorm filter. The command is queued for replay if mpv is not
// yet connected.
func (m *Manager) SetNormalize(enabled bool) error {
	return m.setFilter("loudnorm", "loudnorm", enabled)
}

// SetEqualizer enables or disables the 10-band parametric equalizer via mpv's
// lavfi firequalizer filter. The command is queued for replay if mpv is not
// yet connected.
func (m *Manager) SetEqualizer(enabled bool, bands [10]int) error {
	if !enabled {
		return m.setFilter("equalizer", "", false)
	}
	return m.setFilter("equalizer", buildFireequalizerExpr(bands), true)
}

// SetStereoWide enables or disables stereo widening via mpv's lavfi
// extrastereo filter. amount is in tenths (10 = 1.0x passthrough).
// The command is queued for replay if mpv is not yet connected.
func (m *Manager) SetStereoWide(enabled bool, amount int) error {
	if !enabled {
		return m.setFilter("stereowide", "", false)
	}
	expr := fmt.Sprintf("extrastereo=m=%.1f", float64(amount)/10.0)
	return m.setFilter("stereowide", expr, true)
}

// buildFireequalizerExpr builds the lavfi firequalizer gain expression string
// for the given 10 band gain values (in dB).
func buildFireequalizerExpr(bands [10]int) string {
	entries := make([]string, 0, 10)
	for i, f := range config.EQFrequencies {
		entries = append(entries, fmt.Sprintf("entry(%d,%d)", f, bands[i]))
	}
	return "firequalizer=gain_entry='" + strings.Join(entries, ";") + "'"
}

// SetGapless enables or disables gapless playback via mpv's gapless-audio
// property. The command is queued for replay if mpv is not yet connected.
func (m *Manager) SetGapless(enabled bool) error {
	c, err := m.ipcOrEnqueue(func() error { return m.SetGapless(enabled) })
	if err != nil || c == nil {
		return err
	}
	val := "no"
	if enabled {
		val = "yes"
	}
	_, err = c.SendCommand(cmdSetProperty, propGaplessAudio, val)
	if err != nil {
		return err
	}
	_, err = c.SendCommand(cmdSetProperty, propPrefetchPlaylist, val)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.gaplessEnabled = enabled
	m.mu.Unlock()
	if enabled {
		go m.prefetchNext()
	} else {
		m.clearPrefetch()
	}
	return nil
}
