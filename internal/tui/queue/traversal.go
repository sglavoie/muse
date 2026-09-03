package queue

// This file gives the Queue view a read-only copy of the player-owned playback
// timeline. Under shuffle the order the player will visit rows in bears no
// relation to the durable queue order, so a panel that renders storage order
// tells the user nothing about what plays next. The player owns the timeline;
// the view only ever reads a snapshot of it and never writes back.

// Traversal is an immutable copy of the playback timeline. Order lists live
// queue indices in the order the player will visit them and is a permutation of
// the whole queue: played history first, the current row, the forward history a
// Previous left unreplayed, the unseen pool, and finally the rows the cycle no
// longer schedules. CurrentPosition is the index within Order of the row
// playing now, or -1 when the player sits on no live row.
type Traversal struct {
	Order           []int
	CurrentPosition int
	Shuffled        bool
}

// TraversalSource hands the view a fresh copy of the timeline. Root wires it
// from the player in WithPlayer; a player that exposes no timeline leaves it
// nil and the view falls back to persisted queue order.
type TraversalSource interface {
	QueueTraversal() Traversal
}

// noTraversal is what every fallback path resolves to: no order, nothing
// played.
var noTraversal = Traversal{CurrentPosition: -1}

// traversalSnapshot reads the timeline once. Every queue operation takes
// exactly one snapshot and threads it through its helpers, so rendering and
// input handling can never observe an order that changed halfway through.
func (v View) traversalSnapshot() Traversal {
	if v.Traversal == nil {
		return noTraversal
	}
	return v.Traversal.QueueTraversal()
}

// order returns the row order the timeline dictates, and whether the timeline
// is usable at all. It is only usable when it is shuffled — sequential
// traversal is the persisted order already, so deferring to it would change
// nothing — and when it is a true permutation of the n live rows. Anything else
// is a snapshot that raced a queue mutation, and the caller falls back to
// persisted order rather than rendering a queue that never existed.
func (t Traversal) order(n int) ([]int, bool) {
	if !t.Shuffled || !validPermutation(t.Order, n) {
		return nil, false
	}
	return t.Order, true
}

// validPermutation reports whether order names each of the n queue rows exactly
// once. Length and range alone are not enough: a duplicated index would render
// one row twice and drop another out of the panel entirely.
func validPermutation(order []int, n int) bool {
	if n == 0 || len(order) != n {
		return false
	}
	seen := make([]bool, n)
	for _, idx := range order {
		if idx < 0 || idx >= n || seen[idx] {
			return false
		}
		seen[idx] = true
	}
	return true
}

// timelineRanks maps a queue index to its rank in the traversal, so a row's
// played state follows the timeline rather than the underlying queue index.
type timelineRanks struct {
	ranks   []int // by queue index; -1 for rows the timeline does not name
	current int   // rank of the row playing now; -1 when the timeline names none
	active  bool
}

// newTimelineRanks builds the rank lookup for a snapshot. It deliberately does
// not consult the display order: an explicit metadata sort changes which order
// rows are shown in, never which of them have already played.
func newTimelineRanks(t Traversal, n int) timelineRanks {
	order, ok := t.order(n)
	if !ok {
		return timelineRanks{current: -1}
	}
	ranks := make([]int, n)
	for i := range ranks {
		ranks[i] = -1
	}
	for rank, idx := range order {
		ranks[idx] = rank
	}
	current := -1
	if t.CurrentPosition >= 0 && t.CurrentPosition < len(order) {
		current = t.CurrentPosition
	}
	return timelineRanks{ranks: ranks, current: current, active: true}
}

// played reports whether the timeline has already moved past the row at queue
// index idx. Rows ranked after the current one are still queued even when they
// have played before: the forward history a Previous left behind will play
// again, so it reads as scheduled rather than spent.
func (tr timelineRanks) played(idx int) bool {
	if !tr.active || tr.current < 0 || idx < 0 || idx >= len(tr.ranks) {
		return false
	}
	rank := tr.ranks[idx]
	return rank >= 0 && rank < tr.current
}
