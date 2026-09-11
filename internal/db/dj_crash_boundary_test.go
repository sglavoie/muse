package db_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// Crash boundaries of a DJ batch, checked across a genuinely fresh restoration.
//
// The existing recovery tests restart the Writer over a live handle, which proves
// the recovery statements run. What they cannot show is that the state a crash
// leaves behind is on disk rather than in the connection: WAL contents, the
// pending marker, and the accept transaction all have to survive the handle
// closing. So every case here kills the writer *and* the database, re-opens the
// same file, and only then looks at what the next launch would see.
//
// The four boundaries are the four moments a batch can be interrupted at:
// prepared but not accepted, accepted, a replacement prepared but not accepted,
// and a rejected batch whose tombstones have not been purged yet. In every case
// the requirement is the same — the queue the user sees and the settings the DJ
// would plan from must both be a state the user could have arrived at
// deliberately.

// djProcess is one Muse launch over a database file.
type djProcess struct {
	path   string
	d      *db.DB
	w      *db.Writer
	cancel context.CancelFunc
}

// startDJProcess opens the file, migrates it, and starts a Writer, which is what
// runs startup recovery.
func startDJProcess(t *testing.T, path string) *djProcess {
	t.Helper()
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if err := db.Migrate(d.DB); err != nil {
		d.Close()
		t.Fatalf("migrate %s: %v", path, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &djProcess{path: path, d: d, w: db.NewWriter(ctx, d.DB), cancel: cancel}
	t.Cleanup(func() { p.close() })
	return p
}

// newDJProcess starts the first launch over a fresh temporary file.
func newDJProcess(t *testing.T) *djProcess {
	t.Helper()
	return startDJProcess(t, filepath.Join(t.TempDir(), "muse.db"))
}

// close shuts the launch down. It is idempotent so the cleanup registered at
// start is harmless after an explicit crash.
func (p *djProcess) close() {
	if p.d == nil {
		return
	}
	p.cancel()
	p.w.Wait()
	_ = p.d.Close()
	p.d = nil
}

// relaunch ends this launch and returns the next one over the same file. Nothing
// is carried across in memory: whatever the assertions find afterwards came off
// disk.
func (p *djProcess) relaunch(t *testing.T) *djProcess {
	t.Helper()
	p.close()
	return startDJProcess(t, p.path)
}

// djBoundarySettings is a distinctive applied snapshot, so a session that
// reverted to defaults is obvious.
func djBoundarySettings(pct int) radio.Settings {
	return radio.Settings{
		DiscoveryPercent: pct,
		Anchor:           radio.AnchorFollow,
		Include:          radio.FilterSet{Genres: []string{"Shoegaze"}},
	}
}

// TestCrashBoundary_PreparedBatchNeverPublishes covers a crash between planning
// and acceptance. The hidden rows are the ones a naive recovery would either
// publish (tracks the user never asked for) or leave hidden forever (rows that
// can never be purged), so a fresh launch has to retire them and refuse the
// accept that was in flight.
func TestCrashBoundary_PreparedBatchNeverPublishes(t *testing.T) {
	p := newDJProcess(t)
	manualID, err := p.w.AppendTrack(djTrack(t, p.d, "manual1"), "")
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}
	writeDJSession(t, p.w, db.DJSession{
		Active:     true,
		Settings:   djBoundarySettings(40),
		Seed:       djTestSeed(),
		Allocation: radio.Allocation{CarryUnits: 50},
	})
	prep := prepareDJBatch(t, p.w, djEntries(t, p.d, "dj1", "dj2"))
	assertPending(t, p.d, prep.IDs)

	p = p.relaunch(t)

	assertHashes(t, "visible queue after the crash", queueHashes(t, p.d), []string{"manual1"})
	for _, id := range prep.IDs {
		state := readQueueState(t, p.d, id)
		if !state.tombstoned() {
			t.Errorf("row %d: deleted_at = %v, want an ordinary tombstone", id, state.deletedAt)
		}
		if state.pendingKind != "" {
			t.Errorf("row %d: pending_kind = %q, want it cleared", id, state.pendingKind)
		}
	}
	if _, err := p.w.AcceptDJBatch(prep, db.DJCommit{}); !errors.Is(err, db.ErrQueueBatchStale) {
		t.Fatalf("AcceptDJBatch across the crash: err = %v, want ErrQueueBatchStale", err)
	}

	// The session the batch was planned under is untouched: an interrupted
	// batch says nothing about the settings the user applied.
	session := readDJSession(t, p.d)
	if !session.Active || !session.Settings.Equal(djBoundarySettings(40)) {
		t.Errorf("restored session = %+v, want the applied snapshot still active", session)
	}
	if session.Allocation.CarryUnits != 50 {
		t.Errorf("restored carry = %d, want the pre-crash 50", session.Allocation.CarryUnits)
	}
	if _, ok := queueRowByID(t, p.d, manualID); !ok {
		t.Error("the manual row did not survive the crash")
	}
}

// TestCrashBoundary_AcceptedBatchSurvivesWithItsCommit covers a crash right
// after acceptance. Publication and the DJ state it advanced are one
// transaction, so the next launch must find both or neither — a queue holding a
// batch whose carry was lost would silently shift the discovery share.
func TestCrashBoundary_AcceptedBatchSurvivesWithItsCommit(t *testing.T) {
	p := newDJProcess(t)
	writeDJSession(t, p.w, db.DJSession{Active: true, Settings: djBoundarySettings(40), Seed: djTestSeed()})
	prep := prepareDJBatch(t, p.w, djEntries(t, p.d, "dj1", "dj2"))
	carry := radio.Allocation{CarryUnits: -50}
	acceptDJBatch(t, p.w, prep, db.DJCommit{Allocation: &carry})

	p = p.relaunch(t)

	assertHashes(t, "visible queue after the crash", queueHashes(t, p.d), []string{"dj1", "dj2"})
	assertVisible(t, p.d, prep.IDs)
	for _, id := range prep.IDs {
		row, ok := queueRowByID(t, p.d, id)
		if !ok {
			t.Fatalf("accepted row %d vanished", id)
		}
		if !row.DJOwned || len(row.DJReasons) == 0 {
			t.Errorf("row %d: owned=%v reasons=%v, want ownership and reasons to survive",
				id, row.DJOwned, row.DJReasons)
		}
	}
	if got := readDJSession(t, p.d).Allocation.CarryUnits; got != carry.CarryUnits {
		t.Errorf("restored carry = %d, want the %d committed with the batch", got, carry.CarryUnits)
	}
}

// TestCrashBoundary_InterruptedApplyKeepsPlaybackAndSettings covers the Apply
// gesture, which is the one that touches rows the user is listening to. A
// replacement prepared but not accepted must leave the queue exactly as it was
// and the session on the old settings: half-applied settings with a half-replaced
// queue is the state there is no way for the user to reason about.
func TestCrashBoundary_InterruptedApplyKeepsPlaybackAndSettings(t *testing.T) {
	p := newDJProcess(t)
	old := djBoundarySettings(20)
	writeDJSession(t, p.w, db.DJSession{Active: true, Settings: old, Seed: djTestSeed()})
	live := prepareDJBatch(t, p.w, djEntries(t, p.d, "dj1", "dj2"))
	acceptDJBatch(t, p.w, live, db.DJCommit{})

	// The Apply path: new rows prepared hidden, retiring the live DJ rows.
	replacement, err := p.w.PrepareDJReplacement(djEntries(t, p.d, "dj3", "dj4"), live.IDs)
	if err != nil {
		t.Fatalf("PrepareDJReplacement: %v", err)
	}
	assertPending(t, p.d, replacement.IDs)

	p = p.relaunch(t)

	assertHashes(t, "visible queue after an interrupted Apply", queueHashes(t, p.d), []string{"dj1", "dj2"})
	assertVisible(t, p.d, live.IDs)
	for _, id := range replacement.IDs {
		if state := readQueueState(t, p.d, id); !state.tombstoned() {
			t.Errorf("replacement row %d: deleted_at = %v, want an ordinary tombstone", id, state.deletedAt)
		}
	}
	session := readDJSession(t, p.d)
	if !session.Settings.Equal(old) {
		t.Errorf("restored settings = %+v, want the pre-Apply %+v", session.Settings, old)
	}
	if _, err := p.w.AcceptDJBatch(replacement, db.DJCommit{}); !errors.Is(err, db.ErrQueueBatchStale) {
		t.Fatalf("accepting the interrupted replacement: err = %v, want ErrQueueBatchStale", err)
	}
}

// TestCrashBoundary_AcceptedApplySurvivesWholeOrNotAtAll is the other side of
// Apply: once the replacement is accepted, the new rows, the retired rows, and
// the new settings all have to be what the next launch reads.
func TestCrashBoundary_AcceptedApplySurvivesWholeOrNotAtAll(t *testing.T) {
	p := newDJProcess(t)
	writeDJSession(t, p.w, db.DJSession{Active: true, Settings: djBoundarySettings(20), Seed: djTestSeed()})
	live := prepareDJBatch(t, p.w, djEntries(t, p.d, "dj1", "dj2"))
	acceptDJBatch(t, p.w, live, db.DJCommit{})

	applied := djBoundarySettings(80)
	replacement, err := p.w.PrepareDJReplacement(djEntries(t, p.d, "dj3", "dj4"), live.IDs)
	if err != nil {
		t.Fatalf("PrepareDJReplacement: %v", err)
	}
	session := db.DJSession{
		Active:     true,
		Settings:   applied,
		Seed:       djTestSeed(),
		Allocation: radio.Allocation{},
	}
	acceptDJBatch(t, p.w, replacement, db.DJCommit{Session: &session})

	p = p.relaunch(t)

	assertHashes(t, "visible queue after an applied refresh", queueHashes(t, p.d), []string{"dj3", "dj4"})
	for _, id := range live.IDs {
		if state := readQueueState(t, p.d, id); !state.tombstoned() {
			t.Errorf("retired row %d: deleted_at = %v, want an ordinary tombstone", id, state.deletedAt)
		}
	}
	restored := readDJSession(t, p.d)
	if !restored.Settings.Equal(applied) {
		t.Errorf("restored settings = %+v, want the applied %+v", restored.Settings, applied)
	}
	if restored.Allocation.CarryUnits != 0 {
		t.Errorf("restored carry = %d, want the reset that new settings earn", restored.Allocation.CarryUnits)
	}
}

// TestCrashBoundary_RejectedBatchIsReclaimable covers the cleanup boundary: a
// batch refused in memory whose rows were tombstoned but never purged. The next
// launch must keep them invisible and let the ordinary purge reclaim them, so a
// session that refuses many batches does not grow the queue table forever.
func TestCrashBoundary_RejectedBatchIsReclaimable(t *testing.T) {
	p := newDJProcess(t)
	prep := prepareDJBatch(t, p.w, djEntries(t, p.d, "dj1", "dj2"))
	if err := p.w.RejectDJBatch(prep); err != nil {
		t.Fatalf("RejectDJBatch: %v", err)
	}

	p = p.relaunch(t)

	assertHashes(t, "visible queue after a rejected batch", queueHashes(t, p.d), []string{})
	for _, id := range prep.IDs {
		state := readQueueState(t, p.d, id)
		if !state.tombstoned() {
			t.Errorf("rejected row %d: deleted_at = %v, want an ordinary tombstone", id, state.deletedAt)
		}
	}

	if err := p.w.PurgeDeletedQueueEntries(time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("PurgeDeletedQueueEntries: %v", err)
	}
	for _, id := range prep.IDs {
		if queueRowExists(t, p.d, id) {
			t.Errorf("rejected row %d survived the purge", id)
		}
	}
}

// TestCrashBoundary_RecipesAndStoppedSessionSurvive covers the state the Tune
// panel reopens on. A stopped session keeps its settings on purpose — reopening
// on stock defaults would silently discard the user's filters — and saved
// recipes are the one piece of DJ state that has to outlive every session.
func TestCrashBoundary_RecipesAndStoppedSessionSurvive(t *testing.T) {
	p := newDJProcess(t)
	tuned := djBoundarySettings(60)
	writeDJSession(t, p.w, db.DJSession{Active: true, Settings: tuned, Seed: djTestSeed()})
	recipeID := makeRecipe(t, p.w, "Late Night", tuned)
	if err := p.w.StopDJSession(); err != nil {
		t.Fatalf("StopDJSession: %v", err)
	}

	p = p.relaunch(t)

	session := readDJSession(t, p.d)
	if session.Active {
		t.Error("a stopped session restored as active")
	}
	if !session.Settings.Equal(tuned) {
		t.Errorf("restored settings = %+v, want the tuned %+v a stopped session keeps", session.Settings, tuned)
	}
	if !session.Seed.IsZero() {
		t.Errorf("restored seed = %+v, want it cleared by the stop", session.Seed)
	}
	got := readRecipe(t, p.d, recipeID)
	if !got.Usable() {
		t.Fatalf("restored recipe is unusable: %v", got.Invalid)
	}
	if got.Recipe.Name != "Late Night" || !got.Recipe.Settings.Equal(tuned) {
		t.Errorf("restored recipe = %+v, want %q with the saved settings", got.Recipe, "Late Night")
	}
}
