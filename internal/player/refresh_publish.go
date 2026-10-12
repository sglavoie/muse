package player

// publishRefreshLocked adopts the refreshed queue and rewrites only the
// traversal state the substitution touches.
//
// The order matters. The cursor is re-resolved by identity first, because every
// piece of positional bookkeeping below is measured against it and a numeric
// cursor left alone would rename itself to whatever moved into its slot. The
// shuffle-off insertion record is refreshed next, which prunes the rows that
// left and picks up any successor that landed behind the cursor — where the
// positional scan would never reach it. The unseen pool is substituted last, in
// place, so a successor inherits its predecessor's turn instead of being
// appended behind the whole cycle.
//
// The caller must hold m.mu.
func (m *Manager) publishRefreshLocked(r QueueRefresh, plan refreshPlan) RefreshResult {
	prev := make([]string, len(r.Snapshot.guard.queue))
	for i, id := range r.Snapshot.guard.queue {
		prev[i] = string(id)
	}

	m.queue = make([]QueueTrack, len(r.Queue))
	copy(m.queue, r.Queue)
	m.relocateCurrentLocked(string(r.Snapshot.guard.current))
	m.recordUnreachedAdditionsLocked(prev)
	m.substituteShuffleLocked(plan)

	changed := len(plan.sub) > 0 || plan.added > 0
	return RefreshResult{
		Replaced:            len(plan.sub) - plan.retired,
		Retired:             plan.retired,
		Added:               plan.added,
		CurrentIndex:        m.currentIndex,
		PrefetchInvalidated: changed,
	}
}

// substituteShuffleLocked rewrites the unseen pool in place: a replaced identity
// becomes its successor at the same position, a retired one drops out, and the
// rows the refresh did not name keep the turn the cycle already gave them.
//
// History is not touched at all, and does not need to be: validation has already
// established that no replaced row is the current entry, played history, or
// forward history, and that no unnamed row left the queue. So every identity
// history holds is still live, and the cycle keeps its anchor, its cursor, and
// its permutation.
//
// The caller must hold m.mu.
func (m *Manager) substituteShuffleLocked(plan refreshPlan) {
	if !m.shuffleEnabled {
		return
	}
	ids := queueIdentities(m.queue)
	live := make(map[string]bool, len(ids))
	for _, id := range ids {
		live[id] = true
	}
	m.pruneExclusionsLocked(live)

	seen := make(map[string]bool, len(ids))
	for _, id := range m.shuffleHistory {
		seen[id] = true
	}
	unseen := m.substitutedPoolLocked(plan, live, seen)
	m.shuffleUnseen = m.appendRefreshAdditionsLocked(unseen, ids, seen)
}

// substitutedPoolLocked walks the unseen pool once, swapping each replaced
// identity for its successor at the same position and dropping the retired ones.
// seen accumulates what the rewritten pool already schedules.
//
// The caller must hold m.mu.
func (m *Manager) substitutedPoolLocked(plan refreshPlan, live, seen map[string]bool) []string {
	unseen := make([]string, 0, len(m.shuffleUnseen))
	for _, id := range m.shuffleUnseen {
		if succ, replaced := plan.sub[id]; replaced {
			if succ == "" {
				continue
			}
			id = succ
		}
		if !live[id] || seen[id] {
			continue
		}
		unseen = append(unseen, id)
		seen[id] = true
	}
	return unseen
}

// appendRefreshAdditionsLocked places the rows the refresh gained without naming
// them. They are ordinary additions and join the end of the cycle exactly as
// reconciliation would have placed them — and, as there, a row the cycle already
// gave up on is not brand-new and must not be scheduled again.
//
// The caller must hold m.mu.
func (m *Manager) appendRefreshAdditionsLocked(unseen, ids []string, seen map[string]bool) []string {
	for _, id := range ids {
		if !seen[id] && !m.shuffleExcluded[id] {
			unseen = append(unseen, id)
			seen[id] = true
		}
	}
	return unseen
}
