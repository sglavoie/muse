package main

import (
	"context"
	"os"
	"syscall"
)

// shutdownSignals are the signals that end muse through the graceful Shutdown
// sequence. SIGHUP is among them because a terminal that goes away — a closed
// window, or a host such as muse.app freeing its terminal surface — hangs up
// the whole process group; without it muse would die with the DB writer
// unflushed and the in-flight listen lost.
func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}

// endedBySignal reports whether p.Run's error is the fallout of a shutdown
// signal rather than a TUI failure: the signal context was cancelled, so the
// error is treated as the clean exit the signal asked for.
func endedBySignal(ctx context.Context, err error) bool {
	return err != nil && ctx.Err() != nil
}
