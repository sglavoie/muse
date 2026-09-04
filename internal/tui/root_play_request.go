package tui

import "sync"

// playRequestGate orders the asynchronous player calls that
// handlePlayQueueIndexMsg dispatches.
//
// Every play or replay the queue panel triggers is handed to a tea.Cmd, so the
// call itself happens on its own goroutine. Nothing about those goroutines
// preserves the order the Update loop dispatched them in: the runtime schedules
// them freely, and the player's own playlist mutex then serializes whichever one
// arrives first rather than whichever one the user asked for first. Two Enters
// on two different rows can therefore reach mpv in reverse, leaving the player
// on the row the user abandoned — and emitting that row's start-file event,
// which the TUI applies as the current track.
//
// The gate is the missing order. It is shared by every copy of the Model (it is
// held by pointer, and Model is a value type), so a request dispatched after
// another can be recognised from inside the goroutine that runs the earlier one.
//
// Two rules, together, make execution follow dispatch:
//
//   - exec is held across the player call, so at most one request is inside the
//     player at a time and their effects cannot interleave.
//   - a request that finds a newer one already dispatched is superseded and
//     never reaches the player at all. Running it could only replace the newer
//     target with an older one; the user has already said they want something
//     else.
//
// Whichever goroutine wins the race, the outcome is the same: the newest
// dispatched request is the one that decides what plays.
type playRequestGate struct {
	// exec serializes the player calls themselves. It is never taken on the
	// Update goroutine — only inside the dispatched commands — so a slow mpv
	// round trip cannot stall the UI.
	exec sync.Mutex

	mu sync.Mutex
	// latest is the identity of the most recently dispatched request. It mirrors
	// the model's playRequestSeq, which is the same number carried on the result
	// message; keeping it here is what makes it readable from a goroutine that
	// only holds a stale copy of the Model.
	latest uint64
}

// dispatch records a new request and returns its identity, superseding every
// request dispatched before it. prev is the caller's current identity, so the
// counter is continuous even for a Model built without a gate.
//
// It is called on the Update goroutine and never blocks on the player.
func (g *playRequestGate) dispatch(prev uint64) uint64 {
	req := prev + 1
	if g == nil {
		return req
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if req > g.latest {
		g.latest = req
	}
	return req
}

// run performs request req's player call, unless a newer request has been
// dispatched since. It reports whether the call was made: a superseded request
// is not a failure, it simply never reached the player, and its result message
// is discarded by the same identity check that discards a superseded outcome.
//
// A Model without a gate runs the call unordered, which is the behaviour that
// preceded the gate. Only hand-built test models are in that state; New always
// installs one.
func (g *playRequestGate) run(req uint64, call func() error) (error, bool) {
	if g == nil {
		return call(), true
	}
	g.exec.Lock()
	defer g.exec.Unlock()
	if g.superseded(req) {
		return nil, false
	}
	return call(), true
}

// superseded reports whether a request newer than req has been dispatched.
func (g *playRequestGate) superseded(req uint64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return req < g.latest
}
