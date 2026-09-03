package player

import "strconv"

// trackIdentity returns a stable identity for a single queue entry. The
// persisted queue-row ID is the primary identity so duplicate tracks (same
// hash and path) remain distinct queue members. Zero-ID entries — used in
// tests and before a row is persisted — fall back to a deterministic
// hash/path/occurrence key so duplicates still stay distinct.
func trackIdentity(t QueueTrack, occurrence int) string {
	if t.QueueID != 0 {
		return "q:" + strconv.FormatInt(t.QueueID, 10)
	}
	return "f:" + t.Hash + "|" + t.Path + "|" + strconv.Itoa(occurrence)
}

// queueIdentities returns the identity of every entry in queue, assigning
// occurrence indices to zero-ID duplicates so that repeated hash/path pairs
// stay distinct.
func queueIdentities(queue []QueueTrack) []string {
	ids := make([]string, len(queue))
	occ := make(map[string]int)
	for i, t := range queue {
		if t.QueueID != 0 {
			ids[i] = trackIdentity(t, 0)
			continue
		}
		key := t.Hash + "|" + t.Path
		n := occ[key]
		occ[key]++
		ids[i] = trackIdentity(t, n)
	}
	return ids
}

// identityIndex returns the current queue index of the entry with the given
// identity, or -1 when no entry matches (e.g. the row was removed).
func identityIndex(queue []QueueTrack, id string) int {
	occ := make(map[string]int)
	for i, t := range queue {
		var cur string
		if t.QueueID != 0 {
			cur = trackIdentity(t, 0)
		} else {
			key := t.Hash + "|" + t.Path
			n := occ[key]
			occ[key]++
			cur = trackIdentity(t, n)
		}
		if cur == id {
			return i
		}
	}
	return -1
}

// permuteStrings reorders items according to perm, a permutation of [0,len).
// Out-of-range indices are skipped so a malformed permutation cannot panic.
func permuteStrings(items []string, perm []int) []string {
	out := make([]string, 0, len(items))
	for _, idx := range perm {
		if idx >= 0 && idx < len(items) {
			out = append(out, items[idx])
		}
	}
	return out
}

// permuteLocked returns a permutation of [0,n). It uses the injected permute
// function when set (production wires math/rand/v2; tests supply fixed
// permutations) and otherwise falls back to deterministic identity order so a
// Manager constructed without NewManager still behaves predictably.
//
// The caller must hold m.mu.
func (m *Manager) permuteLocked(n int) []int {
	if m.permute != nil {
		return m.permute(n)
	}
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return p
}

// rebuildShuffleLocked rebuilds the shuffle traversal state anchored at the
// currently playing track (if any). The anchor stays the current track; every
// other queue identity becomes the unseen pool in permuted order. Called when
// shuffle is enabled. The visible/persisted queue order is untouched.
//
// The caller must hold m.mu.
func (m *Manager) rebuildShuffleLocked() {
	ids := queueIdentities(m.queue)
	anchor := -1
	if m.currentIndex >= 0 && m.currentIndex < len(ids) {
		anchor = m.currentIndex
	}
	others := make([]string, 0, len(ids))
	for i, id := range ids {
		if i != anchor {
			others = append(others, id)
		}
	}
	m.shuffleUnseen = permuteStrings(others, m.permuteLocked(len(others)))
	m.shuffleExcluded = nil
	if anchor >= 0 {
		m.shuffleHistory = []string{ids[anchor]}
		m.shufflePos = 0
	} else {
		m.shuffleHistory = nil
		m.shufflePos = -1
	}
}

// clearShuffleLocked discards all shuffle traversal state. Called when shuffle
// is disabled so Next/Previous resume plain visible-order sequencing.
//
// The caller must hold m.mu.
func (m *Manager) clearShuffleLocked() {
	m.shuffleHistory = nil
	m.shufflePos = -1
	m.shuffleUnseen = nil
	m.shuffleExcluded = nil
}

// anchorShuffleLocked resets the shuffle cycle around the entry at index: that
// identity becomes the sole history entry and every other queue identity is
// re-permuted into a fresh unseen pool. Used by explicit selection.
//
// The caller must hold m.mu.
func (m *Manager) anchorShuffleLocked(index int) {
	ids := queueIdentities(m.queue)
	others := make([]string, 0, len(ids))
	for i, id := range ids {
		if i != index {
			others = append(others, id)
		}
	}
	m.shuffleUnseen = permuteStrings(others, m.permuteLocked(len(others)))
	m.shuffleHistory = []string{ids[index]}
	m.shufflePos = 0
	m.shuffleExcluded = nil
}

// restartShuffleCycleLocked generates a brand-new permutation across the whole
// queue and clears the played history, so a fresh cycle visits every identity
// again. When at least two entries exist it avoids placing the just-played
// track first so repeat-all does not immediately replay it.
//
// The caller must hold m.mu.
func (m *Manager) restartShuffleCycleLocked() {
	ids := queueIdentities(m.queue)
	prior := ""
	switch {
	case m.shufflePos >= 0 && m.shufflePos < len(m.shuffleHistory):
		prior = m.shuffleHistory[m.shufflePos]
	case m.currentIndex >= 0 && m.currentIndex < len(ids):
		prior = ids[m.currentIndex]
	}
	all := append([]string(nil), ids...)
	unseen := permuteStrings(all, m.permuteLocked(len(all)))
	if len(unseen) >= 2 && unseen[0] == prior {
		unseen[0], unseen[1] = unseen[1], unseen[0]
	}
	m.shuffleUnseen = unseen
	m.shuffleHistory = nil
	m.shufflePos = -1
	// A fresh cycle visits every identity again, including the ones the previous
	// cycle gave up on: a file that was missing may well be back.
	m.shuffleExcluded = nil
}

// stepForwardLocked advances the shuffle cursor one step and returns the next
// identity to attempt. It first re-walks any forward history (Previous leaves
// the cursor mid-history), then materializes the next entry from the unseen
// pool into history. Returns ok=false when the cycle is exhausted.
//
// The caller must hold m.mu.
func (m *Manager) stepForwardLocked() (string, bool) {
	if m.shufflePos+1 < len(m.shuffleHistory) {
		m.shufflePos++
		return m.shuffleHistory[m.shufflePos], true
	}
	if len(m.shuffleUnseen) > 0 {
		id := m.shuffleUnseen[0]
		m.shuffleUnseen = m.shuffleUnseen[1:]
		m.shuffleHistory = append(m.shuffleHistory, id)
		m.shufflePos = len(m.shuffleHistory) - 1
		return id, true
	}
	return "", false
}

// upcomingShuffleLocked returns the identities the traversal would visit next,
// in order: any not-yet-replayed forward history (left behind by Previous)
// followed by the unseen pool. It does not mutate any state, so a shuffle-aware
// prefetch can resolve the successor identity without consuming it — history is
// only committed once the transition is confirmed.
//
// The caller must hold m.mu.
func (m *Manager) upcomingShuffleLocked() []string {
	out := make([]string, 0, len(m.shuffleHistory)+len(m.shuffleUnseen))
	if next := m.shufflePos + 1; next >= 0 && next < len(m.shuffleHistory) {
		out = append(out, m.shuffleHistory[next:]...)
	}
	out = append(out, m.shuffleUnseen...)
	return out
}

// commitShufflePrefetchLocked advances the shuffle traversal so that identity —
// the successor a prior prefetch resolved but did not consume — becomes the
// current track. Identities the prefetch peek skipped (removed from the queue or
// missing on disk) are materialized then immediately dropped so they are neither
// counted as played nor revisited. It returns the live queue index of identity,
// or -1 when it can no longer be resolved (a concurrent mutation invalidated the
// prefetch); the caller then falls back to a fresh advance.
//
// It also returns the queue entries of any skipped identities that are still
// live queue rows. The prefetch peek skips a live row only when its file is
// missing on disk (a non-not-exist stat error stops prefetch instead — see
// resolveNextShufflePrefetch), so each reported entry is a missing file the
// caller must mark through the track and queue-entry missing APIs once m.mu is
// released, mirroring the durable markers and callbacks non-gapless traversal
// emits. Removed identities (no longer in the queue) are dropped without being
// reported so they are not falsely marked missing.
//
// The caller must hold m.mu.
func (m *Manager) commitShufflePrefetchLocked(identity string) (int, []QueueTrack) {
	var skippedMissing []QueueTrack
	for {
		id, ok := m.stepForwardLocked()
		if !ok {
			return -1, skippedMissing
		}
		if id == identity {
			if idx := identityIndex(m.queue, id); idx >= 0 {
				return idx, skippedMissing
			}
			// The target vanished between prefetch and commit: drop it and
			// let the caller fall back to a fresh advance.
			m.dropCurrentLocked()
			return -1, skippedMissing
		}
		// An intervening candidate the peek skipped as unplayable. If it is still
		// a live queue row it was skipped because its file was missing on disk;
		// report it so the caller marks it missing.
		if idx := identityIndex(m.queue, id); idx >= 0 {
			skippedMissing = append(skippedMissing, m.queue[idx])
		}
		m.dropCurrentLocked()
	}
}

// reconcileShuffleLocked updates the shuffle traversal pools after the queue's
// membership changes (SetQueue, AppendToQueue, RemoveFromQueue). Surviving
// history is preserved (visible reordering never resets playback history),
// removed identities are pruned from history, the unseen pool, and the cycle
// exclusions, and brand-new identities are added only to the unseen pool. The
// currently audible identity is retained in history even if its queue row was
// removed, so the next advance still steps from the correct position. A no-op
// when shuffle is off.
//
// An identity the cycle already excluded is not brand-new: it was skipped as
// unplayable, so an unrelated append or reorder must not schedule it again.
//
// The caller must hold m.mu.
func (m *Manager) reconcileShuffleLocked() {
	if !m.shuffleEnabled {
		return
	}
	ids := queueIdentities(m.queue)
	live := make(map[string]bool, len(ids))
	for _, id := range ids {
		live[id] = true
	}
	m.pruneExclusionsLocked(live)

	current := ""
	if m.shufflePos >= 0 && m.shufflePos < len(m.shuffleHistory) {
		current = m.shuffleHistory[m.shufflePos]
	}

	// Prune removed identities from history, keeping the currently audible one
	// so traversal can still advance correctly if its row was just removed.
	newHist := make([]string, 0, len(m.shuffleHistory))
	newPos := -1
	for i, id := range m.shuffleHistory {
		if !live[id] && id != current {
			continue
		}
		newHist = append(newHist, id)
		if i == m.shufflePos {
			newPos = len(newHist) - 1
		}
	}
	if newPos < 0 {
		newPos = len(newHist) - 1
	}
	m.shuffleHistory = newHist
	m.shufflePos = newPos

	seen := make(map[string]bool, len(ids))
	for _, id := range newHist {
		seen[id] = true
	}
	// Drop removed identities from the unseen pool, preserving order.
	newUnseen := make([]string, 0, len(m.shuffleUnseen))
	for _, id := range m.shuffleUnseen {
		if live[id] && !seen[id] {
			newUnseen = append(newUnseen, id)
			seen[id] = true
		}
	}
	// Append brand-new identities (present in the queue but neither played,
	// pending, nor already dropped by this cycle) to the unseen pool.
	for _, id := range ids {
		if !seen[id] && !m.shuffleExcluded[id] {
			newUnseen = append(newUnseen, id)
			seen[id] = true
		}
	}
	m.shuffleUnseen = newUnseen
}

// excludeIdentityLocked records id as dropped from the current shuffle cycle so
// reconciliation does not mistake it for a brand-new identity and schedule it
// again. Called from every path that gives up on a candidate.
//
// The caller must hold m.mu.
func (m *Manager) excludeIdentityLocked(id string) {
	if id == "" {
		return
	}
	if m.shuffleExcluded == nil {
		m.shuffleExcluded = make(map[string]bool)
	}
	m.shuffleExcluded[id] = true
}

// pruneExclusionsLocked drops exclusions whose queue rows are gone. An identity
// that has left the queue cannot be reintroduced by reconciliation, and keeping
// it would make the set grow without bound across a long session.
//
// The caller must hold m.mu.
func (m *Manager) pruneExclusionsLocked(live map[string]bool) {
	for id := range m.shuffleExcluded {
		if !live[id] {
			delete(m.shuffleExcluded, id)
		}
	}
	if len(m.shuffleExcluded) == 0 {
		m.shuffleExcluded = nil
	}
}

// shuffleSnapshot captures the mutable shuffle traversal pools so a failed load
// can roll back a provisional traversal step, leaving history, cursor, the
// unseen pool, and the cycle exclusions exactly as they were. See
// advanceForward/advanceBackward and the anchored/restarted-cycle entry points
// in mpv_queue.go.
type shuffleSnapshot struct {
	history  []string
	pos      int
	unseen   []string
	excluded map[string]bool
}

// snapshotShuffleLocked copies the current shuffle traversal state so a caller
// can restore it if a subsequent load fails.
//
// The caller must hold m.mu.
func (m *Manager) snapshotShuffleLocked() shuffleSnapshot {
	var excluded map[string]bool
	if len(m.shuffleExcluded) > 0 {
		excluded = make(map[string]bool, len(m.shuffleExcluded))
		for id := range m.shuffleExcluded {
			excluded[id] = true
		}
	}
	return shuffleSnapshot{
		history:  append([]string(nil), m.shuffleHistory...),
		pos:      m.shufflePos,
		unseen:   append([]string(nil), m.shuffleUnseen...),
		excluded: excluded,
	}
}

// restoreShuffleLocked restores traversal state captured by snapshotShuffleLocked,
// discarding any provisional step taken since. Used to undo a cursor/pool mutation
// when a load fails for a reason other than an intentional missing/removed prune,
// so the failed identity is not consumed and a retry re-attempts it.
//
// The caller must hold m.mu.
func (m *Manager) restoreShuffleLocked(s shuffleSnapshot) {
	m.shuffleHistory = s.history
	m.shufflePos = s.pos
	m.shuffleUnseen = s.unseen
	m.shuffleExcluded = s.excluded
}

// dropCurrentLocked removes the identity at the current cursor from history and
// rewinds the cursor by one. Used to discard a candidate that turned out to be
// unplayable (missing on disk or removed from the queue) so it is neither
// counted as played nor revisited by Previous. The dropped identity is excluded
// from the cycle so a later reconciliation does not schedule it again.
//
// The caller must hold m.mu.
func (m *Manager) dropCurrentLocked() {
	p := m.shufflePos
	if p < 0 || p >= len(m.shuffleHistory) {
		return
	}
	m.excludeIdentityLocked(m.shuffleHistory[p])
	m.shuffleHistory = append(m.shuffleHistory[:p], m.shuffleHistory[p+1:]...)
	m.shufflePos = p - 1
}
