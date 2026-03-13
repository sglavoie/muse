//go:build !darwin

package library

// raiseFDLimit is a no-op on non-Darwin platforms where fd exhaustion from
// fsnotify watches is not a common concern.
func raiseFDLimit() {}
