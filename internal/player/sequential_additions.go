package player

// Sequential traversal is positional: the next row is currentIndex+1, and the
// cursor only ever moves forward. That is exact while the queue is stable, but a
// queue can grow anywhere — a paste or a "play next" inserts at the cursor, not
// at the tail — and a row inserted at or before the cursor is a row positional
// arithmetic will never reach even though the traversal never played it.
//
// This file holds the two pieces that close that gap: following the cursor row
// by identity so an insertion cannot silently rename it, and recording the rows
// that landed behind the cursor so the resume can still find them. It is the
// shuffle-off counterpart to the unseen pool in shuffle.go, where reconciliation
// already makes every brand-new identity eligible regardless of position.

// unreachedAddition is a queue row that entered the queue behind the sequential
// cursor and has not been played. The identity travels with the index because
// the caller needs it to forget the row when its file turns out to be missing.
type unreachedAddition struct {
	index    int
	identity string
}

// relocateCurrentLocked re-resolves currentIndex after the queue's membership
// changed, following identity so the cursor keeps naming the same row: an
// insertion before it shifts every later position, and a numeric cursor left
// alone would silently rename itself to whatever moved into its slot. That is
// what makes the row a completed traversal stopped on survive an insertion
// instead of being replayed by the next resume.
//
// identity is the cursor's identity before the mutation; an empty identity, or
// one whose row has left the queue, falls back to clamping into range.
//
// The caller must hold m.mu.
func (m *Manager) relocateCurrentLocked(identity string) {
	if identity != "" {
		if idx := identityIndex(m.queue, identity); idx >= 0 {
			m.currentIndex = idx
			return
		}
	}
	if m.currentIndex >= len(m.queue) {
		if len(m.queue) > 0 {
			m.currentIndex = len(m.queue) - 1
		} else {
			m.currentIndex = -1
		}
	}
}

// recordUnreachedAdditionsLocked notes the rows that are new relative to prev —
// the queue identities from before the mutation — and that sit at or before the
// cursor, where the positional scan will not reach them. Rows added after the
// cursor need no bookkeeping: currentIndex+1 already walks into them, which is
// why a tail append (every Auto-DJ refill, every plain enqueue) records nothing.
//
// Additions accumulate rather than replace, so a second insertion does not make
// the first one unreachable, and identities whose rows have since left the queue
// are pruned here so the set cannot grow across a long session.
//
// The caller must hold m.mu.
func (m *Manager) recordUnreachedAdditionsLocked(prev []string) {
	ids := queueIdentities(m.queue)
	known := make(map[string]bool, len(prev))
	for _, id := range prev {
		known[id] = true
	}
	live := make(map[string]bool, len(ids))
	for _, id := range ids {
		live[id] = true
	}
	for id := range m.unreachedAdditions {
		if !live[id] {
			delete(m.unreachedAdditions, id)
		}
	}
	for i, id := range ids {
		if i > m.currentIndex || known[id] {
			continue
		}
		if m.unreachedAdditions == nil {
			m.unreachedAdditions = make(map[string]bool)
		}
		m.unreachedAdditions[id] = true
	}
	if len(m.unreachedAdditions) == 0 {
		m.unreachedAdditions = nil
	}
}

// unreachedAdditionsLocked returns the recorded additions that sit before limit
// — the first index the positional scan would reach — in queue order, so a
// resume attempts the earliest one first. Identities that no longer resolve to a
// queue row simply drop out.
//
// The caller must hold m.mu.
func (m *Manager) unreachedAdditionsLocked(limit int) []unreachedAddition {
	if len(m.unreachedAdditions) == 0 {
		return nil
	}
	out := make([]unreachedAddition, 0, len(m.unreachedAdditions))
	for i, id := range queueIdentities(m.queue) {
		if i >= limit {
			break
		}
		if m.unreachedAdditions[id] {
			out = append(out, unreachedAddition{index: i, identity: id})
		}
	}
	return out
}

// countUnreachedAdditionsLocked reports how many recorded additions sit at or
// before the cursor. They are rows the traversal will still visit but that the
// "everything after the current index" arithmetic cannot see, so the traversal
// snapshot has to add them to Remaining or a consumer would read an extended
// queue as finished.
//
// The caller must hold m.mu.
func (m *Manager) countUnreachedAdditionsLocked() int {
	return len(m.unreachedAdditionsLocked(m.currentIndex + 1))
}

// forgetAdditionLocked drops an addition the traversal gave up on, so a later
// resume does not attempt the same missing file again. It mirrors what
// excludeIdentityLocked does for the shuffle cycle.
//
// The caller must hold m.mu.
func (m *Manager) forgetAdditionLocked(identity string) {
	delete(m.unreachedAdditions, identity)
	if len(m.unreachedAdditions) == 0 {
		m.unreachedAdditions = nil
	}
}
