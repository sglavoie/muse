package app

import "context"

// SetProbeFn replaces the ffprobe function used by RunDurationAudit. Tests
// call this to inject a stub and restore the original on cleanup.
func SetProbeFn(fn func(context.Context, string) (float64, error)) (restore func()) {
	prev := probeFn
	probeFn = fn
	return func() { probeFn = prev }
}

// ClampVolume is a test-only re-export of the unexported clampVolume helper.
func ClampVolume(v int) int { return clampVolume(v) }
