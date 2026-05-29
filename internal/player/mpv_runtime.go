package player

import "fmt"

// ElapsedPosition queries mpv for the current playback position ("time-pos")
// and track duration ("duration"), returning them as seconds. Returns
// (0, 0, ErrNotConnected) when mpv is not yet connected, so callers can fall
// back gracefully. Satisfies the elapsedReader interface used by the TUI root
// model.
func (m *Manager) ElapsedPosition() (elapsed, total float64, err error) {
	c, ipcErr := m.ipc()
	if ipcErr != nil {
		return 0, 0, ipcErr
	}

	posResp, err := c.SendCommand(cmdGetProperty, propTimePos)
	if err != nil {
		return 0, 0, err
	}
	elapsed, err = secondsOrZero("mpv", propTimePos, posResp)
	if err != nil {
		return 0, 0, err
	}

	durResp, err := c.SendCommand(cmdGetProperty, propDuration)
	if err != nil {
		return 0, 0, err
	}
	total, err = secondsOrZero("mpv", propDuration, durResp)
	if err != nil {
		return 0, 0, err
	}

	return elapsed, total, nil
}

func secondsOrZero(prefix, prop string, resp *ipcResponse) (float64, error) {
	seconds, ok, err := resp.float64Data()
	if err != nil {
		return 0, fmt.Errorf("%s: %s: %w", prefix, prop, err)
	}
	if !ok {
		return 0, nil
	}
	return seconds, nil
}

// SyncPosition queries mpv for the current file path and playback position,
// then persists them via SetCurrentPosition for crash-resume. Safe to call
// when mpv is idle: null properties leave the saved position unchanged.
func (m *Manager) SyncPosition() error {
	c, err := m.ipc()
	if err != nil {
		return err
	}

	pathResp, err := c.SendCommand(cmdGetProperty, propPath)
	if err != nil {
		return err
	}
	path, _, err := pathResp.stringData()
	if err != nil {
		return fmt.Errorf("mpv: %s: %w", propPath, err)
	}

	posResp, err := c.SendCommand(cmdGetProperty, propTimePos)
	if err != nil {
		return err
	}
	elapsed, elapsedOK, err := posResp.float64Data()
	if err != nil {
		return fmt.Errorf("mpv: %s: %w", propTimePos, err)
	}

	if path != "" {
		if !elapsedOK {
			m.mu.RLock()
			samePath := m.lastTrackPath == path
			m.mu.RUnlock()
			if samePath {
				return nil
			}
		}
		m.SetCurrentPosition(path, elapsed)
	}
	return nil
}
