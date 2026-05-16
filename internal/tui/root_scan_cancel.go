package tui

import tea "charm.land/bubbletea/v2"

// handleScanCancelKeyPress intercepts `esc` while a library scan is running
// and asks the scanner pipeline to stop via its derived cancel context. The
// scan goroutine drains, the channel closes, and the regular ScanDoneMsg
// handler clears any remaining state. We surface a notification so the
// cancel feels immediate, and zero the long-op widget so the progress bar
// disappears on the next frame instead of waiting for ScanDoneMsg to land.
func (m Model) handleScanCancelKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.scanning {
		return m, nil, false
	}
	if msg.String() != "esc" {
		return m, nil, false
	}
	if m.scanCancel != nil {
		m.scanCancel()
		m.scanCancel = nil
	}
	m.scanning = false
	m.longOp = longOpState{}
	m.statusBar.notify("Scan canceled")
	return m, nil, true
}
