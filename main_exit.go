package main

import (
	"errors"

	"github.com/sglavoie/muse/internal/config"
)

// exitAlreadyRunning is the exit status when another muse holds the instance
// lock, so a host launching muse can tell that apart from any other failure
// without matching the message text.
const exitAlreadyRunning = 3

// exitCodeFor maps the error runTUI returned to the process exit status.
func exitCodeFor(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, config.ErrAlreadyRunning):
		return exitAlreadyRunning
	default:
		return 1
	}
}
