package tui

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

type failOnceRadioBatchEnqueuer struct {
	writer          *db.Writer
	dequeueFailures int
}

func (e *failOnceRadioBatchEnqueuer) AppendTracks(tracks []library.Track, source string) ([]int64, error) {
	return e.writer.AppendTracks(tracks, source)
}

func (e *failOnceRadioBatchEnqueuer) AppendRadioTracksPending(tracks []library.Track) ([]int64, error) {
	return e.writer.AppendRadioTracksPending(tracks)
}

func (e *failOnceRadioBatchEnqueuer) ActivateRadioTracks(ids []int64) error {
	return e.writer.ActivateRadioTracks(ids)
}

func (e *failOnceRadioBatchEnqueuer) DequeueTracksSoft(ids []int64) error {
	if e.dequeueFailures > 0 {
		e.dequeueFailures--
		return errors.New("injected Radio cleanup failure")
	}
	return e.writer.DequeueTracksSoft(ids)
}

// TestRadioBatchCancellationKeepsDurableQueueConsistent exercises the real
// queue writer and reader across every cancellation boundary. The enqueue
// command is completed before the model mutation, deterministically modeling
// a committed batch whose Bubble Tea result has not been delivered yet.
func TestRadioBatchCancellationKeepsDurableQueueConsistent(t *testing.T) {
	t.Run("disable in flight", func(t *testing.T) {
		m, d := newRadioBatchDBModel(t)
		m, msg := runRadioBatch(t, m)
		assertActiveQueueLen(t, d, 1)

		next, _, _ := m.radioToggle()
		m = next.(Model)
		m, _ = m.handleRadioBatchEnqueuedMsg(msg)

		if m.radio.enqueuePending {
			t.Fatal("disabled Radio retained a pending enqueue")
		}
		if got := len(m.queue.Entries); got != 1 {
			t.Fatalf("in-memory queue length = %d, want original row only", got)
		}
		assertActiveQueueLen(t, d, 1)
	})

	t.Run("clear in flight", func(t *testing.T) {
		m, d := newRadioBatchDBModel(t)
		m, msg := runRadioBatch(t, m)
		assertActiveQueueLen(t, d, 1)

		m = m.handleClearQueueMsg()
		cancelRadioTestPurge(&m)
		m, _ = m.handleRadioBatchEnqueuedMsg(msg)

		if got := len(m.queue.Entries); got != 0 {
			t.Fatalf("in-memory queue length = %d, want empty", got)
		}
		assertActiveQueueLen(t, d, 0)
	})

	t.Run("clear then repopulate before result", func(t *testing.T) {
		m, d := newRadioBatchDBModel(t)
		m, stale := runRadioBatch(t, m)
		assertActiveQueueLen(t, d, 1)

		m = m.handleClearQueueMsg()
		cancelRadioTestPurge(&m)
		replacement := sampleLibraryTracks()[5]
		ids, err := m.batchEnqueuer.AppendTracks([]library.Track{replacement}, "Manual")
		if err != nil {
			t.Fatalf("append replacement: %v", err)
		}
		m.queue.Entries = newQueueEntries([]library.Track{replacement}, ids, "Manual")
		m.playback.currentIndex = 0
		m, radioCmd := m.startRadioEnqueueCmd()
		if radioCmd == nil || !m.radio.enqueuePending {
			t.Fatal("expected a newer Radio request after replacement")
		}

		m, _ = m.handleRadioBatchEnqueuedMsg(stale)

		if !m.radio.enqueuePending {
			t.Fatal("stale result cleared the newer Radio request's pending state")
		}
		if got := len(m.queue.Entries); got != 1 || m.queue.Entries[0].ID != ids[0] {
			t.Fatalf("replacement queue changed after stale result: %+v", m.queue.Entries)
		}
		rows := assertActiveQueueLen(t, d, 1)
		if rows[0].ID != ids[0] {
			t.Fatalf("durable row ID = %d, want replacement ID %d", rows[0].ID, ids[0])
		}
	})

	t.Run("current result appends normally", func(t *testing.T) {
		m, d := newRadioBatchDBModel(t)
		m, msg := runRadioBatch(t, m)
		m, _ = m.handleRadioBatchEnqueuedMsg(msg)

		if m.radio.enqueuePending {
			t.Fatal("current Radio result retained pending state")
		}
		if got := len(m.queue.Entries); got != 6 {
			t.Fatalf("in-memory queue length = %d, want original plus batch", got)
		}
		rows := assertActiveQueueLen(t, d, 6)
		for i, entry := range m.queue.Entries {
			if rows[i].ID != entry.ID {
				t.Fatalf("row %d durable ID = %d, in-memory ID = %d", i, rows[i].ID, entry.ID)
			}
		}
	})
}

// TestRadioBatchCleanupFailureRetainsReconciliation proves a transient
// compensating-delete failure cannot lose the stale batch. Its exact QueueIDs
// survive in Model state, a later retry removes those durable rows, and the
// newer Radio request remains pending throughout.
func TestRadioBatchCleanupFailureRetainsReconciliation(t *testing.T) {
	m, d := newRadioBatchDBModel(t)
	writer := m.batchEnqueuer.(*db.Writer)
	flaky := &failOnceRadioBatchEnqueuer{writer: writer, dequeueFailures: 1}
	m.batchEnqueuer = flaky

	m, stale := runRadioBatch(t, m)
	assertActiveQueueLen(t, d, 1)
	m = m.supersedeRadioBatch()
	m, newerCmd := m.startRadioEnqueueCmd()
	assertRadioRequestPending(t, m, newerCmd)

	m, _ = m.handleRadioBatchEnqueuedMsg(stale)
	assertRadioCleanupRetained(t, m, d, stale.QueueIDs)
	m = runRadioCleanupRetry(t, m)
	assertRadioCleanupRecovered(t, m, d)
}

// TestRadioBatchCleanupSurvivesProcessExit proves the durable pending marker
// outlives Model state. Even when the compensating delete fails and the retry
// FIFO is lost with the process, the next Writer startup keeps the exact stale
// QueueIDs out of ReadQueue and retires them for ordinary purge.
func TestRadioBatchCleanupSurvivesProcessExit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "radio-batch-restart.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	if err := db.Migrate(d.DB); err != nil {
		d.Close()
		t.Fatalf("migrate DB: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	writer := db.NewWriter(ctx, d.DB)
	m := seedRadioBatchDBModel(t, d, writer)
	flaky := &failOnceRadioBatchEnqueuer{writer: writer, dequeueFailures: 1}
	m.batchEnqueuer = flaky

	m, stale := runRadioBatch(t, m)
	m = m.supersedeRadioBatch()
	m, _ = m.handleRadioBatchEnqueuedMsg(stale)
	if len(m.radio.cleanupBatches) != 1 {
		t.Fatalf("failed cleanup was not retained before exit: %v", m.radio.cleanupBatches)
	}
	assertActiveQueueLen(t, d, 1)
	for _, staleID := range stale.QueueIDs {
		var deletedAt int64
		if err := d.QueryRow(`SELECT deleted_at FROM queue WHERE id=?`, staleID).Scan(&deletedAt); err != nil {
			t.Fatalf("read pending Radio row %d before restart: %v", staleID, err)
		}
		if deletedAt != math.MaxInt64 {
			t.Fatalf("pending Radio row %d deleted_at = %d, want reserved marker %d", staleID, deletedAt, int64(math.MaxInt64))
		}
	}

	cancel()
	writer.Wait()
	if err := d.Close(); err != nil {
		t.Fatalf("close pre-restart DB: %v", err)
	}

	restartedDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen DB: %v", err)
	}
	restartedCtx, restartedCancel := context.WithCancel(context.Background())
	restartedWriter := db.NewWriter(restartedCtx, restartedDB.DB)
	t.Cleanup(func() {
		restartedCancel()
		restartedWriter.Wait()
		if err := restartedDB.Close(); err != nil {
			t.Errorf("close restarted DB: %v", err)
		}
	})

	rows := assertActiveQueueLen(t, restartedDB, 1)
	for _, staleID := range stale.QueueIDs {
		if rows[0].ID == staleID {
			t.Fatalf("ReadQueue restored discarded Radio row %d after restart", staleID)
		}
		var deletedAt int64
		if err := restartedDB.QueryRow(`SELECT deleted_at FROM queue WHERE id=?`, staleID).Scan(&deletedAt); err != nil {
			t.Fatalf("read discarded Radio row %d after restart: %v", staleID, err)
		}
		if deletedAt == math.MaxInt64 {
			t.Fatalf("discarded Radio row %d retained reserved pending marker after restart", staleID)
		}
		if deletedAt <= 0 || deletedAt > time.Now().Unix() {
			t.Fatalf("discarded Radio row %d deleted_at = %d, want ordinary soft-delete timestamp", staleID, deletedAt)
		}
	}
}

func assertRadioRequestPending(t *testing.T, m Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil || !m.radio.enqueuePending {
		t.Fatal("expected a newer Radio request to remain pending")
	}
}

func assertRadioCleanupRetained(t *testing.T, m Model, d *db.DB, wantIDs []int64) {
	t.Helper()
	if !m.radio.enqueuePending {
		t.Fatal("failed stale cleanup cleared the newer Radio request's pending state")
	}
	if len(m.queue.Entries) != 1 {
		t.Fatalf("stale batch entered the in-memory queue: %+v", m.queue.Entries)
	}
	if len(m.radio.cleanupBatches) != 1 || !sameRadioQueueIDs(m.radio.cleanupBatches[0], wantIDs) {
		t.Fatalf("retained cleanup batches = %v, want exact QueueIDs %v", m.radio.cleanupBatches, wantIDs)
	}
	assertActiveQueueLen(t, d, 1)
}

func runRadioCleanupRetry(t *testing.T, m Model) Model {
	t.Helper()
	m, tickBatch := m.handleTickMsg()
	if tickBatch == nil || !m.radio.cleanupInFlight {
		t.Fatal("periodic tick did not start the retained cleanup retry")
	}
	rawBatch := tickBatch()
	batch, ok := rawBatch.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("tick returned %T, want a non-empty command batch", rawBatch)
	}
	raw := batch[len(batch)-1]()
	retryMsg, ok := raw.(radioBatchCleanupMsg)
	if !ok {
		t.Fatalf("cleanup retry returned %T", raw)
	}
	if retryMsg.Err != nil {
		t.Fatalf("cleanup retry failed: %v", retryMsg.Err)
	}
	return m.handleRadioBatchCleanupMsg(retryMsg)
}

func assertRadioCleanupRecovered(t *testing.T, m Model, d *db.DB) {
	t.Helper()
	if len(m.radio.cleanupBatches) != 0 || m.radio.cleanupInFlight {
		t.Fatalf("successful cleanup left retry state: batches=%v inFlight=%t", m.radio.cleanupBatches, m.radio.cleanupInFlight)
	}
	if !m.radio.enqueuePending {
		t.Fatal("successful stale cleanup cleared the newer Radio request's pending state")
	}
	rows := assertActiveQueueLen(t, d, 1)
	if rows[0].ID != m.queue.Entries[0].ID {
		t.Fatalf("ReadQueue restored discarded rows: got IDs %v, want only %d", rows, m.queue.Entries[0].ID)
	}
}

func newRadioBatchDBModel(t *testing.T) (Model, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "radio-batch.db"))
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	if err := db.Migrate(d.DB); err != nil {
		d.Close()
		t.Fatalf("migrate DB: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := db.NewWriter(ctx, d.DB)
	t.Cleanup(func() {
		cancel()
		w.Wait()
		if err := d.Close(); err != nil {
			t.Errorf("close DB: %v", err)
		}
	})

	m := seedRadioBatchDBModel(t, d, w)
	return m, d
}
