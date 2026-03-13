package library

// ScanBatchMsg carries a slice of newly scanned tracks for incremental display.
type ScanBatchMsg struct {
	Tracks []Track
}

// ScanProgressMsg carries the current scan progress for the status bar.
// Total is the estimated total file count from the parallel counting goroutine;
// it is zero until the count arrives, at which point formatters can render
// "X / Y tracks" instead of "X tracks".
//
// NOTE: ScanProgressMsg is used as a parameter type passed directly to
// StatusBar.handleScanProgress — it is NOT dispatched as a tea.Msg. Status bar
// updates are driven inline from the ScanBatchMsg and ScanTotalMsg handlers in
// root.go Update(). Do not add a case library.ScanProgressMsg branch to
// Update(); doing so would create a dead code path since no Cmd ever dispatches
// this type as a message.
type ScanProgressMsg struct {
	Scanned int
	Total   int
}

// ScanTotalMsg carries the estimated total audio file count produced by the
// parallel counting goroutine in CountAudioFiles. It is emitted once, early
// in the scan, and lets the status bar switch to the "X / Y" format.
type ScanTotalMsg struct {
	Total int
}

// ScanDoneMsg signals that the scan has completed.
type ScanDoneMsg struct{}
