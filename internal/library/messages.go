package library

// ScanBatchMsg carries a slice of newly scanned tracks for incremental display.
type ScanBatchMsg struct {
	Tracks []Track
}

// ScanProgressMsg carries the current scan progress for the status bar.
type ScanProgressMsg struct {
	Scanned int
	Total   int
}

// ScanDoneMsg signals that the scan has completed.
type ScanDoneMsg struct{}
