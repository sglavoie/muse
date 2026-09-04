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
//
// Teardown and the synchronous player calls join the same ordering through
// supersede and runExclusive: abandoning playback (Stop, a queue clear) or
// starting it from the Update goroutine (autoplay, an extension resume) counts
// as a newer request, so a selection still waiting to run is cancelled rather
// than allowed to restart or retarget playback behind it.
type playRequestGate struct {
	// exec serializes the player calls themselves. Requests dispatched to a
	// goroutine take it there; the Update goroutine takes it only in
	// runExclusive, for a call it is already making synchronously, and only
	// behind a supersede that has already emptied the queue of older requests.
	// Each such call already serializes against the in-flight one inside the
	// player itself, so the gate adds ordering rather than a new way to stall.
	exec sync.Mutex

	mu sync.Mutex
	// latest is the identity of the most recently dispatched request. It mirrors
	// the model's playRequestSeq, which is the same number carried on the result
	// message; keeping it here is what makes it readable from a goroutine that
	// only holds a stale copy of the Model.
	latest uint64
}

// supersede invalidates every request dispatched before now and returns the
// identity that does it. prev is the caller's current identity, so the counter
// is continuous even for a Model built without a gate.
//
// The caller must adopt the returned value as its own playRequestSeq. The two
// halves of a cancellation need both numbers: the gate's copy is what keeps a
// superseded request from reaching the player, and the model's copy is what
// discards the result message that request still produces.
//
// It is called on the Update goroutine and never blocks on the player.
func (g *playRequestGate) supersede(prev uint64) uint64 {
	if g == nil {
		return prev + 1
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	// latest normally already mirrors prev; taking the larger of the two keeps
	// the counter monotonic for a model whose seq ran ahead of the gate.
	if prev > g.latest {
		g.latest = prev
	}
	g.latest++
	return g.latest
}

// dispatch records a new request and returns its identity, superseding every
// request dispatched before it.
func (g *playRequestGate) dispatch(prev uint64) uint64 {
	return g.supersede(prev)
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

// runExclusive performs a player call the Update goroutine is making itself,
// as the newest request. It returns the identity the caller must adopt.
//
// Superseding is not enough on its own for these calls. A request that already
// passed the gate's check is inside the player, and cancellation cannot reach
// it; holding exec makes this call queue behind that one instead of racing it,
// so an older selection can never be the last thing the player was told. What
// it waits on it would have waited on anyway — the player serializes a Stop
// against the command socket and a PlayFromIndex or resume against the playlist
// transaction — so the wait is the same one, taken one layer earlier.
func (g *playRequestGate) runExclusive(prev uint64, call func() error) (uint64, error) {
	req := g.supersede(prev)
	if g == nil {
		return req, call()
	}
	g.exec.Lock()
	defer g.exec.Unlock()
	return req, call()
}

// supersedePlayRequests cancels every play request dispatched so far. Callers
// that abandon playback without touching the player use it directly; those that
// do touch it want playRequestExclusive, which also orders against the request
// that is already inside the player.
func (m *Model) supersedePlayRequests() {
	m.playback.playRequestSeq = m.playback.requests.supersede(m.playback.playRequestSeq)
}

// playRequestExclusive makes a synchronous player call the newest play request,
// cancelling the older ones and waiting out whichever one is already running.
func (m *Model) playRequestExclusive(call func() error) error {
	req, err := m.playback.requests.runExclusive(m.playback.playRequestSeq, call)
	m.playback.playRequestSeq = req
	return err
}
