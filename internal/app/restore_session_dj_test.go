package app_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// djRestoreClock is the capture instant every seed below is stamped with, so a
// restored snapshot can be compared field for field against the one written.
var djRestoreClock = time.Date(2026, 4, 12, 15, 30, 0, 0, time.UTC)

// djSessionMigrationVersion is the migration that introduces DJ state. Rolling
// back below it reproduces a database written by a build that had no DJ.
const djSessionMigrationVersion = 23

// djRestoreSettings is a settings snapshot no default could be mistaken for:
// an Explore target under Follow. Every restore assertion below leans on that,
// so a silent fallback to stock defaults shows up as a failure rather than as a
// coincidence.
func djRestoreSettings() radio.Settings {
	s := radio.DefaultSettings()
	s.DiscoveryPercent = 80
	s.Anchor = radio.AnchorFollow
	s.Exclude = radio.FilterSet{Genres: []string{"Ambient"}}
	return s
}

// assertSeedRestored compares a restored snapshot against the one written.
// CapturedAt is compared with Equal rather than by value: the column holds Unix
// seconds, so a snapshot always comes back in the local zone and reflect would
// call the same instant a difference.
func assertSeedRestored(t *testing.T, got, want radio.SeedSnapshot) {
	t.Helper()
	if !got.CapturedAt.Equal(want.CapturedAt) {
		t.Errorf("CapturedAt = %v, want %v", got.CapturedAt, want.CapturedAt)
	}
	got.CapturedAt, want.CapturedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Seed = %+v, want %+v", got, want)
	}
}

func djRestoreSeed() radio.SeedSnapshot {
	return radio.CaptureSeed(
		library.Track{Hash: "h1", Title: "One", Artist: "Alice", Genre: "Rock", Duration: 200},
		nil,
		radio.FixedClock(djRestoreClock),
	)
}

// TestRestoreSessionFreshInstall covers the first launch after an install: the
// DJ row exists because the migration seeds it, and it has to describe a
// stopped session on usable stock settings rather than an error.
func TestRestoreSessionFreshInstall(t *testing.T) {
	h := newRestoreSessionHarness(t)

	restored := h.restore()

	if restored.DJStateErr != nil {
		t.Fatalf("a fresh install reported DJ state as unreadable: %v", restored.DJStateErr)
	}
	if restored.DJ.Active {
		t.Error("a fresh install restored an active DJ session")
	}
	if !restored.DJ.Seed.IsZero() {
		t.Errorf("a fresh install restored seed %+v, want none", restored.DJ.Seed)
	}
	if err := restored.DJ.Settings.Validate(); err != nil {
		t.Errorf("a fresh install restored unusable settings: %v", err)
	}
	if got, want := restored.DJ.Settings.Preset(), radio.DefaultPreset; got != want {
		t.Errorf("a fresh install restored preset %q, want %q", got, want)
	}
	if !restored.DJ.Allocation.IsZero() {
		t.Errorf("a fresh install restored allocation %+v, want zero", restored.DJ.Allocation)
	}
}

// TestRestoreSessionRestoresActiveDJSession is the restart case: everything the
// running session was configured with comes back, because "restarting Muse
// restores the active session" is what tells a restart apart from a new start.
func TestRestoreSessionRestoresActiveDJSession(t *testing.T) {
	h := newRestoreSessionHarness(t)
	settings := djRestoreSettings()
	seed := djRestoreSeed()
	if err := h.writer.WriteDJSession(db.DJSession{
		Active:     true,
		Settings:   settings,
		Seed:       seed,
		Allocation: radio.Allocation{CarryUnits: 30},
	}); err != nil {
		t.Fatalf("WriteDJSession: %v", err)
	}

	restored := h.restore()

	if restored.DJStateErr != nil {
		t.Fatalf("restoring a valid session reported an error: %v", restored.DJStateErr)
	}
	if !restored.DJ.Active {
		t.Error("the active session came back stopped")
	}
	if want := settings.Normalized(); !reflect.DeepEqual(restored.DJ.Settings, want) {
		t.Errorf("Settings = %+v, want %+v", restored.DJ.Settings, want)
	}
	assertSeedRestored(t, restored.DJ.Seed, seed)
	if restored.DJ.Allocation.CarryUnits != 30 {
		t.Errorf("Allocation.CarryUnits = %d, want the carry the session earned (30)",
			restored.DJ.Allocation.CarryUnits)
	}
}

// TestRestoreSessionRestoresSyntheticSeed is why synthetic seeds carry a hash
// at all: a session started from a collection or from nothing in particular has
// no library track to borrow one from, and would otherwise come back as a
// stopped session with no seed.
func TestRestoreSessionRestoresSyntheticSeed(t *testing.T) {
	cases := map[string]radio.SeedSnapshot{
		"whole library": radio.WholeLibrarySeed(radio.FixedClock(djRestoreClock)),
		"a collection": radio.CaptureCollectionSeed("Mixes ▸ Late night", []library.Track{
			{Hash: "h1", Artist: "Alice", Genre: "Rock", Duration: 200},
			{Hash: "h2", Artist: "Alice", Genre: "Jazz", Duration: 300},
		}, nil, radio.FixedClock(djRestoreClock)),
	}

	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			h := newRestoreSessionHarness(t)
			if err := h.writer.WriteDJSession(db.DJSession{
				Active:   true,
				Settings: radio.DefaultSettings(),
				Seed:     seed,
			}); err != nil {
				t.Fatalf("WriteDJSession: %v", err)
			}

			restored := h.restore()

			if !restored.DJ.Active {
				t.Fatalf("a %s session came back stopped", name)
			}
			assertSeedRestored(t, restored.DJ.Seed, seed)
			if !restored.DJ.Seed.IsSynthetic() {
				t.Error("the restored seed lost its synthetic namespace")
			}
		})
	}
}

// TestRestoreSessionSurfacesInvalidDJState covers persisted state this build
// cannot use. It has to be reported, not repaired: replacing unreadable
// restrictive filters with permissive defaults would silently hand the user an
// unrestricted DJ they never asked for.
func TestRestoreSessionSurfacesInvalidDJState(t *testing.T) {
	h := newRestoreSessionHarness(t)
	if err := h.writer.WriteDJSession(db.DJSession{
		Active:   true,
		Settings: djRestoreSettings(),
		Seed:     djRestoreSeed(),
	}); err != nil {
		t.Fatalf("WriteDJSession: %v", err)
	}
	// A discovery target off the ten-point step is what a build with different
	// bounds, or a hand-edited database, leaves behind.
	if _, err := h.database.Exec(`UPDATE dj_session SET discovery_percent = 45 WHERE id = 1`); err != nil {
		t.Fatalf("mutate dj_session: %v", err)
	}

	restored := h.restore()

	if !errors.Is(restored.DJStateErr, db.ErrDJStateInvalid) {
		t.Fatalf("DJStateErr = %v, want one wrapping ErrDJStateInvalid", restored.DJStateErr)
	}
	if !reflect.DeepEqual(restored.DJ, db.DJSession{}) {
		t.Errorf("unusable state was handed back as %+v, want the zero session", restored.DJ)
	}
	// The rest of the session must still restore: a broken DJ row is no reason
	// to lose the queue.
	if restored.RepeatMode.String() == "" {
		t.Error("a broken DJ row derailed the rest of the restore")
	}
	// The user's filters stay on disk, ready to be repaired by a later build.
	var n int
	if err := h.database.QueryRow(
		`SELECT COUNT(*) FROM dj_filter_values WHERE scope = 'session'`).Scan(&n); err != nil {
		t.Fatalf("count filter values: %v", err)
	}
	if n == 0 {
		t.Error("a failed read dropped the stored filter values")
	}
}

// TestRestoreSessionRestoresQueueProvenance carries ownership and reasons into
// the queue entries the TUI renders. Without it a restart turns every DJ-owned
// row into a manual one, and the DJ would no longer recognise — or be allowed
// to replace — the rows it queued itself.
func TestRestoreSessionRestoresQueueProvenance(t *testing.T) {
	h := newRestoreSessionHarness(t)
	path := h.writeAudioFile("a.flac")
	h.insertTrackAndQueue("manual", path, "Alice", "Manual", 0)
	insertTrack(t, h.database, "owned", path, "Bob", "Owned")

	reasons := radio.Reasons{
		{Code: radio.ReasonDiscoveryNever, Detail: "no counted plays"},
		{Code: radio.ReasonExcludeGenre, Value: "Ambient"},
	}
	if _, err := h.writer.RestoreQueueTrack(
		library.Track{Hash: "owned", Path: path},
		"Auto-DJ",
		db.DJProvenance(reasons),
	); err != nil {
		t.Fatalf("RestoreQueueTrack: %v", err)
	}

	restored := h.restore()

	byHash := make(map[string]int, len(restored.QueueEntries))
	for i, e := range restored.QueueEntries {
		byHash[e.Hash] = i
	}
	manual, ok := byHash["manual"]
	if !ok {
		t.Fatalf("the manual row was dropped: %+v", restored.QueueEntries)
	}
	if restored.QueueEntries[manual].DJOwned {
		t.Error("a manual row came back owned by the DJ")
	}
	if len(restored.QueueEntries[manual].DJReasons) != 0 {
		t.Errorf("a manual row came back with reasons %v", restored.QueueEntries[manual].DJReasons)
	}
	owned, ok := byHash["owned"]
	if !ok {
		t.Fatalf("the DJ row was dropped: %+v", restored.QueueEntries)
	}
	if !restored.QueueEntries[owned].DJOwned {
		t.Error("a DJ-owned row came back as a manual entry")
	}
	if got, want := restored.QueueEntries[owned].DJReasons.Summary(), reasons.Summary(); got != want {
		t.Errorf("restored reasons = %q, want %q", got, want)
	}
}

// TestRestoreSessionDJSeedOutlivesItsTrack is the missing-file case. The seed is
// a frozen snapshot precisely so a session survives the track it was started
// from being deleted, renamed, or rescanned out of the library.
func TestRestoreSessionDJSeedOutlivesItsTrack(t *testing.T) {
	h := newRestoreSessionHarness(t)
	path := h.writeAudioFile("seed.flac")
	insertTrack(t, h.database, "h1", path, "Alice", "One")
	seed := djRestoreSeed()
	if err := h.writer.WriteDJSession(db.DJSession{
		Active:   true,
		Settings: djRestoreSettings(),
		Seed:     seed,
	}); err != nil {
		t.Fatalf("WriteDJSession: %v", err)
	}
	// The file is gone and the row with it, which is what a delete plus a
	// rescan leaves behind.
	if _, err := h.database.Exec(`DELETE FROM tracks WHERE hash='h1'`); err != nil {
		t.Fatalf("delete seed track: %v", err)
	}

	restored := h.restore()

	if restored.DJStateErr != nil {
		t.Fatalf("a seed whose track is gone was reported as unreadable state: %v", restored.DJStateErr)
	}
	if !restored.DJ.Active {
		t.Error("the session stopped itself because its seed track was deleted")
	}
	assertSeedRestored(t, restored.DJ.Seed, seed)
}

// TestRestoreSessionUpgradeFromPreDJDatabase is the migration case seen from
// the startup path rather than from storage: a database written by a build that
// had no DJ at all must come up as a stopped session on stock settings, with
// the user's queue intact and every row still theirs.
func TestRestoreSessionUpgradeFromPreDJDatabase(t *testing.T) {
	h := newRestoreSessionHarness(t)
	path := h.writeAudioFile("legacy.flac")

	// Roll back past both DJ migrations and write the queue the previous build
	// would have left behind, including the "Radio" label its radio path used.
	for {
		version, err := db.MigrationVersion(h.database.DB)
		if err != nil {
			t.Fatalf("MigrationVersion: %v", err)
		}
		if version < djSessionMigrationVersion {
			break
		}
		if err := db.MigrateDown(h.database.DB); err != nil {
			t.Fatalf("MigrateDown from version %d: %v", version, err)
		}
	}
	insertTrack(t, h.database, "legacy1", path, "Alice", "One")
	insertTrack(t, h.database, "legacy2", path, "Alice", "Two")
	if _, err := h.database.Exec(`
		INSERT INTO queue (track_hash, position, added_at, source_playlist)
		VALUES ('legacy1', 1000, 0, ''), ('legacy2', 2000, 0, 'Radio')`); err != nil {
		t.Fatalf("insert pre-DJ queue rows: %v", err)
	}

	if err := db.Migrate(h.database.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	restored := h.restore()

	if restored.DJStateErr != nil {
		t.Fatalf("an upgraded database reported DJ state as unreadable: %v", restored.DJStateErr)
	}
	if restored.DJ.Active || !restored.DJ.Seed.IsZero() {
		t.Errorf("an upgrade invented a running session: active=%v seed=%+v",
			restored.DJ.Active, restored.DJ.Seed)
	}
	if err := restored.DJ.Settings.Validate(); err != nil {
		t.Errorf("an upgrade produced unusable settings: %v", err)
	}
	if len(restored.QueueEntries) != 2 {
		t.Fatalf("queue entries = %d after the upgrade, want the 2 that were there",
			len(restored.QueueEntries))
	}
	for _, e := range restored.QueueEntries {
		// Including the row labelled "Radio": the pre-DJ radio path enqueued on
		// the user's behalf, and the DJ must not treat those rows as its own to
		// replace.
		if e.DJOwned || len(e.DJReasons) != 0 {
			t.Errorf("pre-DJ row %q came back owned: owned=%v reasons=%v",
				e.Hash, e.DJOwned, e.DJReasons)
		}
	}
}
