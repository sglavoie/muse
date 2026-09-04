package tui

import "sync"

// playRequestGate orders asynchronous player calls dispatched by queue
// selections and transport controls.
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
//   - pending requests wait for every earlier request that still matters, so a
//     later Pause, Play, Next, or Previous cannot reach the old physical row
//     while a queue selection is waiting to load.
//   - consecutive selections coalesce before execution. Only the newest target
//     reaches the player, preserving rapid-selection supersession without
//     dropping a selection that an intervening transport control depends on.
//
// The waits happen only inside tea.Cmd goroutines. The Bubble Tea Update loop
// records the request and returns immediately.
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
	// cond wakes requests when an earlier call completes or a queued request is
	// cancelled. It is initialized lazily because some tests construct the gate
	// as a zero value.
	cond *sync.Cond
	// pending contains only requests that can still reach the player. Selection
	// requests are supersedable until they begin executing; transport controls
	// are ordering barriers and are never coalesced with a later selection.
	pending map[uint64]*playRequestState
	// latest is the identity of the most recently dispatched request. It mirrors
	// the model's requestSeq; keeping it here is what makes it readable from a
	// goroutine that only holds a stale copy of the Model.
	latest uint64
}

type playRequestState struct {
	supersedable bool
	running      bool
	waiting      bool
}

func (g *playRequestGate) initLocked() {
	if g.cond == nil {
		g.cond = sync.NewCond(&g.mu)
	}
	if g.pending == nil {
		g.pending = make(map[uint64]*playRequestState)
	}
}

func (g *playRequestGate) nextLocked(prev uint64) uint64 {
	if prev > g.latest {
		g.latest = prev
	}
	g.latest++
	return g.latest
}

// cancelPendingLocked drops requests that have not entered the player. When
// all is false, it coalesces only the trailing run of selections after the most
// recent ordering barrier; an intervening control must still follow the
// selection that preceded it.
func (g *playRequestGate) cancelPendingLocked(all bool) {
	barrier := uint64(0)
	if !all {
		for req, state := range g.pending {
			if state.running || !state.supersedable {
				barrier = max(barrier, req)
			}
		}
	}
	for req, state := range g.pending {
		if state.running || (!all && (!state.supersedable || req <= barrier)) {
			continue
		}
		delete(g.pending, req)
	}
	g.cond.Broadcast()
}

func (g *playRequestGate) hasEarlierLocked(req uint64) bool {
	for earlier := range g.pending {
		if earlier < req {
			return true
		}
	}
	return false
}

// supersede invalidates every request dispatched before now and returns the
// identity that does it. prev is the caller's current identity, so the counter
// is continuous even for a Model built without a gate.
//
// The caller must adopt the returned value as its own requestSeq. Teardown
// callers also copy it to playRequestSeq so a pending selection result becomes
// stale. The two
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
	g.initLocked()
	g.cancelPendingLocked(true)
	return g.nextLocked(prev)
}

// dispatch records a selection and returns its identity. It supersedes only
// the trailing selections that no intervening transport control depends on.
func (g *playRequestGate) dispatch(prev uint64) uint64 {
	if g == nil {
		return prev + 1
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initLocked()
	g.cancelPendingLocked(false)
	req := g.nextLocked(prev)
	g.pending[req] = &playRequestState{supersedable: true}
	return req
}

// dispatchControl records a transport control without superseding the queue
// selection before it. The returned request runs asynchronously in dispatch
// order through run.
func (g *playRequestGate) dispatchControl(prev uint64) uint64 {
	if g == nil {
		return prev + 1
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initLocked()
	req := g.nextLocked(prev)
	g.pending[req] = &playRequestState{}
	return req
}

// run performs request req's player call after every earlier live request. It
// reports whether the call was made: a coalesced or cancelled request is not a
// failure, it simply never reached the player, and its result message is
// discarded by the same identity check that discards a superseded outcome.
//
// A Model without a gate runs the call unordered, which is the behaviour that
// preceded the gate. Only hand-built test models are in that state; New always
// installs one.
func (g *playRequestGate) run(req uint64, call func() error) (error, bool) {
	if g == nil {
		return call(), true
	}
	g.mu.Lock()
	g.initLocked()
	for {
		state, ok := g.pending[req]
		if !ok {
			g.mu.Unlock()
			return nil, false
		}
		if !g.hasEarlierLocked(req) {
			state.waiting = false
			state.running = true
			break
		}
		state.waiting = true
		g.cond.Wait()
	}
	g.mu.Unlock()

	g.exec.Lock()
	err := call()
	g.exec.Unlock()

	g.mu.Lock()
	delete(g.pending, req)
	g.cond.Broadcast()
	g.mu.Unlock()
	return err, true
}

// runExclusive performs a player call the Update goroutine is making itself,
// as the newest request. It returns the identity the caller must adopt.
//
// Superseding is not enough on its own for these calls. A request already
// running inside the player cannot be cancelled, so the exclusive request
// waits for it before entering. What it waits on it would have waited on anyway
// — the player serializes a Stop against the command socket and a PlayFromIndex
// or resume against the playlist transaction — so the wait is the same one,
// taken one layer earlier.
func (g *playRequestGate) runExclusive(prev uint64, call func() error) (uint64, error) {
	if g == nil {
		return prev + 1, call()
	}
	g.mu.Lock()
	g.initLocked()
	g.cancelPendingLocked(true)
	req := g.nextLocked(prev)
	g.pending[req] = &playRequestState{}
	for g.hasEarlierLocked(req) {
		g.cond.Wait()
	}
	g.pending[req].running = true
	g.mu.Unlock()

	g.exec.Lock()
	err := call()
	g.exec.Unlock()

	g.mu.Lock()
	delete(g.pending, req)
	g.cond.Broadcast()
	g.mu.Unlock()
	return req, err
}

// supersedePlayRequests cancels every play request dispatched so far. Callers
// that abandon playback without touching the player use it directly; those that
// do touch it want playRequestExclusive, which also orders against the request
// that is already inside the player.
func (m *Model) supersedePlayRequests() {
	req := m.playback.requests.supersede(m.playback.requestSeq)
	m.playback.requestSeq = req
	m.playback.playRequestSeq = req
}

// playRequestExclusive makes a synchronous player call the newest play request,
// cancelling the older ones and waiting out whichever one is already running.
func (m *Model) playRequestExclusive(call func() error) error {
	req, err := m.playback.requests.runExclusive(m.playback.requestSeq, call)
	m.playback.requestSeq = req
	m.playback.playRequestSeq = req
	return err
}
