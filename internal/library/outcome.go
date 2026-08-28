package library

import (
	"errors"
	"sync"
)

// ErrHashWorkerPanicked reports that a hash worker died to a recovered panic,
// so the hashed-file stream ended with files still unprocessed.
var ErrHashWorkerPanicked = errors.New("hash worker panicked: some files were not hashed and are missing from this scan")

// StageOutcome records whether a background pipeline stage drained its input
// normally or ended early on a recovered panic.
//
// It exists because a closed output channel cannot say which happened: a stage
// that recovers a panic still runs its deferred close, so the stream terminates
// exactly like a completed one and a caller that reads "closed" as "everything
// was processed" acts on a short batch without knowing it. Stages therefore
// hand a *StageOutcome back alongside their channel; Err is meaningful once the
// channel has been drained to close.
//
// The zero value is ready to use and a nil *StageOutcome reports no failure, so
// call sites that never fail need no special casing.
type StageOutcome struct {
	mu  sync.Mutex
	err error
}

// Fail records err as the reason the stage ended early. The first failure wins,
// so a later worker cannot overwrite the panic that actually truncated the
// stream. It is safe to call from several goroutines at once.
func (o *StageOutcome) Fail(err error) {
	if o == nil || err == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.err == nil {
		o.err = err
	}
}

// Err returns the failure that ended the stage early, or nil when it completed
// normally. Read it after the stage's output channel has closed.
func (o *StageOutcome) Err() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.err
}
