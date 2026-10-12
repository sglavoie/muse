package player

import "fmt"

// refreshPlan is the validated form of a QueueRefresh: the substitution to apply
// to the traversal, plus the tallies the result reports.
type refreshPlan struct {
	// sub maps a replaced identity to its successor, or to the empty identity
	// when the slot is retired outright.
	sub     map[string]string
	retired int
	added   int
}

// validateRefreshLocked checks a plan against the snapshot it was built from and
// the queue it publishes. It mutates nothing, so every rejection leaves the
// traversal untouched.
//
// The caller must hold m.mu.
func (m *Manager) validateRefreshLocked(r QueueRefresh) (refreshPlan, error) {
	if !r.Snapshot.guard.captured {
		return refreshPlan{}, fmt.Errorf("%w: the snapshot was not captured from this player", ErrRefreshPlanInvalid)
	}
	if err := m.checkRefreshTransitionLocked(); err != nil {
		return refreshPlan{}, err
	}
	if live := m.captureRefreshLocked(); !live.guard.equal(r.Snapshot.guard) {
		return refreshPlan{}, fmt.Errorf("%w: the queue or playback moved after the snapshot was taken", ErrRefreshStale)
	}
	sub, err := refreshSubstitution(r)
	if err != nil {
		return refreshPlan{}, err
	}
	next, err := refreshQueueMembers(r.Queue)
	if err != nil {
		return refreshPlan{}, err
	}
	if err := checkRefreshSurvivors(r.Snapshot, sub, next); err != nil {
		return refreshPlan{}, err
	}
	if err := checkRefreshSuccessors(r.Snapshot, sub, next); err != nil {
		return refreshPlan{}, err
	}
	return refreshPlan{
		sub:     sub,
		retired: countRetiredSlots(sub),
		added:   countRefreshAdditions(r.Snapshot, sub, next),
	}, nil
}

// checkRefreshTransitionLocked rejects a plan published while an end-file is
// still unresolved.
//
// Holding the playlist transaction lock keeps a replacement from racing the
// publication, but the gapless promotion is not a replacement: mpv started the
// prefetched entry on its own, and between the end-file being read and the
// promotion committing it, every piece of state the fingerprint reads still
// describes the row that just finished. The cursor has not moved and the unseen
// pool still holds the peeked successor, so the queue looks like one nobody
// touched — while the slot the plan is about to hand over is the one already
// coming out of the speakers.
//
// The pending end is exactly that interval, and refusing to publish during it is
// what makes "no intervening playback change" true rather than merely
// unobserved. It is a staleness rejection like any other: the caller captures
// again once the transition has resolved, and the plan it builds then describes
// the row mpv actually moved to.
//
// The caller must hold m.mu.
func (m *Manager) checkRefreshTransitionLocked() error {
	if m.liveness == livenessEnding || m.pendingEOFSettlement {
		return ErrRefreshTransitionPending
	}
	return nil
}

// refreshSubstitution turns the replacement list into the identity substitution
// the publication applies, rejecting anything that names a row the traversal
// will not give up.
func refreshSubstitution(r QueueRefresh) (map[string]string, error) {
	eligible := make(map[RowIdentity]bool, len(r.Snapshot.Eligible))
	for _, row := range r.Snapshot.Eligible {
		eligible[row.Identity] = true
	}
	sub := make(map[string]string, len(r.Replacements))
	successors := make(map[RowIdentity]bool, len(r.Replacements))
	for _, rep := range r.Replacements {
		if err := checkReplacedSlot(r.Snapshot, eligible, sub, rep.Replaced); err != nil {
			return nil, err
		}
		if err := checkSuccessorSlot(successors, rep); err != nil {
			return nil, err
		}
		sub[string(rep.Replaced)] = string(rep.Successor)
	}
	return sub, nil
}

// checkReplacedSlot rejects a replacement that names no row, names one twice, or
// names a row the traversal owns.
func checkReplacedSlot(snap RefreshSnapshot, eligible map[RowIdentity]bool, sub map[string]string, id RowIdentity) error {
	switch {
	case id == "":
		return fmt.Errorf("%w: a replacement named no queue row", ErrRefreshPlanInvalid)
	case eligible[id]:
		if _, dup := sub[string(id)]; dup {
			return fmt.Errorf("%w: queue row %q is replaced twice", ErrRefreshPlanInvalid, id)
		}
		return nil
	case snap.guard.holds(id):
		return fmt.Errorf("%w: queue row %q is not an upcoming slot the traversal will give up", ErrRefreshProtected, id)
	default:
		return fmt.Errorf("%w: queue row %q was not in the snapshot", ErrRefreshPlanInvalid, id)
	}
}

// checkSuccessorSlot rejects a successor that would take over two slots at once
// or that replaces itself.
func checkSuccessorSlot(seen map[RowIdentity]bool, rep RowReplacement) error {
	if rep.Successor == "" {
		return nil
	}
	if rep.Successor == rep.Replaced {
		return fmt.Errorf("%w: queue row %q replaces itself", ErrRefreshPlanInvalid, rep.Successor)
	}
	if seen[rep.Successor] {
		return fmt.Errorf("%w: queue row %q takes over more than one slot", ErrRefreshPlanInvalid, rep.Successor)
	}
	seen[rep.Successor] = true
	return nil
}

// refreshQueueMembers returns the identities of the refreshed queue, rejecting a
// queue that holds the same row twice — two occurrences of one identity would
// make every later resolution ambiguous.
func refreshQueueMembers(queue []QueueTrack) (map[string]bool, error) {
	ids := queueIdentities(queue)
	next := make(map[string]bool, len(ids))
	for _, id := range ids {
		if next[id] {
			return nil, fmt.Errorf("%w: the refreshed queue holds row %q twice", ErrRefreshPlanInvalid, id)
		}
		next[id] = true
	}
	return next, nil
}

// checkRefreshSurvivors requires that every row of the captured queue either was
// named as replaced and is gone, or survived untouched.
//
// This is what keeps a refresh from quietly deleting anything: the plan may only
// remove the slots it declared, and a row that vanished without being named
// would otherwise be reconciled away as an ordinary removal.
func checkRefreshSurvivors(snap RefreshSnapshot, sub map[string]string, next map[string]bool) error {
	eligible := make(map[RowIdentity]bool, len(snap.Eligible))
	for _, row := range snap.Eligible {
		eligible[row.Identity] = true
	}
	for _, id := range snap.guard.queue {
		_, replaced := sub[string(id)]
		switch {
		case replaced && next[string(id)]:
			return fmt.Errorf("%w: replaced row %q is still in the refreshed queue", ErrRefreshPlanInvalid, id)
		case replaced, next[string(id)]:
			continue
		case eligible[id]:
			return fmt.Errorf("%w: the refreshed queue dropped upcoming row %q without replacing it", ErrRefreshPlanInvalid, id)
		default:
			return fmt.Errorf("%w: the refreshed queue dropped protected row %q", ErrRefreshProtected, id)
		}
	}
	return nil
}

// checkRefreshSuccessors requires every successor to be a brand-new row of the
// refreshed queue. A successor that was already in the queue would be moved out
// of its own traversal slot into the one it replaced, reordering an entry the
// refresh never claimed.
func checkRefreshSuccessors(snap RefreshSnapshot, sub map[string]string, next map[string]bool) error {
	prev := make(map[string]bool, len(snap.guard.queue))
	for _, id := range snap.guard.queue {
		prev[string(id)] = true
	}
	for replaced, succ := range sub {
		switch {
		case succ == "":
			continue
		case !next[succ]:
			return fmt.Errorf("%w: successor %q of row %q is not in the refreshed queue", ErrRefreshPlanInvalid, succ, replaced)
		case prev[succ]:
			return fmt.Errorf("%w: successor %q of row %q was already in the queue", ErrRefreshPlanInvalid, succ, replaced)
		}
	}
	return nil
}

func countRetiredSlots(sub map[string]string) int {
	n := 0
	for _, succ := range sub {
		if succ == "" {
			n++
		}
	}
	return n
}

// countRefreshAdditions counts the new rows no replacement named. They are an
// ordinary append that happened to ride along with the refresh, and they join
// the traversal at the end like any other append.
func countRefreshAdditions(snap RefreshSnapshot, sub map[string]string, next map[string]bool) int {
	claimed := make(map[string]bool, len(sub)+len(snap.guard.queue))
	for _, id := range snap.guard.queue {
		claimed[string(id)] = true
	}
	for _, succ := range sub {
		if succ != "" {
			claimed[succ] = true
		}
	}
	n := 0
	for id := range next {
		if !claimed[id] {
			n++
		}
	}
	return n
}
