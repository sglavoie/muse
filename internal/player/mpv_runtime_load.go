package player

import "fmt"

// LoadFilePaused loads the file at path into mpv, seeks to positionMs
// milliseconds from the start, and leaves mpv in the paused state. Used for
// session restore at startup. If mpv is not yet connected the command is
// queued for replay once the IPC socket connects.
func (m *Manager) LoadFilePaused(path string, positionMs int64) error {
	if positionMs < 0 {
		positionMs = 0
	}
	elapsedSecs := float64(positionMs) / 1000.0
	return m.admit(func(c *IPCClient) error {
		if _, err := c.SendCommand(cmdLoadfile, path, "replace", 0, fmt.Sprintf("start=%g,pause=yes", elapsedSecs)); err != nil {
			return err
		}
		// loadfile with "pause=yes" forces mpv into the paused state; eagerly
		// mirror that so a restart firing before the property-change event
		// arrives preserves the paused state.
		m.mu.Lock()
		m.lastPaused = true
		// A restored session is positioned on a live queue entry: paused is not
		// ended, so the gapless successor may be prefetched for it.
		m.markPlaybackLiveLocked()
		m.mu.Unlock()
		return nil
	})
}

// loadFileAt sends mpv a loadfile command with a start= option to resume
// playback at the given elapsed time (in seconds). When paused is true the
// file is loaded in the paused state so the user's pause is preserved
// across auto-restarts.
func (m *Manager) loadFileAt(path string, elapsed float64, paused bool) error {
	c, err := m.ipc()
	if err != nil {
		return err
	}
	pauseOpt := "no"
	if paused {
		pauseOpt = "yes"
	}
	if _, err := c.SendCommand(cmdLoadfile, path, "replace", 0, fmt.Sprintf("start=%g,pause=%s", elapsed, pauseOpt)); err != nil {
		return err
	}
	// The resume reinstates the entry the outage interrupted, so the traversal
	// is live again on the new process. Restarting mpv is process health and
	// says nothing on its own about logical liveness; this load does. It is
	// reached only for a traversal that was live when the outage began —
	// resumeAfterRestart owns that decision, because from here the path of an
	// interrupted track and the path of one that played out look identical.
	m.markPlaybackLive()
	return nil
}
