package player

import (
	"errors"
	"sync"
	"testing"
)

// TestApplyQueueRefresh_RejectsUnresolvedTransition verifies that a pending end
// blocks publication even though nothing about the queue moved.
//
// This is the one intervening playback change the fingerprint cannot see. mpv
// starts the prefetched entry gaplessly as it emits the end-file, and until the
// promotion commits it the cursor and the unseen pool still describe the row
// that finished — so the slot the plan wants to hand over is the one already
// audible. Once the transition resolves, the very same plan is publishable
// again.
func TestApplyQueueRefresh_RejectsUnresolvedTransition(t *testing.T) {
	tests := []struct {
		name  string
		pend  func(m *Manager)
		clear func(m *Manager)
	}{
		{"end-file not yet resolved",
			func(m *Manager) { m.liveness = livenessEnding },
			func(m *Manager) { m.liveness = livenessLive }},
		{"deferred EOF settlement outstanding",
			func(m *Manager) { m.pendingEOFSettlement = true },
			func(m *Manager) { m.pendingEOFSettlement = false }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, q := shuffledRefreshManager(t, "ref-transition")
			fresh := refreshQueue(t, 10)
			snap := m.CaptureQueueRefresh()
			plan := buildRefresh(snap, q, []refreshSwap{swap(5, fresh[0])})

			m.mu.Lock()
			tc.pend(m)
			m.mu.Unlock()
			if _, err := m.ApplyQueueRefresh(plan); !errors.Is(err, ErrRefreshStale) {
				t.Fatalf("ApplyQueueRefresh mid-transition: want ErrRefreshStale, got %v", err)
			}
			assertEligible(t, m, 5, 4, 3, 2)

			m.mu.Lock()
			tc.clear(m)
			m.mu.Unlock()
			if _, err := m.ApplyQueueRefresh(plan); err != nil {
				t.Fatalf("ApplyQueueRefresh once the transition resolved: %v", err)
			}
			assertEligible(t, m, 10, 4, 3, 2)
		})
	}
}

// TestApplyQueueRefresh_RejectsProtectedRows verifies that the traversal's own
// rows cannot be taken over or dropped, whichever direction the plan comes at
// them from.
func TestApplyQueueRefresh_RejectsProtectedRows(t *testing.T) {
	fresh := refreshQueue(t, 10)
	tests := []struct {
		name string
		plan func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh
	}{
		{"replaces the row playing now", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			return buildRefresh(snap, q, []refreshSwap{swap(1, fresh[0])})
		}},
		{"replaces a row that already played", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			return buildRefresh(snap, q, []refreshSwap{swap(5, fresh[0])})
		}},
		{"replaces forward history", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			return buildRefresh(snap, q, []refreshSwap{swap(4, fresh[0])})
		}},
		{"drops a protected row without naming it", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			r := buildRefresh(snap, q, nil)
			kept := make([]QueueTrack, 0, len(r.Queue))
			for _, row := range r.Queue {
				if row.QueueID != 4 { // the forward history entry
					kept = append(kept, row)
				}
			}
			r.Queue = kept
			return r
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, q := shuffledRefreshManager(t, "ref-prot")
			for range 2 { // -> 5, -> 4
				if err := m.Next(); err != nil {
					t.Fatalf("Next: %v", err)
				}
			}
			if err := m.Previous(); err != nil { // back to 5, leaving 4 ahead
				t.Fatalf("Previous: %v", err)
			}
			snap := m.CaptureQueueRefresh()
			assertSnapshot(t, snap, 5, []int64{1}, []int64{4}, []int64{3, 2}, nil)

			if _, err := m.ApplyQueueRefresh(tc.plan(snap, q)); !errors.Is(err, ErrRefreshProtected) {
				t.Fatalf("ApplyQueueRefresh: want ErrRefreshProtected, got %v", err)
			}
			assertSnapshot(t, m.CaptureQueueRefresh(), 5, []int64{1}, []int64{4}, []int64{3, 2}, nil)
		})
	}
}

// TestApplyQueueRefresh_RejectsInconsistentPlans covers the plans that are not
// internally consistent with the queue they publish. Each would otherwise be
// reconciled into a timeline neither the player nor the planner intended.
func TestApplyQueueRefresh_RejectsInconsistentPlans(t *testing.T) {
	fresh := refreshQueue(t, 10, 11)
	tests := []struct {
		name string
		plan func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh
	}{
		{"snapshot was never captured", func(_ RefreshSnapshot, q []QueueTrack) QueueRefresh {
			return buildRefresh(RefreshSnapshot{}, q, nil)
		}},
		{"row replaced twice", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			r := buildRefresh(snap, q, []refreshSwap{swap(2, fresh[0])})
			r.Replacements = append(r.Replacements, RowReplacement{
				Replaced:  QueueRowIdentity(2),
				Successor: QueueRowIdentity(11),
			})
			r.Queue = append(r.Queue, fresh[1])
			return r
		}},
		{"successor takes two slots", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			r := buildRefresh(snap, q, []refreshSwap{swap(2, fresh[0]), swap(3, fresh[0])})
			return r
		}},
		{"replaced row is still in the queue", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			r := buildRefresh(snap, q, []refreshSwap{swap(2, fresh[0])})
			r.Queue = append(r.Queue, q[1]) // row 2 back again
			return r
		}},
		{"successor is not a new row", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			r := buildRefresh(snap, q, nil)
			r.Replacements = []RowReplacement{{
				Replaced:  QueueRowIdentity(2),
				Successor: QueueRowIdentity(3), // already in the queue
			}}
			kept := make([]QueueTrack, 0, len(r.Queue))
			for _, row := range r.Queue {
				if row.QueueID != 2 {
					kept = append(kept, row)
				}
			}
			r.Queue = kept
			return r
		}},
		{"successor is missing from the queue", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			r := buildRefresh(snap, q, []refreshSwap{swap(2, fresh[0])})
			kept := make([]QueueTrack, 0, len(r.Queue))
			for _, row := range r.Queue {
				if row.QueueID != 10 {
					kept = append(kept, row)
				}
			}
			r.Queue = kept
			return r
		}},
		{"upcoming row dropped without being named", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			r := buildRefresh(snap, q, nil)
			kept := make([]QueueTrack, 0, len(r.Queue))
			for _, row := range r.Queue {
				if row.QueueID != 3 {
					kept = append(kept, row)
				}
			}
			r.Queue = kept
			return r
		}},
		{"replacement names no row", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			r := buildRefresh(snap, q, nil)
			r.Replacements = []RowReplacement{{Successor: QueueRowIdentity(10)}}
			return r
		}},
		{"replacement names a row outside the snapshot", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			r := buildRefresh(snap, q, nil)
			r.Replacements = []RowReplacement{{Replaced: QueueRowIdentity(77)}}
			return r
		}},
		{"refreshed queue holds a row twice", func(snap RefreshSnapshot, q []QueueTrack) QueueRefresh {
			r := buildRefresh(snap, q, nil)
			r.Queue = append(r.Queue, q[2])
			return r
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, q := shuffledRefreshManager(t, "ref-bad")
			snap := m.CaptureQueueRefresh()

			if _, err := m.ApplyQueueRefresh(tc.plan(snap, q)); !errors.Is(err, ErrRefreshPlanInvalid) {
				t.Fatalf("ApplyQueueRefresh: want ErrRefreshPlanInvalid, got %v", err)
			}
			assertSnapshot(t, m.CaptureQueueRefresh(), 1, nil, nil, []int64{5, 4, 3, 2}, nil)
			if got := len(managerQueue(m)); got != 5 {
				t.Fatalf("queue length after a rejected plan = %d, want 5", got)
			}
		})
	}
}

// TestApplyQueueRefresh_LeavesUnscheduledRowsAlone verifies that a row the cycle
// gave up on is neither replaceable nor rescheduled by an unrelated refresh: it
// was skipped as missing, and a refresh is not the fresh cycle that gives it
// another chance.
func TestApplyQueueRefresh_LeavesUnscheduledRowsAlone(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "ref-unsched", &mu, &received)

	q := refreshQueue(t, 1, 2, 3)
	q[1].Path = t.TempDir() + "/gone.flac"
	m.mu.Lock()
	m.queue = q
	m.currentIndex = 0
	m.mu.Unlock()
	m.SetMissingMarker(&countingMarker{})
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	if err := m.Next(); err != nil { // skips row 2, plays row 3
		t.Fatalf("Next: %v", err)
	}
	snap := m.CaptureQueueRefresh()
	assertSnapshot(t, snap, 3, []int64{1}, nil, nil, []int64{2})

	fresh := refreshQueue(t, 10)
	if _, err := m.ApplyQueueRefresh(buildRefresh(snap, q, []refreshSwap{swap(2, fresh[0])})); !errors.Is(err, ErrRefreshProtected) {
		t.Fatalf("replacing a row the cycle gave up on: want ErrRefreshProtected, got %v", err)
	}
	// An ordinary addition rides along without rescheduling the skipped row.
	if _, err := m.ApplyQueueRefresh(buildRefresh(snap, q, nil, fresh...)); err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	assertSnapshot(t, m.CaptureQueueRefresh(), 3, []int64{1}, nil, []int64{10}, []int64{2})
}

// TestApplyQueueRefresh_NoOpPlanLeavesThePrefetchAlone verifies that a refresh
// that changes nothing does not invalidate the gapless successor: the successor
// mpv is holding is still the right one, and dropping it would cost a redundant
// playlist round trip on every empty batch.
func TestApplyQueueRefresh_NoOpPlanLeavesThePrefetchAlone(t *testing.T) {
	m, q := shuffledRefreshManager(t, "ref-noop")
	snap := m.CaptureQueueRefresh()

	res, err := m.ApplyQueueRefresh(buildRefresh(snap, q, nil))
	if err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	if res.PrefetchInvalidated {
		t.Fatal("a refresh that changed nothing must not invalidate the prefetch")
	}
	if res.Replaced != 0 || res.Retired != 0 || res.Added != 0 {
		t.Fatalf("result = %+v, want an empty refresh", res)
	}
	assertSnapshot(t, m.CaptureQueueRefresh(), 1, nil, nil, []int64{5, 4, 3, 2}, nil)
}
