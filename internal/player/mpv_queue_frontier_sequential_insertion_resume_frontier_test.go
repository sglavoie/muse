package player

import (
	"testing"
)

// TestSequentialInsertionResumeFrontierRemovalDoesNotReplayCompletedRows is the
// regression for editing the queue *while* a resumed insertion is playing behind
// the rows that already finished.
//
// The completion boundary a resume pins is the only thing separating those rows
// from the ones the traversal has yet to reach, and the row it names is as
// deletable as any other. Losing it must not turn positional continuation back
// on, because from a cursor that stepped backwards that means handing the
// completed rows back as successors.
func TestSequentialInsertionResumeFrontierRemovalDoesNotReplayCompletedRows(t *testing.T) {
	t.Run("deleting the frontier after a head insertion still ends the queue", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-rm-head")
		completed := h.queueSnapshot()

		h.insertAt(0, h.extra[0], h.extra[1])
		h.reset()
		h.resume(0)
		h.waitPrefetch(h.extra[1].Path)

		// The row the traversal finished on is now the last one; deleting it
		// leaves the rows it played behind still completed.
		h.removeAt(4)
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3}, 0, 1, false)
		h.waitLoads(h.extra[1].Path, 2)

		h.endFile()
		h.waitCurrentIndex(1)
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3}, 1, 0, false)

		h.endFile()
		h.waitEnded("the last insertion once the frontier row was deleted")
		h.assertNoReplays(completed...)
	})

	t.Run("deleting the frontier after a middle insertion keeps the pending one reachable", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-rm-mid")
		completed := h.queueSnapshot()

		h.m.SetQueue([]QueueTrack{completed[0], h.extra[0], completed[1], h.extra[1], completed[2]})
		h.reset()
		h.resume(1)
		h.waitPrefetch(h.extra[1].Path)

		// Deleting the frontier drops the boundary onto the completed row before
		// it, which still sits between the cursor and the second insertion.
		h.removeAt(4)
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3}, 1, 1, false)
		h.waitLoads(h.extra[1].Path, 2)

		h.endFile()
		h.waitCurrentIndex(3)
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3}, 3, 0, false)

		h.endFile()
		h.waitEnded("the second middle insertion once the frontier row was deleted")
		h.assertNoReplays(completed...)
	})

	t.Run("the fallback advance honours the surviving boundary when gapless is off", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-rm-nogap")
		completed := h.queueSnapshot()
		if err := h.m.SetGapless(false); err != nil {
			t.Fatalf("SetGapless(false): %v", err)
		}

		h.insertAt(0, h.extra[0], h.extra[1])
		h.reset()
		h.resume(0)
		h.removeAt(4)
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3}, 0, 1, false)

		// With no prefetch to promote, the end-file goes through the traversal
		// advance; it must pick the remaining insertion and nothing else.
		h.endFile()
		h.waitCurrentIndex(1)
		if got := h.loads(h.extra[1].Path); got != 1 {
			t.Fatalf("the second insertion was loaded %d times, want 1", got)
		}

		h.endFile()
		h.waitEnded("the second insertion without gapless once the frontier row was deleted")
		h.assertNoReplays(completed...)
	})

	t.Run("a tail append lands past what is left of the boundary", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-rm-tail")
		completed := h.queueSnapshot()

		h.insertAt(0, h.extra[0])
		h.reset()
		h.resume(0)
		h.removeAt(3)

		// The append still has to be reachable, and the two completed rows
		// between it and the cursor still must not be.
		h.insertAt(3, h.extra[1])
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3}, 0, 1, false)
		h.waitPrefetch(h.extra[1].Path)

		h.endFile()
		h.waitCurrentIndex(3)
		h.endFile()
		h.waitEnded("the tail append once the frontier row was deleted")
		h.assertNoReplays(completed...)
	})

	t.Run("replacing the frontier row through SetQueue keeps the boundary", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-rm-swap")
		completed := h.queueSnapshot()

		h.insertAt(0, h.extra[0], h.extra[1])
		h.reset()
		h.resume(0)
		h.waitPrefetch(h.extra[1].Path)

		// A paste over the frontier row removes it without deleting a row: the
		// track that takes its slot is new and eligible, the rows before it are
		// not.
		h.replaceRowAt(4, h.extra[2])
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 0, 2, false)
		h.waitLoads(h.extra[1].Path, 2)

		h.endFile()
		h.waitCurrentIndex(1)
		h.waitPrefetch(h.extra[2].Path)
		h.endFile()
		h.waitCurrentIndex(4)

		h.endFile()
		h.waitEnded("the row that replaced the frontier")
		h.assertNoReplays(completed...)
	})

	t.Run("an explicit selection still retires the boundary", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-rm-select")

		h.insertAt(0, h.extra[0])
		h.reset()
		h.resume(0)
		h.removeAt(3)

		// Selecting a completed row means "play the queue forward from here",
		// whatever is left of the boundary the resume pinned.
		if err := h.m.PlayFromIndex(1); err != nil {
			t.Fatalf("PlayFromIndex(1): %v", err)
		}
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2}, 1, 1, false)
		h.endFile()
		h.waitCurrentIndex(2)
	})
}

// TestSequentialFrontierKeepsUnplayedInteriorRowsReachable is the regression for
// editing the queue *inside* a standing completion boundary.
//
// The boundary the resume pinned describes which rows already played. It is not
// a description of the span it covers: a "play next" drops a brand-new row
// immediately after the resumed cursor, and a reorder can carry an unplayed tail
// row into the same span. Neither row was ever played, so both are still owed to
// the traversal, and continuation must reach them even though positional
// arithmetic from the cursor runs straight through the completed rows around
// them.
func TestSequentialFrontierKeepsUnplayedInteriorRowsReachable(t *testing.T) {
	t.Run("a play-next insertion inside the boundary is played", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-in-next")
		completed := h.queueSnapshot()

		h.insertAt(0, h.extra[0])
		h.reset()
		h.resume(0)

		// A "play next" while the resumed insertion plays: the new row lands at
		// cursor+1, ahead of every completed row rather than behind the cursor.
		h.insertAt(1, h.extra[1])
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 0, 1, false)
		h.waitPrefetch(h.extra[1].Path)

		h.endFile()
		h.waitCurrentIndex(1)
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 1, 0, false)

		h.endFile()
		h.waitEnded("the play-next insertion inside the boundary")
		h.assertNoReplays(completed...)
	})

	t.Run("a play-next insertion inside the boundary is played without gapless", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-in-next-nogap")
		completed := h.queueSnapshot()
		if err := h.m.SetGapless(false); err != nil {
			t.Fatalf("SetGapless(false): %v", err)
		}

		h.insertAt(0, h.extra[0])
		h.reset()
		h.resume(0)

		h.insertAt(1, h.extra[1])
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 0, 1, false)

		// With no prefetch to promote, the end-file goes through the fallback
		// advance; it must pick the insertion and not a completed row.
		h.endFile()
		h.waitCurrentIndex(1)
		if got := h.loads(h.extra[1].Path); got != 1 {
			t.Fatalf("the play-next insertion was loaded %d times, want 1", got)
		}

		h.endFile()
		h.waitEnded("the play-next insertion without gapless")
		h.assertNoReplays(completed...)
	})

	t.Run("an unplayed tail row moved inside the boundary is played", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-in-move")
		completed := h.queueSnapshot()

		h.insertAt(0, h.extra[0])
		h.reset()
		h.resume(0)

		// Append past the boundary first, so the row is an ordinary unplayed tail
		// row and not a fresh identity when the reorder carries it inwards.
		h.insertAt(4, h.extra[1])
		h.waitPrefetch(h.extra[1].Path)
		h.moveRow(4, 1)

		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 0, 1, false)
		h.waitLoads(h.extra[1].Path, 2)

		h.endFile()
		h.waitCurrentIndex(1)
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 1, 0, false)

		h.endFile()
		h.waitEnded("the tail row moved inside the boundary")
		h.assertNoReplays(completed...)
	})

	t.Run("an unplayed tail row moved inside the boundary is played without gapless", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-in-move-nogap")
		completed := h.queueSnapshot()
		if err := h.m.SetGapless(false); err != nil {
			t.Fatalf("SetGapless(false): %v", err)
		}

		h.insertAt(0, h.extra[0])
		h.reset()
		h.resume(0)

		h.insertAt(4, h.extra[1])
		h.moveRow(4, 1)
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 0, 1, false)

		h.endFile()
		h.waitCurrentIndex(1)
		if got := h.loads(h.extra[1].Path); got != 1 {
			t.Fatalf("the moved tail row was loaded %d times, want 1", got)
		}

		h.endFile()
		h.waitEnded("the tail row moved inside the boundary without gapless")
		h.assertNoReplays(completed...)
	})
}

// TestSequentialFrontierExcludesRowsPlayedAfterTheResume is the other half of
// deriving continuation from played-versus-unplayed identity.
//
// Filtering the span by what played only excludes what the boundary names, and
// the prefix the resume pinned is not the whole of what played: the traversal
// keeps going from there. A row it started after the resume sits ahead of the
// cursor arithmetic only until a reorder carries it back inside the span, and
// from then on nothing but the boundary itself can say it is finished.
func TestSequentialFrontierExcludesRowsPlayedAfterTheResume(t *testing.T) {
	t.Run("a row played after the resume is not handed back by a reorder", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-in-done")
		completed := h.queueSnapshot()

		h.insertAt(0, h.extra[0], h.extra[1])
		h.reset()
		h.resume(0)
		h.waitPrefetch(h.extra[1].Path)
		h.endFile()
		h.waitCurrentIndex(1)

		// The first insertion has played. Moving it between the cursor and the
		// frontier must not make it a successor of the row that followed it.
		h.reset()
		h.moveRow(0, 3)
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 0, 0, false)

		h.endFile()
		h.waitEnded("the traversal once the played insertion was moved inside the boundary")
		h.assertNoReplays(append(completed, h.extra[0])...)
	})

	t.Run("the fallback advance excludes it too when gapless is off", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-in-done-nogap")
		completed := h.queueSnapshot()
		if err := h.m.SetGapless(false); err != nil {
			t.Fatalf("SetGapless(false): %v", err)
		}

		h.insertAt(0, h.extra[0], h.extra[1])
		h.reset()
		h.resume(0)
		h.endFile()
		h.waitCurrentIndex(1)

		h.reset()
		h.moveRow(0, 3)
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 0, 0, false)

		h.endFile()
		h.waitEnded("the fallback advance once the played insertion was moved inside the boundary")
		h.assertNoReplays(append(completed, h.extra[0])...)
	})
}
