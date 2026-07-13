package player

import (
	"fmt"
	"maps"
	"math"
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
	// Record the mode so the restart supervisor can restore it after mpv
	// reconnects. Done before the IPC send so the value is captured even when
	// the command is queued for replay.
	m.mu.Lock()
	m.repeatMode = mode
	m.mu.Unlock()

	return m.admit(func(c *IPCClient) error {
		return sendRepeat(c, mode)
	})
}

// sendRepeat applies the loop-playlist/loop-file properties for the given repeat
// mode directly against an already-resolved client. It is the ungated core of
// SetRepeat, shared with restoreStableProperties so the restart supervisor can
// re-apply the repeat mode inside the activation window (where the gated
// admit path would defer it behind the resume loadfile).
func sendRepeat(c *IPCClient, mode string) error {
	var err error
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

// SetShuffle enables or disables Go-owned shuffle traversal. It never touches
// mpv's own shuffle property: the visible and persisted queue order is left
// unchanged and Next/Previous consult the Manager's traversal state instead.
// Enabling preserves the current track as the anchor and builds a no-repeat
// permutation of every other queue entry. Because it only mutates in-memory
// state under the manager lock, it works identically before connection, while
// connected, and while reconnecting.
func (m *Manager) SetShuffle(enabled bool) error {
	m.mu.Lock()
	if m.shuffleEnabled == enabled {
		m.mu.Unlock()
		return nil
	}
	m.shuffleEnabled = enabled
	if enabled {
		m.rebuildShuffleLocked()
	} else {
		m.clearShuffleLocked()
	}
	m.mu.Unlock()

	// Toggling shuffle changes the traversal model, so the existing gapless
	// prefetch (resolved under the old model) is now invalid. Clear it and
	// rebuild: prefetchNext resolves the successor for whichever model is now
	// active — the next shuffle identity when enabled, or idx+1 when disabled.
	m.clearPrefetch()
	go m.prefetchNext()
	return nil
}

// Play resumes playback.
func (m *Manager) Play() error {
	return m.admit(func(c *IPCClient) error {
		if _, err := c.SendCommand(cmdSetProperty, propPause, false); err != nil {
			return err
		}
		// Mirror the requested pause state so an auto-restart firing before
		// the property-change event arrives resumes in the correct state.
		m.mu.Lock()
		m.lastPaused = false
		m.mu.Unlock()
		return nil
	})
}

// bestEffortUnpause clears mpv's pause property after playback has already
// been advanced by another command. It must not make the caller's successful
// track advance fail if the follow-up pause command is rejected or disconnected.
func (m *Manager) bestEffortUnpause() {
	c, err := m.ipc()
	if err == nil {
		m.bestEffortUnpauseWith(c)
		return
	}
	m.mirrorUnpaused()
}

func (m *Manager) bestEffortUnpauseWith(c *IPCClient) {
	if c != nil {
		_, _ = c.SendCommand(cmdSetProperty, propPause, false)
	}
	m.mirrorUnpaused()
}

func (m *Manager) mirrorUnpaused() {
	m.mu.Lock()
	m.lastPaused = false
	m.mu.Unlock()
}

// Pause pauses playback.
func (m *Manager) Pause() error {
	return m.admit(func(c *IPCClient) error {
		if _, err := c.SendCommand(cmdSetProperty, propPause, true); err != nil {
			return err
		}
		// Mirror the requested pause state so an auto-restart firing before
		// the property-change event arrives resumes in the correct state.
		m.mu.Lock()
		m.lastPaused = true
		m.mu.Unlock()
		return nil
	})
}

// Stop pauses playback and seeks to the beginning of the current track.
func (m *Manager) Stop() error {
	return m.admit(func(c *IPCClient) error {
		if _, err := c.SendCommand(cmdSetProperty, propPause, true); err != nil {
			return err
		}
		// Pause has been applied to mpv. Mirror it immediately so an auto-restart
		// firing before the property-change event arrives preserves the paused
		// state even if the subsequent seek fails.
		m.mu.Lock()
		m.lastPaused = true
		m.mu.Unlock()
		if _, err := c.SendCommand(cmdSeek, 0, "absolute"); err != nil {
			return err
		}
		return nil
	})
}

// SeekAbsolute seeks to an absolute position in seconds within the current track.
// Negative values are clamped to 0.
func (m *Manager) SeekAbsolute(seconds float64) error {
	if seconds < 0 {
		seconds = 0
	}
	return m.admit(func(c *IPCClient) error {
		_, err := c.SendCommand(cmdSeek, seconds, "absolute")
		return err
	})
}

// SeekRelative seeks forward (positive) or backward (negative) by seconds.
func (m *Manager) SeekRelative(seconds float64) error {
	return m.admit(func(c *IPCClient) error {
		_, err := c.SendCommand(cmdSeek, seconds, "relative")
		return err
	})
}

// SetVolumeRelative adjusts volume by the given percentage delta.
func (m *Manager) SetVolumeRelative(delta float64) error {
	// Track the running absolute level (clamped to mpv's 0-100 range) so the
	// restart supervisor can restore it. Only meaningful once an absolute
	// baseline has been established via SetVolumeAbsolute.
	m.mu.Lock()
	if m.volumeKnown {
		m.lastVolume = math.Min(100, math.Max(0, m.lastVolume+delta))
	}
	m.mu.Unlock()

	return m.admit(func(c *IPCClient) error {
		_, err := c.SendCommand(cmdAdd, propVolume, delta)
		return err
	})
}

// SetVolumeAbsolute sets the volume to an absolute level (0–100). The command
// is queued for replay if mpv is not yet connected, so it is safe to call
// immediately after Start before the IPC socket is ready. Values outside the
// 0-100 range are clamped before being forwarded to mpv to avoid driver- or
// platform-dependent behavior at out-of-range values.
func (m *Manager) SetVolumeAbsolute(volume float64) error {
	if math.IsNaN(volume) || volume < 0 {
		volume = 0
	} else if volume > 100 {
		volume = 100
	}
	// Record the absolute level (and mark it known) so the restart supervisor
	// can restore it after mpv reconnects.
	m.mu.Lock()
	m.lastVolume = volume
	m.volumeKnown = true
	m.mu.Unlock()

	return m.admit(func(c *IPCClient) error {
		_, err := c.SendCommand(cmdSetProperty, propVolume, volume)
		return err
	})
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
	case m.client != nil && !m.activating:
		prevExpr, hadPrev := m.audioFilters[name]
		m.updateFilterLocked(name, expr, enabled)
		m.mu.Unlock()
		if err := m.rebuildAudioFilters(); err != nil {
			// Revert the in-memory filter state when the IPC write fails so
			// Manager state does not drift from mpv's active filters.
			m.mu.Lock()
			curExpr, hasCur := m.audioFilters[name]
			unchanged := false
			if enabled {
				unchanged = hasCur && curExpr == expr
			} else {
				unchanged = !hasCur
			}
			if unchanged {
				if hadPrev {
					m.audioFilters[name] = prevExpr
				} else {
					delete(m.audioFilters, name)
				}
			}
			m.mu.Unlock()
			return err
		}
		return nil
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
	return m.admit(func(c *IPCClient) error {
		val := "no"
		if enabled {
			val = "yes"
		}
		if _, err := c.SendCommand(cmdSetProperty, propGaplessAudio, val); err != nil {
			return err
		}
		if _, err := c.SendCommand(cmdSetProperty, propPrefetchPlaylist, val); err != nil {
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
	})
}
