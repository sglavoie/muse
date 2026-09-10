package db_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

func TestReadDJSession_NoSessionYet(t *testing.T) {
	d, _ := openTagDB(t)

	session := readDJSession(t, d)

	if session.Active {
		t.Error("Active = true on a fresh database, want false")
	}
	if !session.Seed.IsZero() {
		t.Errorf("Seed = %+v on a fresh database, want zero", session.Seed)
	}
	// The seeded row must still describe usable stock settings so the Tune tab
	// has something valid to open on before the user applies anything.
	if err := session.Settings.Validate(); err != nil {
		t.Errorf("default settings do not validate: %v", err)
	}
	if got, want := session.Settings.Preset(), radio.DefaultPreset; got != want {
		t.Errorf("default preset = %q, want %q", got, want)
	}
}

func TestWriteDJSession_RoundTrip(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	settings := djTestSettings(rock, live)
	seed := djTestSeed(live, rock)
	writeDJSession(t, w, db.DJSession{
		Active:     true,
		Settings:   settings,
		Seed:       seed,
		Allocation: radio.Allocation{CarryUnits: 30},
	})

	got := readDJSession(t, d)

	if !got.Active {
		t.Error("Active = false after writing an active session")
	}
	if want := settings.Normalized(); !reflect.DeepEqual(got.Settings, want) {
		t.Errorf("Settings = %+v, want %+v", got.Settings, want)
	}
	// Seed tag ids come back sorted ascending, matching radio.CaptureSeed.
	wantSeed := seed
	wantSeed.TagIDs = []int64{rock, live}
	if !reflect.DeepEqual(got.Seed, wantSeed) {
		t.Errorf("Seed = %+v, want %+v", got.Seed, wantSeed)
	}
	if got.Allocation.CarryUnits != 30 {
		t.Errorf("Allocation.CarryUnits = %d, want 30", got.Allocation.CarryUnits)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero after a write")
	}
}

func TestWriteDJSession_ReplacesClearedFilters(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	writeDJSession(t, w, db.DJSession{
		Settings: djTestSettings(rock, live),
	})
	writeDJSession(t, w, db.DJSession{
		Settings: radio.DefaultSettings(),
	})

	got := readDJSession(t, d)
	if !got.Settings.Include.IsEmpty() || !got.Settings.Exclude.IsEmpty() {
		t.Errorf("filters survived a clearing write: include=%+v exclude=%+v",
			got.Settings.Include, got.Settings.Exclude)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_filter_values WHERE scope = 'session'`); n != 0 {
		t.Errorf("dj_filter_values rows = %d after clearing filters, want 0", n)
	}
}

func TestWriteDJAllocation_UpdatesCarryOnly(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	settings := djTestSettings(rock, live)
	writeDJSession(t, w, db.DJSession{Active: true, Settings: settings, Seed: djTestSeed(rock)})

	if err := w.WriteDJAllocation(radio.Allocation{CarryUnits: -20}); err != nil {
		t.Fatalf("WriteDJAllocation: %v", err)
	}

	got := readDJSession(t, d)
	if got.Allocation.CarryUnits != -20 {
		t.Errorf("Allocation.CarryUnits = %d, want -20", got.Allocation.CarryUnits)
	}
	if want := settings.Normalized(); !reflect.DeepEqual(got.Settings, want) {
		t.Errorf("Settings changed by an allocation write: %+v, want %+v", got.Settings, want)
	}
	if got.Seed.IsZero() {
		t.Error("Seed cleared by an allocation write")
	}
}

func TestReadDJSession_ClampsPersistedCarry(t *testing.T) {
	d, w := openTagDB(t)
	writeDJSession(t, w, db.DJSession{Settings: radio.DefaultSettings()})

	// Simulate a carry written by a build with different bounds.
	if _, err := d.Exec(`UPDATE dj_session SET carry_units = 5000 WHERE id = 1`); err != nil {
		t.Fatalf("seed out-of-range carry: %v", err)
	}

	got := readDJSession(t, d)
	if got.Allocation.CarryUnits != 50 {
		t.Errorf("Allocation.CarryUnits = %d, want 50 (clamped)", got.Allocation.CarryUnits)
	}
}

func TestStopDJSession_ClearsSeedAndCarryKeepsSettings(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	settings := djTestSettings(rock, live)
	writeDJSession(t, w, db.DJSession{
		Active:     true,
		Settings:   settings,
		Seed:       djTestSeed(rock, live),
		Allocation: radio.Allocation{CarryUnits: 40},
	})

	if err := w.StopDJSession(); err != nil {
		t.Fatalf("StopDJSession: %v", err)
	}

	got := readDJSession(t, d)
	if got.Active {
		t.Error("Active = true after StopDJSession")
	}
	if !got.Seed.IsZero() {
		t.Errorf("Seed = %+v after StopDJSession, want zero", got.Seed)
	}
	if !got.Allocation.IsZero() {
		t.Errorf("Allocation = %+v after StopDJSession, want zero", got.Allocation)
	}
	if want := settings.Normalized(); !reflect.DeepEqual(got.Settings, want) {
		t.Errorf("Settings = %+v after StopDJSession, want them preserved as %+v", got.Settings, want)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_tag_refs WHERE scope = 'session' AND role = 'seed'`); n != 0 {
		t.Errorf("seed tag refs = %d after StopDJSession, want 0", n)
	}
}

func TestWriteDJSession_RejectsInvalidSettings(t *testing.T) {
	d, w := openTagDB(t)

	settings := radio.DefaultSettings()
	settings.DiscoveryPercent = 45 // not a ten-point increment

	err := w.WriteDJSession(db.DJSession{Settings: settings})
	if err == nil {
		t.Fatal("WriteDJSession accepted an off-step discovery target")
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_session WHERE discovery_percent = 45`); n != 0 {
		t.Error("rejected settings reached the database")
	}
}

func TestWriteDJSession_RejectsActiveSessionWithoutSeed(t *testing.T) {
	_, w := openTagDB(t)

	err := w.WriteDJSession(db.DJSession{Active: true, Settings: radio.DefaultSettings()})
	if err == nil {
		t.Fatal("WriteDJSession accepted an active session with no seed")
	}
}

func TestWriteDJSession_RejectsNonPositiveSeedTagID(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")

	seed := djTestSeed(rock, 0)
	err := w.WriteDJSession(db.DJSession{Active: true, Settings: radio.DefaultSettings(), Seed: seed})
	if !errors.Is(err, radio.ErrInvalidTagID) {
		t.Fatalf("WriteDJSession seed tag id error = %v, want ErrInvalidTagID", err)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_tag_refs WHERE role = 'seed'`); n != 0 {
		t.Errorf("seed tag refs = %d after a rejected write, want 0", n)
	}
}

// TestReadDJSession_InvalidStateSurfaces covers state this build cannot use.
// Each case must be reported, and must not be silently replaced by permissive
// defaults: the stored restrictive filters have to stay on disk untouched.
func TestReadDJSession_InvalidStateSurfaces(t *testing.T) {
	cases := []struct {
		name    string
		mutate  string
		wantErr error
	}{
		{
			name:    "off-step discovery target",
			mutate:  `UPDATE dj_session SET discovery_percent = 45 WHERE id = 1`,
			wantErr: db.ErrDJStateInvalid,
		},
		{
			name:    "out-of-range discovery target",
			mutate:  `UPDATE dj_session SET discovery_percent = 250 WHERE id = 1`,
			wantErr: db.ErrDJStateInvalid,
		},
		{
			name:    "unknown anchor mode",
			mutate:  `UPDATE dj_session SET anchor_mode = 'sideways' WHERE id = 1`,
			wantErr: db.ErrDJStateInvalid,
		},
		{
			name:    "unreadable seed genres",
			mutate:  `UPDATE dj_session SET seed_genres = 'not json' WHERE id = 1`,
			wantErr: db.ErrDJStateInvalid,
		},
		{
			name:    "active session without a seed",
			mutate:  `UPDATE dj_session SET seed_hash = '' WHERE id = 1`,
			wantErr: db.ErrDJStateInvalid,
		},
		{
			name:    "version from a newer build",
			mutate:  `UPDATE dj_session SET version = 99 WHERE id = 1`,
			wantErr: db.ErrDJStateUnsupported,
		},
		{
			name:    "version zero",
			mutate:  `UPDATE dj_session SET version = 0 WHERE id = 1`,
			wantErr: db.ErrDJStateInvalid,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, w := openTagDB(t)
			rock := makeTag(t, w, "rock")
			live := makeTag(t, w, "live")
			writeDJSession(t, w, db.DJSession{
				Active:   true,
				Settings: djTestSettings(rock, live),
				Seed:     djTestSeed(rock),
			})

			if _, err := d.Exec(tc.mutate); err != nil {
				t.Fatalf("mutate: %v", err)
			}

			got, err := d.ReadDJSession()
			if err == nil {
				t.Fatalf("ReadDJSession succeeded on invalid state, returned %+v", got)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want one wrapping %v", err, tc.wantErr)
			}
			// Nothing usable is handed back, so no caller can mistake broken
			// state for an unrestricted DJ.
			if !reflect.DeepEqual(got, db.DJSession{}) {
				t.Errorf("returned session = %+v on failure, want the zero value", got)
			}
			// The user's filters must still be on disk, ready to be repaired.
			if n := countRows(t, d, `SELECT COUNT(*) FROM dj_filter_values WHERE scope = 'session'`); n == 0 {
				t.Error("stored filter values were dropped by a failed read")
			}
			if n := countRows(t, d, `SELECT COUNT(*) FROM dj_tag_refs WHERE scope = 'session'`); n == 0 {
				t.Error("stored tag references were dropped by a failed read")
			}
		})
	}
}

// TestWriteDJSession_LeavesTransportFieldsIntact guards the pre-existing
// session row: DJ state lives in its own table and must not disturb repeat
// mode, shuffle, radio mode, position, or volume.
func TestWriteDJSession_LeavesTransportFieldsIntact(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "abc", "/music/a.mp3")

	if err := w.UpsertSession("abc", 42_000, true, "all"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := w.WriteVolume(70); err != nil {
		t.Fatalf("WriteVolume: %v", err)
	}
	if err := w.WriteRadioMode(true); err != nil {
		t.Fatalf("WriteRadioMode: %v", err)
	}

	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")
	writeDJSession(t, w, db.DJSession{
		Active:   true,
		Settings: djTestSettings(rock, live),
		Seed:     djTestSeed(rock),
	})
	if err := w.StopDJSession(); err != nil {
		t.Fatalf("StopDJSession: %v", err)
	}

	got, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if got == nil {
		t.Fatal("ReadSession returned nil")
	}
	want := db.SessionRow{
		RepeatMode:  "all",
		Shuffle:     true,
		RadioMode:   true,
		CurrentHash: "abc",
		PositionMs:  42_000,
		Volume:      0.7,
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("transport session = %+v, want %+v", *got, want)
	}
}
