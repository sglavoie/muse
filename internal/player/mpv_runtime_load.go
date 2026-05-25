package player

import "fmt"

// LoadFilePaused loads the file at path into mpv, seeks to positionMs
// milliseconds from the start, and leaves mpv in the paused state. Used for
// session restore at startup. If mpv is not yet connected the command is
// queued for replay once the IPC socket connects.
func (m *Manager) LoadFilePaused(path string, positionMs int64) error {
	elapsedSecs := float64(positionMs) / 1000.0
	c, err := m.ipcOrEnqueue(func() error { return m.LoadFilePaused(path, positionMs) })
	if err != nil || c == nil {
		return err
	}
	if _, err = c.SendCommand(cmdLoadfile, path, "replace", 0, fmt.Sprintf("start=%g,pause=yes", elapsedSecs)); err != nil {
		return err
	}
	// loadfile with "pause=yes" forces mpv into the paused state; eagerly
	// mirror that so a restart firing before the property-change event
	// arrives preserves the paused state.
	m.mu.Lock()
	m.lastPaused = true
	m.mu.Unlock()
	return nil
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
	_, err = c.SendCommand(cmdLoadfile, path, "replace", 0, fmt.Sprintf("start=%g,pause=%s", elapsed, pauseOpt))
	return err
}
