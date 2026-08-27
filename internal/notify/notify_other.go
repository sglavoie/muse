//go:build !darwin && !linux

package notify

import (
	"fmt"
	"runtime"
)

// backendCommand always fails on platforms muse has no notification backend
// for. Callers treat the error as advisory: notifications are opt-in and the
// rest of muse works fine without them.
func backendCommand(Track) (string, []string, error) {
	return "", nil, fmt.Errorf("notifications are not supported on %s", runtime.GOOS)
}
