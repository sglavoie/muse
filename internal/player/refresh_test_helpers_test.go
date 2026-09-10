package player

import (
	"fmt"
	"os"
	"testing"
)

// refreshQueue builds playable queue rows carrying the given queue IDs. Every
// row gets its own file so a refresh test can mint successors that are genuinely
// new rows rather than reordered copies of existing ones.
func refreshQueue(t *testing.T, ids ...int64) []QueueTrack {
	t.Helper()
	dir := t.TempDir()
	q := make([]QueueTrack, len(ids))
	for i, id := range ids {
		path := fmt.Sprintf("%s/row-%d.flac", dir, id)
		if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		q[i] = QueueTrack{Path: path, Hash: fmt.Sprintf("h%d", id), QueueID: id}
	}
	return q
}

// refreshSwap is one slot handover expressed in queue IDs: replace old with
// next, or retire the slot when next is nil.
type refreshSwap struct {
	old  int64
	next *QueueTrack
}

// retire returns a swap that drops a slot without a successor.
func retire(old int64) refreshSwap { return refreshSwap{old: old} }

// swap returns a swap that hands a slot to next.
func swap(old int64, next QueueTrack) refreshSwap {
	return refreshSwap{old: old, next: &next}
}

// buildRefresh assembles the plan the database transaction would publish: the
// replaced rows are gone and every new row lands at the tail, which is where an
// accepted batch goes (a retired row's position cannot be reused while its
// soft-deleted row still holds it).
func buildRefresh(snap RefreshSnapshot, queue []QueueTrack, swaps []refreshSwap, extra ...QueueTrack) QueueRefresh {
	drop := make(map[int64]bool, len(swaps))
	reps := make([]RowReplacement, 0, len(swaps))
	for _, s := range swaps {
		drop[s.old] = true
		rep := RowReplacement{Replaced: QueueRowIdentity(s.old)}
		if s.next != nil {
			rep.Successor = QueueRowIdentity(s.next.QueueID)
		}
		reps = append(reps, rep)
	}
	next := make([]QueueTrack, 0, len(queue)+len(swaps)+len(extra))
	for _, row := range queue {
		if !drop[row.QueueID] {
			next = append(next, row)
		}
	}
	for _, s := range swaps {
		if s.next != nil {
			next = append(next, *s.next)
		}
	}
	next = append(next, extra...)
	return QueueRefresh{Snapshot: snap, Queue: next, Replacements: reps}
}

// managerQueue returns a copy of the manager's live queue.
func managerQueue(m *Manager) []QueueTrack {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]QueueTrack(nil), m.queue...)
}

// rowIDs returns the queue IDs of a snapshot class, which is how the assertions
// below name rows.
func rowIDs(rows []RefreshRow) []int64 {
	out := make([]int64, len(rows))
	for i, row := range rows {
		out[i] = row.QueueID
	}
	return out
}

// assertRefreshClass compares one snapshot class against the queue IDs it should
// hold, in order.
func assertRefreshClass(t *testing.T, label string, rows []RefreshRow, want []int64) {
	t.Helper()
	got := rowIDs(rows)
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}
}

// assertSnapshot checks every class of a snapshot at once, so a row that
// silently moved between classes cannot pass unnoticed.
func assertSnapshot(t *testing.T, snap RefreshSnapshot, current int64, played, protected, eligible, unscheduled []int64) {
	t.Helper()
	if snap.Current.QueueID != current {
		t.Fatalf("current row = %d, want %d", snap.Current.QueueID, current)
	}
	assertRefreshClass(t, "played", snap.Played, played)
	assertRefreshClass(t, "protected", snap.Protected, protected)
	assertRefreshClass(t, "eligible", snap.Eligible, eligible)
	assertRefreshClass(t, "unscheduled", snap.Unscheduled, unscheduled)
}

// assertEligible checks only the replaceable upcoming rows.
func assertEligible(t *testing.T, m *Manager, want ...int64) {
	t.Helper()
	assertRefreshClass(t, "eligible", m.CaptureQueueRefresh().Eligible, want)
}

// currentQueueID returns the queue ID of the row the manager is playing, or 0.
func currentQueueID(m *Manager) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.currentIndex < 0 || m.currentIndex >= len(m.queue) {
		return 0
	}
	return m.queue[m.currentIndex].QueueID
}
