package tui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestRadioBatchCancellationKeepsDurableQueueConsistent exercises the real
// queue writer and reader across every cancellation boundary. The enqueue
// command is completed before the model mutation, deterministically modeling
// a committed batch whose Bubble Tea result has not been delivered yet.
func TestRadioBatchCancellationKeepsDurableQueueConsistent(t *testing.T) {
	t.Run("disable in flight", func(t *testing.T) {
		m, d := newRadioBatchDBModel(t)
		m, msg := runRadioBatch(t, m)
		assertActiveQueueLen(t, d, 6)

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
		assertActiveQueueLen(t, d, 6)

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
		assertActiveQueueLen(t, d, 6)

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

	tracks := sampleLibraryTracks()
	now := time.Now().Unix()
	for _, track := range tracks {
		if _, err := d.Exec(
			`INSERT INTO tracks (hash, path, artist, title, album, duration_ms, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			track.Hash, track.Path, track.Artist, track.Title, track.Album, int64(track.Duration*1000), now, now,
		); err != nil {
			t.Fatalf("seed track %s: %v", track.Hash, err)
		}
	}
	ids, err := w.AppendTracks(tracks[:1], "Manual")
	if err != nil {
		t.Fatalf("seed queue: %v", err)
	}
	m := New(context.Background(),
		WithLibraryTracks(tracks),
		WithQueueEntries(newQueueEntries(tracks[:1], ids, "Manual")),
		WithBatchEnqueuer(w),
		WithQueueDequeuer(w),
	)
	m.radio.enabled = true
	m.radio.seedHash = tracks[0].Hash
	m.playback.currentIndex = 0
	return m, d
}

func runRadioBatch(t *testing.T, m Model) (Model, RadioBatchEnqueuedMsg) {
	t.Helper()
	m, cmd := m.startRadioEnqueueCmd()
	if cmd == nil {
		t.Fatal("expected Radio enqueue command")
	}
	raw := cmd()
	msg, ok := raw.(RadioBatchEnqueuedMsg)
	if !ok {
		t.Fatalf("Radio command returned %T", raw)
	}
	if msg.Err != nil {
		t.Fatalf("Radio enqueue: %v", msg.Err)
	}
	if len(msg.QueueIDs) != radioBatchSize {
		t.Fatalf("Radio batch size = %d, want %d", len(msg.QueueIDs), radioBatchSize)
	}
	return m, msg
}

func assertActiveQueueLen(t *testing.T, d *db.DB, want int) []db.QueueRow {
	t.Helper()
	rows, err := d.ReadQueue()
	if err != nil {
		t.Fatalf("read durable queue: %v", err)
	}
	if len(rows) != want {
		t.Fatalf("durable queue length = %d, want %d", len(rows), want)
	}
	return rows
}

func cancelRadioTestPurge(m *Model) {
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
		m.undo.slot.cancelPurge = nil
	}
}

// TestRadioBatchDiscardedWhenDisabled verifies that a RadioBatchEnqueuedMsg
// arriving after the user has toggled radio off does not resurrect playback by
// appending tracks to a queue the user just cleared/disabled.
func TestRadioBatchDiscardedWhenDisabled(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = false
	m.radio.enqueuePending = true
	beforeLen := len(m.queue.Entries)

	msg := RadioBatchEnqueuedMsg{
		Entries: []queue.Entry{
			{ID: 99, Hash: "hx", Path: "/x.flac", Artist: "X", Title: "Y"},
		},
		QueueIDs: []int64{99},
		WasEmpty: true,
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset even when batch is discarded")
	}
	if len(nm.queue.Entries) != beforeLen {
		t.Errorf("queue must not grow when radio is disabled; before=%d after=%d",
			beforeLen, len(nm.queue.Entries))
	}
	if nm.playback.state == playback.StatePlaying {
		t.Error("playback must not start when radio is disabled")
	}
}

// TestRadioSeedTagsSnapshotted verifies that the radio enqueue closure copies
// the seed tag set instead of holding a live reference to m.trackTags[seedHash].
// We exercise the race by mutating the live map concurrently with the closure
// running: the snapshot must isolate the closure from those mutations.
// Run with `go test -race` for the strongest signal.
func TestRadioSeedTagsSnapshotted(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := newModelWithTracks()
	m.batchEnqueuer = be
	m.radio.enabled = true
	m.radio.seedHash = "h1"
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "h1", Path: "/a1.flac"}}
	m.playback.currentIndex = 0
	m.trackTags = map[string]map[int64]struct{}{
		"h1": {7: {}, 8: {}},
		// Populate other tracks so ScoreAll enters the shared-tag inner loop
		// that ranges over Seed.TagIDs for each candidate.
		"h2": {7: {}},
		"h3": {8: {}},
		"h4": {7: {}, 8: {}},
		"h5": {7: {}},
		"h6": {8: {}},
	}

	cmd := m.radioEnqueueCmd()
	if cmd == nil {
		t.Fatal("expected non-nil radio enqueue cmd")
	}

	// Concurrently mutate the live tag set while the closure runs. Without
	// the snapshot the closure's range loop over seed.TagIDs would race
	// with these writes and `go test -race` would flag it.
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				for i := 0; i < 50; i++ {
					m.trackTags["h1"][int64(i)] = struct{}{}
					delete(m.trackTags["h1"], int64(i))
				}
			}
		}
	}()

	// Drive the closure many times so its read window overlaps the mutator.
	for i := 0; i < 50; i++ {
		_ = cmd()
	}
	close(stop)
	<-done
}

// TestRadioBatchDiscardedWhenQueueClearedInFlight verifies that a
// RadioBatchEnqueuedMsg arriving with WasEmpty=false (radio triggered at
// low-water-mark with a non-empty queue) is discarded when the queue is empty
// by the time the batch arrives. Without this guard a Shift+C clear that
// races a radio enqueue would silently re-populate the cleared queue with
// surprise tracks the user did not request.
func TestRadioBatchDiscardedWhenQueueClearedInFlight(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m.radio.enqueuePending = true
	// Simulate post-clear state: queue empty, queueExhausted=false (cleared
	// by resetPlaybackState), state Idle.
	m.queue.Entries = nil
	m.playback.queueExhausted = false
	m.playback.state = playback.StateIdle

	msg := RadioBatchEnqueuedMsg{
		Entries: []queue.Entry{
			{ID: 50, Hash: "hr", Path: "/r.flac", Artist: "R", Title: "S"},
		},
		QueueIDs: []int64{50},
		WasEmpty: false, // captured when queue still had entries
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset even when batch is discarded")
	}
	if len(nm.queue.Entries) != 0 {
		t.Errorf("cleared queue must stay empty; got %d entries", len(nm.queue.Entries))
	}
	if nm.playback.state == playback.StatePlaying {
		t.Error("playback must not start when batch is discarded after queue clear")
	}
}

// TestRadioBatchAppliedWhenQueueWasEmptyAtEnqueueStart verifies the
// queue-cleared-in-flight guard does not over-fire: a batch whose closure was
// launched with WasEmpty=true (e.g. after PlaybackEndedMsg with no remaining
// queue) must still apply and auto-start playback on arrival.
func TestRadioBatchAppliedWhenQueueWasEmptyAtEnqueueStart(t *testing.T) {
	p := &stubCommandPlayer{}
	m := newModelWithTracks()
	m.player = p
	m.radio.enabled = true
	m.radio.enqueuePending = true
	m.queue.Entries = nil
	m.playback.state = playback.StateIdle

	msg := RadioBatchEnqueuedMsg{
		Entries: []queue.Entry{
			{ID: 60, Hash: "hq", Path: "/q.flac", Artist: "Q", Title: "T"},
		},
		QueueIDs: []int64{60},
		WasEmpty: true, // queue truly was empty when enqueue fired
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset after applying batch")
	}
	if len(nm.queue.Entries) != 1 {
		t.Errorf("expected 1 entry appended, got %d", len(nm.queue.Entries))
	}
	if nm.playback.state != playback.StatePlaying {
		t.Errorf("expected PlaybackPlaying after WasEmpty auto-start, got %v", nm.playback.state)
	}
}

// TestRadioBatchAppliedOnNaturalExhaustion verifies that a RadioBatchEnqueuedMsg
// with WasEmpty=false (launched when the queue still held tracks) is NOT
// discarded just because the queue finished playing in flight. The
// distinguishing signal is playback.queueExhausted, which handlePlaybackEndedMsg
// sets when the last track ends naturally (vs. resetPlaybackState clearing it
// after a user Shift+C clear). Without this, the user is left with an empty
// queue and a dead radio session whenever a batch races a natural exhaustion.
func TestRadioBatchAppliedOnNaturalExhaustion(t *testing.T) {
	p := &stubCommandPlayer{}
	m := newModelWithTracks()
	m.player = p
	m.radio.enabled = true
	m.radio.enqueuePending = true
	// Natural exhaustion path: queue is empty but queueExhausted=true (set by
	// handlePlaybackEndedMsg) so auto-resume should fire when the batch lands.
	m.queue.Entries = nil
	m.playback.queueExhausted = true
	m.playback.state = playback.StateIdle

	msg := RadioBatchEnqueuedMsg{
		Entries: []queue.Entry{
			{ID: 70, Hash: "hx", Path: "/x.flac", Artist: "X", Title: "T"},
		},
		QueueIDs: []int64{70},
		WasEmpty: false, // captured at low-water-mark before the last track ended
	}
	nm, _ := m.handleRadioBatchEnqueuedMsg(msg)

	if nm.radio.enqueuePending {
		t.Error("enqueuePending must be reset after applying batch")
	}
	if len(nm.queue.Entries) != 1 {
		t.Fatalf("expected 1 entry appended on natural exhaustion, got %d", len(nm.queue.Entries))
	}
	if nm.playback.state != playback.StatePlaying {
		t.Errorf("expected PlaybackPlaying after natural-exhaust auto-resume, got %v", nm.playback.state)
	}
	if nm.playback.queueExhausted {
		t.Error("queueExhausted should be cleared after auto-resume")
	}
}
