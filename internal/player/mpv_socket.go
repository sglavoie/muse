package player

import (
	"fmt"
	"net"
	"os"
)

// cleanStaleSocket removes the socket file at socketPath if it exists but no
// live process is listening on it. Returns an error if a live process owns it.
// Note: app.Run already removes any stale socket in step 2 (after acquiring
// the PID lockfile), so this function is not called during normal startup.
// It is retained for tests and for any caller that needs the liveness check.
func cleanStaleSocket(socketPath string) error {
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return nil
	}

	conn, err := net.DialTimeout("unix", socketPath, staleSocketDialTimeout)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("socket %s is already in use by a live mpv instance", socketPath)
	}

	return os.Remove(socketPath)
}
