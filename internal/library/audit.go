package library

// DurationFix carries a single corrected duration from the background audit.
type DurationFix struct {
	Hash       string
	DurationMs int64
}

// DurationAuditBatchMsg carries a batch of corrected durations for the TUI.
type DurationAuditBatchMsg struct {
	Fixes []DurationFix
}

// DurationAuditDoneMsg signals the audit has finished.
type DurationAuditDoneMsg struct {
	Checked   int
	Corrected int
}
