package player

// Sequential traversal is positional: the next row is currentIndex+1, and the
// cursor only ever moves forward. That is exact while the queue is stable, but a
// queue can grow anywhere — a paste or a "play next" inserts at the cursor, not
// at the tail — and a row inserted at or before the cursor is a row positional
// arithmetic will never reach even though the traversal never played it.
//
// This file holds the pieces that close that gap: following the cursor row by
// identity so an insertion cannot silently rename it, recording the rows that
// landed behind the cursor so the resume can still find them, and pinning the
// prefix the traversal had already played so that stepping back onto one of
// those rows does not turn all of it into successors again — a boundary that has
// to survive the frontier row itself being deleted or replaced.
// It is the shuffle-off counterpart to the unseen pool in shuffle.go, where
// reconciliation already makes every brand-new identity eligible regardless of
// position.

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

// unreachedAdditionsLocked returns the recorded additions that sit before limit,
// in queue order, so a resume attempts the earliest one first. Identities that no
// longer resolve to a queue row simply drop out.
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

// forgetAdditionsThroughLocked drops every recorded addition at or before limit
// — the ones a step onto limit has just moved past, the row it is starting
// included — and returns what it removed so a failed load can put them back.
//
// The caller must hold m.mu.
func (m *Manager) forgetAdditionsThroughLocked(limit int) []string {
	pending := m.unreachedAdditionsLocked(limit + 1)
	if len(pending) == 0 {
		return nil
	}
	dropped := make([]string, 0, len(pending))
	for _, add := range pending {
		dropped = append(dropped, add.identity)
		delete(m.unreachedAdditions, add.identity)
	}
	if len(m.unreachedAdditions) == 0 {
		m.unreachedAdditions = nil
	}
	return dropped
}

// restoreAdditionsLocked reinstates identities dropped by a step that then
// failed to load, so the rows stay eligible for the next attempt.
//
// The caller must hold m.mu.
func (m *Manager) restoreAdditionsLocked(identities []string) {
	if len(identities) == 0 {
		return
	}
	if m.unreachedAdditions == nil {
		m.unreachedAdditions = make(map[string]bool, len(identities))
	}
	for _, id := range identities {
		m.unreachedAdditions[id] = true
	}
}

// pinSequentialFrontierLocked records the rows the traversal has already played
// — everything at or before the cursor that is not still-pending insertion — as
// the completion boundary a step backwards has to respect.
//
// The whole prefix is recorded, not just the row the cursor sits on. That row is
// the frontier while it lives, but a queue is editable: deleting it, or replacing
// it through SetQueue, would otherwise leave nothing saying where playback had
// got to, and continuation would fall back to plain arithmetic over rows that
// already played. With the prefix recorded the boundary simply falls back onto
// the deepest completed row that survived.
//
// The caller must hold m.mu.
func (m *Manager) pinSequentialFrontierLocked() {
	completed := make(map[string]bool)
	for i, id := range queueIdentities(m.queue) {
		if i > m.currentIndex {
			break
		}
		// An insertion the resume has not started yet sits inside the prefix
		// positionally but is not part of what played, so it must never become
		// the boundary the other insertions are measured against.
		if m.unreachedAdditions[id] {
			continue
		}
		completed[id] = true
	}
	if len(completed) == 0 {
		return
	}
	m.sequentialCompleted = completed
}

// sequentialFrontierLocked resolves the recorded completion boundary to a live
// queue index — the furthest recorded row still in the queue — or -1 when no
// boundary stands or every row it named has left the queue. It is meaningful
// only while it is strictly ahead of the cursor; at or behind it, positional
// continuation has already caught up and the frontier says nothing.
//
// Resolving to the furthest survivor rather than to one pinned identity is what
// keeps a removal from resurrecting the rows behind it: whatever is left of the
// completed prefix still separates it from the rows the traversal has yet to
// reach, and a row appended past the whole prefix still lands beyond the
// boundary.
//
// The caller must hold m.mu.
func (m *Manager) sequentialFrontierLocked() int {
	if len(m.sequentialCompleted) == 0 {
		return -1
	}
	frontier := -1
	for i, id := range queueIdentities(m.queue) {
		if m.sequentialCompleted[id] {
			frontier = i
		}
	}
	return frontier
}

// sequentialSuccessorsLocked returns, in the order an automatic shuffle-off
// continuation must try them, the queue indices it may visit after the row at
// from-1.
//
// With no frontier ahead of the cursor that is the plain positional scan every
// stable queue takes. With one, the cursor is walking rows that were inserted
// behind an exhausted traversal: the rows between it and the frontier are the
// ones that already played, so continuing positionally would hand them back as
// successors. The insertions that are still pending come first, in queue order,
// and the scan resumes beyond the frontier — which is where a later tail append
// lands.
//
// The caller must hold m.mu.
func (m *Manager) sequentialSuccessorsLocked(from int) []int {
	if from < 0 {
		from = 0
	}
	n := len(m.queue)
	frontier := m.sequentialFrontierLocked()
	scanFrom := from
	var out []int
	if frontier >= from {
		for _, add := range m.unreachedAdditionsLocked(frontier + 1) {
			if add.index >= from {
				out = append(out, add.index)
			}
		}
		scanFrom = frontier + 1
	}
	for i := scanFrom; i < n; i++ {
		out = append(out, i)
	}
	return out
}
