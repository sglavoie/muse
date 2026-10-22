package player

// loadFileReplace loads a file with per-file options on both sides of mpv's
// 0.38 command change: older versions have no insertion-index argument.
func (c *IPCClient) loadFileReplace(path, options string) error {
	resp, err := c.SendCommand(cmdLoadfile, path, "replace", 0, options)
	if err != nil && resp != nil && resp.Error == "invalid parameter" {
		// Argument validation rejected the command before playback changed.
		// Retry only this rejection, never a transport or playback failure:
		// those may have already started the file.
		_, err = c.SendCommand(cmdLoadfile, path, "replace", options)
	}
	return err
}
