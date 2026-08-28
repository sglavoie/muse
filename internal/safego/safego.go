// Package safego provides panic recovery for background goroutines.
//
// Bubble Tea recovers panics raised in its own event loop and in the goroutines
// it starts for tea.Cmd values, but muse also runs long-lived goroutines that
// Bubble Tea never sees: the library scan pipeline, the filesystem watcher, the
// mpv IPC event reader and restart supervisor, the duration audit, and the
// startup/shutdown orchestration. A panic in any of those takes the whole
// process down immediately, with no shutdown at all — most visibly leaving an
// orphaned `mpv --idle=yes` child behind, since mpv is not killed when its
// parent dies.
//
// Recover turns such a panic into a logged error so the process survives and the
// stack trace lands in the log file at config.LogPath().
//
// # Usage
//
// Defer Recover as the *first* statement of the goroutine body, keeping any
// existing defers in place:
//
//	go func() {
//		defer safego.Recover("library.Walk")
//		defer close(ch)
//		...
//	}()
//
// Deferred calls all run while a panic unwinds, so an existing `defer close(ch)`
// still executes and downstream receivers still unblock. That property is
// load-bearing: recovering in a pipeline stage that then failed to close its
// outbound channel would convert a crash into a hang, which is worse. Never
// move a channel close out of a defer at a recovered site.
//
// For a goroutine that is a single call, Go wraps the spawn instead:
//
//	safego.Go("player.prefetchNext", m.prefetchNext)
package safego

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// Recover recovers a panic in the goroutine that deferred it and logs the panic
// value together with the stack trace of the panicking goroutine. It must be
// deferred directly (not wrapped in another closure) for recover to take effect.
// When no panic is in flight it does nothing.
//
// name identifies the goroutine in the log and should be stable and specific,
// e.g. "app.runLibraryPipeline".
func Recover(name string) {
	handle(name, recover(), nil)
}

// RecoverTo behaves like Recover and additionally reports the recovered panic to
// handler as an error, so a site that already has an error channel or callback
// can surface the failure instead of silently continuing. handler is called only
// when a panic was actually recovered, and must not panic itself.
func RecoverTo(name string, handler func(error)) {
	handle(name, recover(), handler)
}

// Go runs fn in a new goroutine with Recover installed. Use it for goroutines
// whose body is a single call; goroutines that own a channel close or other
// defers should defer Recover inside their own body instead, so the ordering of
// those defers stays visible at the call site.
func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}

// handle is the shared tail of Recover and RecoverTo. It is a separate function
// so both entry points can call recover() directly in their own frame.
func handle(name string, r any, handler func(error)) {
	if r == nil {
		return
	}
	err := fmt.Errorf("panic in background goroutine %s: %v", name, r)
	slog.Error("recovered panic in background goroutine",
		"goroutine", name,
		"panic", fmt.Sprint(r),
		"stack", string(debug.Stack()),
	)
	if handler != nil {
		handler(err)
	}
}
