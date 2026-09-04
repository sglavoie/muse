package player

// This file exposes the player-owned playback timeline as an immutable
// snapshot. Traversal state is spread across shuffle history, a cursor, an
// unseen pool, and the cycle exclusions; a consumer that read those fields one
// at a time would see a timeline that never existed. QueueTraversal resolves
// them together under a single lock acquisition and hands back plain queue
// indices, so the caller never holds a reference to Manager state.

// QueueTraversal returns an immutable snapshot of the playback timeline. The
// returned slice is freshly allocated and owned by the caller.
func (m *Manager) QueueTraversal() QueueTraversal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.shuffleEnabled {
		return m.shuffledTraversalLocked()
	}
	return m.sequentialTraversalLocked()
}

// sequentialTraversalLocked snapshots plain visible-order traversal: the order
// is the queue itself and everything after the current index is remaining, plus
// the rows that were inserted behind the cursor and are still eligible — the
// resume plays those before it scans forward, so a consumer counting only the
// tail would read an extended queue as finished.
//
// Once a resume has stepped back onto one of those insertions the tail stops
// being what is left: the rows between the cursor and the frontier have already
// played. What remains is then the continuation itself — the insertions still
// ahead plus whatever lies past the frontier — and counting the tail instead
// would report an exhausted queue as having more to play, which is what makes a
// consumer resume onto a row it already heard.
//
// The caller must hold m.mu.
func (m *Manager) sequentialTraversalLocked() QueueTraversal {
	n := len(m.queue)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	// With no current entry the whole queue is still ahead: sequential playback
	// starts at index 0, so every row is remaining.
	t := QueueTraversal{Order: order, CurrentPosition: -1, Remaining: n}
	if m.currentIndex >= 0 && m.currentIndex < n {
		t.CurrentPosition = m.currentIndex
		ahead := n - m.currentIndex - 1
		if m.sequentialFrontierLocked() > m.currentIndex {
			ahead = len(m.sequentialSuccessorsLocked(m.currentIndex + 1))
		}
		t.Remaining = ahead + m.countUnreachedAdditionsLocked()
	}
	return t
}

// shuffledTraversalLocked snapshots shuffle traversal in the order the Manager
// will visit it: played history, the current entry, the forward history a
// Previous left unreplayed, then the unseen pool. Identities are resolved to
// live queue indices, so duplicate rows stay distinct (each occurrence carries
// its own identity) and identities whose rows have left the queue simply drop
// out of the snapshot.
//
// The caller must hold m.mu.
func (m *Manager) shuffledTraversalLocked() QueueTraversal {
	p := newTraversalPlacer(queueIdentities(m.queue))
	position, remaining := m.placeHistoryLocked(p)
	for _, id := range m.shuffleUnseen {
		if p.place(id) {
			remaining++
		}
	}
	p.placeUnscheduled()
	return QueueTraversal{
		Order:           p.order,
		CurrentPosition: position,
		Remaining:       remaining,
		Shuffled:        true,
	}
}

// placeHistoryLocked places the played history, the current entry, and the
// forward history a Previous left unreplayed, in that order. It returns the
// position of the current entry within the order built so far (-1 when the
// cursor names no live row) and how many placed history entries are still ahead
// of it.
//
// The caller must hold m.mu.
func (m *Manager) placeHistoryLocked(p *traversalPlacer) (position, remaining int) {
	position = -1
	for h, id := range m.shuffleHistory {
		if !p.place(id) {
			continue
		}
		switch {
		case h == m.shufflePos:
			position = len(p.order) - 1
		case h > m.shufflePos:
			// Forward history left behind by Previous, or — when the cursor sits
			// at -1 — history that is entirely ahead of playback.
			remaining++
		}
	}
	return position, remaining
}

// traversalPlacer builds a snapshot order by resolving shuffle identities to
// live queue indices, recording each row at most once so the result stays a
// permutation of the live queue.
type traversalPlacer struct {
	byIdentity map[string]int
	placed     []bool
	order      []int
}

func newTraversalPlacer(ids []string) *traversalPlacer {
	byIdentity := make(map[string]int, len(ids))
	for i, id := range ids {
		byIdentity[id] = i
	}
	return &traversalPlacer{
		byIdentity: byIdentity,
		placed:     make([]bool, len(ids)),
		order:      make([]int, 0, len(ids)),
	}
}

// place appends the live queue row named by id and reports whether it was
// appended. An identity that no longer resolves to a queue row — or that the
// traversal state names twice, which reconciliation prevents but a snapshot must
// not depend on — is skipped.
func (p *traversalPlacer) place(id string) bool {
	i, ok := p.byIdentity[id]
	if !ok || p.placed[i] {
		return false
	}
	p.placed[i] = true
	p.order = append(p.order, i)
	return true
}

// placeUnscheduled appends the live rows the cycle no longer schedules — entries
// skipped as missing, anything excluded from the current cycle — so they stay
// visible at the end of the order without counting towards what is left to play.
func (p *traversalPlacer) placeUnscheduled() {
	for i, done := range p.placed {
		if !done {
			p.order = append(p.order, i)
		}
	}
}
