package player

import (
	"errors"
	"fmt"

	"github.com/sglavoie/muse/internal/safego"
)

// This file is the write half of the guarded refresh boundary: one serialized
// operation that swaps a set of upcoming slots for their successors and leaves
// everything else exactly where it was.
//
// It is deliberately not expressible as RemoveFromQueue plus SetQueue. Removing
// the old rows one at a time walks the traversal through intermediate states a
// concurrent advance can land in, and the SetQueue that follows reconciles the
// new rows onto the *end* of the unseen pool — so a refresh of the third
// upcoming entry would silently move it behind everything already scheduled,
// reordering a cycle the user never touched. Neither call can reject a plan that
// playback has moved past, because neither knows what the plan was built
// against.
//
// So the whole thing runs as validate-then-publish under the playlist
// transaction lock: a rejected plan mutates nothing, and an accepted one is
// never observed half-applied.

// Sentinel errors returned by ApplyQueueRefresh.
var (
	// ErrRefreshStale means the queue or playback moved between the capture and
	// the apply, so the plan describes a timeline that no longer exists.
	ErrRefreshStale = errors.New("player: refresh snapshot is stale")

	// ErrRefreshProtected means the plan would take over or drop a row the
	// traversal owns: the entry playing now, one that already played, or the
	// forward history a Previous left unreplayed.
	ErrRefreshProtected = errors.New("player: refresh names a protected queue row")

	// ErrRefreshPlanInvalid means the plan is not internally consistent with the
	// queue it publishes — a slot named twice, a successor that is not a new
	// row, a replaced row still present.
	ErrRefreshPlanInvalid = errors.New("player: refresh plan is invalid")

	// ErrRefreshTransitionPending is the staleness a caller can simply wait out:
	// an end-file has been read and the question of what plays next has not
	// resolved, so no plan can be checked against a timeline that is still
	// moving. It is an ErrRefreshStale, because a plan captured before the
	// transition describes the row that just finished; the distinction is that
	// recapturing immediately would only hit the same window, while recapturing
	// after the transition settles succeeds.
	ErrRefreshTransitionPending = fmt.Errorf("%w: a playback transition has not resolved yet", ErrRefreshStale)
)

// RowReplacement hands one upcoming slot over to a new queue row.
//
// An empty Successor retires the slot without a replacement, which is what a
// planner that came back with fewer tracks than it removed reports. The slot is
// dropped from the traversal; nothing takes its place.
type RowReplacement struct {
	Replaced  RowIdentity
	Successor RowIdentity
}

// QueueRefresh is a complete refresh plan: the snapshot it was built against,
// the queue it publishes, and the slot handover that relates them.
//
// Queue is the whole refreshed queue in visible order, exactly as the durable
// queue now reads. The player does not compute it — ordering is owned by the
// database transaction that accepted the batch — but it does check that Queue
// and Replacements agree, so a mismatch is rejected rather than reconciled into
// a timeline neither side intended.
type QueueRefresh struct {
	Snapshot     RefreshSnapshot
	Queue        []QueueTrack
	Replacements []RowReplacement
}

// RefreshResult reports what an accepted refresh did.
type RefreshResult struct {
	// Replaced counts the upcoming slots a successor took over.
	Replaced int

	// Retired counts the upcoming slots dropped with no successor.
	Retired int

	// Added counts the rows the refreshed queue gained that no replacement
	// named — an ordinary append that rode along with the refresh.
	Added int

	// CurrentIndex is where the row that was playing sits after the refresh, or
	// -1 when nothing was current.
	CurrentIndex int

	// PrefetchInvalidated reports that the refresh may have changed which row
	// comes next, so a fresh prefetch was scheduled. It says nothing about
	// whether mpv's playlist was touched: a successor the refresh did not name
	// is rebound rather than dropped, and the reschedule then resolves the same
	// row and sends nothing.
	PrefetchInvalidated bool

	// PrefetchDropped reports that the gapless successor mpv was holding had
	// been made obsolete by the refresh and was removed from its playlist. The
	// row that is playing is never affected.
	PrefetchDropped bool

	// PrefetchDeferred reports that an obsolete successor could not be taken
	// back — mpv had already started it, or the command did not reach mpv — so
	// the tracking was retained for a later attempt. The refreshed queue is
	// still authoritative; only mpv's playlist is temporarily behind it.
	PrefetchDeferred bool
}

// ApplyQueueRefresh swaps the upcoming slots the plan names for their successors
// and publishes the refreshed queue, or rejects the plan and changes nothing.
//
// What it preserves is the point of the operation. The row playing now keeps
// playing and keeps its place in the timeline; played history and the forward
// history a Previous left behind are untouched; the surviving upcoming entries
// keep the order the cycle gave them, with each successor taking its
// predecessor's slot rather than being appended behind everything else; and the
// cycle is never reanchored or re-permuted, so a refresh is invisible to every
// row it did not name.
//
// It takes the playlist transaction lock before the state lock, the one order
// every playlist mutation uses, so no replacement can be in flight while the plan
// is checked and published — and it never calls back into the caller while
// holding either, because a callback that reached for the queue would deadlock
// against the lock it is already inside.
//
// mpv's playlist is reconciled after the publication, and only where the refresh
// actually landed. The current track is never reloaded, sought, or interrupted:
// a gapless successor the plan left alone is rebound to its new queue position,
// and only one the refresh retired or handed over is taken out of mpv's playlist.
//
// Returns ErrRefreshStale, ErrRefreshProtected, or ErrRefreshPlanInvalid; in
// every case the queue and the traversal are left exactly as they were.
func (m *Manager) ApplyQueueRefresh(refresh QueueRefresh) (RefreshResult, error) {
	m.playlistMu.Lock()

	m.mu.Lock()
	plan, err := m.validateRefreshLocked(refresh)
	if err != nil {
		m.mu.Unlock()
		m.playlistMu.Unlock()
		return RefreshResult{}, err
	}
	res := m.publishRefreshLocked(refresh, plan)
	obsolete := m.rebindPrefetchLocked(refresh.Snapshot.guard.queue, plan)
	m.mu.Unlock()

	if obsolete {
		if m.removePrefetchLocked() {
			res.PrefetchDropped = true
		} else {
			res.PrefetchDeferred = true
		}
	}
	m.playlistMu.Unlock()

	if res.PrefetchInvalidated {
		safego.Go("player.prefetchNext", m.prefetchNext)
	}
	return res, nil
}

// rebindPrefetchLocked re-resolves the tracked gapless successor against the
// queue that was just published, and reports whether it has become obsolete.
//
// The tracking is positional, and a refresh moves rows: retiring an earlier slot
// shifts everything behind it, so a successor left untouched by the plan would
// end up naming a different queue entry. Restamping it is most of the work here.
//
// Dropping the prefetch unconditionally instead would be simpler and is what the
// first cut did, but it is a round trip mpv absorbs while a track is playing — a
// playlist-remove and a fresh append for a successor that did not change — and
// each one is a chance to take the wrong slot away. Rebinding makes the common
// refresh, the one that swaps rows further down the queue, cost mpv nothing:
// prefetchNextLocked resolves the same index, path, and identity and returns
// having sent no command at all.
//
// Obsolete means the plan handed the tracked slot over or retired it, or the row
// it named is no longer in the queue under the same file. Only then does the
// caller reach for mpv's playlist, and only at position 1 — position 0, the row
// coming out of the speakers, is never addressed on either path.
//
// The caller must hold m.mu.
func (m *Manager) rebindPrefetchLocked(prev []RowIdentity, plan refreshPlan) bool {
	p := m.prefetchSnapshotLocked()
	if p.index < 0 {
		return false
	}
	// A sequential prefetch carries no identity of its own, so it is named by
	// the row that stood at its index in the queue the plan was built against —
	// which validation has just established is the queue that was live.
	id := p.identity
	if id == "" {
		if p.index >= len(prev) {
			return true
		}
		id = string(prev[p.index])
	}
	if _, replaced := plan.sub[id]; replaced {
		return true
	}
	idx := identityIndex(m.queue, id)
	if idx < 0 || m.queue[idx].Path != p.path {
		return true
	}
	m.prefetchedIndex = idx
	return false
}

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
