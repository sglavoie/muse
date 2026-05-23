package player

import (
	"errors"
	"fmt"
	"net"
	"os"
)

// cleanStaleSocket removes the Unix socket file at socketPath if it exists but
// no live process is listening on it. Returns an error if a live process owns
// it, or if the path exists but is not a socket.
// Note: app.Run already removes any stale socket in step 2 (after acquiring
// the PID lockfile), so this function is not called during normal startup.
// It is retained for tests and for any caller that needs the liveness check.
func cleanStaleSocket(socketPath string) error {
	info, err := os.Lstat(socketPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("socket %s: stat: %w", socketPath, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("socket %s exists but is not a Unix socket", socketPath)
	}

	conn, err := net.DialTimeout("unix", socketPath, staleSocketDialTimeout)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("socket %s is already in use by a live mpv instance", socketPath)
	}

	return os.Remove(socketPath)
}
