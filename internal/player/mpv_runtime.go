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
	elapsed, err = secondsOrZero("mpv", propTimePos, posResp.Data)
	if err != nil {
		return 0, 0, err
	}

	durResp, err := c.SendCommand(cmdGetProperty, propDuration)
	if err != nil {
		return 0, 0, err
	}
	total, err = secondsOrZero("mpv", propDuration, durResp.Data)
	if err != nil {
		return 0, 0, err
	}

	return elapsed, total, nil
}

func secondsOrZero(prefix, prop string, data any) (float64, error) {
	if data == nil {
		return 0, nil
	}
	seconds, ok := data.(float64)
	if !ok {
		return 0, fmt.Errorf("%s: %s: expected float64 or nil, got %T", prefix, prop, data)
	}
	return seconds, nil
}

// SyncPosition queries mpv for the current file path and playback position,
// then persists them via SetCurrentPosition for crash-resume. Safe to call
// when mpv is idle — errors are silently ignored.
func (m *Manager) SyncPosition() error {
	c, err := m.ipc()
	if err != nil {
		return err
	}

	pathResp, err := c.SendCommand(cmdGetProperty, propPath)
	if err != nil {
		return err
	}
	path, _ := pathResp.Data.(string)

	posResp, err := c.SendCommand(cmdGetProperty, propTimePos)
	if err != nil {
		return err
	}
	elapsed, _ := posResp.Data.(float64)

	if path != "" {
		m.SetCurrentPosition(path, elapsed)
	}
	return nil
}
