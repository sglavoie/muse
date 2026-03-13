//go:build darwin

package library

import (
	"log/slog"
	"syscall"
)

// raiseFDLimit attempts to raise the process soft file-descriptor limit to the
// system hard maximum on macOS. Large music libraries can exhaust the default
// soft limit (256) when fsnotify registers one inotify watch per directory.
// If the Getrlimit or Setrlimit syscall fails, a warning is logged and the
// program continues with whatever limit is already in effect.
func raiseFDLimit() {
	var rl syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl); err != nil {
		slog.Warn("watcher: failed to get fd limit; watch may fail on large libraries", "err", err)
		return
	}
	rl.Cur = rl.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rl); err != nil {
		slog.Warn("watcher: failed to raise fd limit; watch may fail on large libraries", "err", err)
	}
}
