package player

// This file is the read half of the guarded refresh boundary: an immutable
// description of the playback timeline, keyed by queue-row identity, that says
// which rows a regenerated batch is allowed to take over.
//
// QueueTraversal already answers "what order will the Manager visit these rows
// in". It is deliberately not enough to refresh against. It reports how many
// entries are still ahead, not which of them the traversal is free to give up:
// the forward history a Previous left unreplayed is ahead of the cursor and
// counted as remaining, yet it is history the user walked back through, and a
// refresh that overwrote it would delete a row the user just came from. Nor does
// a visible index survive the round trip — the queue is mutated durably between
// the capture and the apply, and positions shift under every insertion — so the
// snapshot names rows by the same identity the traversal follows.
//
// The snapshot also carries the guard that makes the round trip safe. A refresh
// is planned against a timeline that was true when it was captured; by the time
// the plan comes back, playback may have advanced onto one of the rows it wanted
// to replace. The guard is what lets ApplyQueueRefresh reject that plan instead
// of publishing it.

// RowIdentity names one queue row for as long as it stays in the queue. It is
// the same identity shuffle traversal follows, so two queue rows holding the
// same track — a duplicate the user enqueued twice — are distinct identities and
// only one of them can be replaced.
type RowIdentity string

// QueueRowIdentity returns the identity of a persisted queue row, so a caller
// holding database row IDs can name a slot without reading player state. It
// returns an empty identity for a zero ID: an unpersisted row has no stable
// identity a caller outside the package could reconstruct.
func QueueRowIdentity(queueID int64) RowIdentity {
	if queueID == 0 {
		return ""
	}
	return RowIdentity(trackIdentity(QueueTrack{QueueID: queueID}, 0))
}

// RefreshRow describes one live queue row as the snapshot saw it.
//
// Index is the row's visible queue position at capture time and is meant for
// display and correlation only. It is not an address: the durable queue is
// mutated between the capture and the apply, and every replacement is named by
// Identity.
type RefreshRow struct {
	Identity RowIdentity
	Index    int
	QueueID  int64
	Hash     string
}

// RefreshSnapshot is an immutable classification of the live queue against the
// traversal that owns it. Every live row appears in exactly one of Current,
// Played, Protected, Eligible, and Unscheduled.
//
// The returned slices are freshly allocated and owned by the caller.
type RefreshSnapshot struct {
	// Shuffled reports whether the classification came from shuffle traversal.
	Shuffled bool

	// Current is the row playing now. Its Identity is empty when nothing is
	// current — before the first load, or after the current row left the queue.
	Current RefreshRow

	// Played holds the rows the traversal has already visited, in the order it
	// visited them. With shuffle off that is the completed prefix; the rows a
	// completion boundary names as finished are here even when they sit ahead of
	// the cursor, because a resumed traversal has already played them.
	Played []RefreshRow

	// Protected holds upcoming rows a refresh may not take over: the forward
	// history a Previous left unreplayed. It is ahead of the cursor and the next
	// advance will replay it, but it is a row the user navigated away from
	// rather than one the cycle scheduled, so overwriting it would delete
	// playback history from in front of the listener. Always empty with shuffle
	// off, which keeps no forward history.
	Protected []RefreshRow

	// Eligible holds the upcoming rows a refresh may replace, in the order the
	// traversal will reach them: the unseen pool under shuffle, and the pending
	// insertions followed by the positional successors with shuffle off.
	Eligible []RefreshRow

	// Unscheduled holds live rows the current cycle no longer schedules —
	// entries it skipped as missing on disk, anything excluded from the cycle.
	// They are reported so the caller sees the whole queue, but they are not
	// replaceable: the cycle has already given up on them, so a refresh that
	// swapped one in would schedule a slot nothing is going to visit.
	Unscheduled []RefreshRow

	// guard is what ApplyQueueRefresh re-derives to detect an intervening queue
	// or playback change. It is unexported so a caller cannot hand back a
	// fabricated snapshot and bypass the staleness check.
	guard refreshGuard
}

// HasCurrent reports whether a row was playing when the snapshot was taken.
func (s RefreshSnapshot) HasCurrent() bool { return s.Current.Identity != "" }

// EligibleIdentities returns the identities of the replaceable upcoming rows in
// traversal order, which is the form a caller filtering by ownership wants.
func (s RefreshSnapshot) EligibleIdentities() []RowIdentity {
	return refreshRowIdentities(s.Eligible)
}

// refreshGuard fingerprints the state a refresh plan was built against: queue
// membership and visible order, where playback sits, and the timeline still
// ahead of it.
//
// Those three answer every way an intervening change can invalidate a plan. A
// dequeue or an enqueue moves the queue; an advance, a Previous, or an explicit
// selection moves the cursor; and consuming, skipping, or reanchoring the cycle
// moves what is upcoming — including the cases the other two miss, such as a row
// the traversal gave up on as missing while the queue itself stayed put.
type refreshGuard struct {
	// captured distinguishes a real snapshot from the zero value. Without it a
	// caller passing RefreshSnapshot{} would match an empty queue with nothing
	// playing and publish against a timeline it never observed.
	captured bool
	shuffled bool
	queue    []RowIdentity
	current  RowIdentity
	upcoming []RowIdentity
}

// equal reports whether two guards describe the same timeline.
func (g refreshGuard) equal(other refreshGuard) bool {
	return g.captured == other.captured &&
		g.shuffled == other.shuffled &&
		g.current == other.current &&
		identitiesEqual(g.queue, other.queue) &&
		identitiesEqual(g.upcoming, other.upcoming)
}

// holds reports whether id named a live queue row when the guard was built.
func (g refreshGuard) holds(id RowIdentity) bool {
	for _, known := range g.queue {
		if known == id {
			return true
		}
	}
	return false
}

func identitiesEqual(a, b []RowIdentity) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func refreshRowIdentities(rows []RefreshRow) []RowIdentity {
	out := make([]RowIdentity, len(rows))
	for i, row := range rows {
		out[i] = row.Identity
	}
	return out
}

// CaptureQueueRefresh returns an immutable snapshot of the live queue classified
// against the traversal that owns it, resolved under a single lock acquisition
// so the caller never reads a timeline that never existed.
//
// The snapshot is the only input ApplyQueueRefresh accepts, and it carries the
// guard that rejects a plan the queue or playback has since moved out from
// under.
func (m *Manager) CaptureQueueRefresh() RefreshSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.captureRefreshLocked()
}

// captureRefreshLocked builds the snapshot. It is also how the apply path
// re-derives the guard, so the two can never drift apart.
//
// The caller must hold m.mu.
func (m *Manager) captureRefreshLocked() RefreshSnapshot {
	if m.shuffleEnabled {
		return m.shuffledRefreshLocked()
	}
	return m.sequentialRefreshLocked()
}

// shuffledRefreshLocked classifies the queue against shuffle traversal: played
// history, the current entry, the forward history a Previous left behind, the
// unseen pool, and finally the rows the cycle no longer schedules.
//
// The caller must hold m.mu.
func (m *Manager) shuffledRefreshLocked() RefreshSnapshot {
	b := newRefreshBuilder(m.queue)
	snap := RefreshSnapshot{Shuffled: true}
	m.placeShuffleHistoryLocked(b, &snap)
	for _, id := range m.shuffleUnseen {
		b.appendRow(&snap.Eligible, b.index(RowIdentity(id)))
	}
	b.appendUnplaced(&snap.Unscheduled)
	snap.guard = b.guard(&snap)
	return snap
}

// placeShuffleHistoryLocked splits the shuffle history around the cursor. The
// entries before it have played, the entry on it is current, and the entries
// after it are the forward history a Previous left unreplayed — upcoming, but
// history all the same, so they are protected rather than eligible.
//
// A cursor of -1 puts the whole history after it, which is right: nothing has
// played, and every recorded entry is still ahead.
//
// The caller must hold m.mu.
func (m *Manager) placeShuffleHistoryLocked(b *refreshBuilder, snap *RefreshSnapshot) {
	for h, id := range m.shuffleHistory {
		i := b.index(RowIdentity(id))
		switch {
		case h == m.shufflePos:
			if row, ok := b.take(i); ok {
				snap.Current = row
			}
		case h < m.shufflePos:
			b.appendRow(&snap.Played, i)
		default:
			b.appendRow(&snap.Protected, i)
		}
	}
}

// sequentialRefreshLocked classifies the queue against shuffle-off traversal.
//
// What is still ahead is not "everything after the cursor": a resume that
// stepped back onto a row inserted behind an exhausted traversal leaves the
// cursor behind rows that already played, and the insertions it has not started
// yet sit before it. Both are answered by the continuation itself — the pending
// insertions first, then the positional successors — so the snapshot asks it
// rather than recomputing the arithmetic and disagreeing with playback.
// Everything the continuation does not name has played.
//
// Nothing is unscheduled with shuffle off: a missing row is retried by every
// later traversal instead of being excluded from a cycle.
//
// The caller must hold m.mu.
func (m *Manager) sequentialRefreshLocked() RefreshSnapshot {
	b := newRefreshBuilder(m.queue)
	snap := RefreshSnapshot{}
	if row, ok := b.take(m.currentIndex); ok {
		snap.Current = row
	}
	for _, add := range m.unreachedAdditionsLocked(m.currentIndex + 1) {
		b.appendRow(&snap.Eligible, add.index)
	}
	for _, i := range m.sequentialSuccessorsLocked(m.currentIndex + 1) {
		b.appendRow(&snap.Eligible, i)
	}
	b.appendUnplaced(&snap.Played)
	snap.guard = b.guard(&snap)
	return snap
}

// refreshBuilder classifies live queue rows exactly once each, so the snapshot
// stays a partition of the queue however tangled the traversal state is.
type refreshBuilder struct {
	rows       []RefreshRow
	byIdentity map[RowIdentity]int
	placed     []bool
}

func newRefreshBuilder(queue []QueueTrack) *refreshBuilder {
	ids := queueIdentities(queue)
	b := &refreshBuilder{
		rows:       make([]RefreshRow, len(queue)),
		byIdentity: make(map[RowIdentity]int, len(queue)),
		placed:     make([]bool, len(queue)),
	}
	for i, t := range queue {
		id := RowIdentity(ids[i])
		b.rows[i] = RefreshRow{Identity: id, Index: i, QueueID: t.QueueID, Hash: t.Hash}
		b.byIdentity[id] = i
	}
	return b
}

// index resolves an identity to its live queue index, or -1 when the row has
// left the queue.
func (b *refreshBuilder) index(id RowIdentity) int {
	i, ok := b.byIdentity[id]
	if !ok {
		return -1
	}
	return i
}

// take claims the row at index and returns it, reporting false when the index
// names no row or the row is already classified.
func (b *refreshBuilder) take(index int) (RefreshRow, bool) {
	if index < 0 || index >= len(b.rows) || b.placed[index] {
		return RefreshRow{}, false
	}
	b.placed[index] = true
	return b.rows[index], true
}

// appendRow claims the row at index and appends it to dst.
func (b *refreshBuilder) appendRow(dst *[]RefreshRow, index int) bool {
	row, ok := b.take(index)
	if !ok {
		return false
	}
	*dst = append(*dst, row)
	return true
}

// appendUnplaced appends every row the traversal did not account for, in visible
// queue order.
func (b *refreshBuilder) appendUnplaced(dst *[]RefreshRow) {
	for i := range b.rows {
		b.appendRow(dst, i)
	}
}

// guard fingerprints the classification for the staleness check.
func (b *refreshBuilder) guard(snap *RefreshSnapshot) refreshGuard {
	g := refreshGuard{
		captured: true,
		shuffled: snap.Shuffled,
		current:  snap.Current.Identity,
		queue:    make([]RowIdentity, len(b.rows)),
		upcoming: make([]RowIdentity, 0, len(snap.Protected)+len(snap.Eligible)),
	}
	for i, row := range b.rows {
		g.queue[i] = row.Identity
	}
	g.upcoming = append(g.upcoming, refreshRowIdentities(snap.Protected)...)
	g.upcoming = append(g.upcoming, refreshRowIdentities(snap.Eligible)...)
	return g
}
