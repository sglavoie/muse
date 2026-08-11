package player

import (
	"time"
)

// effectiveBaseDelay is the initial restart backoff delay, honouring the
// test-only restartBaseDelay override.
func (m *Manager) effectiveBaseDelay() time.Duration {
	if m.restartBaseDelay != 0 {
		return m.restartBaseDelay
	}
	return restartInitialDelay
}

// effectiveMaxDelay is the backoff cap, honouring the test-only override.
func (m *Manager) effectiveMaxDelay() time.Duration {
	if m.restartMaxDelayOverride != 0 {
		return m.restartMaxDelayOverride
	}
	return restartMaxDelay
}

// effectiveMaxAttempts is the maximum number of launch-and-connect attempts per
// outage, honouring the test-only override.
func (m *Manager) effectiveMaxAttempts() int {
	if m.restartMaxAttemptsOverride > 0 {
		return m.restartMaxAttemptsOverride
	}
	return restartMaxAttempts
}

// effectiveSocketPollTimeout bounds a single connect attempt, honouring the
// test-only override.
func (m *Manager) effectiveSocketPollTimeout() time.Duration {
	if m.socketPollTimeoutOverride != 0 {
		return m.socketPollTimeoutOverride
	}
	return socketPollTimeout
}
