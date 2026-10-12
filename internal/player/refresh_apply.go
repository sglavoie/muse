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
