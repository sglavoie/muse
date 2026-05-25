package app

import "context"

// SetProbeFn replaces the ffprobe function used by RunDurationAudit. Tests
// call this to inject a stub and restore the original on cleanup.
func SetProbeFn(fn func(context.Context, string) (float64, error)) (restore func()) {
	probeFnMu.Lock()
	prev := probeFn
	probeFn = fn
	probeFnMu.Unlock()
	return func() {
		probeFnMu.Lock()
		probeFn = prev
		probeFnMu.Unlock()
	}
}

// ClampVolume is a test-only re-export of the volume clamping logic.
func ClampVolume(v int) int { return max(0, min(v, 100)) }
